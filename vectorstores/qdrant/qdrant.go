package qdrant

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
)

var (
	ErrMissingEmbedder       = errors.New("qdrant: embedder is required but not provided")
	ErrMissingCollectionName = errors.New("qdrant: collection name is required")
	ErrInvalidNumDocuments   = errors.New("qdrant: number of documents must be positive")
	ErrConnectionFailed      = errors.New("qdrant: connection failed")
	ErrInvalidURL            = errors.New("qdrant: invalid URL provided")
	ErrCollectionExists      = errors.New("qdrant: collection already exists")
	ErrEmptyQuery            = errors.New("qdrant: query cannot be empty")
	ErrDimensionMismatch     = errors.New("qdrant: vector dimension mismatch")
	ErrBatchSizeTooLarge     = errors.New("qdrant: batch size exceeds maximum allowed")
	ErrPartialBatchFailure   = errors.New("qdrant: some batches failed to process")
)

const (
	DefaultBatchSize      = 100
	MaxBatchSize          = 1000
	DefaultMaxConcurrency = 8
	DefaultRetryAttempts  = 3
	DefaultRetryDelay     = time.Second
	DefaultMaxRetryDelay  = 30 * time.Second
)

type BatchResult struct {
	TotalProcessed int           `json:"total_processed"`
	TotalFailed    int           `json:"total_failed"`
	Duration       time.Duration `json:"duration"`
	Errors         []error       `json:"errors,omitempty"`
	ProcessedIDs   []string      `json:"processed_ids,omitempty"`
}

type BatchConfig struct {
	BatchSize      int           `json:"batch_size"`
	MaxConcurrency int           `json:"max_concurrency"`
	RetryAttempts  int           `json:"retry_attempts"`
	RetryDelay     time.Duration `json:"retry_delay"`
	MaxRetryDelay  time.Duration `json:"max_retry_delay"`
}

type Store struct {
	client         *qdrant.Client
	embedder       embeddings.Embedder
	collectionName string
	logger         *slog.Logger
	options        options
	batchConfig    BatchConfig
	mu             sync.RWMutex
}

var _ vectorstores.VectorStore = (*Store)(nil)

func New(opts ...Option) (vectorstores.VectorStore, error) {
	storeOptions, err := parseOptions(opts...)
	if err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}
	logger := storeOptions.logger.With("component", "qdrant_store", "collection", storeOptions.collectionName)
	client, err := createQdrantClient(storeOptions, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant client: %w", err)
	}
	batchConfig := BatchConfig{
		BatchSize:      DefaultBatchSize,
		MaxConcurrency: DefaultMaxConcurrency,
		RetryAttempts:  DefaultRetryAttempts,
		RetryDelay:     DefaultRetryDelay,
		MaxRetryDelay:  DefaultMaxRetryDelay,
	}
	store := &Store{
		client:         client,
		embedder:       storeOptions.embedder,
		collectionName: storeOptions.collectionName,
		logger:         logger,
		options:        storeOptions,
		batchConfig:    batchConfig,
	}
	logger.Info("Qdrant store initialized successfully", "config", storeOptions.String(), "batch_config", fmt.Sprintf("%+v", batchConfig))
	return store, nil
}

func (s *Store) SetBatchConfig(config BatchConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if config.BatchSize <= 0 {
		config.BatchSize = DefaultBatchSize
	}
	if config.BatchSize > MaxBatchSize {
		config.BatchSize = MaxBatchSize
	}
	if config.MaxConcurrency <= 0 {
		config.MaxConcurrency = DefaultMaxConcurrency
	}
	if config.RetryAttempts < 0 {
		config.RetryAttempts = DefaultRetryAttempts
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = DefaultRetryDelay
	}
	if config.MaxRetryDelay <= 0 {
		config.MaxRetryDelay = DefaultMaxRetryDelay
	}
	s.batchConfig = config
	s.logger.Info("Batch configuration updated", "config", fmt.Sprintf("%+v", config))
}
func (s *Store) GetBatchConfig() BatchConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.batchConfig
}

func (s *Store) AddDocuments(ctx context.Context, docs []schema.Document, options ...vectorstores.Option) ([]string, error) {
	return s.AddDocumentsBatch(ctx, docs, nil, options...)
}

