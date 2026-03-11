package browser

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"
)

// ElementInfo represents a simplified DOM element with position data.
type ElementInfo struct {
	Tag    string `json:"tag,omitempty"`
	ID     string `json:"id,omitempty"`
	TestID string `json:"testId,omitempty"`
	Aria   string `json:"aria,omitempty"`
	Class  string `json:"class,omitempty"`
	Text   string `json:"text,omitempty"`
	Type   string `json:"type,omitempty"`
	Name   string `json:"name,omitempty"`
	Href   string `json:"href,omitempty"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"w"`
	Height int    `json:"h"`
}

// Browser provides browser automation utilities.
type Browser struct {
	imageQuality int64
}

// Option configures the Browser.
type Option func(*Browser)

// New creates a new Browser instance.
func New(opts ...Option) *Browser {
	b := &Browser{
		imageQuality: 90,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// WithImageQuality sets the screenshot image quality (1-100).
func WithImageQuality(quality int64) Option {
	return func(b *Browser) {
		if quality > 0 && quality <= 100 {
			b.imageQuality = quality
		}
	}
}

// CaptureScreenshot captures a screenshot of the current page.
func (b *Browser) CaptureScreenshot(ctx context.Context) ([]byte, error) {
	var buf []byte
	if err := chromedp.Run(ctx,
		chromedp.CaptureScreenshot(&buf),
	); err != nil {
		return nil, fmt.Errorf("browser: failed to capture screenshot: %w", err)
	}
	return buf, nil
}

// GetSimplifiedDOM extracts interactive elements with their positions.
func (b *Browser) GetSimplifiedDOM(ctx context.Context) (string, error) {
	var result string
	script := `(() => {
		const interactive = ['button', 'a', 'input', 'select', 'textarea', '[onclick]', '[role="button"]', '[tabindex]'];
		const selector = interactive.join(',');
		const elements = document.querySelectorAll(selector);
		
		return Array.from(elements)
			.filter(el => {
				const rect = el.getBoundingClientRect();
				const style = getComputedStyle(el);
				return rect.width > 0 && 
					   rect.height > 0 &&
					   style.visibility !== 'hidden' &&
					   style.display !== 'none';
			})
			.map(el => {
				const rect = el.getBoundingClientRect();
				return {
					tag: el.tagName.toLowerCase(),
					id: el.id || undefined,
					testId: el.getAttribute('data-testid') || undefined,
					aria: el.getAttribute('aria-label') || undefined,
					class: (el.className && typeof el.className === 'string') ? 
						el.className.split(' ').filter(c => c).slice(0, 3).join('.') : undefined,
					text: (el.innerText || el.value || '').trim().slice(0, 60),
					type: el.type || undefined,
					name: el.name || undefined,
					href: el.href || undefined,
					x: Math.round(rect.x),
					y: Math.round(rect.y),
					w: Math.round(rect.width),
					h: Math.round(rect.height)
				};
			})
			.map(el => Object.fromEntries(
				Object.entries(el).filter(([_, v]) => v !== undefined && v !== '')
			));
	})()`

	if err := chromedp.Run(ctx,
		chromedp.Evaluate(script, &result),
	); err != nil {
		return "", fmt.Errorf("browser: failed to get DOM: %w", err)
	}

	var elements []ElementInfo
	if err := json.Unmarshal([]byte(result), &elements); err != nil {
		return "", fmt.Errorf("browser: failed to parse DOM: %w", err)
	}

	formatted, err := json.MarshalIndent(elements, "", "  ")
	if err != nil {
		return "", fmt.Errorf("browser: failed to format DOM: %w", err)
	}

	return string(formatted), nil
}

// GetPageState captures both screenshot and DOM in one call.
func (b *Browser) GetPageState(ctx context.Context) ([]byte, string, error) {
	screenshot, err := b.CaptureScreenshot(ctx)
	if err != nil {
		return nil, "", err
	}

	dom, err := b.GetSimplifiedDOM(ctx)
	if err != nil {
		return nil, "", err
	}

	return screenshot, dom, nil
}
