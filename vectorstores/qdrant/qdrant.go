package qdrant

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
)

// Common errors returned by the Qdrant vector store implementation.
var (
	ErrMissingEmbedder       = errors.New("qdrant: embedder is required but not provided")
	ErrMissingCollectionName = errors.New("qdrant: collection name is required")
	ErrCollectionNotFound    = errors.New("qdrant: collection not found")
	ErrInvalidNumDocuments   = errors.New("qdrant: number of documents must be positive")
	ErrConnectionFailed      = errors.New("qdrant: connection failed")
	ErrInvalidURL            = errors.New("qdrant: invalid URL provided")
	ErrCollectionExists      = errors.New("qdrant: collection already exists")
	ErrEmptyQuery            = errors.New("qdrant: query cannot be empty")
	ErrDimensionMismatch     = errors.New("qdrant: vector dimension mismatch")
)

// Store provides a comprehensive Qdrant vector database integration with
// full CRUD operations, collection management, and advanced search capabilities.
type Store struct {
	client         *qdrant.Client
	embedder       embeddings.Embedder
	collectionName string
	logger         *slog.Logger
	options        options
}

// Compile-time interface check
var _ vectorstores.VectorStore = (*Store)(nil)

func New(opts ...Option) (*Store, error) {
	storeOptions, err := parseOptions(opts...)
	if err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}

	logger := storeOptions.logger.With("component", "qdrant_store",
		"collection", storeOptions.collectionName)

	client, err := createQdrantClient(storeOptions, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant client: %w", err)
	}

	store := &Store{
		client:         client,
		embedder:       storeOptions.embedder,
		collectionName: storeOptions.collectionName,
		logger:         logger,
		options:        storeOptions,
	}

	logger.Info("Qdrant store initialized successfully",
		"config", storeOptions.String())

	return store, nil
}

// createQdrantClient handles the creation of Qdrant client with proper error handling.
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
		portStr = "6334" // Default Qdrant port
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid port %q: %w", ErrInvalidURL, portStr, err)
	}

	// Use Hostname() to get only the host without the port
	hostname := opts.qdrantURL.Hostname()
	logger.Debug("Creating custom Qdrant client", "host", hostname, "port", port)

	config := &qdrant.Config{
		Host: hostname,
		Port: port,
	}

	// Add API key if provided
	if opts.apiKey != "" {
		config.APIKey = opts.apiKey
	}

	client, err := qdrant.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("custom client creation failed: %w", err)
	}

	return client, nil
}

// AddDocuments adds multiple documents to the vector store with comprehensive error handling.
func (s *Store) AddDocuments(ctx context.Context, docs []schema.Document, options ...vectorstores.Option) ([]string, error) {
	start := time.Now()
	s.logger.DebugContext(ctx, "Starting document addition", "count", len(docs))

	if len(docs) == 0 {
		s.logger.DebugContext(ctx, "No documents to add")
		return []string{}, nil
	}

	if s.embedder == nil {
		s.logger.ErrorContext(ctx, "Embedder not provided")
		return nil, ErrMissingEmbedder
	}

	// Parse options
	opts := vectorstores.ParseOptions(options...)
	collectionName := s.getCollectionName(opts)

	// Ensure collection exists
	if err := s.ensureCollection(ctx, collectionName); err != nil {
		s.logger.ErrorContext(ctx, "Failed to ensure collection exists", "error", err)
		return nil, fmt.Errorf("collection preparation failed: %w", err)
	}

	// Extract texts and validate
	texts := make([]string, len(docs))
	for i, doc := range docs {
		if doc.PageContent == "" {
			s.logger.WarnContext(ctx, "Document has empty content", "index", i)
		}
		texts[i] = doc.PageContent
	}

	// Generate embeddings
	embedStart := time.Now()
	s.logger.DebugContext(ctx, "Embedding documents", "count", len(texts))

	vectors, err := s.embedder.EmbedDocuments(ctx, texts)
	embedDuration := time.Since(embedStart)

	if err != nil {
		s.logger.ErrorContext(ctx, "Document embedding failed",
			"error", err, "duration", embedDuration)
		return nil, fmt.Errorf("document embedding failed: %w", err)
	}

	if len(vectors) != len(docs) {
		s.logger.ErrorContext(ctx, "Embedding count mismatch",
			"expected", len(docs), "got", len(vectors))
		return nil, fmt.Errorf("embedder returned %d vectors for %d documents", len(vectors), len(docs))
	}

	// Prepare points for insertion
	points := make([]*qdrant.PointStruct, len(docs))
	ids := make([]string, len(docs))

	for i, doc := range docs {
		docID := s.generateDocumentID(doc)
		ids[i] = docID

		points[i] = &qdrant.PointStruct{
			Id: &qdrant.PointId{
				PointIdOptions: &qdrant.PointId_Uuid{Uuid: docID},
			},
			Vectors: &qdrant.Vectors{
				VectorsOptions: &qdrant.Vectors_Vector{
					Vector: &qdrant.Vector{Data: vectors[i]},
				},
			},
			Payload: s.documentToPayload(doc),
		}
	}

	// Insert points
	insertStart := time.Now()
	wait := true
	_, err = s.client.GetPointsClient().Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collectionName,
		Wait:           &wait,
		Points:         points,
	})
	insertDuration := time.Since(insertStart)

	totalDuration := time.Since(start)
	if err != nil {
		s.logger.ErrorContext(ctx, "Document insertion failed",
			"error", err, "collection", collectionName,
			"insert_duration", insertDuration, "total_duration", totalDuration)
		return nil, fmt.Errorf("failed to upsert points to qdrant: %w", err)
	}

	s.logger.InfoContext(ctx, "Documents added successfully",
		"count", len(docs), "collection", collectionName,
		"embed_duration", embedDuration, "insert_duration", insertDuration,
		"total_duration", totalDuration)

	return ids, nil
}

