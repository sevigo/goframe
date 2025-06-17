package csv_test

import (
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/sevigo/goframe/parsers/csv"
	model "github.com/sevigo/goframe/schema"
)

func TestCSVPlugin_Name(t *testing.T) {
	plugin := csv.NewCSVPlugin(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if got := plugin.Name(); got != "csv" {
		t.Errorf("Name() = %v, want %v", got, "csv")
	}
}

func TestCSVPlugin_Extensions(t *testing.T) {
	plugin := csv.NewCSVPlugin(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	expected := []string{".csv", ".tsv"}
	got := plugin.Extensions()

	if len(got) != len(expected) {
		t.Errorf("Extensions() returned %d extensions, want %d", len(got), len(expected))
	}

	for i, ext := range expected {
		if got[i] != ext {
			t.Errorf("Extensions()[%d] = %v, want %v", i, got[i], ext)
		}
	}
}

func TestCSVPlugin_CanHandle(t *testing.T) {
	plugin := csv.NewCSVPlugin(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"CSV file", "data.csv", true},
		{"TSV file", "data.tsv", true},
		{"CSV uppercase", "DATA.CSV", true},
		{"TSV uppercase", "DATA.TSV", true},
		{"Text file", "data.txt", false},
		{"JSON file", "data.json", false},
		{"No extension", "data", false},
		{"Empty path", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := plugin.CanHandle(tt.path, nil)
			if got != tt.expected {
				t.Errorf("CanHandle(%s) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestCSVPlugin_Chunk(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	plugin := csv.NewCSVPlugin(logger)

	tests := []struct {
		name           string
		content        string
		path           string
		opts           *model.CodeChunkingOptions
		expectedChunks int
	}{
		{
			name:           "Small CSV single chunk",
			content:        "Name,Age\nJohn,30\nJane,25",
			path:           "test.csv",
			opts:           nil,
			expectedChunks: 1,
		},
		{
			name:           "Empty content",
			content:        "",
			path:           "test.csv",
			opts:           nil,
			expectedChunks: 0,
		},
		{
			name:           "Large CSV with headers",
			content:        generateLargeCSV(50),
			path:           "test.csv",
			opts:           &model.CodeChunkingOptions{MaxLinesPerChunk: 10},
			expectedChunks: 11, // 1 header chunk + 10 data chunks
		},
		{
			name:           "Valid numeric CSV",
			content:        generateValidNumericCSV(30),
			path:           "test.csv",
			opts:           &model.CodeChunkingOptions{MaxLinesPerChunk: 15},
			expectedChunks: 3, // Should create chunks based on actual parsing
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks, err := plugin.Chunk(tt.content, tt.path, tt.opts)
			if err != nil {
				t.Errorf("Chunk() unexpected error: %v", err)
				return
			}

			if len(chunks) != tt.expectedChunks {
				t.Errorf("Chunk() created %d chunks, want %d", len(chunks), tt.expectedChunks)
			}

			// Verify chunk types for non-empty results
			if len(chunks) > 0 && tt.expectedChunks > 0 {
				for i, chunk := range chunks {
					if chunk.Type == "" {
						t.Errorf("Chunk %d has empty type", i)
					}
					if chunk.Identifier == "" {
						t.Errorf("Chunk %d has empty identifier", i)
					}
				}
			}
		})
	}
}

func TestCSVPlugin_ExtractMetadata(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	plugin := csv.NewCSVPlugin(logger)

	tests := []struct {
		name               string
		content            string
		path               string
		expectedValid      bool
		expectedRowCount   string
		expectedColCount   string
		expectedHasHeaders string
		expectedSymbols    int
		expectedDefs       int
	}{
		{
			name:               "Valid CSV with clear headers",
			content:            "Name,Age,Salary,Active\nJohn,30,50000.50,true\nJane,25,45000.00,false",
			path:               "test.csv",
			expectedValid:      true,
			expectedRowCount:   "2",
			expectedColCount:   "4",
			expectedHasHeaders: "true",
			expectedSymbols:    4,
			expectedDefs:       4,
		},
		{
			name:               "CSV with text data (detected as headers)",
			content:            "John,30,New York\nJane,25,Boston\nBob,35,Chicago",
			path:               "test.csv",
			expectedValid:      true,
			expectedRowCount:   "2", // One row used as header
			expectedColCount:   "3",
			expectedHasHeaders: "true", // Algorithm detects headers due to text content
			expectedSymbols:    3,
			expectedDefs:       3,
		},
		{
			name:               "Numeric CSV with varying data",
			content:            "100,200,300\n101,201,301\n102,202,302",
			path:               "test.csv",
			expectedValid:      true,
			expectedRowCount:   "2", // Numbers are different, so first row detected as header
			expectedColCount:   "3",
			expectedHasHeaders: "true", // Different values trigger header detection
			expectedSymbols:    3,
			expectedDefs:       3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := plugin.ExtractMetadata(tt.content, tt.path)

			if !tt.expectedValid {
				if err == nil {
					t.Errorf("ExtractMetadata() expected error for invalid CSV, got nil")
				}
				if metadata.Properties["valid"] != "false" {
					t.Errorf("ExtractMetadata() valid = %v, want false", metadata.Properties["valid"])
				}
				return
			}

			if err != nil {
				t.Errorf("ExtractMetadata() unexpected error: %v", err)
				return
			}

			// Test basic properties
			if metadata.Language != "csv" {
				t.Errorf("ExtractMetadata() Language = %v, want csv", metadata.Language)
			}

			if metadata.Properties["valid"] != "true" {
				t.Errorf("ExtractMetadata() valid = %v, want true", metadata.Properties["valid"])
			}

			if metadata.Properties["row_count"] != tt.expectedRowCount {
				t.Errorf("ExtractMetadata() row_count = %v, want %v", metadata.Properties["row_count"], tt.expectedRowCount)
			}

			if metadata.Properties["column_count"] != tt.expectedColCount {
				t.Errorf("ExtractMetadata() column_count = %v, want %v", metadata.Properties["column_count"], tt.expectedColCount)
			}

			if metadata.Properties["has_headers"] != tt.expectedHasHeaders {
				t.Errorf("ExtractMetadata() has_headers = %v, want %v", metadata.Properties["has_headers"], tt.expectedHasHeaders)
			}

			// Test symbols (headers)
			if len(metadata.Symbols) != tt.expectedSymbols {
				t.Errorf("ExtractMetadata() created %d symbols, want %d", len(metadata.Symbols), tt.expectedSymbols)
			}

			// Test definitions
			if len(metadata.Definitions) != tt.expectedDefs {
				t.Errorf("ExtractMetadata() created %d definitions, want %d", len(metadata.Definitions), tt.expectedDefs)
			}

			// Test delimiter detection
			if metadata.Properties["delimiter"] == "" {
				t.Errorf("ExtractMetadata() delimiter not set")
			}

			// Test delimiter name
			if metadata.Properties["delimiter_name"] == "" {
				t.Errorf("ExtractMetadata() delimiter_name not set")
			}
		})
	}
}

func TestCSVPlugin_ChunkTypes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	plugin := csv.NewCSVPlugin(logger)

	// Test with headers and multiple chunks
	content := generateLargeCSV(50)
	chunks, err := plugin.Chunk(content, "test.csv", &model.CodeChunkingOptions{MaxLinesPerChunk: 10})

	if err != nil {
		t.Fatalf("Chunk() unexpected error: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("Expected chunks to be created")
	}

	// First chunk should be headers
	if chunks[0].Type != "csv_headers" {
		t.Errorf("First chunk type = %v, want csv_headers", chunks[0].Type)
	}

	// Remaining chunks should be data
	for i := 1; i < len(chunks); i++ {
		if chunks[i].Type != "csv_data" {
			t.Errorf("Chunk %d type = %v, want csv_data", i, chunks[i].Type)
		}
	}
}

func TestCSVPlugin_ChunkAnnotations(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	plugin := csv.NewCSVPlugin(logger)

	content := "Name,Age,City\nJohn,30,New York\nJane,25,Boston"
	chunks, err := plugin.Chunk(content, "test.csv", nil)

	if err != nil {
		t.Fatalf("Chunk() unexpected error: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}

	chunk := chunks[0]

	// Test required annotations
	requiredAnnotations := []string{"type", "row_count", "column_count", "has_headers", "delimiter"}
	for _, key := range requiredAnnotations {
		if _, exists := chunk.Annotations[key]; !exists {
			t.Errorf("Chunk missing required annotation: %s", key)
		}
	}

	// Test specific values
	if chunk.Annotations["type"] != "csv_document" {
		t.Errorf("Chunk annotation type = %v, want csv_document", chunk.Annotations["type"])
	}

	if chunk.Annotations["row_count"] != "2" {
		t.Errorf("Chunk annotation row_count = %v, want 2", chunk.Annotations["row_count"])
	}

	if chunk.Annotations["has_headers"] != "true" {
		t.Errorf("Chunk annotation has_headers = %v, want true", chunk.Annotations["has_headers"])
	}
}

func TestCSVPlugin_DataTypeDetection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	plugin := csv.NewCSVPlugin(logger)

	content := "Name,Age,Salary,Active,StartDate\nJohn,30,50000.50,true,2023-01-15\nJane,25,45000.00,false,2023-02-20"
	metadata, err := plugin.ExtractMetadata(content, "test.csv")

	if err != nil {
		t.Fatalf("ExtractMetadata() unexpected error: %v", err)
	}

	columnTypes := metadata.Properties["column_types"]
	if columnTypes == "" {
		t.Fatal("column_types property not set")
	}

	types := strings.Split(columnTypes, ",")
	expectedTypes := []string{"text", "integer", "float", "boolean", "date"}

	if len(types) != len(expectedTypes) {
		t.Errorf("Got %d column types, want %d", len(types), len(expectedTypes))
	}

	for i, expectedType := range expectedTypes {
		if i < len(types) && types[i] != expectedType {
			t.Errorf("Column %d type = %v, want %v", i, types[i], expectedType)
		}
	}
}

func TestCSVPlugin_EmptyAndEdgeCases(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	plugin := csv.NewCSVPlugin(logger)

	tests := []struct {
		name    string
		content string
		path    string
	}{
		{
			name:    "Empty content",
			content: "",
			path:    "empty.csv",
		},
		{
			name:    "Whitespace only",
			content: "   \n  \n  ",
			path:    "whitespace.csv",
		},
		{
			name:    "Single header",
			content: "Name",
			path:    "single.csv",
		},
		{
			name:    "Single cell",
			content: "John",
			path:    "cell.csv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test chunking doesn't crash
			chunks, err := plugin.Chunk(tt.content, tt.path, nil)
			if err != nil {
				t.Errorf("Chunk() unexpected error: %v", err)
			}

			// Empty/whitespace content should return empty chunks
			if strings.TrimSpace(tt.content) == "" && len(chunks) != 0 {
				t.Errorf("Expected 0 chunks for empty content, got %d", len(chunks))
			}

			// Test metadata extraction doesn't crash
			_, err = plugin.ExtractMetadata(tt.content, tt.path)
			// We don't test for specific error here as some cases may be valid
			_ = err
		})
	}
}

func TestCSVPlugin_DelimiterVariations(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	plugin := csv.NewCSVPlugin(logger)

	tests := []struct {
		name              string
		content           string
		path              string
		expectedDelimiter string
		expectedDelimName string
	}{
		{
			name:              "Comma separated",
			content:           "a,b,c\n1,2,3",
			path:              "test.csv",
			expectedDelimiter: ",",
			expectedDelimName: "comma",
		},
		{
			name:              "Tab separated",
			content:           "a\tb\tc\n1\t2\t3",
			path:              "test.csv",
			expectedDelimiter: "\t",
			expectedDelimName: "tab",
		},
		{
			name:              "Semicolon separated",
			content:           "a;b;c\n1;2;3",
			path:              "test.csv",
			expectedDelimiter: ";",
			expectedDelimName: "semicolon",
		},
		{
			name:              "Pipe separated",
			content:           "a|b|c\n1|2|3",
			path:              "test.csv",
			expectedDelimiter: "|",
			expectedDelimName: "pipe",
		},
		{
			name:              "TSV file extension forces tab",
			content:           "a,b,c\n1,2,3",
			path:              "test.tsv",
			expectedDelimiter: "\t",
			expectedDelimName: "tab",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := plugin.ExtractMetadata(tt.content, tt.path)
			if err != nil {
				t.Errorf("ExtractMetadata() unexpected error: %v", err)
				return
			}

			if metadata.Properties["delimiter"] != tt.expectedDelimiter {
				t.Errorf("delimiter = %v, want %v", metadata.Properties["delimiter"], tt.expectedDelimiter)
			}

			if metadata.Properties["delimiter_name"] != tt.expectedDelimName {
				t.Errorf("delimiter_name = %v, want %v", metadata.Properties["delimiter_name"], tt.expectedDelimName)
			}
		})
	}
}

func TestCSVPlugin_InvalidCSVHandling(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	plugin := csv.NewCSVPlugin(logger)

	// Test with truly malformed CSV that should cause parsing errors
	tests := []struct {
		name    string
		content string
		path    string
	}{
		{
			name:    "Unclosed quote at EOF",
			content: "name,age\n\"John Smith,30",
			path:    "test.csv",
		},
		{
			name:    "Mismatched column counts",
			content: "a,b,c\n1,2\n3,4,5,6",
			path:    "test.csv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// These should either work with LazyQuotes or return errors gracefully
			chunks, err := plugin.Chunk(tt.content, tt.path, nil)

			// If it errors, that's expected - if not, it should at least return something
			if err == nil && len(chunks) == 0 {
				t.Errorf("Expected either error or chunks, got neither")
			}

			// Same for metadata
			metadata, err := plugin.ExtractMetadata(tt.content, tt.path)
			if err == nil && metadata.Properties["valid"] != "true" && metadata.Properties["valid"] != "false" {
				t.Errorf("Expected valid property to be set")
			}
		})
	}
}

func TestCSVPlugin_FallbackBehavior(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	plugin := csv.NewCSVPlugin(logger)

	// Test that malformed CSV falls back to plain text processing
	content := "malformed\"csv\"content\nwith,inconsistent,field,counts,here\nsingle"

	chunks, err := plugin.Chunk(content, "malformed.csv", nil)
	if err != nil {
		t.Errorf("Chunk() unexpected error: %v", err)
		return
	}

	// Should create fallback chunk
	if len(chunks) == 0 {
		t.Fatal("Expected at least one fallback chunk")
	}

	// Check if it's a fallback chunk
	chunk := chunks[0]
	if chunk.Type != "csv_fallback" {
		t.Errorf("Expected fallback chunk type, got %v", chunk.Type)
	}
}

// Helper function to generate large CSV content for testing
func generateLargeCSV(rows int) string {
	var lines []string
	lines = append(lines, "Name,Age,City")

	for i := range rows {
		lines = append(lines, "Person"+string(rune('A'+i%26))+",25,City"+string(rune('A'+i%26)))
	}

	return strings.Join(lines, "\n")
}

// Helper function to generate valid numeric CSV with varying data
func generateValidNumericCSV(rows int) string {
	var lines []string

	for range rows {
		lines = append(lines, "100,200,300")
	}

	return strings.Join(lines, "\n")
}
