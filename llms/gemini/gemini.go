package gemini

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/iterator"
	"google.golang.org/genai"

	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/httpclient"
	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

var nonRetryablePatterns = []string{
	"API_KEY_INVALID",
	"API_KEY_DISABLED",
	"PERMISSION_DENIED",
	"INVALID_ARGUMENT",
	"QUOTA_EXCEEDED", // Daily quota exceeded — retrying won't help until quota resets
}

var retryablePatterns = []string{
	"RESOURCE_EXHAUSTED",
	"INTERNAL",
	"429",
	"500",
	"503",
	"connection reset",
	"connection refused",
	"timeout",
	"unexpected EOF",
}

type LLM struct {
	client     *genai.Client
	options    options
	logger     *slog.Logger
	httpClient *http.Client
	ownsClient bool
	retryCfg   httpclient.RetryConfig

	dimension int
	dimOnce   sync.Once
	dimMu     sync.Mutex
}

var _ llms.Model = (*LLM)(nil)
var _ embeddings.Embedder = (*LLM)(nil)
var _ embeddings.ImageEmbedder = (*LLM)(nil)

func New(ctx context.Context, opts ...Option) (*LLM, error) {
	o := applyOptions(opts...)

	if o.apiKey == "" {
		o.apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if o.apiKey == "" {
		return nil, ErrNoAPIKey
	}

	if o.model == "" {
		return nil, ErrInvalidModel
	}

	if o.logger == nil {
		o.logger = slog.Default()
	}

	var ownsClient bool
	httpClient := o.httpClient
	if httpClient == nil {
		httpClient = newOptimizedHTTPClient(o.requestTimeout)
		ownsClient = true
	}

	clientConfig := &genai.ClientConfig{
		APIKey:     o.apiKey,
		HTTPClient: httpClient,
	}

	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	llm := &LLM{
		client:     client,
		options:    o,
		logger:     o.logger.With("component", "gemini_llm", "model", o.model),
		httpClient: httpClient,
		ownsClient: ownsClient,
		retryCfg: httpclient.RetryConfig{
			Attempts: o.retry.Attempts,
			Delay:    o.retry.Delay,
			MaxDelay: o.retry.MaxDelay,
			Jitter:   o.retry.Jitter,
		},
	}
	llm.retryCfg.IsRetryable = llm.isRetryableError

	llm.logger.InfoContext(ctx, "Gemini LLM initialized successfully", "api_key_prefix", maskAPIKey(o.apiKey))
	return llm, nil
}

func newOptimizedHTTPClient(timeout time.Duration) *http.Client {
	cfg := httpclient.NewConfig(
		httpclient.WithTimeout(timeout),
	)
	return httpclient.NewClient(cfg)
}

func (g *LLM) Close() error {
	if g.ownsClient && g.httpClient != nil {
		if tr, ok := g.httpClient.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}
	return nil
}

func (g *LLM) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return llms.GenerateFromSinglePrompt(ctx, g, prompt, options...)
}

