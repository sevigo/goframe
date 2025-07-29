package gemini

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/iterator"
	"google.golang.org/genai"

	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

var (
	ErrNoAPIKey      = errors.New("gemini: API key is required")
	ErrInvalidModel  = errors.New("gemini: invalid model specified")
	ErrNoContent     = errors.New("gemini: no content generated")
	ErrSystemMessage = errors.New("gemini: system message must be the first message in the conversation")
	ErrEmbeddings    = errors.New("gemini: failed to generate embeddings")
)

// LLM implements both the Model and Embedder interfaces for Gemini.
type LLM struct {
	client  *genai.Client
	options options
	logger  *slog.Logger

	// dimension is cached after the first call to GetDimension
	dimension int
	dimOnce   sync.Once
}

var _ llms.Model = (*LLM)(nil)
var _ embeddings.Embedder = (*LLM)(nil)

// New creates a new Gemini LLM client.
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

	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: o.apiKey})
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	llm := &LLM{
		client:  client,
		options: o,
		logger:  o.logger.With("component", "gemini_llm", "model", o.model),
	}

	llm.logger.Info("Gemini LLM initialized successfully")
	return llm, nil
}

// Call is a convenience method for a single-turn conversation.
func (g *LLM) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return llms.GenerateFromSinglePrompt(ctx, g, prompt, options...)
}

// GenerateContent handles multi-turn conversations and streaming.
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

	// Create the generation configuration for this specific call.
	genConfig := &genai.GenerateContentConfig{}
	if callOpts.Temperature > 0 {
		genConfig.Temperature = genai.Ptr(float32(callOpts.Temperature))
	}

	geminiHistory, systemInstruction, err := g.convertToGeminiMessages(messages)
	if err != nil {
		return nil, err
	}

	// Prepend systemInstruction if it exists
	if systemInstruction != nil {
		geminiHistory = append([]*genai.Content{systemInstruction}, geminiHistory...)
	}

	if len(geminiHistory) == 0 {
		return nil, errors.New("gemini: no messages to send")
	}

	if callOpts.StreamingFunc == nil {
		resp, err := g.client.Models.GenerateContent(ctx, g.options.model, geminiHistory, genConfig)
		duration := time.Since(start)
		if err != nil {
			g.logger.ErrorContext(ctx, "Gemini client failed", "error", err, "duration", duration)
			return nil, err
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
			return nil, errStream
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

// EmbedDocuments generates embeddings for a slice of texts.
func (g *LLM) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	contents := make([]*genai.Content, len(texts))
	for i, text := range texts {
		contents[i] = genai.NewContentFromText(text, genai.RoleUser)
	}

	res, err := g.client.Models.EmbedContent(ctx, g.options.embeddingModel, contents, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrEmbeddings.Error(), err)
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

// EmbedQuery generates an embedding for a single text query.
func (g *LLM) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	content := genai.NewContentFromText(text, genai.RoleUser)
	res, err := g.client.Models.EmbedContent(ctx, g.options.embeddingModel, []*genai.Content{content}, nil)
	if err != nil {
		return nil, fmt.Errorf("%s for query: %w", ErrEmbeddings.Error(), err)
	}

	if len(res.Embeddings) == 0 || res.Embeddings[0] == nil {
		return nil, fmt.Errorf("%w: embedding is nil or empty", ErrEmbeddings)
	}
	return res.Embeddings[0].Values, nil
}

// EmbedQueries generates embeddings for multiple queries.
func (g *LLM) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	return g.EmbedDocuments(ctx, texts)
}

// GetDimension returns the embedding dimension of the model.
func (g *LLM) GetDimension(ctx context.Context) (int, error) {
	var err error
	g.dimOnce.Do(func() {
		sampleEmbedding, doErr := g.EmbedQuery(ctx, "dimension")
		if doErr != nil {
			err = fmt.Errorf("failed to get dimension by embedding sample text: %w", doErr)
			return
		}
		g.dimension = len(sampleEmbedding)
	})

	if err != nil {
		g.dimOnce = sync.Once{}
		return 0, err
	}

	return g.dimension, nil
}

// convertToGeminiMessages converts the generic schema to Gemini's native types.
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
            systemInstruction = genai.NewContentFromText(msg.GetTextContent(), genai.RoleUser) // Gemini treats system prompts as user roles
            systemMessageFound = true
            continue // Do not add to the main history slice
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

// responseToSchema converts Gemini's response to the generic schema.
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

// extractContentFromResponse safely extracts the text content from a response.
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
