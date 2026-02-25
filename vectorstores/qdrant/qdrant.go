package qdrant

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
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
	ErrEmbeddingTotalFailure = errors.New("qdrant: all embedding batches failed")
	ErrMissingSparseName     = errors.New("qdrant: sparse vector name required for hybrid search configuration")
)

const (
	DefaultBatchSize      = 100
	MaxBatchSize          = 1000
	DefaultMaxConcurrency = 8
	DefaultRetryAttempts  = 3
	DefaultRetryDelay     = 2 * time.Second
	DefaultMaxRetryDelay  = 30 * time.Second
	DefaultRetryJitter    = 1 * time.Second

	// DefaultDenseVectorName is the implicit name for the default dense vector in Qdrant.
	DefaultDenseVectorName = ""
)

type BatchResult struct {
	TotalProcessed int           `json:"total_processed"`
	TotalFailed    int           `json:"total_failed"`
	Duration       time.Duration `json:"duration"`
	Errors         []error       `json:"errors,omitempty"`
	ProcessedIDs   []string      `json:"processed_ids,omitempty"`
}

type BatchConfig struct {
	BatchSize               int           `json:"batch_size"`
	MaxConcurrency          int           `json:"max_concurrency"`
	RetryAttempts           int           `json:"retry_attempts"`
	RetryDelay              time.Duration `json:"retry_delay"`
	MaxRetryDelay           time.Duration `json:"max_retry_delay"`
	EmbeddingBatchSize      int           `json:"embedding_batch_size,omitempty"`
	RetryJitter             time.Duration `json:"retry_jitter"`
	EmbeddingMaxConcurrency int           `json:"embedding_max_concurrency,omitempty"`
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

	// Use the provided batch config, or create a default one.
	var batchConfig BatchConfig
	if storeOptions.batchConfig != nil {
		batchConfig = *storeOptions.batchConfig
	} else {
		batchConfig = BatchConfig{
			BatchSize:      DefaultBatchSize,
			MaxConcurrency: DefaultMaxConcurrency,
			RetryAttempts:  DefaultRetryAttempts,
			RetryDelay:     DefaultRetryDelay,
			MaxRetryDelay:  DefaultMaxRetryDelay,
			RetryJitter:    DefaultRetryJitter,
		}
	}

	store := &Store{
		client:         client,
		embedder:       storeOptions.embedder,
		collectionName: storeOptions.collectionName,
		logger:         logger,
		options:        storeOptions,
	}
	store.SetBatchConfig(batchConfig)

	logger.Info("Qdrant store initialized successfully",
		"config", storeOptions.String(),
		"batch_config", store.batchConfig,
	)
	return store, nil
}

func (s *Store) SetBatchConfig(config BatchConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.batchConfig = s.validateAndNormalizeBatchConfig(config)
	s.logBatchConfigUpdate(s.batchConfig)
}

func (s *Store) validateAndNormalizeBatchConfig(config BatchConfig) BatchConfig {
	// Validate and set primary config
	if config.BatchSize <= 0 {
		config.BatchSize = DefaultBatchSize
	}
	if config.BatchSize > MaxBatchSize {
		config.BatchSize = MaxBatchSize
	}
	if config.MaxConcurrency <= 0 {
		config.MaxConcurrency = DefaultMaxConcurrency
	}

	// Validate optional embedding config (allow 0 for fallback)
	if config.EmbeddingBatchSize < 0 {
		config.EmbeddingBatchSize = 0
	}
	if config.EmbeddingMaxConcurrency <= 0 {
		config.EmbeddingMaxConcurrency = 0
	}

	// Validate retry config
	if config.RetryAttempts < 0 {
		config.RetryAttempts = DefaultRetryAttempts
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = DefaultRetryDelay
	}
	if config.MaxRetryDelay <= 0 {
		config.MaxRetryDelay = DefaultMaxRetryDelay
	}

	return config
}

func (s *Store) logBatchConfigUpdate(config BatchConfig) {
	s.logger.Info("Batch configuration updated",
		"batch_size", config.BatchSize,
		"max_concurrency", config.MaxConcurrency,
		"embedding_batch_size", config.EmbeddingBatchSize,
		"embedding_max_concurrency", config.EmbeddingMaxConcurrency,
		"retry_attempts", config.RetryAttempts,
		"retry_delay", config.RetryDelay,
		"max_retry_delay", config.MaxRetryDelay,
		"retry_jitter", config.RetryJitter,
	)
}