func (s *Store) AddDocumentsBatch(
	ctx context.Context,
	docs []schema.Document,
	progressCallback func(processed, total int, duration time.Duration),
	options ...vectorstores.Option,
) ([]string, error) {
	totalDocs := len(docs)
	if totalDocs == 0 {
		return []string{}, nil
	}

	start := time.Now()
	s.logger.InfoContext(ctx, "Starting optimized document addition pipeline", "total_documents", totalDocs)

	if s.embedder == nil {
		return nil, ErrMissingEmbedder
	}

	opts := vectorstores.ParseOptions(options...)
	collectionName := s.getCollectionName(opts)

	if err := s.ensureCollection(ctx, collectionName); err != nil {
		return nil, fmt.Errorf("collection preparation failed: %w", err)
	}

	embedStart := time.Now()
	texts := make([]string, totalDocs)
	for i, doc := range docs {
		texts[i] = doc.PageContent
	}
	vectors, err := s.embedder.EmbedDocuments(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("document embedding stage failed: %w", err)
	}
	if len(vectors) != totalDocs {
		return nil, fmt.Errorf("embedder returned %d vectors for %d documents", len(vectors), totalDocs)
	}
	s.logger.InfoContext(ctx, "Embedding stage complete", "duration", time.Since(embedStart))

	points := make([]*qdrant.PointStruct, totalDocs)
	allIDs := make([]string, totalDocs)
	for i, doc := range docs {
		docID := s.generateDocumentID(doc)
		allIDs[i] = docID
		points[i] = &qdrant.PointStruct{
			Id:      &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: docID}},
			Vectors: &qdrant.Vectors{VectorsOptions: &qdrant.Vectors_Vector{Vector: &qdrant.Vector{Data: vectors[i]}}},
			Payload: s.documentToPayload(doc),
		}
	}

	upsertResult, err := s.upsertPointsInBatches(ctx, collectionName, points, progressCallback)
	if err != nil {
		if upsertResult != nil && len(upsertResult.ProcessedIDs) > 0 {
			s.logger.WarnContext(ctx, "Partial success in document addition", "processed", len(upsertResult.ProcessedIDs), "failed_batches", len(upsertResult.Errors))
			return upsertResult.ProcessedIDs, err
		}
		return nil, err
	}

	s.logger.InfoContext(ctx, "Document addition pipeline completed successfully",
		"total_processed", upsertResult.TotalProcessed, "duration", time.Since(start))

	return allIDs, nil
}

func (s *Store) upsertPointsInBatches(
	ctx context.Context,
	collectionName string,
	points []*qdrant.PointStruct,
	progressCallback func(processed, total int, duration time.Duration),
) (*BatchResult, error) {
	totalPoints := len(points)
	batchSize := s.batchConfig.BatchSize
	numBatches := int(math.Ceil(float64(totalPoints) / float64(batchSize)))
	start := time.Now()

	semaphore := make(chan struct{}, s.batchConfig.MaxConcurrency)
	resultsChan := make(chan BatchResult, numBatches)
	var wg sync.WaitGroup

	for i := 0; i < totalPoints; i += batchSize {
		wg.Add(1)
		go func(batchIndex int, startIdx int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			endIdx := startIdx + batchSize
			if endIdx > totalPoints {
				endIdx = totalPoints
			}
			batchPoints := points[startIdx:endIdx]

			batchIDs := make([]string, len(batchPoints))
			for j, p := range batchPoints {
				batchIDs[j] = p.GetId().GetUuid()
			}

			err := s.upsertWithRetry(ctx, collectionName, batchPoints)
			if err != nil {
				resultsChan <- BatchResult{TotalFailed: len(batchPoints), Errors: []error{err}}
			} else {
				resultsChan <- BatchResult{TotalProcessed: len(batchPoints), ProcessedIDs: batchIDs}
			}
		}(i/batchSize, i)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	finalResult := &BatchResult{ProcessedIDs: make([]string, 0, totalPoints)}
	var processedCount int
	for res := range resultsChan {
		finalResult.TotalProcessed += res.TotalProcessed
		finalResult.TotalFailed += res.TotalFailed
		finalResult.ProcessedIDs = append(finalResult.ProcessedIDs, res.ProcessedIDs...)
		if len(res.Errors) > 0 {
			finalResult.Errors = append(finalResult.Errors, res.Errors...)
		}
		processedCount += len(res.ProcessedIDs) + res.TotalFailed
		if progressCallback != nil {
			progressCallback(processedCount, totalPoints, time.Since(start))
		}
	}
	finalResult.Duration = time.Since(start)

	if finalResult.TotalFailed > 0 {
		err := ErrPartialBatchFailure
		if finalResult.TotalProcessed == 0 {
			err = fmt.Errorf("all upsert batches failed: %v", finalResult.Errors)
		}
		return finalResult, err
	}

	return finalResult, nil
}

func (s *Store) upsertWithRetry(ctx context.Context, collectionName string, points []*qdrant.PointStruct) error {
	var lastErr error
	delay := s.batchConfig.RetryDelay

	for attempt := 0; attempt <= s.batchConfig.RetryAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
			delay = time.Duration(float64(delay) * 1.5)
			if delay > s.batchConfig.MaxRetryDelay {
				delay = s.batchConfig.MaxRetryDelay
			}
		}

		wait := true
		_, err := s.client.GetPointsClient().Upsert(ctx, &qdrant.UpsertPoints{
			CollectionName: collectionName,
			Wait:           &wait,
			Points:         points,
		})
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return fmt.Errorf("upsert failed after %d attempts: %w", s.batchConfig.RetryAttempts+1, lastErr)
}

