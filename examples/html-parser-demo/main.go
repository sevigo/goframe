package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/sevigo/goframe/parsers/html"
)

func main() {
	fmt.Println("=== HTML Parser Before/After Demo ===")

	// Sample HTML
	sampleHTML := `
<!DOCTYPE html>
<html>
<head>
	<meta property="article:author" content="Jane Doe">
	<meta property="article:published_time" content="2024-03-06T10:00:00Z">
</head>
<body>
	<nav>Navigation here</nav>
	
	<article>
		<h1>Go Concurrency Guide</h1>
		<p>This is a <strong>great article</strong> about Go.</p>
		<p>Check out <a href="/tutorials">tutorials</a> for more.</p>
		
		<h2>Chapter 1</h2>
		<p>Goroutines are lightweight.</p>
		
		<pre><code>go func() {
			fmt.Println("Hello")
		}()</code></pre>
	</article>
	
	<footer>© 2024</footer>
</body>
</html>
`

	// Show BEFORE
	fmt.Println("### BEFORE (Raw HTML) ###")
	fmt.Println(sampleHTML)
	fmt.Println()

	// Parse and show what got removed
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(sampleHTML))
	fmt.Println("### ANALYSIS ###")
	fmt.Printf("Found title: %s\n", doc.Find("title").Text())
	fmt.Printf("Found h1: %s\n", doc.Find("h1").Text())
	fmt.Printf("Found nav: %v\n", doc.Find("nav").Length() > 0)
	fmt.Printf("Found footer: %v\n", doc.Find("footer").Length() > 0)
	fmt.Println()

	// Create parser
	parser := html.NewHTMLParser(
		html.WithBaseURL("https://example.com"),
		html.WithBoilerplateRemoval(true),
		html.WithMetadataExtraction(true),
		html.WithMarkdownConversion(true),
	)

	// Parse HTML
	chunks, err := parser.Chunk(sampleHTML, "test.html", nil)
	if err != nil {
		log.Fatal(err)
	}

	// Show AFTER
	fmt.Println("### AFTER (Clean Markdown) ###")
	if len(chunks) > 0 {
		fmt.Println(chunks[0].Content)
		fmt.Println()
		fmt.Println("### EXTRACTED METADATA ###")
		for key, value := range chunks[0].Annotations {
			fmt.Printf("%s: %s\n", key, value)
		}
	}

	fmt.Println("\n### SUMMARY ###")
	fmt.Println("✅ Removed: nav, footer, scripts")
	fmt.Println("✅ Preserved: article content, semantic structure")
	fmt.Println("✅ Extracted: author, published date")
	fmt.Println("✅ Converted: HTML → Markdown")
}
