package ollama

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ollama/ollama/api"

	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

// defaultHTTPClient provides a properly configured HTTP client with sensible defaults.
var defaultHTTPClient = &http.Client{
	Timeout: DefaultTimeout,
	Transport: &http.Transport{
		MaxIdleConns:        DefaultMaxIdleConns,
		MaxIdleConnsPerHost: DefaultMaxIdleConnsHost,
		IdleConnTimeout:     DefaultIdleConnTimeout,
		TLSHandshakeTimeout: DefaultTLSHandshakeTimeout,
	},
}

type authTransport struct {
	base   http.RoundTripper
	apiKey string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.apiKey != "" {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+t.apiKey)
	}
	return t.base.RoundTrip(req)
}

// maskAPIKey safely masks an API key for logging, showing only the first 4 characters.
func maskAPIKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return key[:4] + "****"
}

type LLM struct {
	client     *api.Client
	options    options
	logger     *slog.Logger
	details    *schema.ModelDetails
	detailsMu  sync.RWMutex
	detailsErr error
}

var (
	_ llms.Model                     = (*LLM)(nil)
	_ embeddings.Embedder            = (*LLM)(nil)
	_ embeddings.EmbedderWithOptions = (*LLM)(nil)
	_ llms.Tokenizer                 = (*LLM)(nil)
)

func New(opts ...Option) (*LLM, error) {
	o := applyOptions(opts...)

	if o.model == "" {
		o.model = "gemma4"
	}

	defaultURL := "http://127.0.0.1:11434"
	if o.apiKey != "" {
		defaultURL = "https://ollama.com/api"
	}

	serverURL, _ := url.Parse(defaultURL)
	if o.ollamaServerURL != nil {
		serverURL = o.ollamaServerURL
	}

	httpClient := o.httpClient
	if httpClient == nil {
		httpClient = cloneDefaultHTTPClient()
	}

	if o.apiKey != "" {
		at := &authTransport{
			base:   httpClient.Transport,
			apiKey: o.apiKey,
		}
		if at.base == nil {
			at.base = newOptimizedTransport()
		}
		httpClient = &http.Client{
			Transport: at,
			Timeout:   httpClient.Timeout,
		}
		o.logger.Debug("Ollama client initialized with API key", "prefix", maskAPIKey(o.apiKey))
	} else if httpClient.Transport == nil {
		httpClient.Transport = newOptimizedTransport()
	}

	client := api.NewClient(serverURL, httpClient)

	llm := &LLM{
		client:  client,
		options: o,
		logger:  o.logger.With("component", "ollama_llm", "model", o.model),
	}

	llm.logger.Info("Ollama LLM initialized successfully")
	return llm, nil
}

// cloneDefaultHTTPClient returns a copy of the default HTTP client
// so the package-level default is never mutated.
func cloneDefaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   defaultHTTPClient.Timeout,
		Transport: defaultHTTPClient.Transport,
	}
}

// newOptimizedTransport creates an http.Transport tuned for concurrent Ollama requests.
func newOptimizedTransport() *http.Transport {
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{
			MaxIdleConns:        DefaultMaxIdleConns,
			MaxIdleConnsPerHost: DefaultMaxIdleConnsHost,
			IdleConnTimeout:     DefaultIdleConnTimeout,
			TLSHandshakeTimeout: DefaultTLSHandshakeTimeout,
		}
	}
	cloned := t.Clone()
	cloned.MaxIdleConns = DefaultMaxIdleConns
	cloned.MaxIdleConnsPerHost = DefaultMaxIdleConnsHost
	cloned.IdleConnTimeout = DefaultIdleConnTimeout
	cloned.TLSHandshakeTimeout = DefaultTLSHandshakeTimeout
	return cloned
}

func (o *LLM) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return llms.GenerateFromSinglePrompt(ctx, o, prompt, options...)
}

