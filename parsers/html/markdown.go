package html

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// toMarkdown converts an HTML document to Markdown format.
// Preserves semantic structure (headers, lists, code blocks, etc.)
// while removing non-essential formatting.
func (p *HTMLParser) toMarkdown(doc *goquery.Document) string {
	if !p.markdownConversion {
		// Return cleaned HTML if not converting
		html, _ := doc.Html()
		return html
	}

	var markdown strings.Builder

	// Find the main content area
	content := doc.Find("article, main, .content, #content, body").First()
	if content.Length() == 0 {
		content = doc.Find("body")
	}

	// Convert each node recursively
	content.Contents().Each(func(i int, s *goquery.Selection) {
		markdown.WriteString(p.nodeToMarkdown(s, 0))
	})

	return strings.TrimSpace(markdown.String())
}

// nodeToMarkdown recursively converts an HTML node to Markdown.
func (p *HTMLParser) nodeToMarkdown(s *goquery.Selection, depth int) string {
	if len(s.Nodes) == 0 {
		return ""
	}

	node := s.Nodes[0]

	// Handle text nodes (node.Data contains text for text nodes)
	if node.Type == 3 { // goquery.TextNode = 3
		return strings.TrimSpace(node.Data)
	}

	// Handle element nodes by tag name
	tagName := strings.ToLower(node.Data)

	switch tagName {
	case "h1":
		return p.headingToMarkdown(s, 1)
	case "h2":
		return p.headingToMarkdown(s, 2)
	case "h3":
		return p.headingToMarkdown(s, 3)
	case "h4":
		return p.headingToMarkdown(s, 4)
	case "h5":
		return p.headingToMarkdown(s, 5)
	case "h6":
		return p.headingToMarkdown(s, 6)
	case "p":
		return p.paragraphToMarkdown(s)
	case "ul", "ol":
		return p.listToMarkdown(s, tagName, depth)
	case "li":
		return p.listItemToMarkdown(s, depth)
	case "a":
		return p.linkToMarkdown(s)
	case "img":
		return p.imageToMarkdown(s)
	case "code":
		return p.codeToMarkdown(s)
	case "pre":
		return p.codeBlockToMarkdown(s)
	case "blockquote":
		return p.blockquoteToMarkdown(s, depth)
	case "strong", "b":
		return p.boldToMarkdown(s)
	case "em", "i":
		return p.italicToMarkdown(s)
	case "br":
		return "\n"
	case "hr":
		return "\n---\n\n"
	case "table":
		return p.tableToMarkdown(s)
	case "div", "section", "article":
		// Generic container - process children
		var result strings.Builder
		s.Contents().Each(func(i int, child *goquery.Selection) {
			result.WriteString(p.nodeToMarkdown(child, depth))
		})
		return result.String()
	default:
		// Unknown element - process children
		var result strings.Builder
		s.Contents().Each(func(i int, child *goquery.Selection) {
			result.WriteString(p.nodeToMarkdown(child, depth))
		})
		return result.String()
	}
}

func (p *HTMLParser) headingToMarkdown(s *goquery.Selection, level int) string {
	prefix := strings.Repeat("#", level) + " "
	text := strings.TrimSpace(s.Text())
	return fmt.Sprintf("\n%s%s\n\n", prefix, text)
}

func (p *HTMLParser) paragraphToMarkdown(s *goquery.Selection) string {
	var result strings.Builder
	s.Contents().Each(func(i int, child *goquery.Selection) {
		result.WriteString(p.nodeToMarkdown(child, 0))
	})
	text := strings.TrimSpace(result.String())
	if text == "" {
		return ""
	}
	return fmt.Sprintf("%s\n\n", text)
}

