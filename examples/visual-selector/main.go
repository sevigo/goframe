//go:build ignore

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/sevigo/goframe/browser"
	"github.com/sevigo/goframe/llms/ollama"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Create chromedp context
	allocCtx, allocCancel := chromedp.NewContext(context.Background())
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	// Navigate to a page first
	if err := chromedp.Run(browserCtx,
		chromedp.Navigate("https://example.com"),
		chromedp.WaitReady("body"),
	); err != nil {
		slog.Error("Failed to navigate", "error", err)
		os.Exit(1)
	}

	// Create LLM client (use a vision-capable model)
	llm, err := ollama.New(ollama.WithModel("qwen3.5:cloud"))
	if err != nil {
		slog.Error("Failed to create LLM", "error", err)
		os.Exit(1)
	}

	// Create browser automation helpers
	br := browser.New()

	// Capture page state
	screenshot, dom, err := br.GetPageState(browserCtx)
	if err != nil {
		slog.Error("Failed to get page state", "error", err)
		os.Exit(1)
	}

	slog.Info("Captured page state", "screenshot_bytes", len(screenshot), "dom_length", len(dom))

	// Create visual selector
	selector := browser.NewVisualSelector(llm, br)

	// Find an element by description
	result, err := selector.FindElementWithState(
		ctx,
		"Find the main heading link",
		screenshot,
		dom,
	)
	if err != nil {
		slog.Error("Failed to find element", "error", err)
		os.Exit(1)
	}

	fmt.Println("\n=== Element Selector Result ===")
	fmt.Printf("Primary Selector:  %s\n", result.PrimarySelector)
	if result.FallbackSelector != "" {
		fmt.Printf("Fallback Selector: %s\n", result.FallbackSelector)
	}
	if result.Coordinates != nil {
		fmt.Printf("Coordinates:       x=%d, y=%d\n", result.Coordinates.X, result.Coordinates.Y)
	}
	fmt.Printf("Confidence:        %s\n", result.Confidence)
	fmt.Printf("Reason:            %s\n", result.Reason)

	// Save screenshot for reference
	if err := os.WriteFile("screenshot.png", screenshot, 0644); err != nil {
		slog.Warn("Failed to save screenshot", "error", err)
	} else {
		slog.Info("Saved screenshot to screenshot.png")
	}

	// Print DOM structure
	fmt.Println("\n=== DOM Structure ===")
	if len(dom) > 500 {
		fmt.Println(dom[:500])
		fmt.Println("... (truncated)")
	} else {
		fmt.Println(dom)
	}
}
