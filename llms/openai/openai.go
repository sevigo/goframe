package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"

	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

var (
	ErrNoAPIKey   = errors.New("openai: API key is required")
	ErrNoChoices  = errors.New("openai: no choices in response")
	ErrEmbeddings = errors.New("openai: failed to generate embeddings")
)

type LLM struct {
	client    openai.Client
	options   options
	logger    *slog.Logger
	dimension int
	dimOnce   sync.Once
	dimMu     sync.Mutex
}

var (
	_ llms.Model                     = (*LLM)(nil)
	_ embeddings.Embedder            = (*LLM)(nil)
	_ embeddings.EmbedderWithOptions = (*LLM)(nil)
)

func New(opts ...Option) (*LLM, error) {
	o := applyOptions(opts...)

	if o.apiKey == "" {
		return nil, ErrNoAPIKey
	}

	if o.model == "" {
		o.model = "gpt-4o"
	}

	if o.logger == nil {
		o.logger = slog.Default()
	}

	clientOpts := []option.RequestOption{
		option.WithAPIKey(o.apiKey),
		option.WithMaxRetries(o.retryAttempts),
	}

	if o.baseURL != "" {
		clientOpts = append(clientOpts, option.WithBaseURL(o.baseURL))
	}

	if o.organization != "" {
		clientOpts = append(clientOpts, option.WithOrganization(o.organization))
	}

	if o.project != "" {
		clientOpts = append(clientOpts, option.WithProject(o.project))
	}

	client := openai.NewClient(clientOpts...)

	llm := &LLM{
		client:  client,
		options: o,
		logger:  o.logger.With("component", "openai_llm", "model", o.model),
	}

	llm.logger.Info("OpenAI LLM initialized successfully", "api_key_prefix", maskAPIKey(o.apiKey))
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

	params := o.buildChatParams(model, messages, opts)

	isStreaming := opts.StreamingFunc != nil

	if isStreaming {
		return o.generateStreamingContent(ctx, params, opts, model, start)
	}
	return o.generateNonStreamingContent(ctx, params, model, start)
}

func (o *LLM) generateNonStreamingContent(
	ctx context.Context,
	params openai.ChatCompletionNewParams,
	model string,
	start time.Time,
) (*schema.ContentResponse, error) {
	resp, err := o.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai chat completion failed: %w", err)
	}

	duration := time.Since(start)

	if len(resp.Choices) == 0 {
		return nil, ErrNoChoices
	}

	choice := resp.Choices[0]

	content := choice.Message.Content
	toolCalls := convertToolCalls(choice.Message.ToolCalls)

	genInfo := map[string]any{
		"CompletionTokens": resp.Usage.CompletionTokens,
		"PromptTokens":     resp.Usage.PromptTokens,
		"TotalTokens":      resp.Usage.TotalTokens,
		"Duration":         duration,
		"Model":            model,
		"FinishReason":     choice.FinishReason,
	}
	if len(toolCalls) > 0 {
		genInfo["ToolCalls"] = toolCalls
	}

	return &schema.ContentResponse{
		Choices: []*schema.ContentChoice{
			{
				Content:        content,
				StopReason:     choice.FinishReason,
				GenerationInfo: genInfo,
			},
		},
	}, nil
}

func (o *LLM) generateStreamingContent(
	ctx context.Context,
	params openai.ChatCompletionNewParams,
	callOpts llms.CallOptions,
	model string,
	start time.Time,
) (*schema.ContentResponse, error) {
	stream := o.client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()

	var fullContent strings.Builder
	var toolCalls []llms.ToolCall
	var toolCallArgs map[int]*strings.Builder
	var toolCallIDs map[int]string
	var finishReason string

	for stream.Next() {
		chunk := stream.Current()

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		if delta.Content != "" {
			fullContent.WriteString(delta.Content)
			if callOpts.StreamingFunc != nil {
				if err := callOpts.StreamingFunc(ctx, []byte(delta.Content)); err != nil {
					return nil, fmt.Errorf("streaming function returned an error: %w", err)
				}
			}
		}

		for _, tc := range delta.ToolCalls {
			idx := int(tc.Index)
			if toolCallArgs == nil {
				toolCallArgs = make(map[int]*strings.Builder)
				toolCallIDs = make(map[int]string)
			}
			if _, exists := toolCallArgs[idx]; !exists {
				toolCallArgs[idx] = &strings.Builder{}
				toolCalls = append(toolCalls, llms.ToolCall{
					Function: llms.FunctionCall{
						Name: tc.Function.Name,
					},
				})
			}
			if tc.ID != "" {
				toolCallIDs[idx] = tc.ID
			}
			if tc.Function.Name != "" {
				toolCalls[idx].Function.Name = tc.Function.Name
			}
			_, _ = toolCallArgs[idx].WriteString(tc.Function.Arguments)
		}

		if chunk.Choices[0].FinishReason != "" {
			finishReason = chunk.Choices[0].FinishReason
		}
	}

	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("openai streaming failed: %w", err)
	}

	duration := time.Since(start)

	for i, tc := range toolCalls {
		if args, ok := toolCallArgs[i]; ok {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(args.String()), &parsed); err == nil {
				tc.Function.Arguments = parsed
			}
			toolCalls[i] = tc
		}
		if id, ok := toolCallIDs[i]; ok {
			toolCalls[i].ID = id
		}
	}

	genInfo := map[string]any{
		"Duration":     duration,
		"Model":        model,
		"FinishReason": finishReason,
	}
	if len(toolCalls) > 0 {
		genInfo["ToolCalls"] = toolCalls
	}

	return &schema.ContentResponse{
		Choices: []*schema.ContentChoice{
			{
				Content:        fullContent.String(),
				StopReason:     finishReason,
				GenerationInfo: genInfo,
			},
		},
	}, nil
}

