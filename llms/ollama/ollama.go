package ollama

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/llms/ollama/ollamaclient"
	"github.com/sevigo/goframe/schema"
)

var (
	ErrEmptyResponse       = errors.New("ollama: empty response received")
	ErrIncompleteEmbedding = errors.New("ollama: not all input texts were embedded")
	ErrNoMessages          = errors.New("ollama: no messages provided")
	ErrNoTextParts         = errors.New("ollama: no text parts found in message")
	ErrModelNotFound       = errors.New("ollama: model not found")
	ErrInvalidModel        = errors.New("ollama: invalid model specified")
	ErrContextCanceled     = errors.New("ollama: context canceled")
)

type LLM struct {
	client       *ollamaclient.Client
	options      options
	logger       *slog.Logger
	details      *schema.ModelDetails
	detailsMutex sync.RWMutex
}

var (
	_ llms.Model          = (*LLM)(nil)
	_ embeddings.Embedder = (*LLM)(nil)
	_ llms.Tokenizer      = (*LLM)(nil)
)

func New(opts ...Option) (*LLM, error) {
	o := applyOptions(opts...)

	if o.model == "" {
		return nil, ErrInvalidModel
	}

	// Initialize logger first
	logger := o.logger.With("component", "ollama_llm", "model", o.model)

	// Create client
	var client *ollamaclient.Client
	var err error

	if o.ollamaServerURL != nil {
		client, err = ollamaclient.NewClient(o.ollamaServerURL, o.httpClient, logger)
	} else {
		client, err = ollamaclient.NewDefaultClient(logger)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create ollama client: %w", err)
	}

	llm := &LLM{
		client:  client,
		options: o,
		logger:  logger,
	}

	logger.Info("Ollama LLM initialized successfully")
	return llm, nil
}

func (o *LLM) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	start := time.Now()
	o.logger.DebugContext(ctx, "Starting simple call", "prompt_length", len(prompt))

	result, err := llms.GenerateFromSinglePrompt(ctx, o, prompt, options...)

	duration := time.Since(start)
	if err != nil {
		o.logger.ErrorContext(ctx, "Call failed", "error", err, "duration", duration)
		return "", err
	}

	o.logger.DebugContext(ctx, "Call completed successfully",
		"response_length", len(result), "duration", duration)
	return result, nil
}

func (o *LLM) GenerateContent(
	ctx context.Context,
	messages []schema.MessageContent,
	options ...llms.CallOption,
) (*schema.ContentResponse, error) {
	start := time.Now()
	o.logger.DebugContext(ctx, "Starting Ollama content generation", "message_count", len(messages))

	opts := llms.CallOptions{}
	for _, opt := range options {
		opt(&opts)
	}
	model := o.determineModel(opts)

	chatMsgs, err := o.convertToOllamaMessages(messages)
	if err != nil {
		o.logger.ErrorContext(ctx, "Failed to convert messages", "error", err)
		return nil, err
	}

	isStreaming := opts.StreamingFunc != nil
	req := &api.ChatRequest{
		Model:    model,
		Messages: chatMsgs,
		Stream:   &isStreaming,
	}

	// Add thinking mode if specified (new in v0.11.7)
	if o.options.thinking != nil {
		if req.Options == nil {
			req.Options = make(map[string]interface{})
		}
		req.Options["think"] = *o.options.thinking
	}

	// Add reasoning effort if specified (new in v0.11.5)
	if o.options.reasoningEffort != "" {
		if req.Options == nil {
			req.Options = make(map[string]interface{})
		}
		req.Options["reasoning_effort"] = o.options.reasoningEffort
	}

	var fullResponse strings.Builder
	var finalResp api.ChatResponse
	var mu sync.Mutex

	fn := func(response api.ChatResponse) error {
		mu.Lock()
		fullResponse.WriteString(response.Message.Content)
		if response.Done {
			finalResp = response
		}
		mu.Unlock()

		// Call streaming function outside the lock
		if opts.StreamingFunc != nil && response.Message.Content != "" {
			if errStream := opts.StreamingFunc(ctx, []byte(response.Message.Content)); errStream != nil {
				return fmt.Errorf("streaming function error: %w", errStream)
			}
		}
		return nil
	}

	err = o.client.GenerateChat(ctx, req, fn)
	duration := time.Since(start)

	if err != nil {
		o.logger.ErrorContext(ctx, "Ollama client failed", "error", err, "duration", duration)
		return nil, err
	}

	response := &schema.ContentResponse{
		Choices: []*schema.ContentChoice{
			{
				Content: fullResponse.String(),
				GenerationInfo: map[string]any{
					"CompletionTokens": finalResp.EvalCount,
					"PromptTokens":     finalResp.PromptEvalCount,
					"TotalTokens":      finalResp.EvalCount + finalResp.PromptEvalCount,
					"Duration":         duration,
					"Model":            model,
				},
			},
		},
	}

	o.logger.InfoContext(ctx, "Content generation completed", "duration", duration)
	return response, nil
}

