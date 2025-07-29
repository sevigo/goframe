package fastapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/sevigo/goframe/embeddings"
)

// embedRequest matches the JSON structure of your FastAPI /embed endpoint's request.
type embedRequest struct {
	Texts []string `json:"texts"`
	Task  string   `json:"task,omitempty"`
}

// embedResponse matches the JSON structure of your FastAPI /embed endpoint's response.
type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embedder communicates with a remote FastAPI server to generate embeddings.
// It directly implements the embeddings.Embedder interface.
type Embedder struct {
	serverURL  string
	httpClient *http.Client
	logger     *slog.Logger
	task       string
	apiKey     string

	// Cached dimension
	dimension int
	dimErr    error
	dimOnce   sync.Once
}

var _ embeddings.Embedder = (*Embedder)(nil)

// New creates a new FastAPI embedder client.
func New(serverURL string, opts ...Option) (embeddings.Embedder, error) {
	if serverURL == "" {
		return nil, fmt.Errorf("server URL cannot be empty")
	}

	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	return &Embedder{
		serverURL:  serverURL,
		httpClient: options.httpClient,
		logger:     options.logger.With("component", "fastapi_embedder"),
		task:       options.task,
		apiKey:     options.apiKey,
	}, nil
}

// EmbedDocuments sends a batch of documents to the FastAPI server for embedding.
func (e *Embedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	// Create the request payload
	reqPayload := embedRequest{
		Texts: texts,
		Task:  e.task,
	}
	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	// Create and send the HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.serverURL+"/embed", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if e.apiKey != "" {
		req.Header.Set("X-Api-Key", e.apiKey)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fastapi server returned non-200 status: %d", resp.StatusCode)
	}

	// Decode the response
	var embedResp embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(embedResp.Embeddings) != len(texts) {
		return nil, fmt.Errorf("mismatch between requested texts (%d) and received embeddings (%d)", len(texts), len(embedResp.Embeddings))
	}

	return embedResp.Embeddings, nil
}

// EmbedQuery embeds a single query.
func (e *Embedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, embeddings.ErrEmptyText
	}

	results, err := e.EmbedDocuments(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no embeddings returned for query")
	}
	return results[0], nil
}

// EmbedQueries embeds a batch of queries.
func (e *Embedder) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	return e.EmbedDocuments(ctx, texts)
}

// GetDimension lazily fetches the embedding dimension by sending a test query.
func (e *Embedder) GetDimension(ctx context.Context) (int, error) {
	e.dimOnce.Do(func() {
		sampleEmbedding, err := e.EmbedQuery(ctx, "dimension_check")
		if err != nil {
			e.dimErr = fmt.Errorf("failed to get dimension: %w", err)
			return
		}
		e.dimension = len(sampleEmbedding)
	})
	return e.dimension, e.dimErr
}
