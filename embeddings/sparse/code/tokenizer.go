package code

import (
	"hash/fnv"
	"regexp"
	"strings"
)

const (
	// vocabularySize is the FNV32a hash space for sparse vector indices.
	// With ~100 unique tokens per chunk the collision probability is ~0.1%.
	vocabularySize = 50000
)

var (
	// camelCaseLower splits a lowercase-to-uppercase boundary: processPayment → process Payment
	camelCaseLower = regexp.MustCompile(`([a-z])([A-Z])`)
	// camelCaseUpper splits an acronym-to-word boundary: XMLParser → XML Parser, HTTPClient → HTTP Client
	camelCaseUpper = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
	// operatorRegex splits on punctuation, operators, and underscores.
	// Including _ here means snake_case is handled before camelCase splitting.
	operatorRegex   = regexp.MustCompile(`[+\-*/%=<>!&|^~@#:.\[\](){};,\\_]+`)
	whitespaceRegex = regexp.MustCompile(`\s+`)
)

// Tokenizer is a code-aware sparse vector provider.
// It splits camelCase and snake_case identifiers into constituent terms,
// filters language keywords, and produces normalized sparse vectors via FNV hashing.
// Register it with sparse.RegisterProvider to replace the default BGE BoW provider
// for source code inputs.
type Tokenizer struct{}

// NewTokenizer creates a new code-aware tokenizer.
func NewTokenizer() *Tokenizer {
	return &Tokenizer{}
}

// Tokenize splits text into lowercase code terms.
// Pipeline: normalize whitespace → split on operators/punctuation/underscores
// → split camelCase → lowercase → filter short tokens and stop words.
func (t *Tokenizer) Tokenize(text string) []string {
	text = whitespaceRegex.ReplaceAllString(text, " ")

	parts := operatorRegex.Split(text, -1)

	var tokens []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tokens = append(tokens, splitCamelCase(part)...)
	}

	filtered := tokens[:0]
	for _, tok := range tokens {
		if len(tok) > 1 && !isStopWord(tok) {
			filtered = append(filtered, tok)
		}
	}
	return filtered
}

// splitCamelCase splits a single identifier on camelCase boundaries and returns lowercase terms.
//
//	processPayment  → [process payment]
//	XMLParser       → [xml parser]
//	HTTPClient      → [http client]
//	simpleWord      → [simpleword]
func splitCamelCase(s string) []string {
	s = camelCaseLower.ReplaceAllString(s, "${1} ${2}")
	s = camelCaseUpper.ReplaceAllString(s, "${1} ${2}")
	return strings.Fields(strings.ToLower(s))
}

// hashToken maps a token to a sparse vector index via FNV32a.
// Tokens are expected to already be lowercase.
func hashToken(token string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(token))
	return h.Sum32() % vocabularySize
}

var stopWords = map[string]bool{
	// Control flow
	"if": true, "else": true, "for": true, "while": true, "do": true,
	"switch": true, "case": true, "default": true, "break": true, "continue": true,
	"return": true, "goto": true,
	// Exception handling
	"try": true, "catch": true, "throw": true, "finally": true,
	"except": true, "raise": true,
	// Type / OOP keywords
	"class": true, "struct": true, "enum": true, "interface": true, "trait": true,
	"public": true, "private": true, "protected": true, "static": true,
	"readonly": true, "abstract": true, "final": true, "volatile": true,
	"synchronized": true, "transient": true, "native": true,
	// Module system
	"import": true, "export": true, "from": true, "require": true, "module": true,
	// Declaration keywords
	"const": true, "let": true, "var": true, "func": true, "fn": true, "def": true,
	"type": true, "typedef": true, "typeof": true,
	// Common literals / builtins
	"new": true, "delete": true, "this": true, "super": true, "self": true,
	"true": true, "false": true, "nil": true, "null": true, "none": true,
	// Concurrency / generators
	"async": true, "await": true, "yield": true,
	// Operators as words
	"in": true, "of": true, "as": true, "is": true,
	"and": true, "or": true, "not": true, "xor": true,
}

// isStopWord reports whether a lowercase token is a language keyword with no retrieval value.
func isStopWord(tok string) bool {
	return stopWords[tok] // tok is already lowercase from splitCamelCase
}
