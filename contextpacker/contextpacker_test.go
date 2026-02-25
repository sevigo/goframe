package contextpacker

import (
	"context"
	"errors"
	"testing"

	"github.com/sevigo/goframe/contextpacker/fake"
	"github.com/sevigo/goframe/schema"
)

func TestNew(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		tokenizer := fake.NewTokenizer()
		packer, err := New(tokenizer, 1000)

		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if packer == nil {
			t.Fatal("New() returned nil packer")
		}
	})

	t.Run("nil tokenizer", func(t *testing.T) {
		_, err := New(nil, 1000)

		if !errors.Is(err, ErrNilTokenizer) {
			t.Errorf("New() error = %v, want ErrNilTokenizer", err)
		}
	})

	t.Run("zero maxTokens", func(t *testing.T) {
		tokenizer := fake.NewTokenizer()
		_, err := New(tokenizer, 0)

		if !errors.Is(err, ErrInvalidMaxTokens) {
			t.Errorf("New() error = %v, want ErrInvalidMaxTokens", err)
		}
	})

	t.Run("negative maxTokens", func(t *testing.T) {
		tokenizer := fake.NewTokenizer()
		_, err := New(tokenizer, -100)

		if !errors.Is(err, ErrInvalidMaxTokens) {
			t.Errorf("New() error = %v, want ErrInvalidMaxTokens", err)
		}
	})

	t.Run("invalid template", func(t *testing.T) {
		tokenizer := fake.NewTokenizer()
		_, err := New(tokenizer, 1000, WithTemplate("{{.invalid"))

		if !errors.Is(err, ErrTemplateParse) {
			t.Errorf("New() error = %v, want ErrTemplateParse", err)
		}
	})
}

func TestPack(t *testing.T) {
	ctx := context.Background()

	t.Run("empty documents", func(t *testing.T) {
		tokenizer := fake.NewTokenizer()
		packer, _ := New(tokenizer, 1000)

		result, err := packer.Pack(ctx, nil)

		if err != nil {
			t.Fatalf("Pack() error = %v", err)
		}
		if result.Content != "" {
			t.Errorf("Pack() content = %q, want empty", result.Content)
		}
		if result.Truncated {
			t.Error("Pack() truncated = true, want false")
		}
	})

	t.Run("documents fit within limit", func(t *testing.T) {
		tokenizer := fake.NewTokenizer()
		packer, _ := New(tokenizer, 100, WithTemplate(CompactTemplate))

		docs := []schema.Document{
			{PageContent: "hello world"},
			{PageContent: "foo bar"},
		}

		result, err := packer.Pack(ctx, docs)

		if err != nil {
			t.Fatalf("Pack() error = %v", err)
		}
		if len(result.UsedDocuments) != 2 {
			t.Errorf("Pack() used %d documents, want 2", len(result.UsedDocuments))
		}
		if result.Truncated {
			t.Error("Pack() truncated = true, want false")
		}
	})

	t.Run("documents exceed limit - atomic packing", func(t *testing.T) {
		tokenizer := fake.NewTokenizer()
		packer, _ := New(tokenizer, 3, WithTemplate(CompactTemplate))

		docs := []schema.Document{
			{PageContent: "one two three"}, // 3 tokens
			{PageContent: "four five six"}, // 3 tokens - won't fit
		}

		result, err := packer.Pack(ctx, docs)

		if err != nil {
			t.Fatalf("Pack() error = %v", err)
		}
		if len(result.UsedDocuments) != 1 {
			t.Errorf("Pack() used %d documents, want 1 (atomic)", len(result.UsedDocuments))
		}
		if !result.Truncated {
			t.Error("Pack() truncated = false, want true")
		}
		if result.TokenStats.DocumentsPacked != 1 {
			t.Errorf("Pack() documents packed = %d, want 1", result.TokenStats.DocumentsPacked)
		}
		if result.TokenStats.DocumentsConsidered != 2 {
			t.Errorf("Pack() documents considered = %d, want 2", result.TokenStats.DocumentsConsidered)
		}
	})

	t.Run("single document too large", func(t *testing.T) {
		tokenizer := fake.NewTokenizer()
		packer, _ := New(tokenizer, 2, WithTemplate(CompactTemplate))

		docs := []schema.Document{
			{PageContent: "one two three four five"}, // 5 tokens, limit is 2
		}

		result, err := packer.Pack(ctx, docs)

		if err != nil {
			t.Fatalf("Pack() error = %v", err)
		}
		if len(result.UsedDocuments) != 0 {
			t.Errorf("Pack() used %d documents, want 0", len(result.UsedDocuments))
		}
		if result.Content != "" {
			t.Errorf("Pack() content = %q, want empty", result.Content)
		}
	})
}

