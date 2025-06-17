package textsplitter

import (
	"context"

	"github.com/sevigo/goframe/schema"
)

type TextSplitter interface {
	SplitDocuments(ctx context.Context, docs []schema.Document) ([]schema.Document, error)
}
