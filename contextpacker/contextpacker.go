// Package contextpacker provides utilities for packing and optimizing context
// for LLM workflows.
package contextpacker

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

// documentWithTokens holds a document with its precomputed token count.
type documentWithTokens struct {
	doc        schema.Document
	tokenCount int
}

// TokenStats holds token usage statistics.
type TokenStats struct {
	// TotalTokens is the total tokens used in the packed content.
	TotalTokens int
	// MaxTokens is the maximum allowed tokens.
	MaxTokens int
	// DocumentsConsidered is the number of documents evaluated.
	DocumentsConsidered int
	// DocumentsPacked is the number of documents that fit.
	DocumentsPacked int
}

// UsedDocument tracks a document that was packed.
type UsedDocument struct {
	// Content is the formatted document content.
	Content string
	// TokenCount is the token count for this document.
	TokenCount int
	// Source is the original document.
	Source schema.Document
}

// PackedResult contains the result of packing documents.
type PackedResult struct {
	// Content is the final packed context string.
	Content string
	// UsedDocuments are the documents that fit in the token budget.
	UsedDocuments []UsedDocument
	// TokenStats contains token usage statistics.
	TokenStats TokenStats
	// Truncated indicates whether some documents were dropped.
	Truncated bool
}

// Packer packs documents into a context window within token limits.
type Packer struct {
	tokenizer llms.Tokenizer
	maxTokens int
	template  *template.Template
	strategy  PackingStrategy
	logger    interface{ Debug(string, ...any) }
}

// parseTemplate parses a template string and returns the template.
func parseTemplate(tmplStr string) (*template.Template, error) {
	tmpl, err := template.New("document").Parse(tmplStr)
	if err != nil {
		return nil, ErrTemplateParse
	}
	return tmpl, nil
}

// Pack packs documents into a context string within token limits.
// Documents are packed atomically - either fully included or dropped.
func (p *Packer) Pack(ctx context.Context, docs []schema.Document) (PackedResult, error) {
	return p.PackWithScores(ctx, docs, nil)
}

// PackWithScores packs documents with optional scores for importance ordering.
// If scores is nil or empty, the default strategy ordering is used.
func (p *Packer) PackWithScores(ctx context.Context, docs []schema.Document, scores []float64) (PackedResult, error) {
	if len(docs) == 0 {
		return PackedResult{
			TokenStats: TokenStats{
				MaxTokens: p.maxTokens,
			},
		}, nil
	}

	ordered := p.strategy.Order(docs, scores)

	docsWithTokens, err := p.countDocumentTokens(ctx, ordered)
	if err != nil {
		return PackedResult{}, err
	}

	return p.packDocuments(ctx, docsWithTokens), nil
}

// PackScored packs ScoredDocuments using their scores for importance ordering.
func (p *Packer) PackScored(ctx context.Context, docs []schema.ScoredDocument) (PackedResult, error) {
	if len(docs) == 0 {
		return PackedResult{
			TokenStats: TokenStats{
				MaxTokens: p.maxTokens,
			},
		}, nil
	}

	regularDocs := make([]schema.Document, len(docs))
	scores := make([]float64, len(docs))
	for i, sd := range docs {
		regularDocs[i] = sd.Document
		scores[i] = sd.Score
	}

	return p.PackWithScores(ctx, regularDocs, scores)
}

// countDocumentTokens precomputes token counts for all documents.
func (p *Packer) countDocumentTokens(ctx context.Context, docs []schema.Document) ([]documentWithTokens, error) {
	result := make([]documentWithTokens, len(docs))
	for i, doc := range docs {
		// Check for context cancellation on each iteration
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		formatted, err := formatDocument(p.template, documentWithTokens{doc: doc})
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrTemplateExecute, err)
		}

		count, err := p.tokenizer.CountTokens(ctx, formatted)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrTokenCountFailed, err)
		}

		result[i] = documentWithTokens{
			doc:        doc,
			tokenCount: count,
		}
	}
	return result, nil
}

// packDocuments packs documents atomically within the token limit.
func (p *Packer) packDocuments(ctx context.Context, docs []documentWithTokens) PackedResult {
	var usedDocs []UsedDocument
	var contentParts []string
	totalTokens := 0

	separator := "\n\n"
	// Compute separator token count using the tokenizer
	separatorTokens, err := p.tokenizer.CountTokens(ctx, separator)
	if err != nil {
		// Fallback to reasonable default for "\n\n"
		separatorTokens = 2
	}

	for i, doc := range docs {
		// Calculate separator cost before adding this document
		// First document has no separator, subsequent ones do
		separatorCost := 0
		if i > 0 {
			separatorCost = separatorTokens
		}

		if totalTokens+doc.tokenCount+separatorCost > p.maxTokens {
			break
		}

		formatted, err := formatDocument(p.template, doc)
		if err != nil {
			// Log and skip documents that fail to format
			p.logger.Debug("skipping document due to formatting error", "error", err)
			continue
		}

		usedDocs = append(usedDocs, UsedDocument{
			Content:    formatted,
			TokenCount: doc.tokenCount,
			Source:     doc.doc,
		})
		contentParts = append(contentParts, formatted)
		totalTokens += doc.tokenCount + separatorCost
	}

	return PackedResult{
		Content:       strings.Join(contentParts, separator),
		UsedDocuments: usedDocs,
		TokenStats: TokenStats{
			TotalTokens:         totalTokens,
			MaxTokens:           p.maxTokens,
			DocumentsConsidered: len(docs),
			DocumentsPacked:     len(usedDocs),
		},
		Truncated: len(usedDocs) < len(docs),
	}
}