func TestPackWithScores(t *testing.T) {
	ctx := context.Background()

	t.Run("importance strategy orders by score", func(t *testing.T) {
		tokenizer := fake.NewTokenizer()
		packer, _ := New(tokenizer, 100,
			WithStrategy(ImportanceStrategy{}),
			WithTemplate(CompactTemplate),
		)

		docs := []schema.Document{
			{PageContent: "low priority"},
			{PageContent: "high priority"},
			{PageContent: "medium priority"},
		}
		scores := []float64{1.0, 10.0, 5.0}

		result, err := packer.PackWithScores(ctx, docs, scores)

		if err != nil {
			t.Fatalf("PackWithScores() error = %v", err)
		}

		if len(result.UsedDocuments) != 3 {
			t.Fatalf("PackWithScores() used %d documents, want 3", len(result.UsedDocuments))
		}

		// First document should be highest score (high priority)
		if result.UsedDocuments[0].Source.PageContent != "high priority" {
			t.Errorf("PackWithScores() first doc = %q, want 'high priority'",
				result.UsedDocuments[0].Source.PageContent)
		}
	})

	t.Run("nil scores uses default ordering", func(t *testing.T) {
		tokenizer := fake.NewTokenizer()
		packer, _ := New(tokenizer, 100, WithTemplate(CompactTemplate))

		docs := []schema.Document{
			{PageContent: "first"},
			{PageContent: "second"},
		}

		result, err := packer.PackWithScores(ctx, docs, nil)

		if err != nil {
			t.Fatalf("PackWithScores() error = %v", err)
		}
		if result.UsedDocuments[0].Source.PageContent != "first" {
			t.Errorf("PackWithScores() first doc = %q, want 'first'",
				result.UsedDocuments[0].Source.PageContent)
		}
	})
}

func TestPackScored(t *testing.T) {
	ctx := context.Background()

	t.Run("packs scored documents", func(t *testing.T) {
		tokenizer := fake.NewTokenizer()
		packer, _ := New(tokenizer, 100,
			WithStrategy(ImportanceStrategy{}),
			WithTemplate(CompactTemplate),
		)

		docs := []schema.ScoredDocument{
			{Document: schema.Document{PageContent: "low"}, Score: 1.0},
			{Document: schema.Document{PageContent: "high"}, Score: 10.0},
		}

		result, err := packer.PackScored(ctx, docs)

		if err != nil {
			t.Fatalf("PackScored() error = %v", err)
		}
		if result.UsedDocuments[0].Source.PageContent != "high" {
			t.Errorf("PackScored() first doc = %q, want 'high'",
				result.UsedDocuments[0].Source.PageContent)
		}
	})

	t.Run("empty scored documents", func(t *testing.T) {
		tokenizer := fake.NewTokenizer()
		packer, _ := New(tokenizer, 100)

		result, err := packer.PackScored(ctx, nil)

		if err != nil {
			t.Fatalf("PackScored() error = %v", err)
		}
		if result.Content != "" {
			t.Errorf("PackScored() content = %q, want empty", result.Content)
		}
	})
}

func TestStrategies(t *testing.T) {
	docs := []schema.Document{
		{PageContent: "a"},
		{PageContent: "b"},
		{PageContent: "c"},
	}
	scores := []float64{3.0, 1.0, 2.0}

	t.Run("greedy preserves order", func(t *testing.T) {
		strategy := GreedyStrategy{}
		result := strategy.Order(docs, scores)

		for i, want := range []string{"a", "b", "c"} {
			if result[i].PageContent != want {
				t.Errorf("GreedyStrategy order[%d] = %q, want %q", i, result[i].PageContent, want)
			}
		}
	})

	t.Run("importance orders by score descending", func(t *testing.T) {
		strategy := ImportanceStrategy{}
		result := strategy.Order(docs, scores)

		for i, want := range []string{"a", "c", "b"} {
			if result[i].PageContent != want {
				t.Errorf("ImportanceStrategy order[%d] = %q, want %q", i, result[i].PageContent, want)
			}
		}
	})

	t.Run("importance with mismatched lengths", func(t *testing.T) {
		strategy := ImportanceStrategy{}
		result := strategy.Order(docs, []float64{1.0}) // mismatched

		// Should return original order
		for i, want := range []string{"a", "b", "c"} {
			if result[i].PageContent != want {
				t.Errorf("ImportanceStrategy order[%d] = %q, want %q", i, result[i].PageContent, want)
			}
		}
	})

	t.Run("metadata strategy descending", func(t *testing.T) {
		docsWithMeta := []schema.Document{
			{PageContent: "a", Metadata: map[string]any{"priority": "high"}},
			{PageContent: "b", Metadata: map[string]any{"priority": "low"}},
			{PageContent: "c", Metadata: map[string]any{"priority": "medium"}},
		}

		// Descending alphabetical: "medium" > "low" > "high"
		strategy := MetadataStrategy{Field: "priority", Ascending: false}
		result := strategy.Order(docsWithMeta, nil)

		// "medium" (c) first alphabetically descending
		if result[0].PageContent != "c" {
			t.Errorf("MetadataStrategy order[0] = %q, want 'c'", result[0].PageContent)
		}
	})

	t.Run("metadata strategy ascending", func(t *testing.T) {
		docsWithMeta := []schema.Document{
			{PageContent: "a", Metadata: map[string]any{"priority": "high"}},
			{PageContent: "b", Metadata: map[string]any{"priority": "low"}},
			{PageContent: "c", Metadata: map[string]any{"priority": "medium"}},
		}

		// Ascending alphabetical: "high" < "low" < "medium"
		strategy := MetadataStrategy{Field: "priority", Ascending: true}
		result := strategy.Order(docsWithMeta, nil)

		// "high" (a) first alphabetically ascending
		if result[0].PageContent != "a" {
			t.Errorf("MetadataStrategy order[0] = %q, want 'a'", result[0].PageContent)
		}
	})
}

