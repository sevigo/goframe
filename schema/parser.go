package schema

import (
	"io/fs"
)

type ParserPlugin interface {
	Name() string
	Extensions() []string
	CanHandle(path string, info fs.FileInfo) bool
	Chunk(content string, path string, opts *CodeChunkingOptions) ([]CodeChunk, error)
	ExtractMetadata(content string, path string) (FileMetadata, error)
}

type FileMetadata struct {
	FilePath    string
	Language    string
	Imports     []string
	Definitions []CodeEntityDefinition
	Symbols     []CodeSymbol
	Properties  map[string]string
}

type CodeEntityDefinition struct {
	Type          string
	Name          string
	LineStart     int
	LineEnd       int
	Visibility    string
	Signature     string
	Documentation string
}

type CodeSymbol struct {
	Name      string
	Type      string
	LineStart int
	LineEnd   int
	IsExport  bool
}

type CodeChunk struct {
	Content         string
	LineStart       int
	LineEnd         int
	Type            string
	Identifier      string
	Annotations     map[string]string
	TokenCount      int
	EnrichedContent string
	ParentContext   string
	ContextLevel    int
}

type CodeChunkingOptions struct {
	ChunkSize         int
	OverlapTokens     int
	PreserveStructure bool
	LanguageHints     []string
	MaxLinesPerChunk  int
	MinCharsPerChunk  int
}