func createQdrantClient(opts options, logger *slog.Logger) (*qdrant.Client, error) {
	if opts.qdrantURL.Host == "" {
		logger.Debug("Creating default Qdrant client")
		client, err := qdrant.DefaultClient()
		if err != nil {
			return nil, fmt.Errorf("default client creation failed: %w", err)
		}
		return client, nil
	}

	portStr := opts.qdrantURL.Port()
	if portStr == "" {
		portStr = "6334"
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid port %q: %w", ErrInvalidURL, portStr, err)
	}

	hostname := opts.qdrantURL.Hostname()
	logger.Debug("Creating custom Qdrant client", "host", hostname, "port", port)

	config := &qdrant.Config{
		Host: hostname,
		Port: port,
	}

	if opts.apiKey != "" {
		config.APIKey = opts.apiKey
	}

	client, err := qdrant.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("custom client creation failed: %w", err)
	}

	return client, nil
}

func (s *Store) SimilaritySearch(
	ctx context.Context,
	query string,
	numDocuments int,
	options ...vectorstores.Option,
) ([]schema.Document, error) {
	if strings.TrimSpace(query) == "" {
		s.logger.WarnContext(ctx, "Empty query provided")
		return []schema.Document{}, nil
	}

	if numDocuments <= 0 {
		s.logger.WarnContext(ctx, "Invalid number of documents requested", "num_documents", numDocuments)
		return nil, ErrInvalidNumDocuments
	}

	if s.embedder == nil {
		s.logger.ErrorContext(ctx, "Embedder not provided for search")
		return nil, ErrMissingEmbedder
	}

	opts := vectorstores.ParseOptions(options...)
	collectionName := s.getCollectionName(opts)

	embedStart := time.Now()
	queryVector, err := s.embedder.EmbedQuery(ctx, query)
	embedDuration := time.Since(embedStart)

	if err != nil {
		s.logger.ErrorContext(ctx, "Query embedding failed",
			"error", err, "duration", embedDuration)
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	searchStart := time.Now()
	searchResult, err := s.client.GetPointsClient().Search(ctx, &qdrant.SearchPoints{
		CollectionName: collectionName,
		Vector:         queryVector,
		Limit:          uint64(numDocuments),
		WithPayload: &qdrant.WithPayloadSelector{
			SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true},
		},
		ScoreThreshold: &opts.ScoreThreshold,
	})
	searchDuration := time.Since(searchStart)

	if err != nil {
		if stat, ok := status.FromError(err); ok && stat.Code() == codes.NotFound {
			s.logger.WarnContext(ctx, "Collection not found during search", "collection", collectionName)
			return nil, vectorstores.ErrCollectionNotFound
		}
		s.logger.ErrorContext(ctx, "Search failed",
			"error", err, "collection", collectionName, "duration", searchDuration)
		return nil, fmt.Errorf("qdrant search failed: %w", err)
	}

	results := searchResult.GetResult()
	docs := make([]schema.Document, 0, len(results))
	for _, point := range results {
		docs = append(docs, s.payloadToDocument(point.GetPayload()))
	}

	return docs, nil
}