func (g *LLM) GenerateContent(
	ctx context.Context,
	messages []schema.MessageContent,
	options ...llms.CallOption,
) (*schema.ContentResponse, error) {
	start := time.Now()

	callOpts := &llms.CallOptions{}
	for _, opt := range options {
		opt(callOpts)
	}

	genConfig := &genai.GenerateContentConfig{}
	if callOpts.TemperatureSet() {
		genConfig.Temperature = genai.Ptr(float32(callOpts.Temperature))
	}

	geminiHistory, systemInstruction, err := g.convertToGeminiMessages(messages)
	if err != nil {
		return nil, err
	}

	if systemInstruction != nil {
		geminiHistory = append([]*genai.Content{systemInstruction}, geminiHistory...)
	}

	if len(geminiHistory) == 0 {
		return nil, ErrNoMessages
	}

	if callOpts.StreamingFunc == nil {
		var resp *genai.GenerateContentResponse
		retryErr := httpclient.DoWithRetry(ctx, &g.retryCfg, "gemini generate content", func() error {
			var genErr error
			resp, genErr = g.client.Models.GenerateContent(ctx, g.options.model, geminiHistory, genConfig)
			return genErr
		})
		duration := time.Since(start)
		if retryErr != nil {
			g.logger.ErrorContext(ctx, "Gemini client failed", "error", retryErr, "duration", duration)
			return nil, retryErr
		}
		return g.responseToSchema(resp, duration)
	}

	var fullResponse strings.Builder
	var finalResp *genai.GenerateContentResponse

	for resp, errStream := range g.client.Models.GenerateContentStream(ctx, g.options.model, geminiHistory, genConfig) {
		if errors.Is(errStream, iterator.Done) {
			break
		}
		if errStream != nil {
			g.logger.ErrorContext(ctx, "Gemini stream error", "error", errStream)
			return nil, fmt.Errorf("gemini stream failed: %w", errStream)
		}

		finalResp = resp
		chunkContent := g.extractContentFromResponse(resp)
		fullResponse.WriteString(chunkContent)
		if err := callOpts.StreamingFunc(ctx, []byte(chunkContent)); err != nil {
			return nil, fmt.Errorf("streaming function returned an error: %w", err)
		}
	}

	duration := time.Since(start)

	var totalTokens int32
	if finalResp != nil && finalResp.UsageMetadata != nil {
		totalTokens = finalResp.UsageMetadata.TotalTokenCount
	}

	return &schema.ContentResponse{
		Choices: []*schema.ContentChoice{
			{
				Content: fullResponse.String(),
				GenerationInfo: map[string]any{
					"TotalTokens": totalTokens,
					"Duration":    duration,
					"Model":       g.options.model,
				},
			},
		},
	}, nil
}

func (g *LLM) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	contents := make([]*genai.Content, len(texts))
	for i, text := range texts {
		contents[i] = genai.NewContentFromText(text, genai.RoleUser)
	}

	var res *genai.EmbedContentResponse
	err := httpclient.DoWithRetry(ctx, &g.retryCfg, "gemini embed documents", func() error {
		var genErr error
		res, genErr = g.client.Models.EmbedContent(ctx, g.options.embeddingModel, contents, nil)
		return genErr
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrEmbeddings, err)
	}

	if len(res.Embeddings) != len(texts) {
		return nil, fmt.Errorf("%w: expected %d embeddings, but got %d", ErrEmbeddings, len(texts), len(res.Embeddings))
	}

	embeddings := make([][]float32, len(res.Embeddings))
	for i, e := range res.Embeddings {
		embeddings[i] = e.Values
	}
	return embeddings, nil
}

func (g *LLM) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	content := genai.NewContentFromText(text, genai.RoleUser)
	var res *genai.EmbedContentResponse
	err := httpclient.DoWithRetry(ctx, &g.retryCfg, "gemini embed query", func() error {
		var genErr error
		res, genErr = g.client.Models.EmbedContent(ctx, g.options.embeddingModel, []*genai.Content{content}, nil)
		return genErr
	})
	if err != nil {
		return nil, fmt.Errorf("%w: query embedding failed: %w", ErrEmbeddings, err)
	}

	if len(res.Embeddings) == 0 || res.Embeddings[0] == nil {
		return nil, fmt.Errorf("%w: embedding is nil or empty", ErrEmbeddings)
	}
	return res.Embeddings[0].Values, nil
}

func (g *LLM) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	return g.EmbedDocuments(ctx, texts)
}

func (g *LLM) EmbedImages(ctx context.Context, images []embeddings.ImageData) ([][]float32, error) {
	if len(images) == 0 {
		return nil, fmt.Errorf("%w: no images provided", ErrEmbeddings)
	}

	contents := make([]*genai.Content, len(images))
	for i, img := range images {
		contents[i] = genai.NewContentFromBytes(img.Data, img.MimeType, genai.RoleUser)
	}

	var res *genai.EmbedContentResponse
	err := httpclient.DoWithRetry(ctx, &g.retryCfg, "gemini embed images", func() error {
		var genErr error
		res, genErr = g.client.Models.EmbedContent(ctx, g.options.embeddingModel, contents, nil)
		return genErr
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrEmbeddings, err)
	}

	if len(res.Embeddings) != len(images) {
		return nil, fmt.Errorf("%w: expected %d embeddings, but got %d", ErrEmbeddings, len(images), len(res.Embeddings))
	}

	result := make([][]float32, len(res.Embeddings))
	for i, e := range res.Embeddings {
		result[i] = e.Values
	}
	return result, nil
}

