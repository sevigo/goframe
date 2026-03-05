// Package openai provides an OpenAI-compatible Text-to-Speech implementation.
// It supports both the official OpenAI API and local OpenAI-compatible servers
// like Kokoro-FastAPI.
//
// The package implements the voice.Synthesizer interface for Text-to-Speech synthesis
// with support for both buffered and streaming audio generation.
//
// Example usage with OpenAI:
//
//	synthesizer, err := openai.NewSynthesizer(
//	    openai.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
//	    openai.WithModel("tts-1"),
//	    openai.WithVoice("alloy"),
//	)
//
// Example usage with local Kokoro:
//
//	synthesizer, err := openai.NewSynthesizer(
//	    openai.WithBaseURL("http://localhost:8880/v1"),
//	    openai.WithModel("kokoro"),
//	    openai.WithVoice("af_bella"),
//	)
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

// Default configuration values.
const (
	defaultBaseURL = "https://api.openai.com/v1"
	defaultModel   = "tts-1"
	defaultVoice   = "alloy"
	defaultFormat  = "mp3"
)

// Error definitions.
var (
	// ErrAPIKeyRequired is returned when an API key is required but not provided.
	// This error occurs when using the default OpenAI endpoint without credentials.
	ErrAPIKeyRequired = errors.New("openai: API key required for OpenAI endpoint")
)

// Compile-time interface check.
var _ voice.Synthesizer = (*Synthesizer)(nil)

// Synthesizer implements voice.Synthesizer using an OpenAI-compatible API.
// It supports both buffered synthesis (Synthesize) and streaming (Stream) modes.
type Synthesizer struct {
	client  *http.Client
	baseURL string
	apiKey  string
	model   string
	voice   string
	format  string
	logger  *slog.Logger
}

// Option is a functional option for configuring the Synthesizer.
type Option func(*Synthesizer)

// WithAPIKey sets the API key for authentication.
// Required when using the default OpenAI endpoint.
func WithAPIKey(apiKey string) Option {
	return func(s *Synthesizer) {
		s.apiKey = strings.TrimSpace(apiKey)
	}
}

// WithBaseURL sets the base URL for the TTS API.
// Use this to point to a local server like Kokoro-FastAPI.
// Defaults to "https://api.openai.com/v1".
func WithBaseURL(baseURL string) Option {
	return func(s *Synthesizer) {
		if baseURL != "" {
			s.baseURL = strings.TrimSuffix(baseURL, "/")
		}
	}
}

// WithModel sets the TTS model to use.
// For OpenAI: "tts-1" (default) or "tts-1-hd".
// For Kokoro: "kokoro".
func WithModel(model string) Option {
	return func(s *Synthesizer) {
		if model != "" {
			s.model = model
		}
	}
}

// WithVoice sets the voice identifier for synthesis.
// OpenAI voices: alloy, echo, fable, onyx, nova, shimmer.
// Kokoro voices: af_bella, af_sarah, af_sky, am_adam, etc.
func WithVoice(voice string) Option {
	return func(s *Synthesizer) {
		if voice != "" {
			s.voice = voice
		}
	}
}

// WithFormat sets the output audio format.
// OpenAI supports: mp3, opus, aac, flac, wav, pcm.
// Kokoro supports: wav, mp3, etc.
func WithFormat(format string) Option {
	return func(s *Synthesizer) {
		if format != "" {
			s.format = format
		}
	}
}

// WithHTTPClient sets a custom HTTP client for making requests.
// Use this to configure timeouts, retries, or custom transport.
func WithHTTPClient(client *http.Client) Option {
	return func(s *Synthesizer) {
		if client != nil {
			s.client = client
		}
	}
}

// WithLogger sets a custom structured logger.
func WithLogger(logger *slog.Logger) Option {
	return func(s *Synthesizer) {
		if logger != nil {
			s.logger = logger
		}
	}
}

// NewSynthesizer creates a new OpenAI-compatible TTS synthesizer.
// It returns ErrAPIKeyRequired if using the default OpenAI endpoint
// without providing an API key.
//
// For local servers like Kokoro, you can omit the API key:
//
//	synthesizer, err := openai.NewSynthesizer(
//	    openai.WithBaseURL("http://localhost:8880/v1"),
//	    openai.WithVoice("af_bella"),
//	)
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

// Synthesize generates audio from text and returns the complete audio data.
// This method buffers the entire response in memory before returning.
// Use Stream for larger texts or when you want to process audio incrementally.
func (s *Synthesizer) Synthesize(ctx context.Context, text string, opts ...voice.Option) (*voice.Audio, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("openai: text cannot be empty")
	}

	options := s.buildOptions(opts)

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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/audio/speech", bytes.NewReader(bodyBytes))
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

// Stream generates audio from text and returns a stream for reading audio chunks.
// The caller must close the returned ReadCloser when done.
// This is more memory-efficient for longer texts as audio is streamed incrementally.
//
// Example:
//
//	stream, err := synthesizer.Stream(ctx, text)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer stream.Close()
//
//	file, _ := os.Create("output.wav")
//	defer file.Close()
//	io.Copy(file, stream)
func (s *Synthesizer) Stream(ctx context.Context, text string, opts ...voice.Option) (io.ReadCloser, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("openai: text cannot be empty")
	}

	options := s.buildOptions(opts)

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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/audio/speech", bytes.NewReader(bodyBytes))
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

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, s.parseError(resp)
	}

	return resp.Body, nil
}

// buildOptions merges default options with per-request options.
func (s *Synthesizer) buildOptions(opts []voice.Option) *voice.SynthesizeOptions {
	options := &voice.SynthesizeOptions{
		Model:  s.model,
		Voice:  s.voice,
		Format: s.format,
	}
	for _, opt := range opts {
		opt(options)
	}
	return options
}

// parseError extracts error details from an HTTP error response.
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

// speechRequest represents the JSON request body for the TTS API.
type speechRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format"`
	Speed          float64 `json:"speed,omitempty"`
}