func (s *Store) SimilaritySearchWithScores(
	ctx context.Context,
	query string,
	numDocuments int,
	options ...vectorstores.Option,
) ([]vectorstores.DocumentWithScore, error) {
	start := time.Now()
	s.logger.DebugContext(ctx, "Starting similarity search with scores",
		"query_length", len(query), "num_documents", numDocuments)

	if strings.TrimSpace(query) == "" {
		s.logger.WarnContext(ctx, "Empty query provided for scored search")
		return []vectorstores.DocumentWithScore{}, nil
	}

	if numDocuments <= 0 {
		return nil, ErrInvalidNumDocuments
	}

	if s.embedder == nil {
		return nil, ErrMissingEmbedder
	}

	opts := vectorstores.ParseOptions(options...)
	collectionName := s.getCollectionName(opts)

	queryVector, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		s.logger.ErrorContext(ctx, "Query embedding failed for scored search", "error", err)
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	filter := buildQdrantFilter(opts.Filters)

	searchResult, err := s.client.GetPointsClient().Search(ctx, &qdrant.SearchPoints{
		CollectionName: collectionName,
		Vector:         queryVector,
		Limit:          uint64(numDocuments),
		WithPayload: &qdrant.WithPayloadSelector{
			SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true},
		},
		ScoreThreshold: &opts.ScoreThreshold,
		Filter:         filter,
	})
	if err != nil {
		if stat, ok := status.FromError(err); ok && stat.Code() == codes.NotFound {
			s.logger.WarnContext(ctx, "Collection not found during scored search", "collection", collectionName)
			return nil, vectorstores.ErrCollectionNotFound
		}
		s.logger.ErrorContext(ctx, "Scored search failed", "error", err, "collection", collectionName)
		return nil, fmt.Errorf("qdrant search failed: %w", err)
	}

	results := searchResult.GetResult()
	docsWithScore := make([]vectorstores.DocumentWithScore, len(results))

	var minScore, maxScore float32 = 1.0, 0.0
	for i, point := range results {
		score := point.GetScore()
		if score < minScore {
			minScore = score
		}
		if score > maxScore {
			maxScore = score
		}

		docsWithScore[i] = vectorstores.DocumentWithScore{
			Document: s.payloadToDocument(point.GetPayload()),
			Score:    score,
		}
	}

	duration := time.Since(start)
	s.logger.InfoContext(ctx, "Similarity search with scores completed",
		"collection", collectionName, "results", len(docsWithScore),
		"min_score", minScore, "max_score", maxScore, "duration", duration)

	return docsWithScore, nil
}

func (s *Store) DeleteDocuments(ctx context.Context, ids []string, options ...vectorstores.Option) error {
	start := time.Now()
	s.logger.DebugContext(ctx, "Starting document deletion", "count", len(ids))

	if len(ids) == 0 {
		s.logger.DebugContext(ctx, "No document IDs provided for deletion")
		return nil
	}

	opts := vectorstores.ParseOptions(options...)
	collectionName := s.getCollectionName(opts)

	pointIds := make([]*qdrant.PointId, len(ids))
	for i, id := range ids {
		pointIds[i] = &qdrant.PointId{
			PointIdOptions: &qdrant.PointId_Uuid{Uuid: id},
		}
	}

	wait := true
	_, err := s.client.GetPointsClient().Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collectionName,
		Wait:           &wait,
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Points{
				Points: &qdrant.PointsIdsList{Ids: pointIds},
			},
		},
	})

	duration := time.Since(start)
	if err != nil {
		s.logger.ErrorContext(ctx, "Document deletion failed",
			"error", err, "collection", collectionName, "duration", duration)
		return fmt.Errorf("failed to delete documents from qdrant: %w", err)
	}

	s.logger.InfoContext(ctx, "Documents deleted successfully",
		"count", len(ids), "collection", collectionName, "duration", duration)
	return nil
}

