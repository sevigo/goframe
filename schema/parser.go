package schema

import (
	"io/fs"
)

// ParserPlugin defines the interface for language-specific code parsing.
// Implementations handle parsing, chunking, and metadata extraction for
// specific programming languages or file types.
type ParserPlugin interface {
	// Name returns the name of the parser (e.g., "go", "typescript").
	Name() string
	// Extensions returns the file extensions this parser handles (e.g., ".go", ".ts").
	Extensions() []string
	// CanHandle returns true if this parser can handle the given file.
	CanHandle(path string, info fs.FileInfo) bool
	// Chunk splits the content into semantic chunks with metadata.
	Chunk(content string, path string, opts *CodeChunkingOptions) ([]CodeChunk, error)
	// ExtractMetadata extracts file-level metadata like package name and imports.
	ExtractMetadata(content string, path string) (FileMetadata, error)
	// IsGenerated returns true if the file appears to be auto-generated.
	IsGenerated(content string, path string) bool
	// ExtractUsedSymbols identifies potential external types/functions
	// being used in the code that might need a definition lookup.
	ExtractUsedSymbols(content string) []string
}

// FileMetadata contains metadata extracted from a source file.
type FileMetadata struct {
	// FilePath is the path to the file.
	FilePath string `json:"file_path"`
	// Language is the programming language (e.g., "go", "typescript").
	Language string `json:"language"`
	// PackageName is the name of the package/module.
	PackageName string `json:"package_name"`
	// Imports is the list of imported packages/modules.
	Imports []string `json:"imports"`
	// Definitions contains the top-level definitions in the file.
	Definitions []CodeEntityDefinition `json:"definitions"`
	// Symbols contains all symbols found in the file.
	Symbols []CodeSymbol `json:"symbols"`
	// Properties contains additional file properties.
	Properties map[string]string `json:"properties"`
}

// CodeEntityDefinition represents a code entity definition (function, struct, etc.).
type CodeEntityDefinition struct {
	// Type is the entity type (e.g., "function", "struct", "interface").
	Type string `json:"type"`
	// Name is the entity name.
	Name string `json:"name"`
	// LineStart is the starting line number.
	LineStart int `json:"line_start"`
	// LineEnd is the ending line number.
	LineEnd int `json:"line_end"`
	// Visibility is the export visibility (e.g., "public", "private").
	Visibility string `json:"visibility"`
	// Signature is the function/method signature.
	Signature string `json:"signature"`
	// Documentation is the doc comment for the entity.
	Documentation string `json:"documentation"`
}

// CodeSymbol represents a symbol found in code.
type CodeSymbol struct {
	// Name is the symbol name.
	Name string `json:"name"`
	// Type is the symbol type (e.g., "function", "variable", "type").
	Type string `json:"type"`
	// LineStart is the starting line number.
	LineStart int `json:"line_start"`
	// LineEnd is the ending line number.
	LineEnd int `json:"line_end"`
	// IsExport indicates if the symbol is exported.
	IsExport bool `json:"is_export"`
}

// CodeChunk represents a chunk of code with metadata.
type CodeChunk struct {
	// Content is the code content of the chunk.
	Content string `json:"content"`
	// LineStart is the starting line number in the source file.
	LineStart int `json:"lineStart"`
	// LineEnd is the ending line number in the source file.
	LineEnd int `json:"lineEnd"`
	// Type is the chunk type (e.g., "function", "struct", "import").
	Type string `json:"type"`
	// Identifier is the primary identifier of the chunk (e.g., function name).
	Identifier string `json:"identifier"`
	// Annotations contains additional annotations for the chunk.
	Annotations map[string]string `json:"annotations"`
	// TokenCount is the estimated number of tokens in the chunk.
	TokenCount int `json:"tokenCount"`
	// EnrichedContent contains the content with added context.
	EnrichedContent string `json:"enrichedContent"`
	// ParentContext contains context from the parent structure.
	ParentContext string `json:"parentContext"`
	// ContextLevel indicates the nesting level of the context.
	ContextLevel int `json:"contextLevel"`
	// Sparse is an optional sparse vector for hybrid search.
	Sparse *SparseVector `json:"sparse,omitempty"`
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

// CodeChunkingOptions configures how code is chunked.
type CodeChunkingOptions struct {
	// ChunkSize is the target size in tokens for each chunk.
	ChunkSize int
	// OverlapTokens is the number of overlapping tokens between chunks.
	OverlapTokens int
	// PreserveStructure attempts to keep related code together.
	PreserveStructure bool
	// LanguageHints provides hints about the language for better chunking.
	LanguageHints []string
	// MaxLinesPerChunk limits the maximum lines per chunk.
	MaxLinesPerChunk int
	// MinCharsPerChunk is the minimum characters required for a valid chunk.
	MinCharsPerChunk int
}