func (p *HTMLParser) listToMarkdown(s *goquery.Selection, listType string, depth int) string {
	var result strings.Builder
	indent := strings.Repeat("  ", depth)

	s.Find("> li").Each(func(i int, li *goquery.Selection) {
		text := strings.TrimSpace(li.Text())
		if listType == "ul" {
			result.WriteString(fmt.Sprintf("%s- %s\n", indent, text))
		} else {
			result.WriteString(fmt.Sprintf("%s%d. %s\n", indent, i+1, text))
		}
	})

	return result.String() + "\n"
}

func (p *HTMLParser) listItemToMarkdown(s *goquery.Selection, depth int) string {
	var result strings.Builder
	s.Contents().Each(func(i int, child *goquery.Selection) {
		result.WriteString(p.nodeToMarkdown(child, depth))
	})
	return result.String()
}

func (p *HTMLParser) linkToMarkdown(s *goquery.Selection) string {
	text := strings.TrimSpace(s.Text())
	href, _ := s.Attr("href")

	if href == "" || text == "" {
		return text
	}

	return fmt.Sprintf("[%s](%s)", text, href)
}

func (p *HTMLParser) imageToMarkdown(s *goquery.Selection) string {
	alt, _ := s.Attr("alt")
	src, _ := s.Attr("src")

	if src == "" {
		return ""
	}

	if alt == "" {
		return fmt.Sprintf("![](%s)", src)
	}

	return fmt.Sprintf("![%s](%s)", alt, src)
}

func (p *HTMLParser) codeToMarkdown(s *goquery.Selection) string {
	text := s.Text()
	if strings.Contains(text, "\n") {
		return fmt.Sprintf("\n```\n%s\n```\n", text)
	}
	return fmt.Sprintf("`%s`", text)
}

func (p *HTMLParser) codeBlockToMarkdown(s *goquery.Selection) string {
	// Try to find language from class
	language := ""
	if classes, exists := s.Attr("class"); exists {
		if strings.Contains(classes, "language-") {
			parts := strings.Split(classes, " ")
			for _, part := range parts {
				if strings.HasPrefix(part, "language-") {
					language = strings.TrimPrefix(part, "language-")
					break
				}
			}
		}
	}

	// Get code content
	code := s.Find("code").Text()
	if code == "" {
		code = s.Text()
	}

	return fmt.Sprintf("\n```%s\n%s\n```\n\n", language, code)
}

func (p *HTMLParser) blockquoteToMarkdown(s *goquery.Selection, depth int) string {
	var result strings.Builder
	lines := strings.Split(strings.TrimSpace(s.Text()), "\n")

	for _, line := range lines {
		if line != "" {
			result.WriteString(fmt.Sprintf("> %s\n", line))
		}
	}

	return result.String() + "\n"
}

func (p *HTMLParser) boldToMarkdown(s *goquery.Selection) string {
	text := strings.TrimSpace(s.Text())
	if text == "" {
		return ""
	}
	return fmt.Sprintf("**%s**", text)
}

func (p *HTMLParser) italicToMarkdown(s *goquery.Selection) string {
	text := strings.TrimSpace(s.Text())
	if text == "" {
		return ""
	}
	return fmt.Sprintf("*%s*", text)
}

func (p *HTMLParser) tableToMarkdown(s *goquery.Selection) string {
	var result strings.Builder

	// Header
	s.Find("thead tr th").Each(func(i int, th *goquery.Selection) {
		result.WriteString(fmt.Sprintf("| %s ", strings.TrimSpace(th.Text())))
	})
	result.WriteString("|\n")

	// Separator
	s.Find("thead tr th").Each(func(i int, th *goquery.Selection) {
		result.WriteString("| --- ")
	})
	result.WriteString("|\n")

	// Body
	s.Find("tbody tr").Each(func(i int, tr *goquery.Selection) {
		tr.Find("td").Each(func(j int, td *goquery.Selection) {
			result.WriteString(fmt.Sprintf("| %s ", strings.TrimSpace(td.Text())))
		})
		result.WriteString("|\n")
	})

	return result.String() + "\n"
}