func (s *Store) ListCollections(ctx context.Context) ([]string, error) {
	start := time.Now()
	s.logger.DebugContext(ctx, "Listing collections")

	resp, err := s.client.GetCollectionsClient().List(ctx, &qdrant.ListCollectionsRequest{})
	duration := time.Since(start)

	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to list collections", "error", err, "duration", duration)
		return nil, fmt.Errorf("failed to list qdrant collections: %w", err)
	}

	collections := resp.GetCollections()
	names := make([]string, len(collections))
	for i, col := range collections {
		names[i] = col.GetName()
	}

	s.logger.DebugContext(ctx, "Collections listed successfully",
		"count", len(names), "duration", duration)
	return names, nil
}

func (s *Store) CreateCollection(ctx context.Context, name string, dimension int, options ...vectorstores.Option) error {
	start := time.Now()
	s.logger.InfoContext(ctx, "Creating collection", "name", name, "dimension", dimension)

	if strings.TrimSpace(name) == "" {
		return ErrMissingCollectionName
	}

	if dimension <= 0 {
		return fmt.Errorf("dimension must be positive, got %d", dimension)
	}

	exists, err := s.collectionExists(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}
	if exists {
		s.logger.WarnContext(ctx, "Collection already exists", "name", name)
		return ErrCollectionExists
	}

	_, err = s.client.GetCollectionsClient().Create(ctx, &qdrant.CreateCollection{
		CollectionName: name,
		VectorsConfig: &qdrant.VectorsConfig{
			Config: &qdrant.VectorsConfig_Params{
				Params: &qdrant.VectorParams{
					Size:     uint64(dimension),
					Distance: qdrant.Distance_Cosine,
				},
			},
		},
	})

	duration := time.Since(start)
	if err != nil {
		s.logger.ErrorContext(ctx, "Collection creation failed",
			"name", name, "error", err, "duration", duration)
		return fmt.Errorf("failed to create qdrant collection: %w", err)
	}

	s.logger.InfoContext(ctx, "Collection created successfully",
		"name", name, "dimension", dimension, "duration", duration)
	return nil
}

func (s *Store) DeleteCollection(ctx context.Context, name string) error {
	start := time.Now()
	s.logger.InfoContext(ctx, "Deleting collection", "name", name)

	if strings.TrimSpace(name) == "" {
		return ErrMissingCollectionName
	}

	_, err := s.client.GetCollectionsClient().Delete(ctx, &qdrant.DeleteCollection{
		CollectionName: name,
	})

	duration := time.Since(start)
	if err != nil {
		if stat, ok := status.FromError(err); ok && stat.Code() == codes.NotFound {
			s.logger.WarnContext(ctx, "Collection not found for deletion", "name", name)
			return vectorstores.ErrCollectionNotFound
		}
		s.logger.ErrorContext(ctx, "Collection deletion failed",
			"name", name, "error", err, "duration", duration)
		return fmt.Errorf("failed to delete collection: %w", err)
	}

	s.logger.InfoContext(ctx, "Collection deleted successfully", "name", name, "duration", duration)
	return nil
}

func (s *Store) DeleteDocumentsByFilter(ctx context.Context, filters map[string]any, options ...vectorstores.Option) error {
	opts := vectorstores.ParseOptions(options...)
	collectionName := s.getCollectionName(opts)

	// buildQdrantFilter is a helper you already have for searching
	qdrantFilter := buildQdrantFilter(filters)
	if qdrantFilter == nil {
		return errors.New("cannot delete with an empty filter")
	}

	wait := true
	pointsSelector := &qdrant.PointsSelector{
		PointsSelectorOneOf: &qdrant.PointsSelector_Filter{
			Filter: qdrantFilter,
		},
	}

	_, err := s.client.GetPointsClient().Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collectionName,
		Wait:           &wait,
		Points:         pointsSelector,
	})
	if err != nil {
		return fmt.Errorf("failed to delete documents by filter: %w", err)
	}
	s.logger.InfoContext(ctx, "Documents deleted successfully by filter", "collection", collectionName, "filter_keys", maps.Keys(filters))
	return nil
}