// buildChatMessages converts schema messages to API messages.
func (o *LLM) buildChatMessages(messages []schema.MessageContent) []api.Message {
	chatMsgs := make([]api.Message, 0, len(messages))
	for _, mc := range messages {
		msg := api.Message{
			Role:    typeToRole(mc.Role),
			Content: mc.String(),
		}
		// Add images if present
		images := mc.GetImages()
		if len(images) > 0 {
			msg.Images = make([]api.ImageData, 0, len(images))
			for _, img := range images {
				// Ollama API expects raw bytes, but schema.ImageContent.Data is base64-encoded
				imageBytes, err := base64.StdEncoding.DecodeString(img.Data)
				if err != nil {
					o.logger.Warn("failed to decode base64 image data, image will be skipped", "error", err)
					continue
				}
				msg.Images = append(msg.Images, api.ImageData(imageBytes))
			}
		}
		chatMsgs = append(chatMsgs, msg)
	}
	return chatMsgs
}

// buildOllamaOptions converts CallOptions to Ollama options map.
// Returns nil when no options are set, avoiding unnecessary allocations.
func buildOllamaOptions(opts llms.CallOptions) map[string]any {
	var ollamaOpts map[string]any
	set := func(key string, val any) {
		if ollamaOpts == nil {
			ollamaOpts = map[string]any{}
		}
		ollamaOpts[key] = val
	}

	if opts.TemperatureSet() {
		set("temperature", float32(opts.Temperature))
	}
	if opts.MaxTokens > 0 {
		set("num_predict", opts.MaxTokens)
	}
	if len(opts.StopWords) > 0 {
		set("stop", opts.StopWords)
	}
	if opts.TopPSet() {
		set("top_p", float32(opts.TopP))
	}
	if opts.TopKSet() {
		set("top_k", opts.TopK)
	}
	if opts.MinPSet() {
		set("min_p", float32(opts.MinP))
	}
	if opts.SeedSet() {
		set("seed", opts.Seed)
	}
	if opts.ContextLength > 0 {
		set("num_ctx", opts.ContextLength)
	}
	return ollamaOpts
}

// chatResponseHandler handles streaming chat responses.
type chatResponseHandler struct {
	fullResponse strings.Builder
	thinking     strings.Builder
	toolCalls    []llms.ToolCall
	finalResp    api.ChatResponse
	streamingFn  func(ctx context.Context, chunk []byte) error
}

func (h *chatResponseHandler) handle(ctx context.Context, response api.ChatResponse) error {
	h.fullResponse.WriteString(response.Message.Content)
	if response.Message.Thinking != "" {
		h.thinking.WriteString(response.Message.Thinking)
	}
	// Handle tool calls
	if len(response.Message.ToolCalls) > 0 {
		for _, tc := range response.Message.ToolCalls {
			args := tc.Function.Arguments.ToMap()
			h.toolCalls = append(h.toolCalls, llms.ToolCall{
				Function: llms.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: args,
				},
			})
		}
	}
	if h.streamingFn != nil && response.Message.Content != "" {
		if err := h.streamingFn(ctx, []byte(response.Message.Content)); err != nil {
			return fmt.Errorf("streaming function returned an error: %w", err)
		}
	}
	if response.Done {
		h.finalResp = response
	}
	return nil
}

func (h *chatResponseHandler) reset() {
	h.fullResponse.Reset()
	h.thinking.Reset()
	h.toolCalls = nil
	h.finalResp = api.ChatResponse{}
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

	req := o.buildChatRequest(model, messages, opts)

	msgCount := len(req.Messages)
	hasImages := false
	for _, m := range req.Messages {
		if len(m.Images) > 0 {
			hasImages = true
			break
		}
	}

	o.logger.DebugContext(ctx, "Sending chat request",
		"model", model,
		"msg_count", msgCount,
		"has_images", hasImages,
		"tool_count", len(req.Tools))

	for i, m := range req.Messages {
		if len(m.Images) > 0 {
			for j, img := range m.Images {
				o.logger.DebugContext(ctx, "Image in request", "msg_index", i, "img_index", j, "size_bytes", len(img))
			}
		}
	}

	handler := &chatResponseHandler{streamingFn: opts.StreamingFunc}
	isStreaming := opts.StreamingFunc != nil
	fn := func() error {
		handler.reset()
		return o.client.Chat(ctx, req, func(response api.ChatResponse) error {
			return handler.handle(ctx, response)
		})
	}

	var err error
	if isStreaming {
		err = fn()
	} else {
		err = o.doWithRetry(ctx, fn)
	}

	duration := time.Since(start)
	if err != nil {
		o.logger.ErrorContext(ctx, "Ollama chat failed", "error", err, "duration", duration)
		return nil, err
	}

	last := finalResp(handler)
	genInfo := o.buildGenerationInfo(handler, last, model, duration)
	response := &schema.ContentResponse{
		Choices: []*schema.ContentChoice{
			{
				Content:          handler.fullResponse.String(),
				StopReason:       last.DoneReason,
				GenerationInfo:   genInfo,
				ReasoningContent: handler.thinking.String(),
			},
		},
	}

	o.logger.DebugContext(ctx, "Content generation completed", "duration", duration)
	return response, nil
}

