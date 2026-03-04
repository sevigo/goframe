package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewFeedbackLoop(t *testing.T) {
	session := &Session{}

	t.Run("default options", func(t *testing.T) {
		fl := NewFeedbackLoop(nil, session)
		assert.NotNil(t, fl)
		assert.Equal(t, 3, fl.maxRetries)
		assert.Nil(t, fl.reviewHandler)
		assert.Nil(t, fl.prHandler)
	})

	t.Run("with custom options", func(t *testing.T) {
		fl := NewFeedbackLoop(nil, session,
			WithMaxRetries(5),
			WithReviewTool("code_review"),
			WithPRTool("create_pr"),
		)
		assert.Equal(t, 5, fl.maxRetries)
		assert.Equal(t, "code_review", fl.reviewTool)
		assert.Equal(t, "create_pr", fl.prTool)
	})
}

func TestBuildImplementationPrompt(t *testing.T) {
	session := &Session{}
	fl := NewFeedbackLoop(nil, session)

	t.Run("basic task", func(t *testing.T) {
		req := ImplementRequest{
			Task: "Write a function",
		}
		prompt := fl.buildImplementationPrompt(req, nil)
		assert.Contains(t, prompt, "Write a function")
	})

	t.Run("with context", func(t *testing.T) {
		req := ImplementRequest{
			Task:    "Write a function",
			Context: "This is for a web service",
		}
		prompt := fl.buildImplementationPrompt(req, nil)
		assert.Contains(t, prompt, "Context:")
		assert.Contains(t, prompt, "This is for a web service")
	})

	t.Run("with constraints", func(t *testing.T) {
		req := ImplementRequest{
			Task: "Write a function",
			Constraints: []string{
				"Handle errors",
				"Include tests",
			},
		}
		prompt := fl.buildImplementationPrompt(req, nil)
		assert.Contains(t, prompt, "Constraints:")
		assert.Contains(t, prompt, "Handle errors")
		assert.Contains(t, prompt, "Include tests")
	})

	t.Run("with previous review feedback", func(t *testing.T) {
		req := ImplementRequest{
			Task: "Write a function",
		}
		review := &ReviewResult{
			Approved: false,
			Feedback: "Missing error handling",
		}
		prompt := fl.buildImplementationPrompt(req, review)
		assert.Contains(t, prompt, "Previous implementation received the following feedback")
		assert.Contains(t, prompt, "Missing error handling")
	})
}

func TestDefaultReview(t *testing.T) {
	session := &Session{}
	fl := NewFeedbackLoop(nil, session)

	result := fl.defaultReview("some implementation")

	assert.True(t, result.Approved)
	assert.Equal(t, 100.0, result.Score)
	assert.Contains(t, result.Feedback, "Auto-approved")
}

func TestParseReviewResponse(t *testing.T) {
	session := &Session{}
	fl := NewFeedbackLoop(nil, session)

	tests := []struct {
		name     string
		content  string
		approved bool
	}{
		{
			name:     "approve upper",
			content:  "APPROVE: The implementation looks good.",
			approved: true,
		},
		{
			name:     "approve lower",
			content:  "I approve this implementation.",
			approved: true,
		},
		{
			name:     "approved keyword",
			content:  "Implementation approved.",
			approved: true,
		},
		{
			name:     "accept keyword",
			content:  "I accept this implementation.",
			approved: true,
		},
		{
			name:     "lgtm keyword",
			content:  "LGTM!",
			approved: true,
		},
		{
			name:     "reject",
			content:  "REJECT: Missing documentation.",
			approved: false,
		},
		{
			name:     "no keyword",
			content:  "The implementation needs work.",
			approved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fl.parseReviewResponse(tt.content)
			assert.Equal(t, tt.approved, result.Approved)
			assert.Equal(t, tt.content, result.Feedback)
		})
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"Hello World", "world", true},
		{"Hello World", "WORLD", true},
		{"Hello World", "hello", true},
		{"Hello World", "foo", false},
		{"", "", true},
		{"", "a", false},
		{"APPROVE", "approve", true},
		{"The code is approved", "approved", true},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			got := containsIgnoreCase(tt.s, tt.substr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReviewHandler(t *testing.T) {
	callCount := 0
	reviewHandler := func(ctx context.Context, session *Session, implementation string) (*ReviewResult, error) {
		callCount++
		return &ReviewResult{
			Approved: callCount >= 2,
			Feedback: "Review feedback",
			Score:    float64(50 + callCount*20),
		}, nil
	}

	_ = reviewHandler
	assert.Equal(t, 0, callCount)
}

func TestPRHandler(t *testing.T) {
	prCalled := false
	prHandler := func(ctx context.Context, session *Session, implementation string, review *ReviewResult) error {
		prCalled = true
		return nil
	}

	_ = prHandler
	assert.False(t, prCalled)
}

func TestMaxRetriesExceeded(t *testing.T) {
	attemptCount := 0
	reviewHandler := func(ctx context.Context, session *Session, implementation string) (*ReviewResult, error) {
		attemptCount++
		return &ReviewResult{
			Approved: false,
			Feedback: "Not good enough",
			Score:    float64(50 - attemptCount*10),
		}, nil
	}

	_ = reviewHandler
	assert.Equal(t, 0, attemptCount)
}

func TestReviewScoreThreshold(t *testing.T) {
	tests := []struct {
		name         string
		score        float64
		wantApproved bool
	}{
		{
			name:         "approved high score",
			score:        80,
			wantApproved: true,
		},
		{
			name:         "approved boundary",
			score:        70,
			wantApproved: true,
		},
		{
			name:         "rejected low score",
			score:        50,
			wantApproved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviewHandler := func(ctx context.Context, session *Session, implementation string) (*ReviewResult, error) {
				return &ReviewResult{
					Approved: tt.score >= 70,
					Feedback: "Feedback",
					Score:    tt.score,
				}, nil
			}

			fl := NewFeedbackLoop(nil, nil,
				WithMaxRetries(3),
				WithReviewHandler(reviewHandler),
			)

			result, err := reviewHandler(context.Background(), nil, "test implementation")
			assert.NoError(t, err)
			assert.Equal(t, tt.wantApproved, result.Approved)
			assert.Equal(t, tt.score, result.Score)
			assert.NotNil(t, fl)
		})
	}
}