func (s *Store) SimilaritySearchBatch(
	ctx context.Context,
	queries []string,
	numDocuments int,
	options ...vectorstores.Option,
) ([][]schema.Document, error) {
	if len(queries) == 0 {
		s.logger.WarnContext(ctx, "No queries provided for batch search")
		return nil, nil
	}

	if numDocuments <= 0 {
		s.logger.WarnContext(ctx, "Invalid number of documents requested", "num_documents", numDocuments)
		return nil, ErrInvalidNumDocuments
	}

	if s.embedder == nil {
		s.logger.ErrorContext(ctx, "Embedder not provided for batch search")
		return nil, ErrMissingEmbedder
	}

	opts := vectorstores.ParseOptions(options...)
	collectionName := s.getCollectionName(opts)

	// Embed all queries at once
	queryVectors, err := s.embedder.EmbedQueries(ctx, queries)
	if err != nil {
		s.logger.ErrorContext(ctx, "Batch query embedding failed", "error", err)
		return nil, fmt.Errorf("failed to embed queries: %w", err)
	}

	searchRequests := make([]*qdrant.SearchPoints, 0, len(queryVectors))
	for _, vector := range queryVectors {
		searchRequests = append(searchRequests, &qdrant.SearchPoints{
			CollectionName: collectionName,
			Vector:         vector,
			Limit:          uint64(numDocuments),
			WithPayload: &qdrant.WithPayloadSelector{
				SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true},
			},
			ScoreThreshold: &opts.ScoreThreshold,
		})
	}

	searchResp, err := s.client.GetPointsClient().SearchBatch(ctx, &qdrant.SearchBatchPoints{
		SearchPoints: searchRequests,
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "Batch search failed", "error", err)
		return nil, fmt.Errorf("qdrant batch search failed: %w", err)
	}

	// Convert results
	batchResults := make([][]schema.Document, len(searchResp.GetResult()))
	for i, result := range searchResp.GetResult() {
		docs := make([]schema.Document, 0, len(result.GetResult()))
		for _, point := range result.GetResult() {
			docs = append(docs, s.payloadToDocument(point.GetPayload()))
		}
		batchResults[i] = docs
	}

	return batchResults, nil
}

func (s *Store) Health(ctx context.Context) error {
	_, err := s.client.GetCollectionsClient().List(ctx, &qdrant.ListCollectionsRequest{})
	if err != nil {
		s.logger.ErrorContext(ctx, "Health check failed", "error", err)
		return fmt.Errorf("qdrant health check failed: %w", err)
	}

	return nil
}

func (s *Store) generateDocumentID(doc schema.Document) string {
	if id, exists := doc.Metadata["id"]; exists {
		if idStr, ok := id.(string); ok && idStr != "" {
			return idStr
		}
	}

	return uuid.New().String()
}

func (s *Store) getCollectionName(opts vectorstores.Options) string {
	if opts.NameSpace != "" {
		return opts.NameSpace
	}
	return s.collectionName
}

func (s *Store) ensureCollection(ctx context.Context, collectionName string) error {
	exists, err := s.collectionExists(ctx, collectionName)
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}

	if exists {
		return nil
	}

	if s.embedder == nil {
		return ErrMissingEmbedder
	}

	dimension, err := s.embedder.GetDimension(ctx)
	if err != nil {
		return fmt.Errorf("could not get embedder dimension: %w", err)
	}

	s.logger.InfoContext(ctx, "Creating collection automatically",
		"collection", collectionName, "dimension", dimension)

	_, err = s.client.GetCollectionsClient().Create(ctx, &qdrant.CreateCollection{
		CollectionName: collectionName,
		VectorsConfig: &qdrant.VectorsConfig{
			Config: &qdrant.VectorsConfig_Params{
				Params: &qdrant.VectorParams{
					Size:     uint64(dimension),
					Distance: qdrant.Distance_Cosine,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create qdrant collection: %w", err)
	}

	select {
	case <-time.After(500 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}

	s.logger.InfoContext(ctx, "Collection created successfully", "collection", collectionName)
	return nil
}

func (s *Store) collectionExists(ctx context.Context, name string) (bool, error) {
	_, err := s.client.GetCollectionsClient().Get(ctx, &qdrant.GetCollectionInfoRequest{
		CollectionName: name,
	})
	if err != nil {
		if stat, ok := status.FromError(err); ok && stat.Code() == codes.NotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Store) documentToPayload(doc schema.Document) map[string]*qdrant.Value {
	payload := make(map[string]*qdrant.Value, len(doc.Metadata)+1)
	payload["page_content"] = &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: doc.PageContent}}

	for key, value := range doc.Metadata {
		if qValue := s.convertToQdrantValue(value); qValue != nil {
			payload[key] = qValue
		}
	}
	return payload
}

func (s *Store) convertToQdrantValue(value any) *qdrant.Value {
	switch v := value.(type) {
	case string:
		return &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: v}}
	case int:
		return &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: int64(v)}}
	case int32:
		return &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: int64(v)}}
	case int64:
		return &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: v}}
	case float32:
		return &qdrant.Value{Kind: &qdrant.Value_DoubleValue{DoubleValue: float64(v)}}
	case float64:
		return &qdrant.Value{Kind: &qdrant.Value_DoubleValue{DoubleValue: v}}
	case bool:
		return &qdrant.Value{Kind: &qdrant.Value_BoolValue{BoolValue: v}}
	case []string:
		values := make([]*qdrant.Value, len(v))
		for i, str := range v {
			values[i] = &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: str}}
		}
		return &qdrant.Value{Kind: &qdrant.Value_ListValue{
			ListValue: &qdrant.ListValue{Values: values},
		}}
	case nil:
		return &qdrant.Value{Kind: &qdrant.Value_NullValue{}}
	default:
		return &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: fmt.Sprintf("%v", v)}}
	}
}

