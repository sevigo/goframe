package output

import (
	"context"
	"errors"
	"testing"
)

type testData struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestJSONParser_Parse(t *testing.T) {
	parser := NewJSONParser[testData]()
	ctx := context.Background()

	tests := []struct {
		name      string
		input     string
		want      testData
		wantErr   bool
		checkType bool
	}{
		{
			name:  "valid json",
			input: `{"name": "Alice", "age": 30}`,
			want:  testData{Name: "Alice", Age: 30},
		},
		{
			name:  "json with markdown fences",
			input: "```json\n{\"name\": \"Bob\", \"age\": 25}\n```",
			want:  testData{Name: "Bob", Age: 25},
		},
		{
			name:  "json with plain code block",
			input: "```\n{\"name\": \"Charlie\", \"age\": 35}\n```",
			want:  testData{Name: "Charlie", Age: 35},
		},
		{
			name:  "json with preamble and postamble",
			input: "Here is the result:\n{\"name\": \"Dave\", \"age\": 40}\nHope this helps!",
			want:  testData{Name: "Dave", Age: 40},
		},
		{
			name:    "invalid json",
			input:   "not a json",
			wantErr: true,
		},
		{
			name:      "nested array (as input)",
			input:     "Results: [{\"name\": \"Eve\", \"age\": 22}]",
			checkType: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.checkType {
				sliceParser := NewJSONParser[[]testData]()
				got, err := sliceParser.Parse(ctx, tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if !tt.wantErr && (len(got) != 1 || got[0].Name != "Eve") {
					t.Errorf("Parse() got = %v, want Eve", got)
				}
				return
			}
			got, err := parser.Parse(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Parse() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJSONParser_ContextCancellation(t *testing.T) {
	parser := NewJSONParser[testData]()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := parser.Parse(ctx, `{"name": "Alice", "age": 30}`)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
