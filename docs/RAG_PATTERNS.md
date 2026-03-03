# GoFrame RAG Patterns

This document describes the core patterns and interfaces used for building Retrieval-Augmented Generation (RAG) applications with GoFrame.

## Core Interfaces

### Schema Interfaces

```go
// Document - The fundamental unit of retrieval
type Document struct {
    PageContent string                 // The text content
    Metadata    map[string]any         // Structured metadata for filtering
    Sparse      *SparseVector          // Optional sparse vector for hybrid search
}

// Retriever - Abstracts document retrieval
type Retriever interface {
    GetRelevantDocuments(ctx context.Context, query string) ([]Document, error)
}

// Reranker - Re-orders documents by relevance
type Reranker interface {
    Rerank(ctx context.Context, query string, docs []Document) ([]ScoredDocument, error)
}
```

### VectorStore Interface

```go
type VectorStore interface {
    // Add documents to the store
    AddDocuments(ctx context.Context, docs []Document, options ...Option) ([]string, error)

    // Search with various options
    SimilaritySearch(ctx context.Context, query string, numDocuments int, options ...Option) ([]Document, error)
    SimilaritySearchWithScores(ctx context.Context, query string, numDocuments int, options ...Option) ([]DocumentWithScore, error)

    // Batch operations
    SimilaritySearchBatch(ctx context.Context, queries []string, numDocuments int, options ...Option) ([][]Document, error)

    // Collection management
    ListCollections(ctx context.Context) ([]string, error)
    DeleteCollection(ctx context.Context, collectionName string) error
}
```

## Search Options

GoFrame uses the functional options pattern for search configuration:

```go
// Hybrid search with sparse vectors
sparseVec, _ := sparse.GenerateSparseVector(ctx, query)
docs, _ := store.SimilaritySearch(ctx, query, 5,
    WithSparseQuery(sparseVec),
    WithFilters(map[string]any{"package_name": "myapp"}),
    WithScoreThreshold(0.7))

// Available options:
WithSparseQuery(*SparseVector)      // Enable hybrid search
WithFilters(map[string]any)         // Metadata filtering
WithScoreThreshold(float32)         // Minimum score cutoff
WithEmbedder(Embedder)              // Override embedder
WithCollectionName(string)          // Target collection
```

## Chain Patterns

### LLMChain - Basic Prompt + LLM + Parser

```go
// Simple string output
chain := chains.NewLLMChain[string](
    llmModel,
    prompts.NewPromptTemplate("Hello, {{.name}}!"),
)
result, _ := chain.Call(ctx, map[string]string{"name": "World"})

// Structured output with custom parser
type Review struct {
    Summary string
    Score   int
}

parser := &ReviewParser{}
chain := chains.NewLLMChain[*Review](
    llmModel,
    prompts.NewPromptTemplate(templateStr),
    chains.WithOutputParser[*Review](parser),
)
review, _ := chain.Call(ctx, nil)
```

### RetrievalQA - Standard RAG Pattern

```go
retriever := vectorstores.ToRetriever(store, 5)
chain := chains.NewRetrievalQA(
    retriever,
    generatorLLM,
    chains.WithPromptBuilder(func(query string, docs []Document) (string, error) {
        context := buildContext(docs)
        return fmt.Sprintf("Context: %s\n\nQuestion: %s", context, query), nil
    }),
)
answer, _ := chain.Call(ctx, query)
```

### ValidatingRetrievalQA - RAG with Context Validation

```go
// Validates retrieved context before generation
chain, _ := chains.NewValidatingRetrievalQA(
    retriever,
    generatorLLM,
    chains.WithValidator(validatorLLM),  // Required
    chains.WithLogger(logger),
)

answer, _ := chain.Call(ctx, query)
// If context is irrelevant, falls back to direct generation
```

**When to use:** When hallucination risk is unacceptable and you have a fast validator LLM available.

### MapReduceChain - Parallel Processing

```go
// Process multiple models in parallel, then synthesize results
chain := chains.NewMapReduceChain[string, ComparisonResult, string](
    // Map: Process each input concurrently
    func(ctx context.Context, modelName string) (ComparisonResult, error) {
        llm, _ := getLLM(modelName)
        response, _ := llm.Call(ctx, prompt)
        return ComparisonResult{Model: modelName, Review: response}, nil
    },
    // Reduce: Synthesize results
    func(ctx context.Context, results []ComparisonResult) (string, error) {
        return synthesizeConsensus(results), nil
    },
    chains.WithMaxConcurrency[string, ComparisonResult, string](5),
    chains.WithQuorum[string, ComparisonResult, string](0.66),  // Need 2/3 success
)

consensus, _ := chain.Call(ctx, []string{"model1", "model2", "model3"})
```

**When to use:** Consensus generation, ensemble methods, batch processing.

## Retriever Patterns

### ToRetriever - Adapter from VectorStore

```go
retriever := vectorstores.ToRetriever(store, 5,
    WithSparseQuery(sparseVec),
    WithFilters(filters))
```

### RerankingRetriever - Two-Stage Retrieval

```go
baseRetriever := vectorstores.ToRetriever(store, 20)  // Get more candidates

reranker := vectorstores.RerankingRetriever{
    Retriever: baseRetriever,
    Reranker:  customReranker,
    TopK:      5,  // Return top 5 after reranking
    CandidateFilter: func(query string, docs []Document) []Document {
        // Pre-filter before expensive reranking
        return preFilterBM25(query, docs, 10)
    },
}

docs, _ := reranker.GetRelevantDocuments(ctx, query)
```

### HyDERetriever - Query Expansion

