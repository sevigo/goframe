package textsplitter

import (
	"io/fs"
	"testing"

	"github.com/sevigo/goframe/schema"
	"github.com/stretchr/testify/assert"
)

func TestIsTestFileInternal(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"foo_test.go", true},
		{"foo.test.ts", true},
		{"foo.spec.ts", true},
		{"foo.test.tsx", true},
		{"foo.spec.tsx", true},
		{"foo.test.js", true},
		{"foo.spec.js", true},
		{"foo.go", false},
		{"test.go", false}, // Go tests must have _test.go suffix
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, isTestFile(tt.path))
		})
	}
}

func TestGenerateParentID(t *testing.T) {
	s, _ := NewCodeAware(&fakeRegistry{}, nil, nil)

	id1 := s.generateParentID("file.go", "Func", 10)
	id2 := s.generateParentID("file.go", "Func", 10)

	assert.Equal(t, id1, id2, "IDs should be deterministic")

	id3 := s.generateParentID("file.go", "Func", 11)
	assert.NotEqual(t, id1, id3, "IDs should differ by line")

	// Check cache hits
	v, ok := s.parentIDCache.Load("file.go:Func:10")
	assert.True(t, ok)
	assert.Equal(t, id1, v)
}

type fakeRegistry struct{}

func (f *fakeRegistry) RegisterParser(p schema.ParserPlugin) error         { return nil }
func (f *fakeRegistry) GetParser(lang string) (schema.ParserPlugin, error) { return nil, nil }
func (f *fakeRegistry) GetParserForFile(path string, info fs.FileInfo) (schema.ParserPlugin, error) {
	return nil, nil
}
func (f *fakeRegistry) GetParserForExtension(ext string) (schema.ParserPlugin, error) {
	return nil, nil
}
func (f *fakeRegistry) GetAllParsers() []schema.ParserPlugin { return nil }
