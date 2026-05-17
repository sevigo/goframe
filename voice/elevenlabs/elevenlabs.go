package elevenlabs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sevigo/goframe/voice"
)

const (
	defaultBaseURL = "https://api.elevenlabs.io"
	defaultModelID = "eleven_multilingual_v2"
)

var (
	// ErrAPIKeyRequired is returned when no API key is provided.
	ErrAPIKeyRequired = errors.New("elevenlabs: API key required")
	// ErrVoiceIDRequired is returned when no voice ID is provided.
	ErrVoiceIDRequired = errors.New("elevenlabs: voice ID required")
)

var _ voice.Synthesizer = (*Synthesizer)(nil)
var _ voice.CaptionedSynthesizer = (*Synthesizer)(nil)

// Synthesizer implements the voice.Synthesizer and voice.CaptionedSynthesizer interfaces.
type Synthesizer struct {
	client          *http.Client
	baseURL         string
	apiKey          string
	voiceID         string
	modelID         string
	format          string
	stability       float64
	similarityBoost float64
}

// Option configures a Synthesizer.
type Option func(*Synthesizer)

// WithAPIKey sets the ElevenLabs API key.
func WithAPIKey(apiKey string) Option {
	return func(s *Synthesizer) {
		s.apiKey = strings.TrimSpace(apiKey)
	}
}

// WithBaseURL sets the API base URL.
func WithBaseURL(baseURL string) Option {
	return func(s *Synthesizer) {
		if baseURL != "" {
			s.baseURL = strings.TrimSuffix(baseURL, "/")
		}
	}
}

// WithVoiceID sets the voice to use for synthesis.
func WithVoiceID(voiceID string) Option {
	return func(s *Synthesizer) {
		s.voiceID = strings.TrimSpace(voiceID)
	}
}

// WithModelID sets the model ID for synthesis.
func WithModelID(modelID string) Option {
	return func(s *Synthesizer) {
		if modelID != "" {
			s.modelID = modelID
		}
	}
}

// WithFormat sets the output audio format.
func WithFormat(format string) Option {
	return func(s *Synthesizer) {
		if format != "" {
			s.format = format
		}
	}
}

// WithStability sets the voice stability (0-1).
func WithStability(stability float64) Option {
	return func(s *Synthesizer) {
		if stability >= 0 && stability <= 1 {
			s.stability = stability
		}
	}
}