func (o *LLM) convertToOllamaMessages(messages []schema.MessageContent) ([]api.Message, error) {
	if len(messages) == 0 {
		return nil, ErrNoMessages
	}

	chatMsgs := make([]api.Message, 0, len(messages))
	for _, mc := range messages {
		msg := api.Message{Role: typeToRole(mc.Role)}

		var sb strings.Builder
		foundText := false

		for _, p := range mc.Parts {
			switch part := p.(type) {
			case schema.TextContent:
				if foundText {
					sb.WriteString("\n")
				} else {
					foundText = true
				}
				sb.WriteString(part.Text)
			default:
				return nil, fmt.Errorf("unsupported content part type: %T", part)
			}
		}

		if !foundText {
			return nil, ErrNoTextParts
		}

		msg.Content = sb.String()
		chatMsgs = append(chatMsgs, msg)
	}
	return chatMsgs, nil
}

func typeToRole(typ schema.ChatMessageType) string {
	switch typ {
	case schema.ChatMessageTypeSystem:
		return "system"
	case schema.ChatMessageTypeAI:
		return "assistant"
	case schema.ChatMessageTypeHuman, schema.ChatMessageTypeGeneric:
		return "user"
	default:
		return "user"
	}
}

func (o *LLM) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	req := &api.EmbedRequest{
		Model: o.options.model,
		Input: texts,
	}

	resp, err := o.client.Embed(ctx, req)
	if err != nil {
		// Check for "not found" error and attempt to pull model
		if isModelNotFoundError(err) {
			o.logger.InfoContext(ctx, "Embedding model not found, attempting to pull", "model", o.options.model)
			if pullErr := o.EnsureModel(ctx); pullErr != nil {
				return nil, fmt.Errorf("failed to pull model: %w", pullErr)
			}
			// Retry embedding after pull
			resp, err = o.client.Embed(ctx, req)
			if err != nil {
				return nil, fmt.Errorf("embedding failed after pulling model: %w", err)
			}
		} else {
			o.logger.ErrorContext(ctx, "Batch embed API call failed", "error", err)
			return nil, fmt.Errorf("batch embedding generation failed: %w", err)
		}
	}

	if len(resp.Embeddings) != len(texts) {
		o.logger.ErrorContext(ctx, "Embedding count mismatch", "expected", len(texts), "got", len(resp.Embeddings))
		return nil, ErrIncompleteEmbedding
	}

	return resp.Embeddings, nil
}

func (o *LLM) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	return o.EmbedDocuments(ctx, texts)
}

func (o *LLM) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	req := &api.EmbedRequest{
		Model: o.options.model,
		Input: text,
	}

	resp, err := o.client.Embed(ctx, req)
	if err != nil {
		if isModelNotFoundError(err) {
			o.logger.InfoContext(ctx, "Model not found, attempting to pull", "model", o.options.model)
			if pullErr := o.EnsureModel(ctx); pullErr != nil {
				return nil, fmt.Errorf("failed to pull model: %w", pullErr)
			}
			resp, err = o.client.Embed(ctx, req)
			if err != nil {
				return nil, fmt.Errorf("query embedding failed after pull: %w", err)
			}
		} else {
			return nil, err
		}
	}

	if len(resp.Embeddings) == 0 {
		return nil, ErrEmptyResponse
	}

	return resp.Embeddings[0], nil
}

func (o *LLM) EnsureModel(ctx context.Context) error {
	// Fast path: check if model details are already cached
	o.detailsMutex.RLock()
	if o.details != nil {
		o.detailsMutex.RUnlock()
		return nil
	}
	o.detailsMutex.RUnlock()

	// Check if model exists
	exists, err := o.ModelExists(ctx)
	if err != nil {
		o.logger.ErrorContext(ctx, "Failed to check model existence", "error", err)
		return fmt.Errorf("model existence check failed: %w", err)
	}

	if exists {
		return nil
	}

	o.logger.InfoContext(ctx, "Model not found locally, initiating pull", "model", o.options.model)

	pullStart := time.Now()
	err = o.PullModel(ctx, func(progress api.ProgressResponse) error {
		if progress.Total > 0 {
			percent := (float64(progress.Completed) / float64(progress.Total)) * 100
			o.logger.InfoContext(ctx, "Model pull progress",
				"status", progress.Status,
				"percent", fmt.Sprintf("%.1f%%", percent),
				"completed", progress.Completed,
				"total", progress.Total)
		} else {
			o.logger.InfoContext(ctx, "Model pull status", "status", progress.Status)
		}
		return nil
	})

	pullDuration := time.Since(pullStart)
	if err != nil {
		o.logger.ErrorContext(ctx, "Model pull failed", "error", err, "duration", pullDuration)
		return fmt.Errorf("model pull failed: %w", err)
	}

	o.logger.InfoContext(ctx, "Model pull completed successfully", "duration", pullDuration)
	return nil
}

