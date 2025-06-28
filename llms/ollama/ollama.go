package ollama

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"log/slog"

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

	client, err := ollamaclient.NewDefaultClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create ollama client: %w", err)
	}

	llm := &LLM{
		client:  client,
		options: o,
		logger:  o.logger.With("component", "ollama_llm", "model", o.model),
	}

	llm.logger.Info("Ollama LLM initialized successfully")
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
	if o.logger == nil {
		o.logger = slog.Default()
	}

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

	isStreamingFunc := opts.StreamingFunc != nil

	req := &api.ChatRequest{
		Model:    model,
		Messages: chatMsgs,
		Stream:   &isStreamingFunc,
	}
	var fullResponse strings.Builder
	var finalResp api.ChatResponse

	fn := func(response api.ChatResponse) error {
		fullResponse.WriteString(response.Message.Content)
		if opts.StreamingFunc != nil {
			if errStream := opts.StreamingFunc(ctx, []byte(response.Message.Content)); errStream != nil {
				return fmt.Errorf("streaming function returned an error: %w", errStream)
			}
		}
		if response.Done {
			finalResp = response
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

	if err := o.EnsureModel(ctx); err != nil {
		return nil, fmt.Errorf("embedding model preparation failed: %w", err)
	}

	type result struct {
		index     int
		embedding []float32
		err       error
	}
	resultCh := make(chan result, len(texts))

	const maxConcurrency = 5
	sem := make(chan struct{}, maxConcurrency)

	for i, text := range texts {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			goto afterLoop
		}
		go func(i int, text string) {
			defer func() { <-sem }()
			embedding, err := o.createSingleEmbedding(ctx, text)
			resultCh <- result{i, embedding, err}
		}(i, text)
	}
afterLoop:

	embeddings := make([][]float32, len(texts))
	var firstErr error

	for range texts {
		res := <-resultCh
		if res.err != nil && firstErr == nil {
			firstErr = res.err
		}
		embeddings[res.index] = res.embedding
	}

	if firstErr != nil {
		return nil, firstErr
	}

	// Defensive check on count
	if len(texts) != len(embeddings) {
		o.logger.ErrorContext(ctx, "Embedding count mismatch",
			"expected", len(texts), "got", len(embeddings))
		return nil, ErrIncompleteEmbedding
	}

	return embeddings, nil
}

func (o *LLM) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	o.logger.DebugContext(ctx, "Creating query embedding", "text_length", len(text))

	if err := o.EnsureModel(ctx); err != nil {
		return nil, fmt.Errorf("query embedding model preparation failed: %w", err)
	}

	return o.createSingleEmbedding(ctx, text)
}

func (o *LLM) createSingleEmbedding(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return []float32{}, nil
	}

	req := &ollamaclient.EmbeddingRequest{
		Model:  o.options.model,
		Prompt: text,
	}

	start := time.Now()
	resp, err := o.client.Embeddings(ctx, req)
	duration := time.Since(start)

	if err != nil {
		o.logger.ErrorContext(ctx, "Embedding API call failed",
			"error", err, "text_length", len(text), "duration", duration)
		return nil, fmt.Errorf("embedding generation failed: %w", err)
	}

	if len(resp.Embedding) == 0 {
		o.logger.WarnContext(ctx, "Received empty embedding response", "text_length", len(text))
		return nil, ErrEmptyResponse
	}

	return ollamaclient.ConvertEmbeddingToFloat32(resp.Embedding), nil
}

func (o *LLM) EnsureModel(ctx context.Context) error {
	o.detailsMutex.RLock()
	if o.details != nil {
		o.detailsMutex.RUnlock()
		return nil
	}
	o.detailsMutex.RUnlock()

	exists, err := o.ModelExists(ctx)
	if err != nil {
		o.logger.ErrorContext(ctx, "Failed to check model existence", "error", err)
		return fmt.Errorf("model existence check failed: %w", err)
	}

	if exists {
		return nil
	}

	o.logger.InfoContext(ctx, "Model not found locally, initiating pull")

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
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
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
	o.logger.DebugContext(ctx, "Retrieving model details")

	showResp, err := o.client.Show(ctx, &api.ShowRequest{Name: o.options.model})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			o.logger.WarnContext(ctx, "Model not found when getting details")
			return nil, ErrModelNotFound
		}
		return nil, fmt.Errorf("failed to retrieve model information: %w", err)
	}

	var dim int64
	testEmb, err := o.createSingleEmbedding(ctx, "dimension test")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "does not support") {
			o.logger.DebugContext(ctx, "Model does not support embeddings, setting dimension to 0")
			dim = 0
		} else {
			o.logger.ErrorContext(ctx, "Failed to determine embedding dimension", "error", err)
			return nil, fmt.Errorf("failed to determine embedding dimension: %w", err)
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

	o.logger.InfoContext(ctx, "Model details retrieved",
		"family", details.Family,
		"parameters", details.ParameterSize,
		"quantization", details.Quantization,
		"dimension", details.Dimension)

	o.detailsMutex.Lock()
	o.details = details
	o.detailsMutex.Unlock()

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
	tokens := strings.Fields(text)
	return len(tokens), nil
}

func (o *LLM) determineModel(opts llms.CallOptions) string {
	if opts.Model != "" {
		o.logger.DebugContext(context.Background(), "Using model from call options", "model", opts.Model)
		return opts.Model
	}
	return o.options.model
}
