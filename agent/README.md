# GoFrame Native Agent Harness

The `agent` package provides a highly decoupled, native framework for orchestrating LLMs using a **Think-Act-Observe loop**. It is built for absolute safety, modular extensibility, and transparency, stripping away unnecessary external SDKs and focusing purely on robust agent execution primitives.

## Overview

This package enables programmatic construction of autonomous AI agents by wiring together:

- **Core Agent Loop** - A native LLM reasoning cycle tracking token usage, contexts, and looping execution dynamically.
- **Action Middlewares** - A functional pipeline intercepting tool calls to inject Risk Assessment, Security Verifications, and specific actions gracefully.
- **Agent Governance** - Integrity checks that strictly define what tools and parameters fall within safety constraints before execution.
- **Telemetry Hooks** - An `AgentObserver` providing real-time visibility into iteration stages, execution times, and LLM thoughts without polluting internal loops.

---

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	
	"github.com/sevigo/goframe/agent"
	"github.com/sevigo/goframe/llms/ollama"
)

func main() {
	ctx := context.Background()

	// 1. Initialize an LLM model
	model := ollama.NewModel("qwen2.5-coder")

	// 2. Build a Tool Registry
	registry := agent.NewRegistry()
	registry.Register(myFileReadTool) // A tool matching llms.Tool interface

	// 3. Build the Native Agent Loop
	ag, err := agent.NewAgentLoop(model, registry,
		agent.WithLoopSystemPrompt("You are an autonomous engineering agent."),
		agent.WithLoopMaxIterations(10),
	)
	if err != nil {
		panic(err)
	}

	// 4. Provide Initial Task
	task := agent.Task{
		Goal: "Read main.go and summarize its purpose.",
	}

	// 5. Run the Autonomous Loop
	// The loop will Think->Act->Observe until the LLM returns a final string answer.
	result, err := ag.Run(ctx, task, nil)
	if err != nil {
		fmt.Printf("Execution failed: %v\n", err)
	} else {
		fmt.Printf("Final Response: %s\n", result.Response)
		fmt.Printf("Total Iterations: %d\n", result.Iterations)
		fmt.Printf("Token Input: %.0f, Output: %.0f\n", result.Tokens.Input, result.Tokens.Output)
	}
}
```

---

## Middleware Pipeline

Middlewares (`ActionMiddleware`) wrap native tool execution. They do not duplicate the loop; instead, they cleanly intercept `Registry.Execute(...)`.

### 1. Risk Assessment & Human-In-The-Loop
A middleware to detect risky agent actions (like `rm -rf`, `format_disk`, or HTTP requests) and block until an explicit human signal over channel/webhook is provided.

```go
// Create a default risk assessor that tags tools to Low/Medium/High Risk natively:
assessor := agent.NewDefaultRiskAssessor()
assessor.AddHighRiskTool("delete_file")

// Create an approval handler (UI or terminal based):
approvalHandler := myTerminalPromptHandler

// Wrap tool execution in Risk Assessment:
riskMiddleware := agent.RiskAssessmentMiddleware(
	assessor, 
	approvalHandler, 
	agent.RiskHigh,      // Only ask for approval if RiskLevel >= High
	60 * time.Second,   // Approval timeout
)

// Attach to the Agent
ag, _ := agent.NewAgentLoop(model, registry, 
	agent.WithLoopMiddleware(riskMiddleware),
)
```

### 2. Action Verification (Self-Healing)
Sometimes agents mistakenly believe an action succeeded (e.g., clicking a button on a web page). The Action Verification middleware takes the actual result, feeds it through an objective validation (`ActionVerifier`), and injects failure context back into the conversation for the LLM to self-correct during its next `think()` phase.

```go
// Define how verifications occur
verifier := myCustomActionVerifier

// Any verified failure throws a structured ErrActionFailedVerification.
// The Base Loop catches this and feeds "Suggested Correction: ..." back to the LLM.
verifyMiddleware := agent.ActionVerificationMiddleware(verifier)

ag, _ := agent.NewAgentLoop(model, registry, 
	agent.WithLoopMiddleware(verifyMiddleware),
)
```

---

## Telemetry & Observability

To bridge metrics directly to OpenTelemetry, Datadog, Prometheus, or simple logging files, implement the `AgentObserver` interface.

```go
type MyTelemetry struct{}

func (t *MyTelemetry) OnIterationStart(ctx context.Context, iteration int) {
	fmt.Printf("====== STARTING ITERATION %d ======\n", iteration)
}

func (t *MyTelemetry) OnThinkComplete(ctx context.Context, response string, toolCalls []llms.ToolCall, tokens agent.TokenUsage, err error) {
	fmt.Printf("Thinking burned %.0f input tokens.\n", tokens.Input)
}

func (t *MyTelemetry) OnToolCall(ctx context.Context, toolName string, params map[string]any) { }

func (t *MyTelemetry) OnToolResult(ctx context.Context, toolName string, params map[string]any, result any, duration time.Duration, err error) {
	fmt.Printf("Tool %s executed in %v\n", toolName, duration)
}

func (t *MyTelemetry) OnLoopComplete(ctx context.Context, result *agent.LoopResult, err error) { }

// Inject into the Agent
ag, _ := agent.NewAgentLoop(model, registry, 
	agent.WithLoopObserver(&MyTelemetry{}),
)
```

---

## Agent Governance

While middlewares handle tool-invocation dynamic interception, **Governance** enforces strict system boundaries. `Governance` asserts invariants irrespective of the specific tools:

```go
authCheck := &myRateLimitCheck{}

governance := agent.NewGovernance([]agent.RuleCheck{authCheck})

ag, _ := agent.NewAgentLoop(model, registry, 
	agent.WithLoopGovernance(governance),
)
```

---

## Memory Compression (Compaction)

As the agent runs over several iterations, `compactionHook` avoids LLM context boundaries by trimming historical inputs down automatically without dumping the initial directives:

```go
ag, _ := agent.NewAgentLoop(model, registry, 
	agent.WithLoopCompactionHook(func(ctx context.Context, msgs []schema.MessageContent, t agent.TokenUsage) []schema.MessageContent {
		if t.Input > 60000 {
			// e.g. Call another LLM just to summarize history and condense!
			return summarizedMessages
		}
		return msgs
	}),
)
```