// Package browser provides browser automation utilities for UI element selection.
//
// This package combines visual information (screenshots) with structural data (DOM)
// to enable intelligent element selection using vision-capable LLMs.
//
// # Key Features
//
//   - Screenshot capture with configurable quality
//   - Simplified DOM extraction focusing on interactive elements
//   - Element position tracking for cross-referencing
//   - Integration with chromedp for browser control
//
// # Basic Usage
//
//	ctx, cancel := chromedp.NewContext(context.Background())
//	defer cancel()
//
//	browser := browser.New()
//	screenshot, _ := browser.CaptureScreenshot(ctx)
//	dom, _ := browser.GetSimplifiedDOM(ctx)
//
// # Element Selection Strategy
//
// The package extracts interactive elements (buttons, links, inputs) with their:
//   - Position (x, y, width, height)
//   - Unique identifiers (id, data-testid, aria-label)
//   - Visible text content
//   - CSS classes
//
// This data combined with screenshots enables LLMs to find reliable selectors.
package browser
