package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrReviewRejected  = errors.New("review rejected")
	ErrMaxRetries      = errors.New("max retries exceeded")
	ErrToolNotFound    = errors.New("tool not found")
	ErrReviewFailed    = errors.New("review failed")
	ErrExecutionFailed = errors.New("execution failed")
)

type ReviewResult struct {
	Approved bool
	Feedback string
	Score    float64
	Details  map[string]interface{}
}

type ExecutionResult struct {
	Success      bool
	Output       string
	FilesChanged []string
	Error        error
}

type FeedbackLoop struct {
	agent         *Agent
	session       *Session
	reviewTool    string
	prTool        string
	maxRetries    int
	reviewHandler ReviewHandler
	prHandler     PRHandler
}

type ReviewHandler func(ctx context.Context, session *Session, implementation string) (*ReviewResult, error)
type PRHandler func(ctx context.Context, session *Session, implementation string, review *ReviewResult) error

type FeedbackLoopOption func(*FeedbackLoop)

func WithReviewTool(tool string) FeedbackLoopOption {
	return func(fl *FeedbackLoop) {
		fl.reviewTool = tool
	}
}

func WithPRTool(tool string) FeedbackLoopOption {
	return func(fl *FeedbackLoop) {
		fl.prTool = tool
	}
}

func WithMaxRetries(retries int) FeedbackLoopOption {
	return func(fl *FeedbackLoop) {
		fl.maxRetries = retries
	}
}

func WithReviewHandler(handler ReviewHandler) FeedbackLoopOption {
	return func(fl *FeedbackLoop) {
		fl.reviewHandler = handler
	}
}

func WithPRHandler(handler PRHandler) FeedbackLoopOption {
	return func(fl *FeedbackLoop) {
		fl.prHandler = handler
	}
}

func NewFeedbackLoop(agent *Agent, session *Session, opts ...FeedbackLoopOption) *FeedbackLoop {
	fl := &FeedbackLoop{
		agent:      agent,
		session:    session,
		maxRetries: 3,
	}
	for _, opt := range opts {
		opt(fl)
	}
	return fl
}

type ImplementRequest struct {
	Task        string
	Context     string
	Files       []string
	Constraints []string
}

type ImplementResult struct {
	Implementation string
	Response       *Response
	FilesCreated   []string
	FilesModified  []string
}

func (fl *FeedbackLoop) ImplementWithReview(ctx context.Context, req ImplementRequest) (*ImplementResult, error) {
	var lastResult *ImplementResult
	var lastReview *ReviewResult

	for range fl.maxRetries {
		implementPrompt := fl.buildImplementationPrompt(req, lastReview)

		response, err := fl.session.Prompt(ctx, implementPrompt)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrExecutionFailed, err)
		}

		lastResult = &ImplementResult{
			Implementation: response.Content,
			Response:       response,
		}

		switch {
		case fl.reviewHandler != nil:
			lastReview, err = fl.reviewHandler(ctx, fl.session, response.Content)
		case fl.reviewTool != "":
			lastReview, err = fl.runReviewTool(ctx, response.Content)
		default:
			lastReview = fl.defaultReview(response.Content)
		}

		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrReviewFailed, err)
		}

		if lastReview.Approved {
			switch {
			case fl.prHandler != nil:
				if err := fl.prHandler(ctx, fl.session, response.Content, lastReview); err != nil {
					return nil, fmt.Errorf("PR handler failed: %w", err)
				}
			case fl.prTool != "":
				if err := fl.runPRTool(ctx, response.Content, lastReview); err != nil {
					return nil, fmt.Errorf("PR tool failed: %w", err)
				}
			}
			return lastResult, nil
		}
	}

	return lastResult, fmt.Errorf("%w after %d attempts", ErrMaxRetries, fl.maxRetries)
}

func (fl *FeedbackLoop) buildImplementationPrompt(req ImplementRequest, lastReview *ReviewResult) string {
	var builder strings.Builder

	builder.WriteString(req.Task)

	if req.Context != "" {
		builder.WriteString("\n\nContext:\n")
		builder.WriteString(req.Context)
	}

	if len(req.Constraints) > 0 {
		builder.WriteString("\n\nConstraints:\n")
		for _, c := range req.Constraints {
			builder.WriteString("- ")
			builder.WriteString(c)
			builder.WriteString("\n")
		}
	}

	if lastReview != nil && !lastReview.Approved {
		builder.WriteString("\n\nPrevious implementation received the following feedback:\n")
		builder.WriteString(lastReview.Feedback)
		builder.WriteString("\n\nPlease address this feedback and improve the implementation.")
	}

	return builder.String()
}

func (fl *FeedbackLoop) runReviewTool(ctx context.Context, implementation string) (*ReviewResult, error) {
	reviewPrompt := fmt.Sprintf(
		"Review the following implementation for quality, correctness, and best practices.\n"+
			"Provide a score (0-100) and specific feedback.\n\n"+
			"Implementation:\n%s\n\n"+
			"Respond with APPROVE if the implementation is acceptable (score >= 70) or REJECT with feedback otherwise.",
		implementation,
	)

	response, err := fl.session.Prompt(ctx, reviewPrompt)
	if err != nil {
		return nil, err
	}

	return fl.parseReviewResponse(response.Content), nil
}

func (fl *FeedbackLoop) parseReviewResponse(content string) *ReviewResult {
	result := &ReviewResult{
		Approved: false,
		Feedback: content,
	}

	approvedKeywords := []string{"APPROVE", "approved", "approved.", "accept", "lgtm"}
	for _, keyword := range approvedKeywords {
		if containsIgnoreCase(content, keyword) {
			result.Approved = true
			break
		}
	}

	return result
}

func (fl *FeedbackLoop) runPRTool(ctx context.Context, implementation string, review *ReviewResult) error {
	prPrompt := fmt.Sprintf(
		"The implementation has been approved with a score of %.1f.\n"+
			"Please create a pull request with the following changes:\n\n%s",
		review.Score,
		implementation,
	)

	_, err := fl.session.Prompt(ctx, prPrompt)
	return err
}

func (fl *FeedbackLoop) defaultReview(implementation string) *ReviewResult {
	return &ReviewResult{
		Approved: true,
		Feedback: "Auto-approved (no review handler configured)",
		Score:    100,
	}
}

func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
