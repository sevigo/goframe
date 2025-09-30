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
	FilePath    string                 `json:"file_path"`
	Language    string                 `json:"language"`
	Imports     []string               `json:"imports"`
	Definitions []CodeEntityDefinition `json:"definitions"`
	Symbols     []CodeSymbol           `json:"symbols"`
	Properties  map[string]string      `json:"properties"`
}

type CodeEntityDefinition struct {
	Type          string `json:"type"`
	Name          string `json:"name"`
	LineStart     int    `json:"line_start"`
	LineEnd       int    `json:"line_end"`
	Visibility    string `json:"visibility"`
	Signature     string `json:"signature"`
	Documentation string `json:"documentation"`
}

type CodeSymbol struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	IsExport  bool   `json:"is_export"`
}

type CodeChunk struct {
	Content         string            `json:"content"`
	LineStart       int               `json:"lineStart"`
	LineEnd         int               `json:"lineEnd"`
	Type            string            `json:"type"`
	Identifier      string            `json:"identifier"`
	Annotations     map[string]string `json:"annotations"`
	TokenCount      int               `json:"tokenCount"`
	EnrichedContent string            `json:"enrichedContent"`
	ParentContext   string            `json:"parentContext"`
	ContextLevel    int               `json:"contextLevel"`
}

type CodeChunkingOptions struct {
	ChunkSize         int
	OverlapTokens     int
	PreserveStructure bool
	LanguageHints     []string
	MaxLinesPerChunk  int
	MinCharsPerChunk  int
}
