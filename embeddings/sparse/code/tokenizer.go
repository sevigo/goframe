package code

import (
	"hash/fnv"
	"regexp"
	"strings"
	"unicode"
)

const (
	vocabularySize = 50000
)

var (
	camelCaseRegex  = regexp.MustCompile(`([a-z])([A-Z])|([A-Z])([A-Z][a-z])`)
	operatorRegex   = regexp.MustCompile(`[+\-*/%=<>!&|^~@#:.\[\](){};,\\]+`)
	whitespaceRegex = regexp.MustCompile(`\s+`)
)

type Tokenizer struct{}

func NewTokenizer() *Tokenizer {
	return &Tokenizer{}
}

func (t *Tokenizer) Tokenize(text string) []string {
	var tokens []string

	text = whitespaceRegex.ReplaceAllString(text, " ")

	parts := operatorRegex.Split(text, -1)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		subTokens := t.splitCamelCaseAndSnake(part)
		tokens = append(tokens, subTokens...)
	}

	filtered := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		tok = strings.ToLower(tok)
		if len(tok) > 1 && !isStopWord(tok) {
			filtered = append(filtered, tok)
		}
	}

	return filtered
}

func (t *Tokenizer) splitCamelCaseAndSnake(text string) []string {
	if text == "" {
		return nil
	}

	var result []string
	var current strings.Builder

	for i := 0; i < len(text); i++ {
		r := rune(text[i])

		if r == '_' {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
			continue
		}

		if unicode.IsUpper(r) {
			if current.Len() > 0 {
				last := current.String()
				if len(last) > 1 && !unicode.IsUpper(rune(last[len(last)-1])) {
					result = append(result, last)
					current.Reset()
				}
			}
		}

		current.WriteRune(r)
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

var stopWords = map[string]bool{
	"if": true, "else": true, "for": true, "while": true, "do": true,
	"switch": true, "case": true, "default": true, "break": true, "continue": true,
	"return": true, "goto": true,
	"try": true, "catch": true, "throw": true, "finally": true,
	"class": true, "struct": true, "enum": true, "interface": true, "trait": true,
	"public": true, "private": true, "protected": true, "static": true,
	"readonly": true, "abstract": true, "final": true, "volatile": true,
	"synchronized": true, "transient": true, "native": true,
	"import": true, "export": true, "from": true, "require": true, "module": true,
	"const": true, "let": true, "var": true, "func": true, "fn": true, "def": true,
	"type": true, "typedef": true, "typeof": true,
	"new": true, "delete": true, "this": true, "super": true, "self": true,
	"true": true, "false": true, "nil": true, "null": true, "none": true,
	"async": true, "await": true, "yield": true,
	"except": true, "raise": true,
	"in": true, "of": true, "as": true, "is": true,
	"and": true, "or": true, "not": true, "xor": true,
}

func isStopWord(tok string) bool {
	return stopWords[strings.ToLower(tok)]
}

func (t *Tokenizer) hashToken(token string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(token)))
	return h.Sum32() % vocabularySize
}