func (o *LLM) buildChatParams(
	model string,
	messages []schema.MessageContent,
	opts llms.CallOptions,
) openai.ChatCompletionNewParams {
	apiMessages := o.convertMessages(messages)

	params := openai.ChatCompletionNewParams{
		Model:    model,
		Messages: apiMessages,
	}

	if opts.TemperatureSet() {
		params.Temperature = openai.Float(opts.Temperature)
	}

	if opts.MaxTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(opts.MaxTokens))
	}

	if opts.TopPSet() {
		params.TopP = openai.Float(opts.TopP)
	}

	if opts.SeedSet() {
		params.Seed = openai.Int(int64(opts.Seed))
	}

	if len(opts.StopWords) > 0 {
		params.Stop = openai.ChatCompletionNewParamsStopUnion{
			OfStringArray: opts.StopWords,
		}
	}

	if opts.JSONMode {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
		}
	} else if opts.JSONSchema != nil {
		schemaMap, ok := opts.JSONSchema.(map[string]any)
		if ok {
			params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
					JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
						Name:   "response",
						Schema: schemaMap,
					},
				},
			}
		}
	}

	if len(opts.Tools) > 0 {
		params.Tools = convertTools(opts.Tools)
	}

	if o.options.reasoningEffort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(o.options.reasoningEffort)
	}

	return params
}

func (o *LLM) convertMessages(messages []schema.MessageContent) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))

	for _, msg := range messages {
		switch msg.Role {
		case schema.ChatMessageTypeSystem:
			text := msg.GetTextContent()
			result = append(result, openai.SystemMessage(text))

		case schema.ChatMessageTypeHuman:
			result = append(result, o.convertHumanMessage(msg))

		case schema.ChatMessageTypeAI:
			result = append(result, o.convertAIMessage(msg))

		case schema.ChatMessageTypeTool:
			result = append(result, o.convertToolMessage(msg))

		default:
			text := msg.GetTextContent()
			result = append(result, openai.UserMessage(text))
		}
	}

	return result
}

func (o *LLM) convertHumanMessage(msg schema.MessageContent) openai.ChatCompletionMessageParamUnion {
	images := msg.GetImages()
	if len(images) == 0 {
		return openai.UserMessage(msg.GetTextContent())
	}

	parts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(msg.Parts))
	for _, part := range msg.Parts {
		switch p := part.(type) {
		case schema.TextContent:
			parts = append(parts, openai.TextContentPart(p.Text))
		case schema.ImageContent:
			dataURI := "data:" + p.MimeType + ";base64," + p.Data
			parts = append(parts, openai.ImageContentPart(
				openai.ChatCompletionContentPartImageImageURLParam{URL: dataURI},
			))
		}
	}

	return openai.UserMessage(parts)
}

func (o *LLM) convertAIMessage(msg schema.MessageContent) openai.ChatCompletionMessageParamUnion {
	return openai.ChatCompletionMessageParamUnion{
		OfAssistant: &openai.ChatCompletionAssistantMessageParam{
			Content: openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: openai.String(msg.GetTextContent()),
			},
		},
	}
}

func (o *LLM) convertToolMessage(msg schema.MessageContent) openai.ChatCompletionMessageParamUnion {
	var content string
	var toolName string
	for _, part := range msg.Parts {
		if tr, ok := part.(schema.ToolResultContent); ok {
			content = tr.Content
			toolName = tr.ToolName
		}
	}
	return openai.ToolMessage(content, toolName)
}

func convertTools(tools []llms.ToolDefinition) []openai.ChatCompletionToolParam {
	result := make([]openai.ChatCompletionToolParam, 0, len(tools))
	for _, t := range tools {
		tool := openai.ChatCompletionToolParam{
			Type: "function",
			Function: shared.FunctionDefinitionParam{
				Name:        t.Function.Name,
				Description: openai.String(t.Function.Description),
			},
		}
		if t.Function.Parameters != nil {
			if params, ok := t.Function.Parameters.(map[string]any); ok {
				tool.Function.Parameters = shared.FunctionParameters(params)
			}
		}
		result = append(result, tool)
	}
	return result
}