```go
retriever := vectorstores.NewHyDERetriever(
    baseRetriever,
    // Function to generate hypothetical document
    func(ctx context.Context, query string) (string, error) {
        return llm.Call(ctx, hydePrompt(query))
    },
    vectorstores.WithNumGenerations(3),  // Generate 3 variations
)

docs, _ := retriever.GetRelevantDocuments(ctx, patch)
```

### MultiQueryRetriever - Query Variations

```go
retriever := vectorstores.MultiQueryRetriever{
    Store:        scopedStore,
    LLM:          queryLLM,
    NumDocuments: 10,
    Count:        3,  // Generate 3 query variations
    SparseGenFunc: func(ctx context.Context, queries []string) ([]*SparseVector, error) {
        // Generate sparse vectors for each query
        var vecs []*SparseVector
        for _, q := range queries {
            v, _ := sparse.GenerateSparseVector(ctx, q)
            vecs = append(vecs, v)
        }
        return vecs, nil
    },
}

docs, _ := retriever.GetRelevantDocuments(ctx, description)
```

### DependencyRetriever - Graph Traversal

```go
retriever, err := vectorstores.NewDependencyRetriever(store)
if err != nil {
    log.Fatal(err)
}
network, _ := retriever.GetContextNetwork(ctx, packageName, imports)

// network.Dependencies - upstream dependencies
// network.Dependents - downstream impact
```

### DefinitionRetriever - Symbol Lookup

```go
retriever, err := vectorstores.NewDefinitionRetriever(store)
if err != nil {
    log.Fatal(err)
}
docs, _ := retriever.GetDefinition(ctx, "MyFunction")
// Returns document where identifier="MyFunction" AND is_definition=true
// Now uses hybrid search (dense + sparse) for better exact matching
```

## Text Splitting

### RecursiveCharacter - General Purpose

```go
splitter := textsplitter.NewRecursiveCharacter(
    textsplitter.WithChunkSize(1000),
    textsplitter.WithChunkOverlap(200),
)

chunks, _ := splitter.SplitText(ctx, content)
```

### Code-Aware Splitting

For code, use language-specific splitters that respect AST boundaries:

```go
// Go-specific chunking
parser, _ := registry.GetParserForFile("main.go", nil)
chunks, _ := parser.Chunk(ctx, content, "main.go")
// Chunks include metadata: package_name, imports, identifier, chunk_type
```

## Prompt Templates

```go
// Define template
template := prompts.NewPromptTemplate(`
Use the following context to answer the question.

Context:
{{.context}}

Question: {{.query}}

Answer:`)

// Render
prompt := template.Format(map[string]string{
    "context": contextStr,
    "query":   question,
})
```

## Sparse Vectors

Sparse vectors enable keyword-based filtering alongside dense embeddings:

```go
// Generate sparse vector (keyword-weighted)
sparseVec, err := sparse.GenerateSparseVector(ctx, query)

// Use in search
docs, _ := store.SimilaritySearch(ctx, query, 5,
    WithSparseQuery(sparseVec))

// Sparse vectors are stored in documents
doc := schema.NewDocument(content, metadata)
doc.Sparse = sparseVec  // For hybrid indexing
```

## Best Practices

### 1. Always Propagate Context

```go
func (r *Service) Search(ctx context.Context, query string) ([]Document, error) {
    // Check for cancellation
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    // ... do work
}
```

### 2. Use Functional Options

```go
func NewService(opts ...Option) *Service {
    s := &Service{defaults...}
    for _, opt := range opts {
        opt(s)
    }
    return s
}
```

### 3. Handle Empty Results Gracefully

```go
docs, _ := retriever.GetRelevantDocuments(ctx, query)
if len(docs) == 0 {
    logger.Info("no documents found, using direct generation")
    return llm.Call(ctx, query)  // Fallback
}
```

### 4. Deduplicate Results

```go
seenDocs := make(map[string]struct{})
for _, doc := range docs {
    key := getDocKey(doc)  // Use parent_id if available
    if _, exists := seenDocs[key]; exists {
        continue
    }
    seenDocs[key] = struct{}{}
    // Process unique document
}
```

### 5. Validate Retrieved Context

```go
if contextString == "" {
    logger.Warn("no context retrieved - high hallucination risk")
    // Add warning to prompt or abort
}
```

## Testing Patterns

### Mock Retriever

```go
type MockRetriever struct {
    Documents []Document
}

func (m *MockRetriever) GetRelevantDocuments(ctx context.Context, query string) ([]Document, error) {
    return m.Documents, nil
}
```

### Fake Vector Store

```go
import "github.com/sevigo/goframe/vectorstores/fake"

store := fake.NewVectorStore()
store.AddDocuments(ctx, docs)
results, _ := store.SimilaritySearch(ctx, "query", 5)
```

## Common Pitfalls

1. **Not checking for nil retriever/LLM:**
   ```go
   chain, err := chains.NewValidatingRetrievalQA(retriever, generator, ...)
   if err != nil {
       return err  // Catches nil inputs
   }
   ```

2. **Forgetting context cancellation in loops:**
   ```go
   for _, query := range queries {
       select {
       case <-ctx.Done():
           return ctx.Err()
       default:
       }
       // ... process
   }
   ```

3. **Not deduplicating multi-stage results:**
   ```go
   // When combining results from multiple retrievers:
   seenDocs := make(map[string]struct{})
   for _, doc := range allDocs {
       key := getDocKey(doc)
       if _, exists := seenDocs[key]; exists {
           continue  // Skip duplicate
       }
       seenDocs[key] = struct{}{}
   }
   ```

4. **Ignoring sparse vector errors:**
   ```go
   sparseVec, err := sparse.GenerateSparseVector(ctx, query)
   if err != nil {
       logger.Warn("sparse generation failed, using dense only", "error", err)
       // Continue without sparse - don't fail the whole operation
   }
   ```