func finalResp(h *chatResponseHandler) api.ChatResponse {
	return h.finalResp
}

// buildChatRequest creates a ChatRequest from the given parameters.
func (o *LLM) buildChatRequest(model string, messages []schema.MessageContent, opts llms.CallOptions) *api.ChatRequest {
	req := &api.ChatRequest{
		Model:    model,
		Messages: o.buildChatMessages(messages),
		Stream:   new(bool),
	}
	ollamaOpts := buildOllamaOptions(opts)
	if len(ollamaOpts) > 0 {
		req.Options = ollamaOpts
	}
	*req.Stream = opts.StreamingFunc != nil

	// Handle thinking/reasoning mode
	o.applyThinkOption(req, opts)

	if opts.KeepAlive != "" {
		req.KeepAlive = &api.Duration{Duration: parseKeepAlive(opts.KeepAlive)}
	} else if o.options.keepAlive > 0 {
		req.KeepAlive = &api.Duration{Duration: o.options.keepAlive}
	}

	// Handle structured output format
	o.applyFormatOption(req, opts)

	// Handle tools
	if len(opts.Tools) > 0 {
		req.Tools = convertToolsToAPI(opts.Tools)
	}

	return req
}

// applyThinkOption applies the thinking/reasoning option to the request.
func (o *LLM) applyThinkOption(req *api.ChatRequest, opts llms.CallOptions) {
	if opts.Think != nil {
		req.Think = toThinkValue(opts.Think)
	} else if o.options.thinking != nil && *o.options.thinking {
		req.Think = &api.ThinkValue{Value: true}
		if o.options.reasoningEffort != "" {
			req.Think = &api.ThinkValue{Value: o.options.reasoningEffort}
		}
	}
}

// applyFormatOption applies the format option for structured outputs.
func (o *LLM) applyFormatOption(req *api.ChatRequest, opts llms.CallOptions) {
	if opts.JSONMode {
		req.Format = json.RawMessage(`"json"`)
	} else if opts.JSONSchema != nil {
		schemaBytes, err := json.Marshal(opts.JSONSchema)
		if err != nil {
			o.logger.Warn("failed to marshal JSONSchema, structured output will not be applied", "error", err)
		} else {
			req.Format = schemaBytes
		}
	}
}

// buildGenerationInfo creates the generation info map from the response.
func (o *LLM) buildGenerationInfo(handler *chatResponseHandler, finalResp api.ChatResponse, model string, duration time.Duration) map[string]any {
	genInfo := map[string]any{
		"CompletionTokens": finalResp.EvalCount,
		"PromptTokens":     finalResp.PromptEvalCount,
		"TotalTokens":      finalResp.EvalCount + finalResp.PromptEvalCount,
		"Duration":         duration,
		"Model":            model,
	}
	if len(handler.toolCalls) > 0 {
		genInfo["ToolCalls"] = handler.toolCalls
	}
	return genInfo
}

func typeToRole(typ schema.ChatMessageType) string {
	switch typ {
	case schema.ChatMessageTypeSystem:
		return "system"
	case schema.ChatMessageTypeAI:
		return "assistant"
	case schema.ChatMessageTypeHuman, schema.ChatMessageTypeGeneric:
		return "user"
	case schema.ChatMessageTypeTool:
		return "tool"
	default:
		return "user"
	}
}

// toThinkValue converts an any value to *api.ThinkValue.
func toThinkValue(v any) *api.ThinkValue {
	switch val := v.(type) {
	case bool:
		return &api.ThinkValue{Value: val}
	case string:
		return &api.ThinkValue{Value: val}
	default:
		return nil
	}
}