// embedAndCreatePointsInParallel processes documents in parallel to generate embeddings and create Qdrant points.
// It uses a fail-fast mechanism with context cancellation to stop all work on the first error.
func (s *Store) embedBatchWithRetry(ctx context.Context, batchDocs []schema.Document) ([]schema.Document, [][]float32, error) {
	validDocs := make([]schema.Document, 0, len(batchDocs))
	texts := make([]string, 0, len(batchDocs))
	for _, doc := range batchDocs {
		trimmedContent := strings.TrimSpace(doc.PageContent)
		if trimmedContent != "" {
			validDocs = append(validDocs, doc)
			texts = append(texts, trimmedContent)
		} else {
			s.logger.WarnContext(ctx, "Skipping embedding for empty document in batch")
		}
	}

	if len(validDocs) == 0 {
		return []schema.Document{}, [][]float32{}, nil
	}

	var vectors [][]float32
	var err error
	delay := s.batchConfig.RetryDelay

	for attempt := 0; attempt <= s.batchConfig.RetryAttempts; attempt++ {
		if attempt > 0 {
			if retryErr := s.waitForRetryDelay(ctx, delay, attempt, err); retryErr != nil {
				return nil, nil, retryErr
			}
			delay = s.calculateNextDelay(delay)
		}

		vectors, err = s.embedder.EmbedDocuments(ctx, texts)
		if err == nil {
			break
		}

		if !s.isRetryableError(err) {
			break
		}
	}

	if err != nil {
		finalErr := fmt.Errorf("batch embedding failed after %d attempts: %w",
			s.batchConfig.RetryAttempts+1, err)
		s.logger.ErrorContext(ctx, "Permanent embedding failure for batch", "error", finalErr)
		return nil, nil, finalErr
	}

	return validDocs, vectors, nil
}

func (s *Store) waitForRetryDelay(ctx context.Context, delay time.Duration, attempt int, err error) error {
	jitter := time.Duration(rand.IntN(int(s.batchConfig.RetryJitter.Milliseconds()))) * time.Millisecond //nolint:gosec // rand.IntN is sufficient for retry jitter
	totalDelay := delay + jitter

	s.logger.WarnContext(ctx, "Retrying embedding for batch",
		"attempt", fmt.Sprintf("%d/%d", attempt, s.batchConfig.RetryAttempts),
		"delay", totalDelay, "error", err)

	select {
	case <-time.After(totalDelay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Store) calculateNextDelay(delay time.Duration) time.Duration {
	delay *= 2
	if delay > s.batchConfig.MaxRetryDelay {
		delay = s.batchConfig.MaxRetryDelay
	}
	return delay
}

func (s *Store) isRetryableError(err error) bool {
	errStr := err.Error()
	retryableErrors := []string{
		"Error 500",
		"Error 503",
		"Status: INTERNAL",
		"Status: UNAVAILABLE",
		"Status: RESOURCE_EXHAUSTED",
		"Error 429",
		"RESOURCE_EXHAUSTED",
		"unexpected EOF",
		"connection refused",
		"connection reset",
		"context deadline exceeded",
		"transport is closing",
		"client connection is closing",
	}

	for _, retryableErr := range retryableErrors {
		if strings.Contains(errStr, retryableErr) {
			return true
		}
	}

	// Check for gRPC status codes
	if stat, ok := status.FromError(err); ok {
		switch stat.Code() {
		case codes.Unavailable, codes.ResourceExhausted, codes.DeadlineExceeded, codes.Internal:
			return true
		}
	}

	return false
}

// doWithRetry executes a function with retry logic for transient errors.
func (s *Store) doWithRetry(ctx context.Context, operation string, fn func() error) error {
	retryAttempts := s.options.retryAttempts
	if retryAttempts == 0 {
		retryAttempts = s.batchConfig.RetryAttempts
	}

	if retryAttempts == 0 {
		return fn()
	}

	delay := s.options.retryDelay
	if delay == 0 {
		delay = s.batchConfig.RetryDelay
	}

	maxDelay := s.options.maxRetryDelay
	if maxDelay == 0 {
		maxDelay = s.batchConfig.MaxRetryDelay
	}

	jitter := s.options.retryJitter
	if jitter == 0 {
		jitter = s.batchConfig.RetryJitter
	}

	var lastErr error
	for attempt := 0; attempt <= retryAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if we've exhausted our retries
		if attempt >= retryAttempts {
			break
		}

		// Only retry on transient errors
		if !s.isRetryableError(err) {
			break
		}

		// Wait before retrying
		if retryErr := s.waitForRetryDelayWithJitter(ctx, delay, attempt+1, operation, err, jitter); retryErr != nil {
			return retryErr
		}

		delay = s.calculateNextDelayWithMax(delay, maxDelay)
	}

	return fmt.Errorf("%s failed after %d attempts: %w", operation, retryAttempts+1, lastErr)
}

// waitForRetryDelayWithJitter waits for the specified delay with jitter.
func (s *Store) waitForRetryDelayWithJitter(ctx context.Context, delay time.Duration, attempt int, operation string, err error, jitter time.Duration) error {
	var jitterDur time.Duration
	if jitter > 0 {
		jitterDur = time.Duration(rand.IntN(int(jitter.Milliseconds()))) * time.Millisecond //nolint:gosec // rand.IntN is sufficient for retry jitter
	}
	totalDelay := delay + jitterDur

	s.logger.WarnContext(ctx, "Retrying operation",
		"operation", operation,
		"attempt", fmt.Sprintf("%d/%d", attempt, s.options.retryAttempts),
		"delay", totalDelay,
		"error", err)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(totalDelay):
		return nil
	}
}

