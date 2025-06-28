package csv

import (
	"encoding/csv"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/sevigo/goframe/schema"
)

// CSVStructure represents the analyzed structure of a CSV file
type CSVStructure struct {
	Headers     []string
	Records     [][]string
	Delimiter   rune
	HasHeaders  bool
	RowCount    int
	ColumnCount int
}

// Chunk breaks CSV into semantic chunks based on rows or logical sections
func (p *CSVPlugin) Chunk(content string, path string, opts *schema.CodeChunkingOptions) ([]schema.CodeChunk, error) {
	if strings.TrimSpace(content) == "" {
		return []schema.CodeChunk{}, nil
	}

	structure, err := p.parseCSVStructure(content, path)
	if err != nil {
		p.logger.Warn("Failed to parse CSV, treating as plain text", "error", err)
		return p.createFallbackChunk(content, path), nil
	}

	chunks := p.createChunks(structure, content, path, opts)

	p.logger.Debug("CSV chunking completed", "path", path, "chunks_created", len(chunks))
	return chunks, nil
}

// parseCSVStructure analyzes the CSV file structure
func (p *CSVPlugin) parseCSVStructure(content string, path string) (*CSVStructure, error) {
	delimiter := p.detectDelimiter(content, path)

	reader := csv.NewReader(strings.NewReader(content))
	reader.Comma = delimiter
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV: %w", err)
	}

	if len(records) == 0 {
		return nil, errors.New("empty CSV file")
	}

	structure := &CSVStructure{
		Records:     records,
		Delimiter:   delimiter,
		RowCount:    len(records),
		ColumnCount: len(records[0]),
	}

	// Detect if first row contains headers
	structure.HasHeaders = p.detectHeaders(records)

	if structure.HasHeaders && len(records) > 0 {
		structure.Headers = records[0]
		structure.Records = records[1:] // Remove header row from records
		structure.RowCount--            // Adjust row count
	}

	return structure, nil
}

// detectDelimiter attempts to detect the CSV delimiter
func (p *CSVPlugin) detectDelimiter(content string, path string) rune {
	// Check file extension first
	if strings.HasSuffix(strings.ToLower(path), ".tsv") {
		return '\t'
	}

	// Count occurrences of common delimiters in first few lines
	lines := strings.Split(content, "\n")
	sampleLines := 3
	if len(lines) < sampleLines {
		sampleLines = len(lines)
	}

	sample := strings.Join(lines[:sampleLines], "\n")

	delimiters := map[rune]int{
		',':  strings.Count(sample, ","),
		';':  strings.Count(sample, ";"),
		'\t': strings.Count(sample, "\t"),
		'|':  strings.Count(sample, "|"),
	}

	// Find delimiter with highest count
	maxCount := 0
	bestDelimiter := ','

	for delim, count := range delimiters {
		if count > maxCount {
			maxCount = count
			bestDelimiter = delim
		}
	}

	return bestDelimiter
}

// detectHeaders determines if the first row contains headers
func (p *CSVPlugin) detectHeaders(records [][]string) bool {
	if len(records) < 2 {
		return true // Assume single row is headers
	}

	firstRow := records[0]
	secondRow := records[1]

	if len(firstRow) != len(secondRow) {
		return false
	}

	// Check if first row looks like headers
	headerScore := 0
	for i, cell := range firstRow {
		// Headers are usually text
		if p.looksLikeText(cell) {
			headerScore++
		}

		// Headers are usually different from data
		if len(records) > 1 && cell != secondRow[i] {
			headerScore++
		}

		// Headers are usually shorter
		if len(cell) < 30 {
			headerScore++
		}
	}

	// If more than half the criteria suggest headers, assume they exist
	return headerScore > len(firstRow)
}

// looksLikeText determines if a cell value looks like text rather than a number
func (p *CSVPlugin) looksLikeText(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}

	// Try to parse as number
	_, err := strconv.ParseFloat(value, 64)
	return err != nil
}