func convertToolCalls(toolCalls []openai.ChatCompletionMessageToolCall) []llms.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	result := make([]llms.ToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		var args map[string]any
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		result = append(result, llms.ToolCall{
			ID: tc.ID,
			Function: llms.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: args,
			},
		})
	}
	return result
}

func (o *LLM) determineModel(opts llms.CallOptions) string {
	if opts.Model != "" {
		return opts.Model
	}
	return o.options.model
}

func (o *LLM) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	return o.EmbedDocumentsWithOpts(ctx, texts, embeddings.EmbeddingOptions{Truncate: true})
}

func (o *LLM) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return o.EmbedQueryWithOpts(ctx, text, embeddings.EmbeddingOptions{Truncate: true})
}

func (o *LLM) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	return o.EmbedDocumentsWithOpts(ctx, texts, embeddings.EmbeddingOptions{Truncate: true})
}

func (o *LLM) EmbedDocumentsWithOpts(ctx context.Context, texts []string, opts embeddings.EmbeddingOptions) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	var resp *openai.CreateEmbeddingResponse

	err := o.doWithRetry(ctx, func() error {
		params := openai.EmbeddingNewParams{
			Model: o.options.embeddingModel,
			Input: openai.EmbeddingNewParamsInputUnion{
				OfArrayOfStrings: texts,
			},
		}
		if opts.Dimensions > 0 {
			params.Dimensions = openai.Int(int64(opts.Dimensions))
		}
		var apiErr error
		resp, apiErr = o.client.Embeddings.New(ctx, params)
		return apiErr
	})

	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrEmbeddings, err)
	}

	if len(resp.Data) != len(texts) {
		return nil, fmt.Errorf("%w: expected %d embeddings, but got %d", ErrEmbeddings, len(texts), len(resp.Data))
	}

	result := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		result[i] = make([]float32, len(d.Embedding))
		for j, v := range d.Embedding {
			result[i][j] = float32(v)
		}
	}
	return result, nil
}

func (o *LLM) EmbedQueryWithOpts(ctx context.Context, text string, opts embeddings.EmbeddingOptions) ([]float32, error) {
	embeddings, err := o.EmbedDocumentsWithOpts(ctx, []string{text}, opts)
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

func (o *LLM) GetDimension(ctx context.Context) (int, error) {
	o.dimMu.Lock()
	if o.dimension > 0 {
		dim := o.dimension
		o.dimMu.Unlock()
		return dim, nil
	}
	o.dimMu.Unlock()

	var onceErr error
	o.dimOnce.Do(func() {
		sampleEmbedding, err := o.EmbedQuery(ctx, "dimension")
		if err != nil {
			onceErr = fmt.Errorf("failed to get dimension by embedding sample text: %w", err)
			return
		}
		o.dimMu.Lock()
		o.dimension = len(sampleEmbedding)
		o.dimMu.Unlock()
	})

	if onceErr != nil {
		o.dimMu.Lock()
		o.dimOnce = sync.Once{}
		o.dimMu.Unlock()
		return 0, onceErr
	}

	return o.dimension, nil
}

func (o *LLM) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	errStr := strings.ToLower(err.Error())
	patterns := []string{
		"connection reset",
		"connection refused",
		"unexpected eof",
		"network is unreachable",
		"no such host",
		"timeout",
		"rate_limit",
		"rate limit",
		"429",
		"503",
		"502",
		"server_error",
	}

	for _, pattern := range patterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

func (o *LLM) calculateNextDelay(delay time.Duration) time.Duration {
	delay *= 2
	if delay > o.options.maxRetryDelay {
		delay = o.options.maxRetryDelay
	}
	return delay
}

func (o *LLM) waitForRetryDelay(ctx context.Context, delay time.Duration, attempt int, err error) error {
	var jitter time.Duration
	if o.options.retryJitter > 0 {
		jitter = time.Duration(rand.IntN(int(o.options.retryJitter.Milliseconds()))) * time.Millisecond //nolint:gosec // rand.IntN is sufficient for retry jitter
	}
	totalDelay := delay + jitter

	o.logger.Warn("Retrying OpenAI API call",
		"attempt", fmt.Sprintf("%d/%d", attempt, o.options.retryAttempts),
		"delay", totalDelay,
		"error", err)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(totalDelay):
		return nil
	}
}

func (o *LLM) doWithRetry(ctx context.Context, fn func() error) error {
	if o.options.retryAttempts == 0 {
		return fn()
	}

	var lastErr error
	delay := o.options.retryDelay

	for attempt := 0; attempt <= o.options.retryAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		if attempt >= o.options.retryAttempts {
			break
		}

		if !o.isRetryableError(err) {
			break
		}

		if retryErr := o.waitForRetryDelay(ctx, delay, attempt+1, err); retryErr != nil {
			return retryErr
		}
		delay = o.calculateNextDelay(delay)
	}

	return lastErr
}

func (o *LLM) InvalidateDimensionCache() {
	o.dimMu.Lock()
	defer o.dimMu.Unlock()
	o.dimension = 0
	o.dimOnce = sync.Once{}
}

func maskAPIKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return key[:4] + "****"
}