// convertToolsToAPI converts llms.ToolDefinition to api.Tool definitions.
func convertToolsToAPI(tools []llms.ToolDefinition) []api.Tool {
	result := make([]api.Tool, 0, len(tools))
	for _, t := range tools {
		tool := api.Tool{
			Type: t.Type,
			Function: api.ToolFunction{
				Name:        t.Function.Name,
				Description: t.Function.Description,
			},
		}
		// Convert parameters if provided
		if t.Function.Parameters != nil {
			if params, ok := t.Function.Parameters.(map[string]any); ok {
				tool.Function.Parameters = convertToToolFunctionParameters(params)
			}
		}
		result = append(result, tool)
	}
	return result
}

// convertToToolFunctionParameters converts a map to api.ToolFunctionParameters.
func convertToToolFunctionParameters(params map[string]any) api.ToolFunctionParameters {
	result := api.ToolFunctionParameters{
		Required:   getStringSlice(params, "required"),
		Properties: api.NewToolPropertiesMap(),
	}
	if t, ok := params["type"].(string); ok {
		result.Type = t
	}
	if items, ok := params["items"]; ok {
		result.Items = items
	}
	if props, ok := params["properties"].(map[string]any); ok {
		for k, v := range props {
			if prop, ok := v.(map[string]any); ok {
				tp := convertToToolProperty(prop)
				result.Properties.Set(k, tp)
			}
		}
	}
	return result
}

// convertToToolProperty converts a map to api.ToolProperty.
func convertToToolProperty(m map[string]any) api.ToolProperty {
	tp := api.ToolProperty{}
	if t, ok := m["type"].(string); ok {
		tp.Type = api.PropertyType([]string{t})
	}
	if d, ok := m["description"].(string); ok {
		tp.Description = d
	}
	if items, ok := m["items"]; ok {
		tp.Items = items
	}
	if enum, ok := m["enum"].([]any); ok {
		tp.Enum = enum
	}
	if props, ok := m["properties"].(map[string]any); ok {
		tp.Properties = api.NewToolPropertiesMap()
		for k, v := range props {
			if p, ok := v.(map[string]any); ok {
				tp.Properties.Set(k, convertToToolProperty(p))
			}
		}
	}
	return tp
}

