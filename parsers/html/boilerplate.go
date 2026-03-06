package html

import "github.com/PuerkitoBio/goquery"

// Boilerplate selectors to remove for LLM-optimized content.
// Based on common patterns that add noise but not semantic value.
var boilerplateSelectors = []string{
	// Navigation elements
	"nav", "header", "footer",

	// Scripts and styles
	"script", "style", "noscript",

	// Common noise elements
	"aside", ".sidebar", ".advertisement", ".ad", ".ads",
	".navigation", ".menu", ".breadcrumb",
	".social-share", ".share-buttons", ".social",
	".comments", "#comments", ".comment-section",
	".related-posts", ".recommended", ".suggestions",
	".newsletter", ".subscribe",

	// Tracking pixels
	"img[src*='pixel']", "img[src*='track']", "img[src*='beacon']",

	// Common ad class patterns
	"[class*='ad-']", "[id*='ad-']",
	"[class*='banner']", "[id*='banner']",
}

// removeBoilerplate removes non-content elements from the HTML document.
// This eliminates noise that degrades LLM performance and embedding quality.
//
// Elements removed:
//   - Navigation (nav, header, footer)
//   - Scripts and styles (script, style, noscript)
//   - Sidebars and ads (aside, .sidebar, .advertisement)
//   - Social sharing (.social-share, .share-buttons)
//   - Comments (.comments, #comments)
//   - Tracking pixels (img[src*='pixel'])
func (p *HTMLParser) removeBoilerplate(doc *goquery.Document) {
	if !p.boilerplateRemoval {
		return
	}

	// Remove standard boilerplate elements
	for _, selector := range boilerplateSelectors {
		doc.Find(selector).Remove()
	}

	// Remove elements with common noise patterns in class/ID
	noisePatterns := []string{
		"sidebar", "footer", "nav-", "menu",
		"advertisement", "banner", "promo",
		"social", "share", "comment",
		"related", "recommended",
	}

	for _, pattern := range noisePatterns {
		doc.Find("[class*='" + pattern + "'], [id*='" + pattern + "']").Remove()
	}

	// Remove empty elements after cleaning
	doc.Find("*:not(html):not(body):not(script):not(style)").Each(func(i int, s *goquery.Selection) {
		if s.Children().Length() == 0 && s.Text() == "" {
			s.Remove()
		}
	})
}
