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

// LLM implements llms.Model, embeddings.Embedder, and embeddings.EmbedderWithOptions
// for OpenAI models. It supports chat completions, streaming, function calling,
// structured output, and embeddings.
type LLM struct {
	client    openai.Client
	options   options
	logger    *slog.Logger
	dimension int
	dimMu     sync.Mutex
}

var (
	_ llms.Model                     = (*LLM)(nil)
	_ embeddings.Embedder            = (*LLM)(nil)
	_ embeddings.EmbedderWithOptions = (*LLM)(nil)
)

// New creates a new OpenAI LLM with the given options.
// An API key is required; if omitted New returns ErrNoAPIKey.
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
		option.WithMaxRetries(0), // we implement our own retry logic
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

	if o.requestTimeout > 0 {
		clientOpts = append(clientOpts, option.WithRequestTimeout(o.requestTimeout))
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

// Call generates a completion for a single prompt string.
func (o *LLM) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return llms.GenerateFromSinglePrompt(ctx, o, prompt, options...)
}

// GenerateContent generates a chat completion for the provided message history.
// Supports streaming when a StreamingFunc is provided via call options.
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
	var resp *openai.ChatCompletion

	err := o.doWithRetry(ctx, func() error {
		var apiErr error
		resp, apiErr = o.client.Chat.Completions.New(ctx, params)
		return apiErr
	})

	if err != nil {
		return nil, fmt.Errorf("openai chat completion failed: %w", err)
	}

	duration := time.Since(start)

	if len(resp.Choices) == 0 {
		return nil, ErrNoChoices
	}

	choice := resp.Choices[0]

	content := choice.Message.Content
	toolCalls := convertToolCalls(o.logger, choice.Message.ToolCalls)

	genInfo := map[string]any{
		"CompletionTokens": resp.Usage.CompletionTokens,
		"PromptTokens":     resp.Usage.PromptTokens,
		"TotalTokens":      resp.Usage.TotalTokens,
		"ReasoningTokens":  resp.Usage.CompletionTokensDetails.ReasoningTokens,
		"CacheRead":        resp.Usage.PromptTokensDetails.CachedTokens,
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
	var fullContent strings.Builder
	var accumulatedToolCalls map[int]llms.ToolCall
	var toolCallArgs map[int]*strings.Builder
	var finishReason string

	err := o.doWithRetry(ctx, func() error {
		fullContent.Reset()
		accumulatedToolCalls = make(map[int]llms.ToolCall)
		toolCallArgs = make(map[int]*strings.Builder)
		finishReason = ""

		stream := o.client.Chat.Completions.NewStreaming(ctx, params)
		defer stream.Close()

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
						return fmt.Errorf("streaming function returned an error: %w", err)
					}
				}
			}

			for _, tc := range delta.ToolCalls {
				idx := int(tc.Index)
				existing, exists := accumulatedToolCalls[idx]
				if !exists {
					existing = llms.ToolCall{}
				}
				if tc.ID != "" {
					existing.ID = tc.ID
				}
				if tc.Function.Name != "" {
					existing.Function.Name = tc.Function.Name
				}
				accumulatedToolCalls[idx] = existing

				if _, ok := toolCallArgs[idx]; !ok {
					toolCallArgs[idx] = &strings.Builder{}
				}
				_, _ = toolCallArgs[idx].WriteString(tc.Function.Arguments)
			}

			if chunk.Choices[0].FinishReason != "" {
				finishReason = chunk.Choices[0].FinishReason
			}
		}

		return stream.Err()
	})

	if err != nil {
		return nil, fmt.Errorf("openai streaming failed: %w", err)
	}

	duration := time.Since(start)

	// Convert map to sorted slice.
	toolCalls := make([]llms.ToolCall, 0, len(accumulatedToolCalls))
	for i := range len(accumulatedToolCalls) {
		if tc, ok := accumulatedToolCalls[i]; ok {
			if args, hasArgs := toolCallArgs[i]; hasArgs {
				var parsed map[string]any
				if err := json.Unmarshal([]byte(args.String()), &parsed); err != nil {
					o.logger.Warn("failed to parse streaming tool call arguments", "error", err, "arguments", args.String())
				} else {
					tc.Function.Arguments = parsed
				}
			}
			toolCalls = append(toolCalls, tc)
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

	reasoningEffort := o.options.reasoningEffort
	if o.options.thinking != nil && *o.options.thinking && reasoningEffort == "" {
		reasoningEffort = "medium"
	}
	if reasoningEffort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(reasoningEffort)
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
	var contentParts []openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion
	var toolCalls []openai.ChatCompletionMessageToolCallParam
	for _, part := range msg.Parts {
		switch p := part.(type) {
		case schema.TextContent:
			if p.Text != "" {
				contentParts = append(contentParts, openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{
					OfText: &openai.ChatCompletionContentPartTextParam{Text: p.Text},
				})
			}
		case schema.ToolCallContent:
			argsJSON, err := json.Marshal(p.Arguments)
			if err != nil {
				o.logger.Warn("failed to marshal tool call arguments", "error", err, "function", p.FunctionName)
				argsJSON = []byte("{}")
			}
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallParam{
				ID: p.ID,
				Function: openai.ChatCompletionMessageToolCallFunctionParam{
					Name:      p.FunctionName,
					Arguments: string(argsJSON),
				},
			})
		}
	}

	assistantMsg := &openai.ChatCompletionAssistantMessageParam{}

	switch {
	case len(contentParts) > 0:
		assistantMsg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
			OfArrayOfContentParts: contentParts,
		}
	case len(toolCalls) > 0:
		// Tool-call-only message: leave Content empty, only set ToolCalls.
	default:
		text := msg.GetTextContent()
		if text != "" {
			assistantMsg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: openai.String(text),
			}
		}
	}

	if len(toolCalls) > 0 {
		assistantMsg.ToolCalls = toolCalls
	}

	return openai.ChatCompletionMessageParamUnion{
		OfAssistant: assistantMsg,
	}
}