// calculateNextDelayWithMax calculates the next retry delay with exponential backoff.
func (s *Store) calculateNextDelayWithMax(delay, maxDelay time.Duration) time.Duration {
	delay *= 2
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

func (s *Store) createQdrantPoints(batchDocs []schema.Document, vectors [][]float32) ([]*qdrant.PointStruct, []string) {
	batchPoints := make([]*qdrant.PointStruct, len(batchDocs))
	batchIDs := make([]string, len(batchDocs))

	for j, doc := range batchDocs {
		docID := s.generateDocumentID(doc)
		batchIDs[j] = docID

		point := &qdrant.PointStruct{
			Id:      &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: docID}},
			Payload: s.documentToPayload(doc),
		}

		// Configure vectors (dense + optional sparse)
		if doc.Sparse != nil && len(s.options.sparseVectors) > 0 {
			// Sparse vector configuration (Hybrid)
			sparseName := s.options.sparseVectors[0]
			point.Vectors = &qdrant.Vectors{
				VectorsOptions: &qdrant.Vectors_Vectors{
					Vectors: &qdrant.NamedVectors{
						Vectors: map[string]*qdrant.Vector{
							DefaultDenseVectorName: {Data: vectors[j]},
							sparseName: {
								Data: doc.Sparse.Values,
								Indices: &qdrant.SparseIndices{
									Data: doc.Sparse.Indices,
								},
							},
						},
					},
				},
			}
		} else {
			// Dense only configuration
			point.Vectors = &qdrant.Vectors{
				VectorsOptions: &qdrant.Vectors_Vector{
					Vector: &qdrant.Vector{Data: vectors[j]},
				},
			}
		}

		batchPoints[j] = point
	}

	return batchPoints, batchIDs
}

func (s *Store) GetBatchConfig() BatchConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.batchConfig
}

// GetEmbedder returns the embedder used by the store.
func (s *Store) GetEmbedder() embeddings.Embedder {
	return s.embedder
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

	if s.embedder == nil {
		return nil, ErrMissingEmbedder
	}

	opts := vectorstores.ParseOptions(options...)
	collectionName := s.getCollectionName(opts)

	if err := s.ensureCollection(ctx, collectionName); err != nil {
		return nil, fmt.Errorf("collection preparation failed: %w", err)
	}

	batchSize := s.batchConfig.BatchSize
	numBatches := int(math.Ceil(float64(totalDocs) / float64(batchSize)))

	s.logger.InfoContext(ctx, "Starting streaming document addition pipeline",
		"total_documents", totalDocs, "num_batches", numBatches)

	start := time.Now()
	allIDs := make([]string, totalDocs)
	var finalErrors []error
	var mu sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, s.batchConfig.MaxConcurrency)

	processedCount := 0
	for i := 0; i < totalDocs; i += batchSize {
		end := i + batchSize
		if end > totalDocs {
			end = totalDocs
		}

		batchIdx := i
		batchDocs := docs[i:end]

		wg.Add(1)
		go func(idx int, bDocs []schema.Document) {
			defer wg.Done()

			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				mu.Lock()
				finalErrors = append(finalErrors, ctx.Err())
				mu.Unlock()
				return
			}

			ids, err := s.processBatch(ctx, collectionName, bDocs)
			mu.Lock()
			if err != nil {
				finalErrors = append(finalErrors, err)
			} else {
				for j, id := range ids {
					allIDs[idx+j] = id
				}
			}
			processedCount += len(bDocs)
			currentCount := processedCount
			mu.Unlock()

			// Call progress callback outside the lock to prevent potential deadlock
			// if the callback panics or blocks
			if progressCallback != nil {
				progressCallback(currentCount, totalDocs, time.Since(start))
			}
		}(batchIdx, batchDocs)
	}

	wg.Wait()

	if len(finalErrors) > 0 {
		combinedErr := errors.Join(finalErrors...)
		if processedCount == 0 {
			return nil, fmt.Errorf("%w: %w", ErrEmbeddingTotalFailure, combinedErr)
		}
		return allIDs, fmt.Errorf("%w: %w", ErrPartialBatchFailure, combinedErr)
	}

	s.logger.InfoContext(ctx, "Document addition pipeline completed successfully",
		"total_processed", processedCount, "duration", time.Since(start))

	return allIDs, nil
}

