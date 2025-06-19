package gemini

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

// Common errors returned by the Gemini LLM implementation.
var (
	ErrNoAPIKey      = errors.New("gemini: API key is required")
	ErrInvalidModel  = errors.New("gemini: invalid model specified")
	ErrNoContent     = errors.New("gemini: no content generated")
	ErrSystemMessage = errors.New("gemini: does not support system messages in the middle of a conversation. Use SystemInstruction option")
)

// LLM provides an integration with the Google Gemini API.
type LLM struct {
	client  *genai.GenerativeModel
	options options
	logger  *slog.Logger
}

// Compile-time interface check
var _ llms.Model = (*LLM)(nil)

// New creates a new Gemini LLM client.
// It requires a GEMINI_API_KEY environment variable or the WithAPIKey option.
func New(ctx context.Context, opts ...Option) (*LLM, error) {
	o := applyOptions(opts...)

	// Get API Key from options or environment variable
	if o.apiKey == "" {
		o.apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if o.apiKey == "" {
		return nil, ErrNoAPIKey
	}

	if o.model == "" {
		return nil, ErrInvalidModel
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(o.apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	modelClient := client.GenerativeModel(o.model)

	llm := &LLM{
		client:  modelClient,
		options: o,
		logger:  o.logger.With("component", "gemini_llm", "model", o.model),
	}

	llm.logger.Info("Gemini LLM initialized successfully")
	return llm, nil
}

// Call implements simple prompt-based text generation.
func (g *LLM) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return llms.GenerateFromSinglePrompt(ctx, g, prompt, options...)
}

// GenerateContent handles structured message-based content generation.
func (g *LLM) GenerateContent(
	ctx context.Context,
	messages []schema.MessageContent,
	options ...llms.CallOption,
) (*schema.ContentResponse, error) {
	start := time.Now()
	g.logger.DebugContext(ctx, "Starting Gemini content generation", "message_count", len(messages))

	// Apply call options
	callOpts := &llms.CallOptions{}
	for _, opt := range options {
		opt(callOpts)
	}

	// Prepare Gemini-specific client instance with call options
	client := g.client
	if callOpts.Temperature > 0 {
		client.Temperature = new(float32)
		*client.Temperature = float32(callOpts.Temperature)
	}

	// Convert messages to Gemini's format
	history, systemInstruction, err := g.convertToGeminiMessages(messages)
	if err != nil {
		return nil, err
	}
	if systemInstruction != nil {
		client.SystemInstruction = systemInstruction
	}

	if len(history) == 0 {
		return nil, errors.New("gemini: no messages to send")
	}
	prompt := history[len(history)-1]
	chatHistory := history[:len(history)-1]

	session := client.StartChat()
	session.History = chatHistory

	isStreamingFunc := callOpts.StreamingFunc != nil

	if !isStreamingFunc {
		// Non-streaming generation
		resp, err := session.SendMessage(ctx, prompt.Parts...)
		duration := time.Since(start)
		if err != nil {
			g.logger.ErrorContext(ctx, "Gemini client failed", "error", err, "duration", duration)
			return nil, err
		}
		return g.responseToSchema(resp, duration)
	}

	// Streaming generation
	iter := session.SendMessageStream(ctx, prompt.Parts...)
	var fullResponse strings.Builder
	var finalResp *genai.GenerateContentResponse

	for {
		resp, errStream := iter.Next()
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

	var totalTokens uint32
	if finalResp != nil && finalResp.UsageMetadata != nil {
		totalTokens = uint32(finalResp.UsageMetadata.TotalTokenCount)
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

// convertToGeminiMessages converts the framework's message schema to Gemini's.
func (g *LLM) convertToGeminiMessages(messages []schema.MessageContent) ([]*genai.Content, *genai.Content, error) {
	geminiContents := make([]*genai.Content, 0, len(messages))
	var systemInstruction *genai.Content

	for i, msg := range messages {
		var role string
		switch msg.Role {
		case schema.ChatMessageTypeHuman:
			role = "user"
		case schema.ChatMessageTypeAI:
			role = "model"
		case schema.ChatMessageTypeSystem:
			// System instructions must be at the start.
			if i == 0 {
				systemInstruction = &genai.Content{
					Parts: []genai.Part{genai.Text(msg.GetTextContent())},
				}
				continue
			}
			return nil, nil, ErrSystemMessage
		default:
			role = "user"
		}

		parts := make([]genai.Part, 0, len(msg.Parts))
		for _, p := range msg.Parts {
			switch part := p.(type) {
			case schema.TextContent:
				parts = append(parts, genai.Text(part.Text))
			default:
				return nil, nil, fmt.Errorf("unsupported content part type: %T", part)
			}
		}

		geminiContents = append(geminiContents, &genai.Content{
			Role:  role,
			Parts: parts,
		})
	}

	return geminiContents, systemInstruction, nil
}

// responseToSchema converts a Gemini response to the framework's schema.
func (g *LLM) responseToSchema(resp *genai.GenerateContentResponse, duration time.Duration) (*schema.ContentResponse, error) {
	if len(resp.Candidates) == 0 {
		return nil, ErrNoContent
	}

	choice := resp.Candidates[0]
	if choice.Content == nil || len(choice.Content.Parts) == 0 {
		return nil, ErrNoContent
	}

	content := g.extractContentFromResponse(resp)

	var totalTokens uint32
	if resp.UsageMetadata != nil {
		totalTokens = uint32(resp.UsageMetadata.TotalTokenCount)
	}

	return &schema.ContentResponse{
		Choices: []*schema.ContentChoice{
			{
				Content:    content,
				StopReason: choice.FinishReason.String(),
				GenerationInfo: map[string]any{
					"TotalTokens": totalTokens,
					"Duration":    duration,
					"Model":       g.options.model,
				},
			},
		},
	}, nil
}

// extractContentFromResponse safely extracts text from a Gemini response.
func (g *LLM) extractContentFromResponse(resp *genai.GenerateContentResponse) string {
	var builder strings.Builder
	if resp == nil || len(resp.Candidates) == 0 {
		return ""
	}
	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if txt, ok := part.(genai.Text); ok {
					builder.WriteString(string(txt))
				}
			}
		}
	}
	return builder.String()
}
