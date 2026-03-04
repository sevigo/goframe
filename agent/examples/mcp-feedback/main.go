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

	baseURL := getEnv("OPENCODE_BASE_URL", "http://localhost:3000")
	model := getEnv("OPENCODE_MODEL", "ollama/llama3")

	mcpRegistry := agent.NewMCPRegistry(
		agent.LocalMCPServer("code-review",
			[]string{"mcp-code-review", "--strict"},
			agent.WithEnabled(true),
		),
		agent.LocalMCPServer("github",
			[]string{"mcp-github"},
			agent.WithEnv(map[string]string{
				"GITHUB_TOKEN": os.Getenv("GITHUB_TOKEN"),
			}),
			agent.WithEnabled(true),
		),
	)

	ag, err := agent.New(
		agent.WithBaseURL(baseURL),
		agent.WithModel(model),
		agent.WithMCPRegistry(mcpRegistry),
		agent.WithWorkingDir("."),
		agent.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, nil))),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating agent: %v\n", err)
		os.Exit(1)
	}

	session, err := ag.NewSession(ctx, agent.WithTitle("MCP Feedback Loop Demo"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating session: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ag.DeleteSession(ctx, session.ID) }()

	fmt.Println("=== Example: MCP-Based Feedback Loop ===")
	exampleMCPFeedbackLoop(ctx, ag, session)

	fmt.Println("\n=== Example: Custom Review Criteria ===")
	exampleCustomReviewCriteria(ctx, ag, session)

	fmt.Println("\n=== Example: Multi-Stage Implementation ===")
	exampleMultiStageImplementation(ctx, ag, session)
}

func exampleMCPFeedbackLoop(ctx context.Context, ag *agent.Agent, session *agent.Session) {
	reviewHandler := func(ctx context.Context, session *agent.Session, implementation string) (*agent.ReviewResult, error) {
		fmt.Println("Running MCP code review...")

		reviewPrompt := fmt.Sprintf(
			"Use the code_review MCP tool to analyze this implementation:\n\n%s\n\n"+
				"Check for:\n"+
				"1. Code quality and best practices\n"+
				"2. Error handling\n"+
				"3. Security vulnerabilities\n"+
				"4. Test coverage\n\n"+
				"Return APPROVE if score >= 70, otherwise REJECT with specific feedback.",
			implementation,
		)

		response, err := session.Prompt(ctx, reviewPrompt)
		if err != nil {
			return nil, fmt.Errorf("review failed: %w", err)
		}

		return parseReviewResponse(response.Content), nil
	}

	prHandler := func(ctx context.Context, session *agent.Session, implementation string, review *agent.ReviewResult) error {
		if !review.Approved {
			fmt.Println("Review not approved, skipping PR creation")
			return nil
		}

		fmt.Println("Creating pull request via MCP...")

		prPrompt := fmt.Sprintf(
			"Use the github MCP tool to create a pull request with this title:\n"+
				"'Implement user input validation'\n\n"+
				"Branch: feature/input-validation\n"+
				"Base: main\n\n"+
				"Implementation:\n%s",
			implementation,
		)

		_, err := session.Prompt(ctx, prPrompt)
		if err != nil {
			return fmt.Errorf("PR creation failed: %w", err)
		}

		fmt.Println("Pull request created successfully!")
		return nil
	}

	fl := agent.NewFeedbackLoop(ag, session,
		agent.WithMaxRetries(3),
		agent.WithReviewHandler(reviewHandler),
		agent.WithPRHandler(prHandler),
	)

	req := agent.ImplementRequest{
		Task: "Add input validation to the user registration endpoint",
		Context: "We have a REST API endpoint at POST /api/users/register\n" +
			"Request body: {email: string, password: string, name: string}",
		Constraints: []string{
			"Validate email format using regex",
			"Password must be at least 8 characters",
			"Password must contain uppercase, lowercase, and number",
			"Name must not be empty and max 100 characters",
			"Return 400 Bad Request for invalid input",
			"Return 201 Created for success",
			"Include proper error messages in response",
		},
	}

	result, err := fl.ImplementWithReview(ctx, req)
	if err != nil {
		fmt.Printf("Feedback loop error: %v\n", err)
		return
	}

	fmt.Printf("\nImplementation completed!\n")
	fmt.Printf("Response length: %d characters\n", len(result.Implementation))
}

