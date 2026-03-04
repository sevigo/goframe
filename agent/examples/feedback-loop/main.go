package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/sevigo/goframe/agent"
)

func main() {
	ctx := context.Background()

	ag, err := agent.New(
		agent.WithBaseURL(getEnv("OPENCODE_BASE_URL", "http://localhost:3000")),
		agent.WithModel(getEnv("OPENCODE_MODEL", "ollama/glm-5:cloud")),
		agent.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, nil))),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating agent: %v\n", err)
		os.Exit(1)
	}

	session, err := ag.NewSession(ctx, agent.WithTitle("Feedback Loop Demo"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating session: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ag.DeleteSession(ctx, session.ID) }()

	fmt.Println("=== Example 1: Basic Feedback Loop ===")
	exampleBasicFeedbackLoop(ctx, session)

	fmt.Println("\n=== Example 2: Custom Review Handler ===")
	exampleCustomReviewHandler(ctx, session)

	fmt.Println("\n=== Example 3: With MCP Tools ===")
	exampleWithMCPTools(ctx, ag, session)
}

func exampleBasicFeedbackLoop(ctx context.Context, session *agent.Session) {
	fl := agent.NewFeedbackLoop(nil, session,
		agent.WithMaxRetries(2),
	)

	req := agent.ImplementRequest{
		Task: "Write a simple Go function that calculates the factorial of a number",
		Constraints: []string{
			"Handle edge cases (negative numbers, zero)",
			"Include documentation",
			"Follow Go naming conventions",
		},
	}

	result, err := fl.ImplementWithReview(ctx, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Implementation completed after retries\n")
	fmt.Printf("Response length: %d characters\n", len(result.Implementation))
}

func exampleCustomReviewHandler(ctx context.Context, session *agent.Session) {
	reviewHandler := func(ctx context.Context, session *agent.Session, implementation string) (*agent.ReviewResult, error) {
		fmt.Println("Running custom review...")

		criteria := []string{
			"Error handling",
			"Code documentation",
			"Naming conventions",
			"Edge cases",
		}

		var issues []string
		score := 100.0

		if !strings.Contains(implementation, "error") && !strings.Contains(implementation, "Error") {
			issues = append(issues, "Missing error handling")
			score -= 20
		}

		if !strings.Contains(implementation, "//") && !strings.Contains(implementation, "/*") {
			issues = append(issues, "Missing documentation")
			score -= 15
		}

		if len(implementation) < 50 {
			issues = append(issues, "Implementation too short")
			score -= 10
		}

		var feedback strings.Builder
		fmt.Fprintf(&feedback, "Review Criteria: %v\n\n", criteria)
		fmt.Fprintf(&feedback, "Score: %.1f/100\n\n", score)

		if len(issues) > 0 {
			fmt.Fprintf(&feedback, "Issues found:\n")
			for _, issue := range issues {
				fmt.Fprintf(&feedback, "- %s\n", issue)
			}
			fmt.Fprintf(&feedback, "\nPlease address these issues.")
		} else {
			fmt.Fprintf(&feedback, "All criteria met. Approved!")
		}

		return &agent.ReviewResult{
			Approved: score >= 70,
			Feedback: feedback.String(),
			Score:    score,
		}, nil
	}

	prHandler := func(ctx context.Context, session *agent.Session, implementation string, review *agent.ReviewResult) error {
		fmt.Println("Creating pull request...")
		fmt.Printf("Score: %.1f/100\n", review.Score)
		fmt.Println("Implementation approved for PR!")
		return nil
	}

	fl := agent.NewFeedbackLoop(nil, session,
		agent.WithMaxRetries(3),
		agent.WithReviewHandler(reviewHandler),
		agent.WithPRHandler(prHandler),
	)

	req := agent.ImplementRequest{
		Task: "Write a Go function that reverses a string",
		Constraints: []string{
			"Handle empty strings",
			"Handle unicode characters",
			"Include tests",
		},
	}

	result, err := fl.ImplementWithReview(ctx, req)
	if err != nil {
		fmt.Printf("Result: %v\n", err)
		return
	}

	fmt.Printf("Final implementation score: %.1f\n", result.Response.Tokens.Output)
}

func exampleWithMCPTools(ctx context.Context, ag *agent.Agent, session *agent.Session) {
	mcpRegistry := agent.NewMCPRegistry(
		agent.LocalMCPServer("code-review",
			[]string{"mcp-code-review", "--strict"},
			agent.WithEnabled(true),
		),
		agent.LocalMCPServer("github-pr",
			[]string{"mcp-github", "--repo", "owner/repo"},
			agent.WithEnv(map[string]string{
				"GITHUB_TOKEN": os.Getenv("GITHUB_TOKEN"),
			}),
			agent.WithEnabled(true),
		),
	)

	fmt.Printf("MCP Servers configured: %d\n", mcpRegistry.Count())
	for _, server := range mcpRegistry.List() {
		fmt.Printf("  - %s (%s)\n", server.Name, server.Type)
	}

	reviewHandler := func(ctx context.Context, session *agent.Session, implementation string) (*agent.ReviewResult, error) {
		reviewPrompt := fmt.Sprintf(
			"Use the code_review tool to review this implementation:\n\n%s",
			implementation,
		)

		response, err := session.Prompt(ctx, reviewPrompt)
		if err != nil {
			return nil, err
		}

		return parseToolReviewResponse(response.Content), nil
	}

	prHandler := func(ctx context.Context, session *agent.Session, implementation string, review *agent.ReviewResult) error {
		if !review.Approved {
			return nil
		}

		prPrompt := "Use the create_pull_request tool to create a PR with the approved changes."
		_, err := session.Prompt(ctx, prPrompt)
		return err
	}

	fl := agent.NewFeedbackLoop(ag, session,
		agent.WithMaxRetries(3),
		agent.WithReviewHandler(reviewHandler),
		agent.WithPRHandler(prHandler),
	)

	req := agent.ImplementRequest{
		Task: "Add input validation to the user registration endpoint",
		Context: "We have an HTTP endpoint at POST /users/register\n" +
			"Request body: {email, password, name}",
		Constraints: []string{
			"Validate email format",
			"Password must be at least 8 characters",
			"Name must not be empty",
			"Return appropriate HTTP status codes",
			"Use proper logging",
		},
	}

	_, err := fl.ImplementWithReview(ctx, req)
	if err != nil {
		fmt.Printf("Feedback loop result: %v\n", err)
	}
}

func parseToolReviewResponse(content string) *agent.ReviewResult {
	approved := strings.Contains(strings.ToUpper(content), "APPROVE") ||
		strings.Contains(strings.ToUpper(content), "PASSED") ||
		strings.Contains(strings.ToUpper(content), "LGTM")

	var score float64 = 50
	if approved {
		score = 80
	}

	return &agent.ReviewResult{
		Approved: approved,
		Feedback: content,
		Score:    score,
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