// SimilaritySearch performs vector similarity search with comprehensive logging.
func (s *Store) SimilaritySearch(
	ctx context.Context,
	query string,
	numDocuments int,
	options ...vectorstores.Option,
) ([]schema.Document, error) {
	start := time.Now()
	s.logger.DebugContext(ctx, "Starting similarity search",
		"query_length", len(query), "num_documents", numDocuments)

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

	// Embed query
	embedStart := time.Now()
	queryVector, err := s.embedder.EmbedQuery(ctx, query)
	embedDuration := time.Since(embedStart)

	if err != nil {
		s.logger.ErrorContext(ctx, "Query embedding failed",
			"error", err, "duration", embedDuration)
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	// Perform search
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
			return nil, ErrCollectionNotFound
		}
		s.logger.ErrorContext(ctx, "Search failed",
			"error", err, "collection", collectionName, "duration", searchDuration)
		return nil, fmt.Errorf("qdrant search failed: %w", err)
	}

	// Process results
	results := searchResult.GetResult()
	docs := make([]schema.Document, 0, len(results))
	for _, point := range results {
		docs = append(docs, s.payloadToDocument(point.GetPayload()))
	}

	totalDuration := time.Since(start)
	s.logger.InfoContext(ctx, "Similarity search completed",
		"collection", collectionName, "query_length", len(query),
		"results_found", len(docs), "embed_duration", embedDuration,
		"search_duration", searchDuration, "total_duration", totalDuration)

	return docs, nil
}

// SimilaritySearchWithScores performs similarity search returning documents with similarity scores.
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

	// Embed query
	queryVector, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		s.logger.ErrorContext(ctx, "Query embedding failed for scored search", "error", err)
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	filter := buildQdrantFilter(opts.Filters)

	// Perform search with scores
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
			return nil, ErrCollectionNotFound
		}
		s.logger.ErrorContext(ctx, "Scored search failed", "error", err, "collection", collectionName)
		return nil, fmt.Errorf("qdrant search failed: %w", err)
	}

	// Process results with scores
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

// DeleteDocuments removes documents from the vector store by their IDs.
func (s *Store) DeleteDocuments(ctx context.Context, ids []string, options ...vectorstores.Option) error {
	start := time.Now()
	s.logger.DebugContext(ctx, "Starting document deletion", "count", len(ids))

	if len(ids) == 0 {
		s.logger.DebugContext(ctx, "No document IDs provided for deletion")
		return nil
	}

	opts := vectorstores.ParseOptions(options...)
	collectionName := s.getCollectionName(opts)

	// Convert string IDs to Qdrant point IDs
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

// ListCollections returns all collection names in the Qdrant instance.
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

// CreateCollection creates a new collection with specified parameters.
func (s *Store) CreateCollection(ctx context.Context, name string, dimension int, options ...vectorstores.Option) error {
	start := time.Now()
	s.logger.InfoContext(ctx, "Creating collection", "name", name, "dimension", dimension)

	if strings.TrimSpace(name) == "" {
		return ErrMissingCollectionName
	}

	if dimension <= 0 {
		return fmt.Errorf("dimension must be positive, got %d", dimension)
	}

	// Check if collection already exists
	exists, err := s.collectionExists(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}
	if exists {
		s.logger.WarnContext(ctx, "Collection already exists", "name", name)
		return ErrCollectionExists
	}

	// TODO: fix me
	// opts := qdrant.ParseOptions(options...)
	// distance := qdrant.Distance_Cosine
	// if opts.DistanceMetric != "" {
	// 	if d, ok := parseDistanceMetric(opts.DistanceMetric); ok {
	// 		distance = d
	// 	}
	// }

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

// DeleteCollection removes a collection and all its data.
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
			return ErrCollectionNotFound
		}
		s.logger.ErrorContext(ctx, "Collection deletion failed",
			"name", name, "error", err, "duration", duration)
		return fmt.Errorf("failed to delete collection: %w", err)
	}

	s.logger.InfoContext(ctx, "Collection deleted successfully", "name", name, "duration", duration)
	return nil
}

