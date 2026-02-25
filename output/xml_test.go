package output

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type testXMLData struct {
	Name string `xml:"name"`
	Age  int    `xml:"age"`
}

func TestXMLParser_Parse(t *testing.T) {
	parser := NewXMLParser[testXMLData]("user")
	ctx := context.Background()

	tests := []struct {
		name    string
		input   string
		want    testXMLData
		wantErr bool
	}{
		{
			name:  "valid xml",
			input: `<user><name>Alice</name><age>30</age></user>`,
			want:  testXMLData{Name: "Alice", Age: 30},
		},
		{
			name:  "xml with markdown fences",
			input: "```xml\n<user><name>Bob</name><age>25</age></user>\n```",
			want:  testXMLData{Name: "Bob", Age: 25},
		},
		{
			name:  "xml with preamble",
			input: "Here is the user:\n<user><name>Charlie</name><age>35</age></user>",
			want:  testXMLData{Name: "Charlie", Age: 35},
		},
		{
			name:  "truncation recovery (missing closing tag)",
			input: "<user><name>Dave</name><age>40</age>",
			want:  testXMLData{Name: "Dave", Age: 40},
		},
		{
			name:  "case-insensitive root tag matching",
			input: "<User><name>Eve</name><age>22</age></User>",
			want:  testXMLData{Name: "Eve", Age: 22},
		},
		{
			name:  "tokenization artifact fixing",
			input: "<user><name>Frank</name><age>50</age></ user>",
			want:  testXMLData{Name: "Frank", Age: 50},
		},
		{
			name:    "invalid xml (no root tag)",
			input:   "<other><name>Grace</name></other>",
			wantErr: true,
		},
		{
			name: "invalid xml (no root tag)",
			input: `<comment>this is a comment with "1 < 2"</comment>`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.Parse(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestXMLParser_ContextCancellation(t *testing.T) {
	parser := NewXMLParser[testXMLData]("user")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := parser.Parse(ctx, `<user><name>Alice</name><age>30</age></user>`)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