func (g *LLM) EmbedImage(ctx context.Context, image embeddings.ImageData) ([]float32, error) {
	embeddings, err := g.EmbedImages(ctx, []embeddings.ImageData{image})
	if err != nil {
		return nil, err
	}

	if len(embeddings) == 0 {
		return nil, fmt.Errorf("%w: embedding is nil or empty", ErrEmbeddings)
	}
	return embeddings[0], nil
}

func (g *LLM) GetDimension(ctx context.Context) (int, error) {
	g.dimMu.Lock()
	if g.dimension > 0 {
		dim := g.dimension
		g.dimMu.Unlock()
		return dim, nil
	}
	g.dimMu.Unlock()

	var onceErr error
	g.dimOnce.Do(func() {
		sampleEmbedding, err := g.EmbedQuery(ctx, "dimension")
		if err != nil {
			onceErr = fmt.Errorf("failed to get dimension by embedding sample text: %w", err)
			return
		}
		g.dimMu.Lock()
		g.dimension = len(sampleEmbedding)
		g.dimMu.Unlock()
	})

	if onceErr != nil {
		g.dimMu.Lock()
		g.dimOnce = sync.Once{}
		g.dimMu.Unlock()
		return 0, onceErr
	}

	return g.dimension, nil
}

func (g *LLM) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	errStr := err.Error()
	for _, pattern := range nonRetryablePatterns {
		if strings.Contains(errStr, pattern) {
			return false
		}
	}

	if httpclient.IsRetryableError(err) {
		return true
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

func (g *LLM) convertToGeminiMessages(messages []schema.MessageContent) ([]*genai.Content, *genai.Content, error) {
	geminiContents := make([]*genai.Content, 0, len(messages))
	var systemInstruction *genai.Content
	var systemMessageFound bool

	for i, msg := range messages {
		var role genai.Role
		switch msg.Role {
		case schema.ChatMessageTypeHuman:
			role = genai.RoleUser
		case schema.ChatMessageTypeAI:
			role = genai.RoleModel
		case schema.ChatMessageTypeSystem:
			if i != 0 || systemMessageFound {
				return nil, nil, ErrSystemMessage
			}
			systemInstruction = genai.NewContentFromText(msg.GetTextContent(), genai.RoleUser)
			systemMessageFound = true
			continue
		default:
			role = genai.RoleUser
		}

		parts := make([]*genai.Part, 0, len(msg.Parts))
		for _, p := range msg.Parts {
			switch part := p.(type) {
			case schema.TextContent:
				parts = append(parts, genai.NewPartFromText(part.String()))
			default:
				return nil, nil, fmt.Errorf("unsupported content part type: %T", part)
			}
		}
		geminiContents = append(geminiContents, genai.NewContentFromParts(parts, role))
	}
	return geminiContents, systemInstruction, nil
}

func (g *LLM) responseToSchema(resp *genai.GenerateContentResponse, duration time.Duration) (*schema.ContentResponse, error) {
	if len(resp.Candidates) == 0 {
		return nil, ErrNoContent
	}

	choice := resp.Candidates[0]
	if choice.Content == nil || len(choice.Content.Parts) == 0 {
		return nil, ErrNoContent
	}

	content := g.extractContentFromResponse(resp)
	var totalTokens int32
	if resp.UsageMetadata != nil {
		totalTokens = resp.UsageMetadata.TotalTokenCount
	}

	return &schema.ContentResponse{
		Choices: []*schema.ContentChoice{
			{
				Content:    content,
				StopReason: string(choice.FinishReason),
				GenerationInfo: map[string]any{
					"TotalTokens": totalTokens,
					"Duration":    duration,
					"Model":       g.options.model,
				},
			},
		},
	}, nil
}

func (g *LLM) extractContentFromResponse(resp *genai.GenerateContentResponse) string {
	var builder strings.Builder
	if resp == nil {
		return ""
	}
	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				builder.WriteString(part.Text)
			}
		}
	}
	return builder.String()
}