// WithSimilarityBoost sets the voice similarity boost (0-1).
func WithSimilarityBoost(boost float64) Option {
	return func(s *Synthesizer) {
		if boost >= 0 && boost <= 1 {
			s.similarityBoost = boost
		}
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(s *Synthesizer) {
		if client != nil {
			s.client = client
		}
	}
}

// NewSynthesizer creates an ElevenLabs synthesizer with the given options.
func NewSynthesizer(opts ...Option) (*Synthesizer, error) {
	s := &Synthesizer{
		client:  http.DefaultClient,
		baseURL: defaultBaseURL,
		modelID: defaultModelID,
		format:  "mp3_44100_128",
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.apiKey == "" {
		return nil, ErrAPIKeyRequired
	}
	if s.voiceID == "" {
		return nil, ErrVoiceIDRequired
	}

	return s, nil
}

type ttsRequest struct {
	Text          string         `json:"text"`
	ModelID       string         `json:"model_id,omitempty"`
	VoiceSettings *voiceSettings `json:"voice_settings,omitempty"`
}

type voiceSettings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
}

type timestampedResponse struct {
	AudioBase64         string     `json:"audio_base64"`
	Alignment           *alignment `json:"alignment"`
	NormalizedAlignment *alignment `json:"normalized_alignment"`
}

type alignment struct {
	CharStartTimesMs []int    `json:"charStartTimesMs"`
	CharDurationsMs  []int    `json:"charDurationsMs"`
	Chars            []string `json:"chars"`
}

// Synthesize generates audio from text.
func (s *Synthesizer) Synthesize(ctx context.Context, text string, opts ...voice.Option) (*voice.Audio, error) {
	options := s.buildOptions(opts)

	reqBody := &ttsRequest{
		Text:    text,
		ModelID: options.Model,
	}
	if s.stability > 0 || s.similarityBoost > 0 {
		reqBody.VoiceSettings = &voiceSettings{
			Stability:       s.stability,
			SimilarityBoost: s.similarityBoost,
		}
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/text-to-speech/%s", s.baseURL, s.voiceID)
	if s.format != "" {
		url = fmt.Sprintf("%s?output_format=%s", url, s.format)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Xi-Api-Key", s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, s.parseError(resp)
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: failed to read response: %w", err)
	}

	return &voice.Audio{
		Data:   audioData,
		Format: s.format,
	}, nil
}

// Stream returns a streaming audio response as an io.ReadCloser.
func (s *Synthesizer) Stream(ctx context.Context, text string, opts ...voice.Option) (io.ReadCloser, error) {
	options := s.buildOptions(opts)

	reqBody := &ttsRequest{
		Text:    text,
		ModelID: options.Model,
	}
	if s.stability > 0 || s.similarityBoost > 0 {
		reqBody.VoiceSettings = &voiceSettings{
			Stability:       s.stability,
			SimilarityBoost: s.similarityBoost,
		}
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/text-to-speech/%s/stream", s.baseURL, s.voiceID)
	if s.format != "" {
		url = fmt.Sprintf("%s?output_format=%s", url, s.format)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Xi-Api-Key", s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, s.parseError(resp)
	}

	return resp.Body, nil
}

// SynthesizeCaptioned generates audio with word-level timestamps.
func (s *Synthesizer) SynthesizeCaptioned(ctx context.Context, text string, opts ...voice.Option) (*voice.CaptionedAudio, error) {
	options := s.buildOptions(opts)

	reqBody := &ttsRequest{
		Text:    text,
		ModelID: options.Model,
	}
	if s.stability > 0 || s.similarityBoost > 0 {
		reqBody.VoiceSettings = &voiceSettings{
			Stability:       s.stability,
			SimilarityBoost: s.similarityBoost,
		}
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: failed to marshal request: %w", err)
	}

	outputFormat := s.format
	if options.Format != "" {
		outputFormat = options.Format
	}

	url := fmt.Sprintf("%s/v1/text-to-speech/%s/stream/with-timestamps", s.baseURL, s.voiceID)
	if outputFormat != "" {
		url = fmt.Sprintf("%s?output_format=%s", url, outputFormat)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Xi-Api-Key", s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, s.parseError(resp)
	}

	var audioData []byte
	var alignments []*alignment

	decoder := json.NewDecoder(resp.Body)
	for {
		var chunk timestampedResponse
		if err := decoder.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("elevenlabs: failed to decode response: %w", err)
		}

		if chunk.AudioBase64 != "" {
			decoded, err := base64.StdEncoding.DecodeString(chunk.AudioBase64)
			if err != nil {
				return nil, fmt.Errorf("elevenlabs: failed to decode audio: %w", err)
			}
			audioData = append(audioData, decoded...)
		}

		if chunk.NormalizedAlignment != nil {
			alignments = append(alignments, chunk.NormalizedAlignment)
		} else if chunk.Alignment != nil {
			alignments = append(alignments, chunk.Alignment)
		}
	}

	timestamps := convertAlignmentsToWords(alignments, text)

	var durationMs int
	if len(timestamps) > 0 {
		durationMs = timestamps[len(timestamps)-1].EndMs
	}

	return &voice.CaptionedAudio{
		Data:       audioData,
		Format:     outputFormat,
		Timestamps: timestamps,
		DurationMs: durationMs,
	}, nil
}

// StreamCaptioned is not implemented; use SynthesizeCaptioned instead.
func (s *Synthesizer) StreamCaptioned(ctx context.Context, text string, opts ...voice.Option) (io.ReadCloser, error) {
	return nil, errors.New("elevenlabs: StreamCaptioned not implemented; use SynthesizeCaptioned")
}

func (s *Synthesizer) buildOptions(opts []voice.Option) *voice.SynthesizeOptions {
	options := &voice.SynthesizeOptions{
		Model:  s.modelID,
		Format: s.format,
	}
	for _, opt := range opts {
		opt(options)
	}
	return options
}

func (s *Synthesizer) parseError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("elevenlabs: request failed with status %d", resp.StatusCode)
	}

	var errResp struct {
		Detail struct {
			Message string `json:"message"`
		} `json:"detail"`
		Status string `json:"status"`
	}

	if err := json.Unmarshal(body, &errResp); err == nil {
		if errResp.Detail.Message != "" {
			return fmt.Errorf("elevenlabs: %s (status %d)", errResp.Detail.Message, resp.StatusCode)
		}
		if errResp.Status != "" {
			return fmt.Errorf("elevenlabs: %s (status %d)", errResp.Status, resp.StatusCode)
		}
	}

	return fmt.Errorf("elevenlabs: request failed with status %d: %s", resp.StatusCode, string(body))
}

func convertAlignmentsToWords(alignments []*alignment, originalText string) []voice.WordTimestamp {
	if len(alignments) == 0 {
		return nil
	}

	var timestamps []voice.WordTimestamp
	var currentWord strings.Builder
	wordStartMs := 0
	inWord := false

	textIdx := 0
	for _, align := range alignments {
		if align == nil || len(align.Chars) == 0 {
			continue
		}

		for i, char := range align.Chars {
			if i >= len(align.CharStartTimesMs) || i >= len(align.CharDurationsMs) {
				break
			}

			startMs := align.CharStartTimesMs[i]
			_ = startMs + align.CharDurationsMs[i] // endMs not needed for word boundary detection

			if isWordCharacter(char) {
				if !inWord {
					wordStartMs = startMs
					inWord = true
				}
				currentWord.WriteString(char)
			} else if inWord && currentWord.Len() > 0 {
				word := currentWord.String()
				timestamps = append(timestamps, voice.WordTimestamp{
					Word:    word,
					StartMs: wordStartMs,
					EndMs:   startMs,
				})

				currentWord.Reset()
				inWord = false
			}
			textIdx++
		}
	}

	if inWord && currentWord.Len() > 0 {
		lastAlign := alignments[len(alignments)-1]
		if lastAlign != nil && len(lastAlign.CharStartTimesMs) > 0 && len(lastAlign.CharDurationsMs) > 0 {
			lastIdx := len(lastAlign.CharStartTimesMs) - 1
			endMs := lastAlign.CharStartTimesMs[lastIdx] + lastAlign.CharDurationsMs[lastIdx]
			timestamps = append(timestamps, voice.WordTimestamp{
				Word:    currentWord.String(),
				StartMs: wordStartMs,
				EndMs:   endMs,
			})
		}
	}

	return timestamps
}

func isWordCharacter(char string) bool {
	if len(char) != 1 {
		return false
	}
	c := char[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '\'' || c == '-'
}
