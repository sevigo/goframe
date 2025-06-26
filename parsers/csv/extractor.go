package csv

import (
	"fmt"
	"strconv"
	"strings"

	model "github.com/sevigo/goframe/schema"
)

// ExtractMetadata extracts metadata from CSV content
func (p *CSVPlugin) ExtractMetadata(content string, path string) (model.FileMetadata, error) {
	metadata := model.FileMetadata{
		Language:    "csv",
		FilePath:    path,
		Imports:     []string{},
		Definitions: []model.CodeEntityDefinition{},
		Symbols:     []model.CodeSymbol{},
		Properties:  map[string]string{},
	}

	structure, err := p.parseCSVStructure(content, path)
	if err != nil {
		metadata.Properties["error"] = err.Error()
		metadata.Properties["valid"] = "false"
		return metadata, err
	}

	metadata.Properties["valid"] = "true"

	lines := strings.Split(content, "\n")
	metadata.Properties["total_lines"] = strconv.Itoa(len(lines))
	metadata.Properties["size_bytes"] = strconv.Itoa(len(content))

	p.extractCSVMetadata(structure, &metadata)
	p.analyzeDataTypes(structure, &metadata)

	return metadata, nil
}

// extractCSVMetadata extracts CSV-specific metadata
func (p *CSVPlugin) extractCSVMetadata(structure *CSVStructure, metadata *model.FileMetadata) {
	metadata.Properties["row_count"] = strconv.Itoa(structure.RowCount)
	metadata.Properties["column_count"] = strconv.Itoa(structure.ColumnCount)
	metadata.Properties["has_headers"] = strconv.FormatBool(structure.HasHeaders)
	metadata.Properties["delimiter"] = string(structure.Delimiter)

	// Add delimiter name
	switch structure.Delimiter {
	case ',':
		metadata.Properties["delimiter_name"] = "comma"
	case ';':
		metadata.Properties["delimiter_name"] = "semicolon"
	case '\t':
		metadata.Properties["delimiter_name"] = "tab"
	case '|':
		metadata.Properties["delimiter_name"] = "pipe"
	default:
		metadata.Properties["delimiter_name"] = "other"
	}

	if structure.HasHeaders {
		metadata.Properties["headers"] = strings.Join(structure.Headers, ",")

		for i, header := range structure.Headers {
			symbol := model.CodeSymbol{
				Name:      header,
				Type:      "column",
				LineStart: 1,
				LineEnd:   1,
				IsExport:  true,
			}
			metadata.Symbols = append(metadata.Symbols, symbol)

			def := model.CodeEntityDefinition{
				Type:       "column",
				Name:       header,
				LineStart:  1,
				LineEnd:    1,
				Visibility: "public",
				Signature:  fmt.Sprintf("column[%d]", i),
			}
			metadata.Definitions = append(metadata.Definitions, def)
		}
	}

	if structure.RowCount > 0 && structure.ColumnCount > 0 {
		totalCells := structure.RowCount * structure.ColumnCount
		emptyCells := p.countEmptyCells(structure)
		density := float64(totalCells-emptyCells) / float64(totalCells)
		metadata.Properties["data_density"] = strconv.FormatFloat(density, 'f', 3, 64)
		metadata.Properties["empty_cells"] = strconv.Itoa(emptyCells)
	}
}

// analyzeDataTypes analyzes the data types in each column
func (p *CSVPlugin) analyzeDataTypes(structure *CSVStructure, metadata *model.FileMetadata) {
	if structure.ColumnCount == 0 || len(structure.Records) == 0 {
		return
	}

	columnTypes := make([]string, structure.ColumnCount)

	for colIndex := range structure.ColumnCount {
		columnTypes[colIndex] = p.detectColumnType(structure, colIndex)
	}

	metadata.Properties["column_types"] = strings.Join(columnTypes, ",")

	typeCounts := make(map[string]int)
	for _, colType := range columnTypes {
		typeCounts[colType]++
	}

	for dataType, count := range typeCounts {
		metadata.Properties[fmt.Sprintf("%s_columns", dataType)] = strconv.Itoa(count)
	}
}

// detectColumnType analyzes a column to determine its data type
func (p *CSVPlugin) detectColumnType(structure *CSVStructure, columnIndex int) string {
	if columnIndex >= structure.ColumnCount {
		return "unknown"
	}

	sampleSize := 20
	if len(structure.Records) < sampleSize {
		sampleSize = len(structure.Records)
	}

	typeScores := map[string]int{
		"integer": 0,
		"float":   0,
		"boolean": 0,
		"date":    0,
		"text":    0,
		"empty":   0,
	}

	for i := range sampleSize {
		if i >= len(structure.Records) {
			break
		}

		row := structure.Records[i]
		if columnIndex >= len(row) {
			typeScores["empty"]++
			continue
		}

		cell := strings.TrimSpace(row[columnIndex])
		if cell == "" {
			typeScores["empty"]++
			continue
		}

		if p.isBooleanValue(cell) {
			typeScores["boolean"]++
			continue
		}

		if p.isIntegerValue(cell) {
			typeScores["integer"]++
			continue
		}

		if p.isFloatValue(cell) {
			typeScores["float"]++
			continue
		}

		if p.isDateValue(cell) {
			typeScores["date"]++
			continue
		}

		typeScores["text"]++
	}

	maxScore := 0
	detectedType := "text"

	for dataType, score := range typeScores {
		if score > maxScore {
			maxScore = score
			detectedType = dataType
		}
	}

	return detectedType
}

// Type detection helper functions
func (p *CSVPlugin) isBooleanValue(value string) bool {
	lower := strings.ToLower(value)
	booleans := []string{"true", "false", "yes", "no", "y", "n", "1", "0", "on", "off"}

	for _, b := range booleans {
		if lower == b {
			return true
		}
	}
	return false
}

func (p *CSVPlugin) isIntegerValue(value string) bool {
	_, err := strconv.ParseInt(value, 10, 64)
	return err == nil
}

func (p *CSVPlugin) isFloatValue(value string) bool {
	_, err := strconv.ParseFloat(value, 64)
	return err == nil
}

func (p *CSVPlugin) isDateValue(value string) bool {
	// Simple date pattern detection
	datePatterns := []string{
		"2006-01-02", "01/02/2006", "02/01/2006", "2006/01/02",
		"Jan 2, 2006", "2 Jan 2006", "January 2, 2006",
	}

	for _, pattern := range datePatterns {
		if len(value) == len(pattern) {
			// Basic pattern matching (could be improved with actual date parsing)
			if strings.Count(value, "-") == 2 || strings.Count(value, "/") == 2 || strings.Contains(value, " ") {
				return true
			}
		}
	}
	return false
}

// countEmptyCells counts the number of empty cells in the CSV
func (p *CSVPlugin) countEmptyCells(structure *CSVStructure) int {
	emptyCells := 0

	for _, row := range structure.Records {
		for _, cell := range row {
			if strings.TrimSpace(cell) == "" {
				emptyCells++
			}
		}
	}

	return emptyCells
}