// getStringSlice extracts a []string from a map.
func getStringSlice(m map[string]any, key string) []string {
	if v, ok := m[key].([]string); ok {
		return v
	}
	if v, ok := m[key].([]any); ok {
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// parseKeepAlive parses a keep_alive duration string (e.g., "5m", "10m", "0").
func parseKeepAlive(s string) time.Duration {
	if s == "0" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 5 * time.Minute // default
	}
	return d
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

// EmbedDocumentsWithOpts generates embeddings with additional options.
func (o *LLM) EmbedDocumentsWithOpts(ctx context.Context, texts []string, opts embeddings.EmbeddingOptions) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	req := &api.EmbedRequest{
		Model:    o.options.model,
		Input:    texts,
		Truncate: &opts.Truncate,
	}
	if opts.Dimensions > 0 {
		req.Dimensions = opts.Dimensions
	}

	var resp *api.EmbedResponse
	err := o.doWithRetry(ctx, func() error {
		var err error
		resp, err = o.client.Embed(ctx, req)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("ollama embed failing: %w", err)
	}

	if len(resp.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama: embedding count mismatch: sent %d texts, got %d embeddings", len(texts), len(resp.Embeddings))
	}

	return resp.Embeddings, nil
}

// EmbedQueryWithOpts generates an embedding with additional options.
func (o *LLM) EmbedQueryWithOpts(ctx context.Context, text string, opts embeddings.EmbeddingOptions) ([]float32, error) {
	req := &api.EmbedRequest{
		Model:    o.options.model,
		Input:    text,
		Truncate: &opts.Truncate,
	}
	if opts.Dimensions > 0 {
		req.Dimensions = opts.Dimensions
	}

	var resp *api.EmbedResponse
	err := o.doWithRetry(ctx, func() error {
		var err error
		resp, err = o.client.Embed(ctx, req)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("ollama embed failing: %w", err)
	}

	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("ollama: empty embedding response")
	}

	return resp.Embeddings[0], nil
}

func (o *LLM) GetModelDetails(ctx context.Context) (*schema.ModelDetails, error) {
	o.detailsMu.RLock()
	if o.details != nil && o.detailsErr == nil {
		d := o.details
		o.detailsMu.RUnlock()
		return d, nil
	}
	o.detailsMu.RUnlock()

	o.detailsMu.Lock()
	defer o.detailsMu.Unlock()

	if o.details != nil && o.detailsErr == nil {
		return o.details, nil
	}

	details, err := o.fetchModelDetails(ctx)
	if err != nil {
		o.detailsErr = err
		return nil, err
	}
	o.details = details
	o.detailsErr = nil
	return details, nil
}

// InvalidateModelDetailsCache clears the cached model details so the next
// call to GetModelDetails will fetch fresh data from the server.
func (o *LLM) InvalidateModelDetailsCache() {
	o.detailsMu.Lock()
	defer o.detailsMu.Unlock()
	o.details = nil
	o.detailsErr = nil
}

func (o *LLM) fetchModelDetails(ctx context.Context) (*schema.ModelDetails, error) {
	showResp, err := o.client.Show(ctx, &api.ShowRequest{Name: o.options.model})
	if err != nil {
		return nil, fmt.Errorf("fetching model details: %w", err)
	}

	var dim int64
	if dim = extractEmbeddingLength(showResp.ModelInfo); dim == 0 {
		testEmb, embErr := o.EmbedQuery(ctx, "test")
		if embErr == nil {
			dim = int64(len(testEmb))
		}
	}

	return &schema.ModelDetails{
		Family:        showResp.Details.Family,
		ParameterSize: showResp.Details.ParameterSize,
		Quantization:  showResp.Details.QuantizationLevel,
		Dimension:     dim,
	}, nil
}

// extractEmbeddingLength extracts the embedding dimension from Ollama model_info.
// The key follows the pattern "<family>.embedding_length" (e.g., "gemma3.embedding_length": 2560).
func extractEmbeddingLength(modelInfo map[string]any) int64 {
	if modelInfo == nil {
		return 0
	}
	for key, val := range modelInfo {
		if strings.HasSuffix(key, ".embedding_length") {
			if n, ok := toInt64(val); ok {
				return n
			}
		}
	}
	return 0
}

// toInt64 converts numeric values (int, float64, json.Number) to int64.
func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return i, true
	}
	return 0, false
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
			"num_predict": 1, // Generate 1 token to get prompt eval count (Ollama rejects 0)
		},
	}
	if o.options.keepAlive > 0 {
		req.KeepAlive = &api.Duration{Duration: o.options.keepAlive}
	}

	var tokenCount int
	fn := func(resp api.GenerateResponse) error {
		if resp.Done {
			tokenCount = resp.PromptEvalCount
		}
		return nil
	}

	err := o.doWithRetry(ctx, func() error {
		tokenCount = 0
		return o.client.Generate(ctx, req, fn)
	})
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

func (o *LLM) PullModel(ctx context.Context, name string) error {
	o.logger.Info("Pulling model", "model", name)
	lastLogged := -1
	return o.client.Pull(ctx, &api.PullRequest{Name: name}, func(resp api.ProgressResponse) error {
		if resp.Total > 0 {
			milestone := int(float64(resp.Completed)/float64(resp.Total)*100) / 25 * 25
			if milestone != lastLogged {
				lastLogged = milestone
				o.logger.Debug("Pull progress", "model", name, "percent", fmt.Sprintf("%d%%", milestone))
			}
		}
		return nil
	})
}

func (o *LLM) HasModel(ctx context.Context, name string) (bool, error) {
	list, err := o.client.List(ctx)
	if err != nil {
		return false, err
	}
	// Normalize: Ollama lists models as "name:tag". If the caller omits the tag,
	// append ":latest" so that HasModel("llama3") matches "llama3:latest".
	normalized := name
	if !strings.Contains(name, ":") {
		normalized = name + ":latest"
	}
	for _, m := range list.Models {
		if m.Name == name || m.Name == normalized {
			return true, nil
		}
	}
	return false, nil
}

// ListModels returns all locally available models.
func (o *LLM) ListModels(ctx context.Context) ([]ModelInfo, error) {
	list, err := o.client.List(ctx)
	if err != nil {
		return nil, err
	}
	models := make([]ModelInfo, 0, len(list.Models))
	for _, m := range list.Models {
		models = append(models, ModelInfo{
			Name:       m.Name,
			Model:      m.Model,
			ModifiedAt: m.ModifiedAt,
			Size:       m.Size,
			Digest:     m.Digest,
		})
	}
	return models, nil
}