func (s *Store) payloadToDocument(payload map[string]*qdrant.Value) schema.Document {
	doc := schema.Document{
		Metadata: make(map[string]any),
	}

	for key, value := range payload {
		if key == "page_content" {
			doc.PageContent = value.GetStringValue()
			continue
		}

		if convertedValue := s.convertFromQdrantValue(value); convertedValue != nil {
			doc.Metadata[key] = convertedValue
		}
	}

	return doc
}

func (s *Store) convertFromQdrantValue(value *qdrant.Value) any {
	switch v := value.GetKind().(type) {
	case *qdrant.Value_StringValue:
		return v.StringValue
	case *qdrant.Value_IntegerValue:
		return v.IntegerValue
	case *qdrant.Value_DoubleValue:
		return v.DoubleValue
	case *qdrant.Value_BoolValue:
		return v.BoolValue
	case *qdrant.Value_ListValue:
		// Handle list values
		list := make([]any, len(v.ListValue.GetValues()))
		for i, val := range v.ListValue.GetValues() {
			list[i] = s.convertFromQdrantValue(val)
		}
		return list
	case *qdrant.Value_NullValue:
		return nil
	default:
		return nil
	}
}

func buildQdrantFilter(filters map[string]any) *qdrant.Filter {
	if len(filters) == 0 {
		return nil
	}

	conditions := make([]*qdrant.Condition, 0, len(filters))

	for key, value := range filters {
		var match *qdrant.Match

		switch v := value.(type) {
		case string:
			match = &qdrant.Match{MatchValue: &qdrant.Match_Keyword{Keyword: v}}
		case int:
			match = &qdrant.Match{MatchValue: &qdrant.Match_Integer{Integer: int64(v)}}
		case int64:
			match = &qdrant.Match{MatchValue: &qdrant.Match_Integer{Integer: v}}
		case bool:
			match = &qdrant.Match{MatchValue: &qdrant.Match_Boolean{Boolean: v}}
		case []string:
			match = &qdrant.Match{MatchValue: &qdrant.Match_Keywords{Keywords: &qdrant.RepeatedStrings{Strings: v}}}
		case []int64:
			match = &qdrant.Match{MatchValue: &qdrant.Match_Integers{Integers: &qdrant.RepeatedIntegers{Integers: v}}}
		case []int:
			int64Slice := make([]int64, len(v))
			for i, num := range v {
				int64Slice[i] = int64(num)
			}
			match = &qdrant.Match{MatchValue: &qdrant.Match_Integers{Integers: &qdrant.RepeatedIntegers{Integers: int64Slice}}}
		default:
			slog.Warn("Unsupported filter type for key", "key", key, "type", fmt.Sprintf("%T", v))
			continue
		}

		condition := &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key:   key,
					Match: match,
				},
			},
		}
		conditions = append(conditions, condition)
	}

	if len(conditions) == 0 {
		return nil
	}

	return &qdrant.Filter{
		Must: conditions,
	}
}
