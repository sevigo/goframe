package ollama

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"log/slog"

	"github.com/ollama/ollama/api"
	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

type LLM struct {
	client       *api.Client
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
		o.model = "llama3" // Default model if none specified
	}

	serverURL, _ := url.Parse("http://127.0.0.1:11434")
	if o.ollamaServerURL != nil {
		serverURL = o.ollamaServerURL
	}

	client := api.NewClient(serverURL, o.httpClient)

	llm := &LLM{
		client:  client,
		options: o,
		logger:  o.logger.With("component", "ollama_llm", "model", o.model),
	}

	llm.logger.Info("Ollama LLM initialized successfully")
	return llm, nil
}

func (o *LLM) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return llms.GenerateFromSinglePrompt(ctx, o, prompt, options...)
}

func (o *LLM) GenerateContent(
	ctx context.Context,
	messages []schema.MessageContent,
	options ...llms.CallOption,
) (*schema.ContentResponse, error) {
	start := time.Now()

	opts := llms.CallOptions{}
	for _, opt := range options {
		opt(&opts)
	}
	model := o.determineModel(opts)

	chatMsgs := make([]api.Message, 0, len(messages))
	for _, mc := range messages {
		msg := api.Message{
			Role:    typeToRole(mc.Role),
			Content: mc.String(),
		}
		chatMsgs = append(chatMsgs, msg)
	}

	ollamaOpts := map[string]any{}
	if opts.Temperature > 0 {
		ollamaOpts["temperature"] = float32(opts.Temperature)
	}
	if opts.MaxTokens > 0 {
		ollamaOpts["num_predict"] = opts.MaxTokens
	}
	if len(opts.StopWords) > 0 {
		ollamaOpts["stop"] = opts.StopWords
	}
	if opts.TopP > 0 {
		ollamaOpts["top_p"] = float32(opts.TopP)
	}
	if opts.TopK > 0 {
		ollamaOpts["top_k"] = opts.TopK
	}
	if opts.Seed > 0 {
		ollamaOpts["seed"] = opts.Seed
	}

	req := &api.ChatRequest{
		Model:    model,
		Messages: chatMsgs,
		Options:  ollamaOpts,
		Stream:   new(bool),
	}
	*req.Stream = opts.StreamingFunc != nil

	var fullResponse strings.Builder
	var finalResp api.ChatResponse

	fn := func(response api.ChatResponse) error {
		fullResponse.WriteString(response.Message.Content)
		if opts.StreamingFunc != nil && response.Message.Content != "" {
			if errStream := opts.StreamingFunc(ctx, []byte(response.Message.Content)); errStream != nil {
				return fmt.Errorf("streaming function returned an error: %w", errStream)
			}
		}
		if response.Done {
			finalResp = response
		}
		return nil
	}

	err := o.client.Chat(ctx, req, fn)
	duration := time.Since(start)

	if err != nil {
		o.logger.ErrorContext(ctx, "Ollama chat failed", "error", err, "duration", duration)
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
		return nil, fmt.Errorf("ollama embed failing: %w", err)
	}

	return resp.Embeddings, nil
}

func (o *LLM) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	req := &api.EmbedRequest{
		Model: o.options.model,
		Input: text,
	}

	resp, err := o.client.Embed(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("ollama: empty embedding response")
	}

	return resp.Embeddings[0], nil
}

func (o *LLM) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	return o.EmbedDocuments(ctx, texts)
}

func (o *LLM) GetModelDetails(ctx context.Context) (*schema.ModelDetails, error) {
	o.detailsMutex.RLock()
	if o.details != nil {
		return o.details, nil
	}
	o.detailsMutex.RUnlock()

	o.detailsMutex.Lock()
	defer o.detailsMutex.Unlock()

	showResp, err := o.client.Show(ctx, &api.ShowRequest{Name: o.options.model})
	if err != nil {
		return nil, fmt.Errorf("fetching model details: %w", err)
	}

	var dim int64
	testEmb, err := o.EmbedQuery(ctx, "test")
	if err == nil {
		dim = int64(len(testEmb))
	}

	o.details = &schema.ModelDetails{
		Family:        showResp.Details.Family,
		ParameterSize: showResp.Details.ParameterSize,
		Quantization:  showResp.Details.QuantizationLevel,
		Dimension:     dim,
	}

	return o.details, nil
}

func (o *LLM) CountTokens(ctx context.Context, text string) (int, error) {
	if text == "" {
		return 0, nil
	}

	req := &api.GenerateRequest{
		Model:  o.options.model,
		Prompt: text,
		Stream: new(bool), // Defaults to false
		Options: map[string]any{
			"num_predict": 0, // Just count tokens, don't generate
		},
	}

	var tokenCount int
	fn := func(resp api.GenerateResponse) error {
		if resp.Done {
			tokenCount = resp.PromptEvalCount
		}
		return nil
	}

	err := o.client.Generate(ctx, req, fn)
	if err != nil {
		return 0, fmt.Errorf("token counting via generation failed: %w", err)
	}

	return tokenCount, nil
}

func (o *LLM) GetDimension(ctx context.Context) (int, error) {
	details, err := o.GetModelDetails(ctx)
	if err != nil {
		return 0, err
	}
	return int(details.Dimension), nil
}

func (o *LLM) determineModel(opts llms.CallOptions) string {
	if opts.Model != "" {
		return opts.Model
	}
	return o.options.model
}