// RunningModel represents a model currently loaded in memory.
type RunningModel struct {
	Name          string
	Model         string
	Size          int64
	SizeVRAM      int64
	Digest        string
	ExpiresAt     time.Time
	ContextLength int
}

// ListRunningModels returns all models currently loaded in memory.
func (o *LLM) ListRunningModels(ctx context.Context) ([]RunningModel, error) {
	ps, err := o.client.ListRunning(ctx)
	if err != nil {
		return nil, err
	}
	models := make([]RunningModel, 0, len(ps.Models))
	for _, m := range ps.Models {
		models = append(models, RunningModel{
			Name:          m.Name,
			Model:         m.Model,
			Size:          m.Size,
			SizeVRAM:      m.SizeVRAM,
			Digest:        m.Digest,
			ExpiresAt:     m.ExpiresAt,
			ContextLength: m.ContextLength,
		})
	}
	return models, nil
}

// DeleteModel removes a model from local storage.
func (o *LLM) DeleteModel(ctx context.Context, name string) error {
	o.logger.Info("Deleting model", "model", name)
	return o.client.Delete(ctx, &api.DeleteRequest{Name: name})
}

// CopyModel creates a copy of a model with a new name.
func (o *LLM) CopyModel(ctx context.Context, source, destination string) error {
	o.logger.Info("Copying model", "source", source, "destination", destination)
	return o.client.Copy(ctx, &api.CopyRequest{Source: source, Destination: destination})
}

// ModelInfo represents information about a locally available model.
type ModelInfo struct {
	Name       string
	Model      string
	ModifiedAt time.Time
	Size       int64
	Digest     string
}

// VersionInfo contains Ollama server version information.
type VersionInfo struct {
	Version string
}

// GetVersion returns the Ollama server version.
func (o *LLM) GetVersion(ctx context.Context) (*VersionInfo, error) {
	version, err := o.client.Version(ctx)
	if err != nil {
		return nil, err
	}
	return &VersionInfo{Version: version}, nil
}

// retryableErrorPatterns contains error patterns that indicate a transient failure.
var retryableErrorPatterns = []string{
	"http2: server sent GOAWAY",
	"connection reset by peer",
	"connection refused",
	"unexpected EOF",
	"io: read/write on closed pipe",
	"network is unreachable",
	"no such host",
	"timeout",
	"ECONNRESET",
	"ECONNREFUSED",
	"ETIMEDOUT",
}

// isRetryableError determines if an error is transient and should be retried.
// Context cancellation and deadline errors from the caller's context are not retryable —
// retrying would immediately fail again with the same expired context.
func (o *LLM) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	errStr := strings.ToLower(err.Error())
	for _, pattern := range retryableErrorPatterns {
		if strings.Contains(errStr, strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

// calculateNextDelay calculates the next retry delay with exponential backoff.
func (o *LLM) calculateNextDelay(delay time.Duration) time.Duration {
	delay *= 2
	if delay > o.options.maxRetryDelay {
		delay = o.options.maxRetryDelay
	}
	return delay
}

// waitForRetryDelay waits for the specified delay with jitter, respecting context cancellation.
func (o *LLM) waitForRetryDelay(ctx context.Context, delay time.Duration, attempt int, err error) error {
	var jitter time.Duration
	if o.options.retryJitter > 0 {
		jitter = time.Duration(rand.IntN(int(o.options.retryJitter.Milliseconds()))) * time.Millisecond //nolint:gosec // rand.IntN is sufficient for retry jitter
	}
	totalDelay := delay + jitter

	o.logger.WarnContext(ctx, "Retrying Ollama API call",
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

// doWithRetry executes a function with retry logic for transient errors.
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

		// Check if we've exhausted our retries
		if attempt >= o.options.retryAttempts {
			break
		}

		// Only retry on transient errors
		if !o.isRetryableError(err) {
			break
		}

		// Wait before retrying
		if retryErr := o.waitForRetryDelay(ctx, delay, attempt+1, err); retryErr != nil {
			return retryErr
		}
		delay = o.calculateNextDelay(delay)
	}

	return lastErr
}