func (s *Store) processBatch(ctx context.Context, collectionName string, batchDocs []schema.Document) ([]string, error) {
	// 1. Embed the batch
	validDocs, vectors, err := s.embedBatchWithRetry(ctx, batchDocs)
	if err != nil {
		return nil, err
	}

	if len(validDocs) == 0 {
		return []string{}, nil
	}

	// 2. Create Qdrant points
	points, ids := s.createQdrantPoints(validDocs, vectors)

	// 3. Upsert immediately
	if err := s.upsertWithRetry(ctx, collectionName, points); err != nil {
		return nil, err
	}

	return ids, nil
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
	// Build gRPC dial options with connection pooling and keepalive
	grpcOpts := buildGrpcOptions(opts)

	if opts.qdrantURL.Host == "" {
		logger.Debug("Creating default Qdrant client with gRPC options",
			"timeout", opts.timeout,
			"keepalive_time", opts.keepaliveTime,
			"keepalive_timeout", opts.keepaliveTimeout)

		client, err := qdrant.NewClient(&qdrant.Config{
			GrpcOptions: grpcOpts,
		})
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
	logger.Debug("Creating custom Qdrant client",
		"host", hostname, "port", port,
		"timeout", opts.timeout,
		"keepalive_time", opts.keepaliveTime,
		"keepalive_timeout", opts.keepaliveTimeout)

	config := &qdrant.Config{
		Host:        hostname,
		Port:        port,
		GrpcOptions: grpcOpts,
	}

	if opts.apiKey != "" {
		config.APIKey = opts.apiKey
	}

	if opts.useTLS {
		config.UseTLS = true
	}

	client, err := qdrant.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("custom client creation failed: %w", err)
	}

	return client, nil
}

// buildGrpcOptions creates gRPC dial options for connection management.
func buildGrpcOptions(opts options) []grpc.DialOption {
	kaParams := keepalive.ClientParameters{
		Time:                opts.keepaliveTime,
		Timeout:             opts.keepaliveTimeout,
		PermitWithoutStream: true,
	}

	grpcOpts := []grpc.DialOption{
		grpc.WithKeepaliveParams(kaParams),
		grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingConfig": [{"round_robin":{}}], "methodConfig": [{"name": [{"service": ""}], "timeout": "%.9fs"}]}`, opts.timeout.Seconds())),
	}

	// Append any custom gRPC options
	if len(opts.grpcOptions) > 0 {
		grpcOpts = append(grpcOpts, opts.grpcOptions...)
	}

	return grpcOpts
}

