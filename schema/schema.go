package schema

import (
	"context"
	"fmt"
)

type Document struct {
	PageContent string
	Metadata    map[string]any
}

func NewDocument(content string, metadata map[string]any) Document {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	return Document{
		PageContent: content,
		Metadata:    metadata,
	}
}

func (d Document) String() string {
	return d.PageContent
}

type ModelDetails struct {
	Family        string
	ParameterSize string
	Quantization  string
	Dimension     int64
}

func (md ModelDetails) String() string {
	return fmt.Sprintf("%s (%s, %s, dim: %d)",
		md.Family, md.ParameterSize, md.Quantization, md.Dimension)
}

type Retriever interface {
	GetRelevantDocuments(ctx context.Context, query string) ([]Document, error)
}
