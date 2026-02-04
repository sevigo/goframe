package textsplitter

import (
	"context"
	"strings"
	"testing"

	"github.com/sevigo/goframe/schema"
)

func TestTruncateParentText(t *testing.T) {
	c := &CodeAwareTextSplitter{}

	// Test short text
	short := "short text"
	if got := c.truncateParentText(short); got != short {
		t.Errorf("expected %q, got %q", short, got)
	}

	// Test long text
	long := strings.Repeat("a", MaxParentTextLength+100)
	got := c.truncateParentText(long)

	if len(got) > MaxParentTextLength {
		t.Errorf("result length %d exceeds MaxParentTextLength %d", len(got), MaxParentTextLength)
	}

	if !strings.Contains(got, "...") {
		t.Errorf("result should contain ellipsis")
	}

	// Test UTF-8 safety
	utf8Text := strings.Repeat("😀", 2100) // 2100 runes > 2000 runes
	gotUTF8 := c.truncateParentText(utf8Text)
	if !strings.Contains(gotUTF8, "😀") {
		t.Errorf("UTF-8 text should preserve emoji")
	}
	if !strings.Contains(gotUTF8, "...") {
		t.Errorf("UTF-8 text should be truncated and contain ellipsis")
	}
	if len([]rune(gotUTF8)) > MaxParentTextLength {
		t.Errorf("UTF-8 result length %d exceeds MaxParentTextLength %d", len([]rune(gotUTF8)), MaxParentTextLength)
	}
}

func TestGenerateParentID_Uniqueness(t *testing.T) {
	c := &CodeAwareTextSplitter{}

	id1 := c.generateParentID("file1.go", "func1", 10)
	id2 := c.generateParentID("file1.go", "func1", 11) // Different line
	id3 := c.generateParentID("file2.go", "func1", 10) // Different file
	id4 := c.generateParentID("file1.go", "", 10)      // Empty identifier

	if id1 == id2 {
		t.Errorf("ids should be different for different lines")
	}
	if id1 == id3 {
		t.Errorf("ids should be different for different files")
	}
	if id1 == id4 {
		t.Errorf("ids should be different if identifier is empty")
	}

	// Cache test
	id1Cached := c.generateParentID("file1.go", "func1", 10)
	if id1 != id1Cached {
		t.Errorf("cached id should match original")
	}
}

func TestParentContextPropagation(t *testing.T) {
	c := &CodeAwareTextSplitter{}
	original := schema.CodeChunk{
		Content:        "parent content",
		ParentID:       "parent123",
		FullParentText: "full parent text",
		LineStart:      1,
		LineEnd:        100,
		Identifier:     "parentIdentifier",
	}

	subChunk := c.createSubChunk(original, "sub content", 10, 20, 0)

	if subChunk.ParentID != original.ParentID {
		t.Errorf("ParentID not propagated, got %q, want %q", subChunk.ParentID, original.ParentID)
	}
	if subChunk.FullParentText != original.FullParentText {
		t.Errorf("FullParentText not propagated, got %q, want %q", subChunk.FullParentText, original.FullParentText)
	}
	if !strings.Contains(subChunk.Identifier, original.Identifier) {
		t.Errorf("Sub-chunk identifier should include original identifier, got %q", subChunk.Identifier)
	}
}

func TestFallbackChunkParentContext(t *testing.T) {
	c := &CodeAwareTextSplitter{}
	ctx := context.Background()
	content := "line 1\nline 2\nline 3"
	path := "test.txt"
	params := chunkingParameters{
		ChunkSize:     10,
		OverlapTokens: 0,
	}

	chunks, err := c.intelligentFallbackChunk(ctx, content, path, params, "")
	if err != nil {
		t.Fatalf("fallback failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("no chunks produced")
	}

	for _, chunk := range chunks {
		if chunk.ParentID == "" {
			t.Errorf("fallback chunk missing ParentID")
		}
		if chunk.FullParentText == "" {
			t.Errorf("fallback chunk missing FullParentText")
		}
	}
}

func TestParentContextConfig(t *testing.T) {
	config := ParentContextConfig{
		MaxTextLength: 10,
	}
	c := &CodeAwareTextSplitter{
		parentConfig: config,
	}

	text := "this is a long text"
	got := c.truncateParentText(text)

	// MaxTextLength 10: half=(10-5)/2 = 2. "th" + "\n...\n" + "xt"
	expected := "th\n...\nxt"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}