// executeSearch is the shared search implementation used by both SimilaritySearch
// and SimilaritySearchWithScores. It handles embedding, hybrid vs dense branching,
// metadata filters, and error handling in a single place.
func (s *Store) executeSearch(
	ctx context.Context,
	query string,
	numDocuments int,
	opts vectorstores.Options,
) ([]*qdrant.ScoredPoint, string, error) {
	collectionName := s.getCollectionName(opts)

	queryVector, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		s.logger.ErrorContext(ctx, "Query embedding failed", "error", err)
		return nil, collectionName, fmt.Errorf("failed to embed query: %w", err)
	}

	filter := buildQdrantFilter(opts.Filters)
	limit := uint64(numDocuments)
	searchStart := time.Now()

	var results []*qdrant.ScoredPoint

	if len(s.options.sparseVectors) > 0 && opts.SparseQuery != nil {
		sparseName := s.options.sparseVectors[0]

		var denseNamePtr *string
		if DefaultDenseVectorName != "" {
			name := DefaultDenseVectorName
			denseNamePtr = &name
		}

		queryPoints := &qdrant.QueryPoints{
			CollectionName: collectionName,
			Query: &qdrant.Query{
				Variant: &qdrant.Query_Fusion{
					Fusion: qdrant.Fusion_RRF,
				},
			},
			Prefetch: []*qdrant.PrefetchQuery{
				{
					Query: &qdrant.Query{
						Variant: &qdrant.Query_Nearest{
							Nearest: qdrant.NewVectorInputDense(queryVector),
						},
					},
					Using:  denseNamePtr,
					Limit:  &limit,
					Filter: filter,
				},
				{
					Query: &qdrant.Query{
						Variant: &qdrant.Query_Nearest{
							Nearest: qdrant.NewVectorInputSparse(opts.SparseQuery.Indices, opts.SparseQuery.Values),
						},
					},
					Using:  &sparseName,
					Limit:  &limit,
					Filter: filter,
				},
			},
			WithPayload: &qdrant.WithPayloadSelector{
				SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true},
			},
			ScoreThreshold: &opts.ScoreThreshold,
		}

		err = s.doWithRetry(ctx, "hybrid_query", func() error {
			var retryErr error
			queryResult, retryErr := s.client.GetPointsClient().Query(ctx, queryPoints)
			if retryErr != nil {
				return retryErr
			}
			results = queryResult.GetResult()
			return nil
		})
		if err != nil {
			return nil, collectionName, fmt.Errorf("qdrant hybrid query failed: %w", err)
		}

		s.logger.DebugContext(ctx, "Hybrid search executed",
			"fusion", "RRF", "results", len(results),
			"duration", time.Since(searchStart))
	} else {
		err = s.doWithRetry(ctx, "similarity_search", func() error {
			searchResult, retryErr := s.client.GetPointsClient().Search(ctx, &qdrant.SearchPoints{
				CollectionName: collectionName,
				Vector:         queryVector,
				Limit:          limit,
				WithPayload: &qdrant.WithPayloadSelector{
					SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true},
				},
				ScoreThreshold: &opts.ScoreThreshold,
				Filter:         filter,
			})
			if retryErr != nil {
				// Check for non-retryable NotFound error
				if stat, ok := status.FromError(retryErr); ok && stat.Code() == codes.NotFound {
					return fmt.Errorf("%w: %w", vectorstores.ErrCollectionNotFound, retryErr)
				}
				return retryErr
			}
			results = searchResult.GetResult()
			return nil
		})
		if err != nil {
			if errors.Is(err, vectorstores.ErrCollectionNotFound) {
				s.logger.WarnContext(ctx, "Collection not found during search", "collection", collectionName)
				return nil, collectionName, vectorstores.ErrCollectionNotFound
			}
			s.logger.ErrorContext(ctx, "Search failed",
				"error", err, "collection", collectionName, "duration", time.Since(searchStart))
			return nil, collectionName, fmt.Errorf("qdrant search failed: %w", err)
		}

		s.logger.DebugContext(ctx, "Dense search executed",
			"results", len(results), "duration", time.Since(searchStart))
	}

	return results, collectionName, nil
}

