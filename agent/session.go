package agent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	opencode "github.com/sst/opencode-sdk-go"
)

type Session struct {
	ID        string
	Title     string
	Directory string
	ProjectID string
	Created   float64
	Updated   float64
	client    *opencode.Client
	logger    *slog.Logger
}

var _ io.Closer = (*Session)(nil)

func (s *Session) Close() error {
	return s.Abort(context.Background())
}

type SessionManager struct {
	client *opencode.Client
	logger *slog.Logger
}

func NewSessionManager(client *opencode.Client, logger *slog.Logger) *SessionManager {
	return &SessionManager{
		client: client,
		logger: logger,
	}
}

func (s *Session) Prompt(ctx context.Context, prompt string, opts ...PromptOption) (*Response, error) {
	config := &PromptConfig{}
	for _, opt := range opts {
		opt(config)
	}

	parts := make([]opencode.SessionPromptParamsPartUnion, 0)

	if prompt != "" {
		parts = append(parts, opencode.TextPartInputParam{
			Type: opencode.F(opencode.TextPartInputTypeText),
			Text: opencode.F(prompt),
		})
	}

	for _, part := range config.Parts {
		input, err := part.ToInput()
		if err != nil {
			return nil, fmt.Errorf("converting prompt part: %w", err)
		}
		parts = append(parts, input)
	}

	params := opencode.SessionPromptParams{
		Parts: opencode.F(parts),
	}

	if config.Agent != "" {
		params.Agent = opencode.F(config.Agent)
	}

	resp, err := s.client.Session.Prompt(ctx, s.ID, params)
	if err != nil {
		return nil, newSessionError("prompt", s.ID, err)
	}

	response := &Response{
		SessionID: s.ID,
		MessageID: resp.Info.ID,
	}

	response.Content = extractTextFromParts(resp.Parts)
	response.Parts = convertParts(resp.Parts)
	response.Tokens = extractTokenUsage(&resp.Info)
	response.Cost = resp.Info.Cost

	return response, nil
}

func (s *Session) PromptStream(ctx context.Context, prompt string, opts ...PromptOption) (<-chan Event, error) {
	events := make(chan Event, 100)

	go func() {
		defer close(events)

		resp, err := s.Prompt(ctx, prompt, opts...)
		if err != nil {
			select {
			case events <- Event{
				Type:      EventTypeError,
				SessionID: s.ID,
				Error:     err,
			}:
			case <-ctx.Done():
			}
			return
		}

		select {
		case events <- Event{
			Type:      EventTypeComplete,
			SessionID: s.ID,
			MessageID: resp.MessageID,
			Data:      *resp,
		}:
		case <-ctx.Done():
		}
	}()

	return events, nil
}

func (s *Session) Messages(ctx context.Context) ([]Message, error) {
	resp, err := s.client.Session.Messages(ctx, s.ID, opencode.SessionMessagesParams{})
	if err != nil {
		return nil, newSessionError("messages", s.ID, err)
	}

	messages := make([]Message, 0, len(*resp))
	for _, m := range *resp {
		var createdAt int64
		if timeMap, ok := m.Info.Time.(map[string]interface{}); ok {
			if created, ok := timeMap["created"].(float64); ok {
				createdAt = int64(created)
			}
		}

		msg := Message{
			ID:        m.Info.ID,
			SessionID: m.Info.SessionID,
			CreatedAt: createdAt,
			Role:      string(m.Info.Role),
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

func (s *Session) Abort(ctx context.Context) error {
	_, err := s.client.Session.Abort(ctx, s.ID, opencode.SessionAbortParams{})
	if err != nil {
		return newSessionError("abort", s.ID, err)
	}
	return nil
}

func (s *Session) Summarize(ctx context.Context) error {
	_, err := s.client.Session.Summarize(ctx, s.ID, opencode.SessionSummarizeParams{})
	if err != nil {
		return newSessionError("summarize", s.ID, err)
	}
	return nil
}

func (s *Session) Share(ctx context.Context) (string, error) {
	resp, err := s.client.Session.Share(ctx, s.ID, opencode.SessionShareParams{})
	if err != nil {
		return "", newSessionError("share", s.ID, err)
	}

	if resp.Share.URL != "" {
		return resp.Share.URL, nil
	}
	return "", nil
}

func (s *Session) Unshare(ctx context.Context) error {
	_, err := s.client.Session.Unshare(ctx, s.ID, opencode.SessionUnshareParams{})
	if err != nil {
		return newSessionError("unshare", s.ID, err)
	}
	return nil
}

func (s *Session) Revert(ctx context.Context, messageID string) error {
	_, err := s.client.Session.Revert(ctx, s.ID, opencode.SessionRevertParams{
		MessageID: opencode.F(messageID),
	})
	if err != nil {
		return newSessionError("revert", s.ID, err)
	}
	return nil
}

func (s *Session) UpdateTitle(ctx context.Context, title string) error {
	_, err := s.client.Session.Update(ctx, s.ID, opencode.SessionUpdateParams{
		Title: opencode.F(title),
	})
	if err != nil {
		return newSessionError("update", s.ID, err)
	}

	s.Title = title
	s.Updated = float64(time.Now().Unix())
	return nil
}

func (s *Session) Refresh(ctx context.Context) error {
	resp, err := s.client.Session.Get(ctx, s.ID, opencode.SessionGetParams{})
	if err != nil {
		return newSessionError("refresh", s.ID, err)
	}

	s.Title = resp.Title
	s.Directory = resp.Directory
	s.ProjectID = resp.ProjectID
	s.Updated = resp.Time.Updated

	return nil
}

func extractTextFromParts(parts []opencode.Part) string {
	var text string
	for _, part := range parts {
		if part.Text != "" {
			text += part.Text
		}
	}
	return text
}

func convertParts(parts []opencode.Part) []Part {
	result := make([]Part, 0, len(parts))
	for _, part := range parts {
		p := Part{
			ID:   part.ID,
			Type: string(part.Type),
		}

		if part.Text != "" {
			p.Text = part.Text
		}

		if part.Tool != "" {
			tool := &ToolCall{
				ID:   part.ID,
				Name: part.Tool,
			}
			p.Tool = tool
		}

		if part.Filename != "" || part.Mime != "" {
			file := &FileInfo{}
			if part.Filename != "" {
				file.Path = part.Filename
			}
			if part.Mime != "" {
				file.MimeType = part.Mime
			}
			p.File = file
		}

		result = append(result, p)
	}
	return result
}

func extractTokenUsage(msg *opencode.AssistantMessage) TokenUsage {
	usage := TokenUsage{}
	if msg.Tokens.Input > 0 {
		usage.Input = msg.Tokens.Input
	}
	if msg.Tokens.Output > 0 {
		usage.Output = msg.Tokens.Output
	}
	if msg.Tokens.Reasoning > 0 {
		usage.Reasoning = msg.Tokens.Reasoning
	}
	if msg.Tokens.Cache.Read > 0 {
		usage.CacheRead = msg.Tokens.Cache.Read
	}
	if msg.Tokens.Cache.Write > 0 {
		usage.CacheWrite = msg.Tokens.Cache.Write
	}
	return usage
}