func exampleCustomReviewCriteria(ctx context.Context, ag *agent.Agent, session *agent.Session) {
	type ReviewCriterion struct {
		Name        string
		Description string
		Weight      float64
		Check       func(string) bool
	}

	criteria := []ReviewCriterion{
		{
			Name:        "Error Handling",
			Description: "Code handles all error cases",
			Weight:      0.3,
			Check: func(code string) bool {
				return strings.Contains(code, "if err") ||
					strings.Contains(code, "error") ||
					strings.Contains(code, "Error")
			},
		},
		{
			Name:        "Documentation",
			Description: "Code has proper comments/docs",
			Weight:      0.2,
			Check: func(code string) bool {
				return strings.Contains(code, "//") ||
					strings.Contains(code, "/*") ||
					strings.Contains(code, "*")
			},
		},
		{
			Name:        "Tests",
			Description: "Code includes tests",
			Weight:      0.3,
			Check: func(code string) bool {
				return strings.Contains(code, "func Test") ||
					strings.Contains(code, "describe(") ||
					strings.Contains(code, "it(")
			},
		},
		{
			Name:        "Validation",
			Description: "Input validation present",
			Weight:      0.2,
			Check: func(code string) bool {
				return strings.Contains(code, "validate") ||
					strings.Contains(code, "Validate") ||
					strings.Contains(code, "check")
			},
		},
	}

	reviewHandler := func(ctx context.Context, session *agent.Session, implementation string) (*agent.ReviewResult, error) {
		fmt.Println("Running custom review criteria check...")

		var score float64
		var feedback strings.Builder
		feedback.WriteString("Review Results:\n\n")

		for _, criterion := range criteria {
			passed := criterion.Check(implementation)
			contribution := 0.0
			if passed {
				contribution = criterion.Weight * 100
				feedback.WriteString(fmt.Sprintf("✓ %s: PASSED (%.0f%%)\n", criterion.Name, contribution*100/criterion.Weight))
			} else {
				feedback.WriteString(fmt.Sprintf("✗ %s: FAILED - %s\n", criterion.Name, criterion.Description))
			}
			score += contribution
		}

		feedback.WriteString(fmt.Sprintf("\nOverall Score: %.1f/100\n", score))

		approved := score >= 70
		if !approved {
			feedback.WriteString("\nSuggestions:\n")
			for _, criterion := range criteria {
				if !criterion.Check(implementation) {
					feedback.WriteString(fmt.Sprintf("- Add %s\n", criterion.Name))
				}
			}
		} else {
			feedback.WriteString("\nAPPROVE: Implementation meets minimum requirements.\n")
		}

		return &agent.ReviewResult{
			Approved: approved,
			Feedback: feedback.String(),
			Score:    score,
		}, nil
	}

	fl := agent.NewFeedbackLoop(ag, session,
		agent.WithMaxRetries(3),
		agent.WithReviewHandler(reviewHandler),
	)

	req := agent.ImplementRequest{
		Task: "Write a Go function that validates user credentials",
		Constraints: []string{
			"Return error for invalid inputs",
			"Include godoc comments",
			"Handle edge cases",
		},
	}

	result, err := fl.ImplementWithReview(ctx, req)
	if err != nil {
		fmt.Printf("Result: %v\n", err)
		return
	}

	fmt.Printf("Final score: %.1f\n", result.Response.Tokens.Output)
}

func exampleMultiStageImplementation(ctx context.Context, ag *agent.Agent, session *agent.Session) {
	stages := []struct {
		name        string
		task        string
		constraints []string
	}{
		{
			name: "Design",
			task: "Design the data structures for a todo list application",
			constraints: []string{
				"Include Todo struct with ID, Title, Completed, CreatedAt",
				"Consider JSON serialization",
				"Add validation methods",
			},
		},
		{
			name: "Implementation",
			task: "Implement CRUD operations for the todo list",
			constraints: []string{
				"Create, Read, Update, Delete operations",
				"Thread-safe storage",
				"Return appropriate errors",
			},
		},
		{
			name: "Testing",
			task: "Add comprehensive tests for the todo list",
			constraints: []string{
				"Test all CRUD operations",
				"Test edge cases",
				"Test concurrent access",
			},
		},
	}

	stageReviewHandler := func(stageName string) agent.ReviewHandler {
		return func(ctx context.Context, session *agent.Session, implementation string) (*agent.ReviewResult, error) {
			fmt.Printf("Reviewing stage: %s\n", stageName)

			reviewPrompt := fmt.Sprintf(
				"Review the implementation for stage '%s':\n\n%s\n\n"+
					"Check if it meets all stage requirements.\n"+
					"Return APPROVE if complete, REJECT with issues otherwise.",
				stageName,
				implementation,
			)

			response, err := session.Prompt(ctx, reviewPrompt)
			if err != nil {
				return nil, err
			}

			return parseReviewResponse(response.Content), nil
		}
	}

	for _, stage := range stages {
		fmt.Printf("\n--- Stage: %s ---\n", stage.name)

		fl := agent.NewFeedbackLoop(ag, session,
			agent.WithMaxRetries(2),
			agent.WithReviewHandler(stageReviewHandler(stage.name)),
		)

		req := agent.ImplementRequest{
			Task:        stage.task,
			Constraints: stage.constraints,
		}

		_, err := fl.ImplementWithReview(ctx, req)
		if err != nil {
			fmt.Printf("Stage %s failed: %v\n", stage.name, err)
			continue
		}

		fmt.Printf("Stage %s completed successfully!\n", stage.name)
	}
}

func parseReviewResponse(content string) *agent.ReviewResult {
	contentUpper := strings.ToUpper(content)
	approved := strings.Contains(contentUpper, "APPROVE") ||
		strings.Contains(contentUpper, "APPROVED") ||
		strings.Contains(contentUpper, "LGTM") ||
		strings.Contains(contentUpper, "PASSED")

	var score float64 = 50
	if approved {
		score = 80
		if strings.Contains(contentUpper, "EXCELLENT") ||
			strings.Contains(contentUpper, "PERFECT") {
			score = 95
		}
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