func (o *LLM) convertToolMessage(msg schema.MessageContent) openai.ChatCompletionMessageParamUnion {
	var content string
	var toolCallID string
	for _, part := range msg.Parts {
		if tr, ok := part.(schema.ToolResultContent); ok {
			content = tr.Content
			toolCallID = tr.ToolCallID
		}
	}
	return openai.ToolMessage(content, toolCallID)
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

func convertToolCalls(logger *slog.Logger, toolCalls []openai.ChatCompletionMessageToolCall) []llms.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	result := make([]llms.ToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			logger.Warn("failed to parse tool call arguments", "error", err, "arguments", tc.Function.Arguments)
		}
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

// EmbedDocuments generates embeddings for a batch of documents.
func (o *LLM) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	return o.EmbedDocumentsWithOpts(ctx, texts, embeddings.EmbeddingOptions{Truncate: true})
}

// EmbedQuery generates an embedding for a single query string.
func (o *LLM) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return o.EmbedQueryWithOpts(ctx, text, embeddings.EmbeddingOptions{Truncate: true})
}

// EmbedQueries generates embeddings for multiple query strings.
func (o *LLM) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	return o.EmbedDocumentsWithOpts(ctx, texts, embeddings.EmbeddingOptions{Truncate: true})
}

// EmbedDocumentsWithOpts generates embeddings for documents with additional options.
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

// EmbedQueryWithOpts generates an embedding for a single query with additional options.
func (o *LLM) EmbedQueryWithOpts(ctx context.Context, text string, opts embeddings.EmbeddingOptions) ([]float32, error) {
	embs, err := o.EmbedDocumentsWithOpts(ctx, []string{text}, opts)
	if err != nil {
		return nil, err
	}
	return embs[0], nil
}

// GetDimension returns the embedding dimension by making a sample embedding call.
// The result is cached after the first call. InvalidateDimensionCache can be used
// to reset the cache (e.g., if the model is changed at runtime).
func (o *LLM) GetDimension(ctx context.Context) (int, error) {
	o.dimMu.Lock()
	if o.dimension > 0 {
		dim := o.dimension
		o.dimMu.Unlock()
		return dim, nil
	}
	o.dimMu.Unlock()

	sampleEmbedding, err := o.EmbedQuery(ctx, "dimension")
	if err != nil {
		return 0, fmt.Errorf("failed to get dimension by embedding sample text: %w", err)
	}

	o.dimMu.Lock()
	o.dimension = len(sampleEmbedding)
	o.dimMu.Unlock()

	return len(sampleEmbedding), nil
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

// InvalidateDimensionCache clears the cached embedding dimension so the next
// call to GetDimension will make a fresh embedding request.
func (o *LLM) InvalidateDimensionCache() {
	o.dimMu.Lock()
	defer o.dimMu.Unlock()
	o.dimension = 0
}

func maskAPIKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return key[:4] + "****"
}