func TestTemplates(t *testing.T) {
	ctx := context.Background()

	t.Run("default template includes metadata", func(t *testing.T) {
		tokenizer := fake.NewTokenizer()
		packer, _ := New(tokenizer, 1000)

		docs := []schema.Document{
			{PageContent: "content here", Metadata: map[string]any{"key": "value"}},
		}

		result, err := packer.Pack(ctx, docs)

		if err != nil {
			t.Fatalf("Pack() error = %v", err)
		}
		if result.Content == "" {
			t.Error("Pack() content is empty")
		}
	})

	t.Run("compact template excludes metadata", func(t *testing.T) {
		tokenizer := fake.NewTokenizer()
		packer, _ := New(tokenizer, 1000, WithTemplate(CompactTemplate))

		docs := []schema.Document{
			{PageContent: "just content", Metadata: map[string]any{"key": "value"}},
		}

		result, err := packer.Pack(ctx, docs)

		if err != nil {
			t.Fatalf("Pack() error = %v", err)
		}
		if result.Content != "just content" {
			t.Errorf("Pack() content = %q, want 'just content'", result.Content)
		}
	})
}

func TestTokenCountingError(t *testing.T) {
	ctx := context.Background()
	tokenizer := &fake.Tokenizer{Err: errors.New("token error")}
	packer, _ := New(tokenizer, 1000, WithTemplate(CompactTemplate))

	docs := []schema.Document{{PageContent: "test"}}

	_, err := packer.Pack(ctx, docs)

	if !errors.Is(err, ErrTokenCountFailed) {
		t.Errorf("Pack() error = %v, want ErrTokenCountFailed", err)
	}
}

func TestUsedDocumentTracking(t *testing.T) {
	ctx := context.Background()
	tokenizer := fake.NewTokenizer()
	packer, _ := New(tokenizer, 100, WithTemplate(CompactTemplate))

	docs := []schema.Document{
		{PageContent: "first document", Metadata: map[string]any{"id": "1"}},
		{PageContent: "second document", Metadata: map[string]any{"id": "2"}},
	}

	result, err := packer.Pack(ctx, docs)

	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}

	if len(result.UsedDocuments) != 2 {
		t.Fatalf("Pack() used %d documents, want 2", len(result.UsedDocuments))
	}

	for i, used := range result.UsedDocuments {
		if used.Content == "" {
			t.Errorf("UsedDocuments[%d].Content is empty", i)
		}
		if used.TokenCount <= 0 {
			t.Errorf("UsedDocuments[%d].TokenCount = %d, want > 0", i, used.TokenCount)
		}
		if used.Source.PageContent == "" {
			t.Errorf("UsedDocuments[%d].Source is empty", i)
		}
	}
}

func TestDeterministicMetadataFormatting(t *testing.T) {
	// Test that metadata formatting is deterministic regardless of map iteration order
	metadata := map[string]any{
		"zeta":  "last",
		"alpha": "first",
		"beta":  "second",
		"gamma": "third",
	}

	// Call formatMetadata multiple times and ensure consistent output
	outputs := make([]string, 10)
	for i := range 10 {
		outputs[i] = formatMetadata(metadata)
	}

	// All outputs should be identical
	expected := outputs[0]
	for i, output := range outputs {
		if output != expected {
			t.Errorf("formatMetadata() iteration %d = %q, want %q", i, output, expected)
		}
	}

	// Output should have keys in alphabetical order
	if expected != "{alpha: first, beta: second, gamma: third, zeta: last}" {
		t.Errorf("formatMetadata() = %q, want keys sorted alphabetically", expected)
	}
}

func TestFloat64Precision(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{0.95, "0.95"},
		{0.4, "0.4"},
		{0.001, "0.001"},
		{1.23456789, "1.23456789"},
		{100.5, "100.5"},
	}

	for _, tt := range tests {
		result := formatValue(tt.input)
		if result != tt.expected {
			t.Errorf("formatValue(%v) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestContextCancellation(t *testing.T) {
	tokenizer := fake.NewTokenizer()
	packer, _ := New(tokenizer, 1000, WithTemplate(CompactTemplate))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	docs := []schema.Document{
		{PageContent: "test document"},
	}

	_, err := packer.Pack(ctx, docs)

	if err == nil {
		t.Error("Pack() expected error for cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Pack() error = %v, want context.Canceled", err)
	}
}
