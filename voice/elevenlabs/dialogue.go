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

// DialogueInput represents a single speaker's text in a multi-turn dialogue.
type DialogueInput struct {
	Text    string `json:"text"`
	VoiceID string `json:"voice_id"`
}

// DialogueRequest is the request body for the dialogue API.
type DialogueRequest struct {
	Inputs    []DialogueInput `json:"inputs"`
	ModelID   string          `json:"model_id,omitempty"`
	Seed      int             `json:"seed,omitempty"`
	Stability float64         `json:"stability,omitempty"`
}

// DialogueResult contains the synthesized dialogue audio and metadata.
type DialogueResult struct {
	Audio      []byte
	Format     string
	Segments   []DialogueSegmentResult
	DurationMs int
	Subtitles  string
}

// DialogueSegmentResult contains per-segment metadata for a synthesized dialogue.
type DialogueSegmentResult struct {
	Speaker        string
	Text           string
	StartMs        int
	EndMs          int
	VoiceID        string
	WordTimestamps []voice.WordTimestamp
}

type dialogueResponse struct {
	AudioBase64         string              `json:"audio_base64"`
	Alignment           *characterAlignment `json:"alignment"`
	NormalizedAlignment *characterAlignment `json:"normalized_alignment"`
	VoiceSegments       []voiceSegment      `json:"voice_segments"`
}

type characterAlignment struct {
	Characters                 []string  `json:"characters"`
	CharacterStartTimesSeconds []float64 `json:"character_start_times_seconds"`
	CharacterEndTimesSeconds   []float64 `json:"character_end_times_seconds"`
}

type voiceSegment struct {
	VoiceID             string  `json:"voice_id"`
	StartTimeSeconds    float64 `json:"start_time_seconds"`
	EndTimeSeconds      float64 `json:"end_time_seconds"`
	CharacterStartIndex int     `json:"character_start_index"`
	CharacterEndIndex   int     `json:"character_end_index"`
	DialogueInputIndex  int     `json:"dialogue_input_index"`
}

var (
	// ErrNoInputs is returned when no dialogue inputs are provided.
	ErrNoInputs = errors.New("elevenlabs: dialogue requires at least one input")
)

// SynthesizeDialogue generates multi-speaker dialogue audio.
func (s *Synthesizer) SynthesizeDialogue(ctx context.Context, segments []voice.DialogueSegment) (*DialogueResult, error) {
	if len(segments) == 0 {
		return nil, ErrNoInputs
	}

	inputs := make([]DialogueInput, len(segments))
	for i, seg := range segments {
		voiceID, ok := s.mapSpeakerToVoiceID(seg.Speaker)
		if !ok {
			return nil, fmt.Errorf("elevenlabs: no voice mapping for speaker %q", seg.Speaker)
		}
		inputs[i] = DialogueInput{
			Text:    seg.Text,
			VoiceID: voiceID,
		}
	}

	return s.synthesizeDialogueInputs(ctx, inputs, segments)
}

// SynthesizeDialogueWithVoices generates dialogue using an explicit voice mapping.
func (s *Synthesizer) SynthesizeDialogueWithVoices(ctx context.Context, voiceMap map[string]string, segments []voice.DialogueSegment) (*DialogueResult, error) {
	if len(segments) == 0 {
		return nil, ErrNoInputs
	}

	inputs := make([]DialogueInput, len(segments))
	for i, seg := range segments {
		voiceID, ok := voiceMap[seg.Speaker]
		if !ok {
			return nil, fmt.Errorf("elevenlabs: no voice mapping for speaker %q", seg.Speaker)
		}
		inputs[i] = DialogueInput{
			Text:    seg.Text,
			VoiceID: voiceID,
		}
	}

	return s.synthesizeDialogueInputs(ctx, inputs, segments)
}

func (s *Synthesizer) synthesizeDialogueInputs(ctx context.Context, inputs []DialogueInput, segments []voice.DialogueSegment) (*DialogueResult, error) {
	reqBody := &DialogueRequest{
		Inputs:  inputs,
		ModelID: s.modelID,
	}
	if s.stability > 0 {
		reqBody.Stability = s.stability
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/text-to-dialogue/with-timestamps", s.baseURL)
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

	var dialogueResp dialogueResponse
	decoder := json.NewDecoder(resp.Body)
	if decodeErr := decoder.Decode(&dialogueResp); decodeErr != nil {
		return nil, fmt.Errorf("elevenlabs: failed to decode response: %w", decodeErr)
	}

	audioData, err := base64.StdEncoding.DecodeString(dialogueResp.AudioBase64)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: failed to decode audio: %w", err)
	}

	segmentResults := s.buildSegmentResults(dialogueResp, segments)

	var durationMs int
	if len(segmentResults) > 0 {
		durationMs = segmentResults[len(segmentResults)-1].EndMs
	}

	subtitles := s.generateSubtitles(segmentResults)

	return &DialogueResult{
		Audio:      audioData,
		Format:     s.format,
		Segments:   segmentResults,
		DurationMs: durationMs,
		Subtitles:  subtitles,
	}, nil
}