func (o *LLM) ModelExists(ctx context.Context) (bool, error) {
	_, err := o.client.Show(ctx, &api.ShowRequest{Name: o.options.model})
	if err != nil {
		if isModelNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("model existence check failed: %w", err)
	}
	return true, nil
}

func (o *LLM) PullModel(ctx context.Context, progressFn func(api.ProgressResponse) error) error {
	req := &ollamaclient.PullRequest{
		Model:  o.options.model,
		Stream: true,
	}
	return o.client.Pull(ctx, req, progressFn)
}

func (o *LLM) GetModelDetails(ctx context.Context) (*schema.ModelDetails, error) {
	// Fast path: return cached details
	o.detailsMutex.RLock()
	if o.details != nil {
		cachedDetails := o.details
		o.detailsMutex.RUnlock()
		return cachedDetails, nil
	}
	o.detailsMutex.RUnlock()

	// Slow path: fetch and cache details
	o.detailsMutex.Lock()
	defer o.detailsMutex.Unlock()

	// Double-check after acquiring write lock
	if o.details != nil {
		return o.details, nil
	}

	o.logger.DebugContext(ctx, "Model details not cached, fetching from API")
	details, err := o.fetchModelDetails(ctx)
	if err != nil {
		return nil, err
	}

	o.details = details
	return o.details, nil
}

func (o *LLM) fetchModelDetails(ctx context.Context) (*schema.ModelDetails, error) {
	showResp, err := o.client.Show(ctx, &api.ShowRequest{Name: o.options.model})
	if err != nil {
		if isModelNotFoundError(err) {
			o.logger.WarnContext(ctx, "Model not found when fetching details", "model", o.options.model)
			return nil, ErrModelNotFound
		}
		return nil, fmt.Errorf("failed to retrieve model information: %w", err)
	}

	// Determine embedding dimension
	var dim int64
	testEmb, err := o.EmbedQuery(ctx, "test")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "does not support") ||
			strings.Contains(strings.ToLower(err.Error()), "embedding") {
			o.logger.DebugContext(ctx, "Model does not support embeddings; dimension set to 0")
			dim = 0
		} else {
			o.logger.WarnContext(ctx, "Failed to perform test embedding for dimension check", "error", err)
			// Don't fail here, just set dimension to 0
			dim = 0
		}
	} else {
		dim = int64(len(testEmb))
		o.logger.DebugContext(ctx, "Determined embedding dimension", "dimension", dim)
	}

	details := &schema.ModelDetails{
		Family:        showResp.Details.Family,
		ParameterSize: showResp.Details.ParameterSize,
		Quantization:  showResp.Details.QuantizationLevel,
		Dimension:     dim,
	}

	o.logger.InfoContext(ctx, "Model details retrieved and cached",
		"family", details.Family,
		"parameters", details.ParameterSize,
		"quantization", details.Quantization,
		"dimension", details.Dimension)

	return details, nil
}

func (o *LLM) GetDimension(ctx context.Context) (int, error) {
	details, err := o.GetModelDetails(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get embedding dimension: %w", err)
	}
	return int(details.Dimension), nil
}

func (o *LLM) CountTokens(ctx context.Context, text string) (int, error) {
	if text == "" {
		return 0, nil
	}

	stream := false
	req := &ollamaclient.GenerateRequest{
		Model:  o.options.model,
		Prompt: text,
		Stream: &stream,
		Options: map[string]any{
			"num_predict": 0,
		},
	}

	var tokenCount int
	err := o.client.Generate(ctx, req, func(resp ollamaclient.GenerateResponse) error {
		if resp.Done {
			tokenCount = resp.PromptEvalCount
		}
		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("token counting failed: %w", err)
	}

	return tokenCount, nil
}

func (o *LLM) determineModel(opts llms.CallOptions) string {
	if opts.Model != "" {
		o.logger.Debug("Using model from call options", "model", opts.Model)
		return opts.Model
	}
	return o.options.model
}

// Helper function to check if error indicates model not found
func isModelNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "not found") || strings.Contains(errStr, "does not exist")
}
