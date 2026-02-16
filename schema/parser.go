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
	IsGenerated(content string, path string) bool
}

type FileMetadata struct {
	FilePath    string                 `json:"file_path"`
	Language    string                 `json:"language"`
	PackageName string                 `json:"package_name"`
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
	Sparse          *SparseVector     `json:"sparse,omitempty"`
	// ParentID uniquely identifies the parent code structure (function/class) this chunk belongs to.
	// Empty for top-level chunks that are not split.
	ParentID string `json:"parentID,omitempty"`

	// FullParentText contains the complete text of the parent structure.
	// WARNING: This can be large. Logic should truncate this before storage.
	FullParentText string `json:"fullParentText,omitempty"`

	// IsDefinition is true if this chunk represents the primary source-of-truth definition of a symbol.
	IsDefinition bool `json:"is_definition"`

	// SymbolType is the category of the symbol (e.g., struct, interface, function).
	SymbolType string `json:"symbol_type"`
}

type CodeChunkingOptions struct {
	ChunkSize         int
	OverlapTokens     int
	PreserveStructure bool
	LanguageHints     []string
	MaxLinesPerChunk  int
	MinCharsPerChunk  int
}