func (s *Store) SimilaritySearch(
	ctx context.Context,
	query string,
	numDocuments int,
	options ...vectorstores.Option,
) ([]schema.Document, error) {
	if strings.TrimSpace(query) == "" {
		opts := vectorstores.ParseOptions(options...)
		if len(opts.Filters) == 0 {
			s.logger.WarnContext(ctx, "Empty query provided with no filters")
			return []schema.Document{}, nil
		}
	}

	if numDocuments <= 0 {
		return nil, ErrInvalidNumDocuments
	}
	if s.embedder == nil {
		return nil, ErrMissingEmbedder
	}

	opts := vectorstores.ParseOptions(options...)
	if err := s.validateHybridSearchOptions(opts); err != nil {
		return nil, err
	}

	results, _, err := s.executeSearch(ctx, query, numDocuments, opts)
	if err != nil {
		return nil, err
	}

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
	if err := s.validateHybridSearchOptions(opts); err != nil {
		return nil, err
	}

	results, collectionName, err := s.executeSearch(ctx, query, numDocuments, opts)
	if err != nil {
		return nil, err
	}

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

	s.logger.InfoContext(ctx, "Similarity search with scores completed",
		"collection", collectionName, "results", len(docsWithScore),
		"min_score", minScore, "max_score", maxScore, "duration", time.Since(start))

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
	err := s.doWithRetry(ctx, "delete_documents", func() error {
		_, retryErr := s.client.GetPointsClient().Delete(ctx, &qdrant.DeletePoints{
			CollectionName: collectionName,
			Wait:           &wait,
			Points: &qdrant.PointsSelector{
				PointsSelectorOneOf: &qdrant.PointsSelector_Points{
					Points: &qdrant.PointsIdsList{Ids: pointIds},
				},
			},
		})
		return retryErr
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

	req := &qdrant.CreateCollection{
		CollectionName: name,
		VectorsConfig: &qdrant.VectorsConfig{
			Config: &qdrant.VectorsConfig_Params{
				Params: &qdrant.VectorParams{
					Size:     uint64(dimension),
					Distance: qdrant.Distance_Cosine,
				},
			},
		},
	}

	req.SparseVectorsConfig = s.buildSparseVectorConfig()

	// Apply Binary Quantization if configured in store options
	if s.options.binaryQuantization {
		s.logger.DebugContext(ctx, "CreateCollection: Enabling binary quantization")
		always := true
		req.QuantizationConfig = &qdrant.QuantizationConfig{
			Quantization: &qdrant.QuantizationConfig_Binary{
				Binary: &qdrant.BinaryQuantization{
					AlwaysRam: &always,
				},
			},
		}
	}

	_, err = s.client.GetCollectionsClient().Create(ctx, req)

	duration := time.Since(start)
	if err != nil {
		s.logger.ErrorContext(ctx, "Collection creation failed",
			"name", name, "error", err, "duration", duration)
		return fmt.Errorf("failed to create qdrant collection: %w", err)
	}

	s.logger.InfoContext(ctx, "Collection created successfully",
		"name", name, "dimension", dimension, "duration", duration)

	// Apply Payload Indexes if configured
	s.applyPayloadIndexes(ctx, name)

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

	limit := uint64(numDocuments)
	var batchResults [][]schema.Document

	if len(s.options.sparseVectors) > 0 && len(opts.SparseQueries) > 0 {
		if len(opts.SparseQueries) != len(queryVectors) {
			return nil, fmt.Errorf("sparse query count (%d) must match query count (%d)",
				len(opts.SparseQueries), len(queryVectors))
		}
		sparseName := s.options.sparseVectors[0]
		var denseNamePtr *string
		if DefaultDenseVectorName != "" {
			name := DefaultDenseVectorName
			denseNamePtr = &name
		}

		queryRequests := make([]*qdrant.QueryPoints, 0, len(queryVectors))
		for i, vector := range queryVectors {
			queryRequests = append(queryRequests, &qdrant.QueryPoints{
				CollectionName: collectionName,
				Query: &qdrant.Query{
					Variant: &qdrant.Query_Fusion{
						Fusion: qdrant.Fusion_RRF,
					},
				},
				Prefetch: []*qdrant.PrefetchQuery{
					{
						Query: &qdrant.Query{
							Variant: &qdrant.Query_Nearest{
								Nearest: qdrant.NewVectorInputDense(vector),
							},
						},
						Using: denseNamePtr,
						Limit: &limit,
					},
					{
						Query: &qdrant.Query{
							Variant: &qdrant.Query_Nearest{
								Nearest: qdrant.NewVectorInputSparse(opts.SparseQueries[i].Indices, opts.SparseQueries[i].Values),
							},
						},
						Using: &sparseName,
						Limit: &limit,
					},
				},
				WithPayload: &qdrant.WithPayloadSelector{
					SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true},
				},
				ScoreThreshold: &opts.ScoreThreshold,
			})
		}

		batchQueryResp, err := s.client.GetPointsClient().QueryBatch(ctx, &qdrant.QueryBatchPoints{
			CollectionName: collectionName,
			QueryPoints:    queryRequests,
		})
		if err != nil {
			return nil, fmt.Errorf("qdrant hybrid batch query failed: %w", err)
		}

		batchResults = make([][]schema.Document, len(batchQueryResp.GetResult()))
		for i, resp := range batchQueryResp.GetResult() {
			results := resp.GetResult()
			docs := make([]schema.Document, 0, len(results))
			for _, point := range results {
				docs = append(docs, s.payloadToDocument(point.GetPayload()))
			}
			batchResults[i] = docs
		}
	} else {
		if len(opts.SparseQueries) > 0 {
			s.logger.WarnContext(ctx, "Sparse queries provided but no sparse vectors configured")
		}
		searchRequests := make([]*qdrant.SearchPoints, 0, len(queryVectors))
		for _, vector := range queryVectors {
			searchRequests = append(searchRequests, &qdrant.SearchPoints{
				CollectionName: collectionName,
				Vector:         vector,
				Limit:          limit,
				WithPayload: &qdrant.WithPayloadSelector{
					SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true},
				},
				ScoreThreshold: &opts.ScoreThreshold,
			})
		}

		searchResp, err := s.client.GetPointsClient().SearchBatch(ctx, &qdrant.SearchBatchPoints{
			SearchPoints:   searchRequests,
			CollectionName: collectionName,
		})
		if err != nil {
			s.logger.ErrorContext(ctx, "Batch search failed", "error", err)
			return nil, fmt.Errorf("qdrant batch search failed: %w", err)
		}

		batchResults = make([][]schema.Document, len(searchResp.GetResult()))
		for i, result := range searchResp.GetResult() {
			docs := make([]schema.Document, 0, len(result.GetResult()))
			for _, point := range result.GetResult() {
				docs = append(docs, s.payloadToDocument(point.GetPayload()))
			}
			batchResults[i] = docs
		}
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

// Close closes the gRPC connection to the Qdrant server.
// It should be called when the Store is no longer needed to release resources.
func (s *Store) Close() error {
	if s.client == nil {
		return nil
	}
	if err := s.client.Close(); err != nil {
		s.logger.Error("Failed to close Qdrant client", "error", err)
		return fmt.Errorf("failed to close qdrant client: %w", err)
	}
	s.logger.Info("Qdrant client closed successfully")
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
	if opts.CollectionName != "" {
		return opts.CollectionName
	}
	if opts.NameSpace != "" {
		return opts.NameSpace
	}
	return s.collectionName
}

func (s *Store) ensureCollection(ctx context.Context, collectionName string) error {
	s.logger.DebugContext(ctx, "EnsureCollection: Starting check", "collection", collectionName)

	exists, err := s.collectionExists(ctx, collectionName)
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}

	if exists {
		s.logger.DebugContext(ctx, "EnsureCollection: Collection already exists, proceeding.", "collection", collectionName)
		return nil
	}

	s.logger.InfoContext(ctx, "EnsureCollection: Collection does not exist, attempting to create it.", "collection", collectionName)
	if s.embedder == nil {
		s.logger.ErrorContext(ctx, "EnsureCollection: Cannot create collection without an embedder.")
		return ErrMissingEmbedder
	}

	s.logger.DebugContext(ctx, "EnsureCollection: Getting vector dimension from embedder...")
	dimension, err := s.embedder.GetDimension(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "EnsureCollection: Failed to get dimension from embedder", "error", err)
		return fmt.Errorf("could not get embedder dimension: %w", err)
	}

	s.logger.DebugContext(ctx, "EnsureCollection: Sending CreateCollection request to Qdrant...")
	req := &qdrant.CreateCollection{
		CollectionName: collectionName,
		VectorsConfig: &qdrant.VectorsConfig{
			Config: &qdrant.VectorsConfig_Params{
				Params: &qdrant.VectorParams{
					Size:     uint64(dimension),
					Distance: qdrant.Distance_Cosine,
				},
			},
		},
	}

	if s.options.binaryQuantization {
		s.logger.DebugContext(ctx, "EnsureCollection: Enabling binary quantization")
		always := true
		req.QuantizationConfig = &qdrant.QuantizationConfig{
			Quantization: &qdrant.QuantizationConfig_Binary{
				Binary: &qdrant.BinaryQuantization{
					AlwaysRam: &always,
				},
			},
		}
	}

	req.SparseVectorsConfig = s.buildSparseVectorConfig()

	_, err = s.client.GetCollectionsClient().Create(ctx, req)
	if err != nil {
		// Check if error is "AlreadyExists" (race condition during concurrent ops)
		if stat, ok := status.FromError(err); ok && stat.Code() == codes.AlreadyExists {
			s.logger.DebugContext(ctx, "EnsureCollection: Collection created by another process concurrently", "collection", collectionName)
			return nil
		}
		s.logger.ErrorContext(ctx, "EnsureCollection: gRPC call to create collection failed", "error", err)
		return fmt.Errorf("failed to create qdrant collection: %w", err)
	}

	// Apply Payload Indexes if configured
	s.applyPayloadIndexes(ctx, collectionName)

	select {
	case <-time.After(500 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}

	s.logger.InfoContext(ctx, "EnsureCollection: Collection created successfully", "collection", collectionName)
	return nil
}

func (s *Store) createPayloadIndex(ctx context.Context, collectionName, key string) error {
	wait := true
	_, err := s.client.GetPointsClient().CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
		CollectionName: collectionName,
		FieldName:      key,
		FieldType:      qdrant.FieldType_FieldTypeKeyword.Enum(),
		Wait:           &wait,
	})
	return err
}