// createChunks creates chunks based on CSV structure and size constraints
func (p *CSVPlugin) createChunks(
	structure *CSVStructure,
	content string,
	path string,
	opts *schema.CodeChunkingOptions,
) []schema.CodeChunk {
	var chunks []schema.CodeChunk

	// For small CSV files, create a single chunk
	if structure.RowCount <= 20 {
		return []schema.CodeChunk{p.createSingleChunk(structure, content, path)}
	}

	// Determine rows per chunk
	rowsPerChunk := p.calculateRowsPerChunk(structure, opts)

	// Create header chunk if headers exist
	if structure.HasHeaders {
		headerChunk := p.createHeaderChunk(structure, path)
		chunks = append(chunks, headerChunk)
	}

	// Create data chunks
	dataChunks := p.createDataChunks(structure, path, rowsPerChunk)
	chunks = append(chunks, dataChunks...)

	return chunks
}

// calculateRowsPerChunk determines optimal number of rows per chunk
func (p *CSVPlugin) calculateRowsPerChunk(structure *CSVStructure, opts *schema.CodeChunkingOptions) int {
	defaultRowsPerChunk := 100

	if opts == nil {
		return defaultRowsPerChunk
	}

	// Estimate based on MaxLinesPerChunk
	if opts.MaxLinesPerChunk > 0 {
		// Reserve some lines for headers and formatting
		return opts.MaxLinesPerChunk - 5
	}

	// Estimate based on chunk size (rough calculation)
	if opts.ChunkSize > 0 {
		avgRowLength := p.estimateAverageRowLength(structure)
		if avgRowLength > 0 {
			return opts.ChunkSize / avgRowLength
		}
	}

	return defaultRowsPerChunk
}

// estimateAverageRowLength estimates the average character length of a row
func (p *CSVPlugin) estimateAverageRowLength(structure *CSVStructure) int {
	if len(structure.Records) == 0 {
		return 50 // Default estimate
	}

	totalLength := 0
	sampleSize := 10
	if len(structure.Records) < sampleSize {
		sampleSize = len(structure.Records)
	}

	for i := range sampleSize {
		row := structure.Records[i]
		rowLength := 0
		for _, cell := range row {
			rowLength += len(cell) + 1 // +1 for delimiter
		}
		totalLength += rowLength
	}

	return totalLength / sampleSize
}

// createSingleChunk creates a single chunk for small CSV files
func (p *CSVPlugin) createSingleChunk(structure *CSVStructure, content string, path string) schema.CodeChunk {
	lines := strings.Split(content, "\n")
	identifier := p.generateFileIdentifier(path)

	annotations := map[string]string{
		"type":         "csv_document",
		"row_count":    strconv.Itoa(structure.RowCount),
		"column_count": strconv.Itoa(structure.ColumnCount),
		"has_headers":  strconv.FormatBool(structure.HasHeaders),
		"delimiter":    string(structure.Delimiter),
	}

	if structure.HasHeaders {
		annotations["headers"] = strings.Join(structure.Headers, ",")
	}

	return schema.CodeChunk{
		Content:     content,
		LineStart:   1,
		LineEnd:     len(lines),
		Type:        "csv_document",
		Identifier:  identifier,
		Annotations: annotations,
	}
}

// createHeaderChunk creates a chunk for CSV headers
func (p *CSVPlugin) createHeaderChunk(structure *CSVStructure, path string) schema.CodeChunk {
	headerContent := p.formatCSVRow(structure.Headers, structure.Delimiter)
	identifier := fmt.Sprintf("%s - Headers", p.generateFileIdentifier(path))

	annotations := map[string]string{
		"type":         "csv_headers",
		"column_count": strconv.Itoa(len(structure.Headers)),
		"delimiter":    string(structure.Delimiter),
		"headers":      strings.Join(structure.Headers, ","),
	}

	return schema.CodeChunk{
		Content:     headerContent,
		LineStart:   1,
		LineEnd:     1,
		Type:        "csv_headers",
		Identifier:  identifier,
		Annotations: annotations,
	}
}

