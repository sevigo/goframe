package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/sevigo/goframe/httpclient"
	"github.com/sevigo/goframe/voice"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
	defaultModel   = "tts-1"
	defaultVoice   = "alloy"
	defaultFormat  = "mp3"
)

var (
	ErrAPIKeyRequired = errors.New("openai: API key required for OpenAI endpoint")
)

var _ voice.Synthesizer = (*Synthesizer)(nil)

type Synthesizer struct {
	client  *http.Client
	baseURL string
	apiKey  string
	model   string
	voice   string
	format  string
	logger  *slog.Logger
}

type Option func(*Synthesizer)

func WithAPIKey(apiKey string) Option {
	return func(s *Synthesizer) {
		s.apiKey = strings.TrimSpace(apiKey)
	}
}

func WithBaseURL(baseURL string) Option {
	return func(s *Synthesizer) {
		if baseURL != "" {
			s.baseURL = strings.TrimSuffix(baseURL, "/")
		}
	}
}

func WithModel(model string) Option {
	return func(s *Synthesizer) {
		if model != "" {
			s.model = model
		}
	}
}

func WithVoice(voice string) Option {
	return func(s *Synthesizer) {
		if voice != "" {
			s.voice = voice
		}
	}
}

func WithFormat(format string) Option {
	return func(s *Synthesizer) {
		if format != "" {
			s.format = format
		}
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(s *Synthesizer) {
		if client != nil {
			s.client = client
		}
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(s *Synthesizer) {
		if logger != nil {
			s.logger = logger
		}
	}
}

func NewSynthesizer(opts ...Option) (*Synthesizer, error) {
	s := &Synthesizer{
		client:  httpclient.DefaultClient,
		baseURL: defaultBaseURL,
		model:   defaultModel,
		voice:   defaultVoice,
		format:  defaultFormat,
		logger:  slog.Default(),
	}

	for _, opt := range opts {
		opt(s)
	}

	s.baseURL = strings.TrimSuffix(s.baseURL, "/")

	if s.baseURL == defaultBaseURL && s.apiKey == "" {
		return nil, ErrAPIKeyRequired
	}

	return s, nil
}

func (s *Synthesizer) Synthesize(ctx context.Context, text string, opts ...voice.Option) (*voice.Audio, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("openai: text cannot be empty")
	}

	options := &voice.SynthesizeOptions{
		Model:  s.model,
		Voice:  s.voice,
		Format: s.format,
	}
	for _, opt := range opts {
		opt(options)
	}

	reqBody := &speechRequest{
		Model:          options.Model,
		Input:          text,
		Voice:          options.Voice,
		ResponseFormat: options.Format,
	}
	if options.Speed > 0 {
		reqBody.Speed = options.Speed
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to marshal request: %w", err)
	}

	url := s.baseURL + "/audio/speech"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("openai: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, s.parseError(resp)
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to read response body: %w", err)
	}

	return &voice.Audio{
		Data:   audioData,
		Format: options.Format,
	}, nil
}

func (s *Synthesizer) parseError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("openai: request failed with status %d (unable to read error body)", resp.StatusCode)
	}

	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		return fmt.Errorf("openai: %s (status %d)", errResp.Error.Message, resp.StatusCode)
	}

	return fmt.Errorf("openai: request failed with status %d: %s", resp.StatusCode, string(body))
}

type speechRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format"`
	Speed          float64 `json:"speed,omitempty"`
}

var _ voice.Synthesizer = (*Synthesizer)(nil)
