package textsplitter

import (
	"strings"
	"testing"
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
}

func TestGenerateParentID_Uniqueness(t *testing.T) {
	c := &CodeAwareTextSplitter{}

	id1 := c.generateParentID("file1.go", "func1", 10)
	id2 := c.generateParentID("file1.go", "func1", 11) // Different line
	id3 := c.generateParentID("file2.go", "func1", 10) // Different file

	if id1 == id2 {
		t.Errorf("ids should be different for different lines")
	}
	if id1 == id3 {
		t.Errorf("ids should be different for different files")
	}

	// Cache test
	id1Cached := c.generateParentID("file1.go", "func1", 10)
	if id1 != id1Cached {
		t.Errorf("cached id should match original")
	}
}