// createDataChunks creates chunks for CSV data rows
func (p *CSVPlugin) createDataChunks(structure *CSVStructure, path string, rowsPerChunk int) []schema.CodeChunk {
	var chunks []schema.CodeChunk

	baseIdentifier := p.generateFileIdentifier(path)
	startLine := 1
	if structure.HasHeaders {
		startLine = 2 // Account for header line
	}

	for i := 0; i < len(structure.Records); i += rowsPerChunk {
		end := i + rowsPerChunk
		if end > len(structure.Records) {
			end = len(structure.Records)
		}

		chunkRecords := structure.Records[i:end]
		chunkContent := p.formatCSVRows(chunkRecords, structure.Delimiter)

		chunkNumber := (i / rowsPerChunk) + 1
		identifier := fmt.Sprintf("%s - Rows %d-%d", baseIdentifier, i+1, end)

		annotations := map[string]string{
			"type":         "csv_data",
			"chunk_number": strconv.Itoa(chunkNumber),
			"start_row":    strconv.Itoa(i + 1),
			"end_row":      strconv.Itoa(end),
			"row_count":    strconv.Itoa(len(chunkRecords)),
			"column_count": strconv.Itoa(structure.ColumnCount),
			"delimiter":    string(structure.Delimiter),
		}

		if structure.HasHeaders {
			annotations["headers"] = strings.Join(structure.Headers, ",")
		}

		chunk := schema.CodeChunk{
			Content:     chunkContent,
			LineStart:   startLine + i,
			LineEnd:     startLine + end - 1,
			Type:        "csv_data",
			Identifier:  identifier,
			Annotations: annotations,
		}

		chunks = append(chunks, chunk)
	}

	return chunks
}

// formatCSVRow formats a single row back to CSV format
func (p *CSVPlugin) formatCSVRow(row []string, delimiter rune) string {
	var result strings.Builder

	for i, cell := range row {
		if i > 0 {
			result.WriteRune(delimiter)
		}

		// Quote cell if it contains delimiter, quotes, or newlines
		if p.needsQuoting(cell, delimiter) {
			result.WriteString(fmt.Sprintf("\"%s\"", strings.ReplaceAll(cell, "\"", "\"\"")))
		} else {
			result.WriteString(cell)
		}
	}

	return result.String()
}

// formatCSVRows formats multiple rows back to CSV format
func (p *CSVPlugin) formatCSVRows(rows [][]string, delimiter rune) string {
	var lines []string

	for _, row := range rows {
		lines = append(lines, p.formatCSVRow(row, delimiter))
	}

	return strings.Join(lines, "\n")
}

// needsQuoting determines if a CSV cell needs to be quoted
func (p *CSVPlugin) needsQuoting(cell string, delimiter rune) bool {
	return strings.ContainsRune(cell, delimiter) ||
		strings.Contains(cell, "\"") ||
		strings.Contains(cell, "\n") ||
		strings.Contains(cell, "\r")
}

// generateFileIdentifier creates an identifier from the filename
func (p *CSVPlugin) generateFileIdentifier(path string) string {
	filename := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if filename == "" {
		return "CSV Data"
	}

	// Convert to title case
	identifier := strings.ReplaceAll(filename, "_", " ")
	identifier = strings.ReplaceAll(identifier, "-", " ")
	identifier = cases.Title(language.English).String(identifier)

	return identifier
}

// createFallbackChunk creates a fallback chunk when CSV parsing fails
func (p *CSVPlugin) createFallbackChunk(content string, path string) []schema.CodeChunk {
	lines := strings.Split(content, "\n")
	identifier := p.generateFileIdentifier(path)

	chunk := schema.CodeChunk{
		Content:    content,
		LineStart:  1,
		LineEnd:    len(lines),
		Type:       "csv_fallback",
		Identifier: identifier,
		Annotations: map[string]string{
			"type":   "csv_fallback",
			"reason": "parsing_failed",
		},
	}

	return []schema.CodeChunk{chunk}
}
