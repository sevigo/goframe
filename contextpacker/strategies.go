package contextpacker

import (
	"sort"

	"github.com/sevigo/goframe/schema"
)

// PackingStrategy defines how documents are ordered before packing.
type PackingStrategy interface {
	// Name returns the strategy name for logging and debugging.
	Name() string
	// Order returns documents in the order they should be packed.
	Order(docs []schema.Document, scores []float64) []schema.Document
}

// GreedyStrategy packs documents in their original order.
type GreedyStrategy struct{}

func (GreedyStrategy) Name() string { return "greedy" }
func (GreedyStrategy) Order(docs []schema.Document, _ []float64) []schema.Document {
	return docs
}

// ImportanceStrategy packs documents ordered by score (highest first).
type ImportanceStrategy struct{}

func (ImportanceStrategy) Name() string { return "importance" }
func (ImportanceStrategy) Order(docs []schema.Document, scores []float64) []schema.Document {
	if len(scores) == 0 || len(docs) != len(scores) {
		return docs
	}

	type indexedDoc struct {
		doc   schema.Document
		score float64
	}

	indexed := make([]indexedDoc, len(docs))
	for i, doc := range docs {
		indexed[i] = indexedDoc{doc: doc, score: scores[i]}
	}

	sort.Slice(indexed, func(i, j int) bool {
		return indexed[i].score > indexed[j].score
	})

	result := make([]schema.Document, len(indexed))
	for i, id := range indexed {
		result[i] = id.doc
	}
	return result
}

// MetadataStrategy packs documents ordered by a metadata field.
type MetadataStrategy struct {
	Field     string
	Ascending bool
}

func (s MetadataStrategy) Name() string { return "metadata" }
func (s MetadataStrategy) Order(docs []schema.Document, _ []float64) []schema.Document {
	if s.Field == "" {
		return docs
	}

	type indexedDoc struct {
		doc   schema.Document
		value any
	}

	indexed := make([]indexedDoc, len(docs))
	for i, doc := range docs {
		var val any
		if doc.Metadata != nil {
			val = doc.Metadata[s.Field]
		}
		indexed[i] = indexedDoc{doc: doc, value: val}
	}

	sort.SliceStable(indexed, func(i, j int) bool {
		vi, viOK := indexed[i].value.(string)
		vj, vjOK := indexed[j].value.(string)
		if !viOK || !vjOK {
			return false
		}
		if s.Ascending {
			return vi < vj
		}
		return vi > vj
	})

	result := make([]schema.Document, len(indexed))
	for i, id := range indexed {
		result[i] = id.doc
	}
	return result
}