func (s *Synthesizer) mapSpeakerToVoiceID(speaker string) (string, bool) {
	return s.voiceID, s.voiceID != ""
}

func (s *Synthesizer) buildSegmentResults(resp dialogueResponse, segments []voice.DialogueSegment) []DialogueSegmentResult {
	if len(resp.VoiceSegments) == 0 {
		return nil
	}

	results := make([]DialogueSegmentResult, 0, len(resp.VoiceSegments))
	alignment := resp.NormalizedAlignment
	if alignment == nil {
		alignment = resp.Alignment
	}

	for _, seg := range resp.VoiceSegments {
		var text string
		if seg.DialogueInputIndex < len(segments) {
			text = segments[seg.DialogueInputIndex].Text
		}

		var wordTimestamps []voice.WordTimestamp
		if alignment != nil && seg.CharacterEndIndex > seg.CharacterStartIndex {
			wordTimestamps = extractWordTimestamps(
				alignment,
				seg.CharacterStartIndex,
				seg.CharacterEndIndex,
			)
		}

		results = append(results, DialogueSegmentResult{
			Speaker:        segments[seg.DialogueInputIndex].Speaker,
			Text:           text,
			StartMs:        int(seg.StartTimeSeconds * 1000),
			EndMs:          int(seg.EndTimeSeconds * 1000),
			VoiceID:        seg.VoiceID,
			WordTimestamps: wordTimestamps,
		})
	}

	return results
}

func extractWordTimestamps(alignment *characterAlignment, startIdx, endIdx int) []voice.WordTimestamp {
	if alignment == nil || startIdx >= len(alignment.Characters) {
		return nil
	}

	if endIdx > len(alignment.Characters) {
		endIdx = len(alignment.Characters)
	}

	var timestamps []voice.WordTimestamp
	var currentWord strings.Builder
	wordStartSec := 0.0
	inWord := false

	for i := startIdx; i < endIdx; i++ {
		char := alignment.Characters[i]
		startSec := alignment.CharacterStartTimesSeconds[i]
		endSec := alignment.CharacterEndTimesSeconds[i]

		if isWordChar(char) {
			if !inWord {
				wordStartSec = startSec
				inWord = true
			}
			currentWord.WriteString(char)
		} else if inWord && currentWord.Len() > 0 {
			timestamps = append(timestamps, voice.WordTimestamp{
				Word:    currentWord.String(),
				StartMs: int(wordStartSec * 1000),
				EndMs:   int(endSec * 1000),
			})
			currentWord.Reset()
			inWord = false
		}
	}

	if inWord && currentWord.Len() > 0 && endIdx > 0 {
		lastEndSec := alignment.CharacterEndTimesSeconds[endIdx-1]
		timestamps = append(timestamps, voice.WordTimestamp{
			Word:    currentWord.String(),
			StartMs: int(wordStartSec * 1000),
			EndMs:   int(lastEndSec * 1000),
		})
	}

	return timestamps
}

func isWordChar(char string) bool {
	if len(char) != 1 {
		return false
	}
	c := char[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '\'' || c == '-'
}

func (s *Synthesizer) generateSubtitles(segments []DialogueSegmentResult) string {
	if len(segments) == 0 {
		return ""
	}

	var sb strings.Builder
	entryNum := 1

	for _, seg := range segments {
		if len(seg.WordTimestamps) == 0 {
			fmt.Fprintf(&sb, "%d\n%02d:%02d:%02d,%03d --> %02d:%02d:%02d,%03d\n%s\n\n",
				entryNum,
				seg.StartMs/3600000, (seg.StartMs%3600000)/60000, (seg.StartMs%60000)/1000, seg.StartMs%1000,
				seg.EndMs/3600000, (seg.EndMs%3600000)/60000, (seg.EndMs%60000)/1000, seg.EndMs%1000,
				seg.Text)
			entryNum++
			continue
		}

		for _, word := range seg.WordTimestamps {
			fmt.Fprintf(&sb, "%d\n%02d:%02d:%02d,%03d --> %02d:%02d:%02d,%03d\n%s\n\n",
				entryNum,
				word.StartMs/3600000, (word.StartMs%3600000)/60000, (word.StartMs%60000)/1000, word.StartMs%1000,
				word.EndMs/3600000, (word.EndMs%3600000)/60000, (word.EndMs%60000)/1000, word.EndMs%1000,
				word.Word)
			entryNum++
		}
	}

	return sb.String()
}

// StreamDialogue is not implemented; use SynthesizeDialogue instead.
func (s *Synthesizer) StreamDialogue(ctx context.Context, segments []voice.DialogueSegment) (io.ReadCloser, error) {
	return nil, errors.New("elevenlabs: StreamDialogue not implemented; use SynthesizeDialogue")
}