func (s *Store) applyPayloadIndexes(ctx context.Context, collectionName string) {
	s.logger.InfoContext(ctx, "Creating mandatory symbol mapping indexes", "collection", collectionName)
	for _, key := range []string{"identifier", "is_definition"} {
		if err := s.createPayloadIndex(ctx, collectionName, key); err != nil {
			s.logger.WarnContext(ctx, "Failed to create mandatory payload index", "key", key, "collection", collectionName, "error", err)
		}
	}

	if len(s.options.payloadIndexes) > 0 {
		s.logger.InfoContext(ctx, "Creating additional payload indexes", "collection", collectionName, "keys", s.options.payloadIndexes)
		for _, key := range s.options.payloadIndexes {
			if err := s.createPayloadIndex(ctx, collectionName, key); err != nil {
				s.logger.WarnContext(ctx, "Failed to create payload index", "key", key, "collection", collectionName, "error", err)
			}
		}
	}
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
		case []any:
			match = buildMatchFromSlice(key, v)
		default:
			slog.Warn("Unsupported filter type for key", "key", key, "type", fmt.Sprintf("%T", v))
			continue
		}

		if match == nil {
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

func buildMatchFromSlice(key string, v []any) *qdrant.Match {
	if len(v) == 0 {
		return nil
	}
	// Determine the type of the first non-nil element
	var firstValid any
	for _, elem := range v {
		if elem != nil {
			firstValid = elem
			break
		}
	}
	if firstValid == nil {
		return nil
	}

	switch firstValid.(type) {
	case string:
		strSlice := make([]string, 0, len(v))
		for _, elem := range v {
			if s, ok := elem.(string); ok {
				strSlice = append(strSlice, s)
			}
		}
		return &qdrant.Match{MatchValue: &qdrant.Match_Keywords{Keywords: &qdrant.RepeatedStrings{Strings: strSlice}}}
	case int, int32, int64:
		intSlice := make([]int64, 0, len(v))
		for _, elem := range v {
			switch val := elem.(type) {
			case int:
				intSlice = append(intSlice, int64(val))
			case int32:
				intSlice = append(intSlice, int64(val))
			case int64:
				intSlice = append(intSlice, val)
			}
		}
		return &qdrant.Match{MatchValue: &qdrant.Match_Integers{Integers: &qdrant.RepeatedIntegers{Integers: intSlice}}}
	default:
		slog.Warn("Unsupported slice element type in filter", "key", key, "type", fmt.Sprintf("%T", firstValid))
		return nil
	}
}

func (s *Store) validateHybridSearchOptions(opts vectorstores.Options) error {
	if opts.SparseQuery != nil && len(s.options.sparseVectors) == 0 {
		return ErrMissingSparseName
	}
	if opts.SparseQuery != nil {
		if len(opts.SparseQuery.Indices) != len(opts.SparseQuery.Values) {
			return fmt.Errorf("sparse vector indices and values length mismatch")
		}
	}
	return nil
}

func (s *Store) buildSparseVectorConfig() *qdrant.SparseVectorConfig {
	if len(s.options.sparseVectors) == 0 {
		return nil
	}

	sparseConfig := make(map[string]*qdrant.SparseVectorParams)
	onDisk := true
	for _, sparseName := range s.options.sparseVectors {
		sparseConfig[sparseName] = &qdrant.SparseVectorParams{
			Index: &qdrant.SparseIndexConfig{
				OnDisk: &onDisk,
			},
		}
	}
	return &qdrant.SparseVectorConfig{
		Map: sparseConfig,
	}
}