// Health checks the health of the Qdrant connection.
func (s *Store) Health(ctx context.Context) error {
	s.logger.DebugContext(ctx, "Checking Qdrant health")

	// Try to list collections as a health check
	_, err := s.client.GetCollectionsClient().List(ctx, &qdrant.ListCollectionsRequest{})
	if err != nil {
		s.logger.ErrorContext(ctx, "Health check failed", "error", err)
		return fmt.Errorf("qdrant health check failed: %w", err)
	}

	s.logger.DebugContext(ctx, "Qdrant health check passed")
	return nil
}

// Helper methods

// generateDocumentID creates a unique ID for a document.
func (s *Store) generateDocumentID(doc schema.Document) string {
	// Use custom ID if provided in metadata
	if id, exists := doc.Metadata["id"]; exists {
		if idStr, ok := id.(string); ok && idStr != "" {
			return idStr
		}
	}

	// Generate UUID
	return uuid.New().String()
}

// getCollectionName determines the collection name from options or store default.
func (s *Store) getCollectionName(opts vectorstores.Options) string {
	if opts.NameSpace != "" {
		return opts.NameSpace
	}
	return s.collectionName
}

// ensureCollection ensures the specified collection exists, creating it if necessary.
func (s *Store) ensureCollection(ctx context.Context, collectionName string) error {
	exists, err := s.collectionExists(ctx, collectionName)
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}

	if exists {
		s.logger.DebugContext(ctx, "Collection already exists", "collection", collectionName)
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

	// Brief wait for collection initialization
	select {
	case <-time.After(500 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}

	s.logger.InfoContext(ctx, "Collection created successfully", "collection", collectionName)
	return nil
}

// collectionExists checks if a collection exists.
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

// documentToPayload converts a schema.Document to Qdrant payload format.
func (s *Store) documentToPayload(doc schema.Document) map[string]*qdrant.Value {
	payload := make(map[string]*qdrant.Value, len(doc.Metadata)+1)
	payload["page_content"] = &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: doc.PageContent}}

	for key, value := range doc.Metadata {
		if qValue := s.convertToQdrantValue(value); qValue != nil {
			payload[key] = qValue
		} else {
			s.logger.DebugContext(context.Background(), "Skipping unsupported metadata type",
				"key", key, "type", fmt.Sprintf("%T", value))
		}
	}
	return payload
}

// convertToQdrantValue converts Go values to Qdrant value format with comprehensive type support.
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
		// Support for string arrays
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
		// For unsupported types, convert to string representation
		return &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: fmt.Sprintf("%v", v)}}
	}
}

// payloadToDocument converts Qdrant payload back to schema.Document.
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

// convertFromQdrantValue converts Qdrant values back to Go types.
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

// buildQdrantFilter converts a generic filter map to a Qdrant Filter.
func buildQdrantFilter(filters map[string]any) *qdrant.Filter {
	if len(filters) == 0 {
		return nil
	}

	conditions := make([]*qdrant.Condition, 0, len(filters))

	for key, value := range filters {
		var match *qdrant.Match

		// Create the appropriate Match type based on the Go type of the value.
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
			// Handle multiple keyword matches
			match = &qdrant.Match{MatchValue: &qdrant.Match_Keywords{Keywords: &qdrant.RepeatedStrings{Strings: v}}}
		case []int64:
			// Handle multiple integer matches
			match = &qdrant.Match{MatchValue: &qdrant.Match_Integers{Integers: &qdrant.RepeatedIntegers{Integers: v}}}
		case []int:
			// Convert []int to []int64 for the protobuf
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

	// Combine all conditions using "Must" (AND logic)
	return &qdrant.Filter{
		Must: conditions,
	}
}
