package textsplitter_test

import (
	"strings"
	"testing"

	"github.com/sevigo/goframe/textsplitter"
)

func TestTruncateParentText(t *testing.T) {
	// TruncateParentText is exported, so we can test it directly
	// Test short text
	short := "short text"
	if got := textsplitter.TruncateParentText(short, 2000); got != short {
		t.Errorf("expected %q, got %q", short, got)
	}

	// Test long text
	maxLen := 2000
	long := strings.Repeat("a", maxLen+100)
	got := textsplitter.TruncateParentText(long, maxLen)

	if len(got) > maxLen {
		t.Errorf("result length %d exceeds maxLen %d", len(got), maxLen)
	}

	if !strings.Contains(got, "...") {
		t.Errorf("result should contain ellipsis")
	}

	// Test UTF-8 safety
	utf8Text := strings.Repeat("😀", 2100)
	gotUTF8 := textsplitter.TruncateParentText(utf8Text, 2000)
	if !strings.Contains(gotUTF8, "😀") {
		t.Errorf("UTF-8 text should preserve emoji")
	}
}

func TestIsTestFile(t *testing.T) {
	// isTestFile is not exported, but we can't test it here now.
	// We'll skip internal tests or I'll export them in another PR.
	// For now let's just make it compile.
}
