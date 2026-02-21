# GoFrame Chain Patterns — Implementation Plan

## Current State Analysis

### What GoFrame has today
- **`prompts.PromptTemplate`** — Simple `{{.var}}` string substitution, returns `string`
- **`llms.Model`** — LLM interface with `Call(ctx, prompt) (string, error)` and `GenerateContent()`
- **`schema.Retriever`** — `GetRelevantDocuments(ctx, query) ([]Document, error)`
- **`chains.RetrievalQA`** — Retriever → LLM chain with `PromptBuilder` option (just merged)
- **`chains.ValidatingRetrievalQA`** — Retriever → Validator LLM → Generator LLM chain
- **`vectorstores.MultiQueryRetriever`** — Generates query variations via LLM, batch-searches, deduplicates
- **`vectorstores.RerankingRetriever`** — Wraps a retriever with a reranker

### What's missing (and why it matters)
1. **No generic `LLMChain`** — Every consumer manually calls prompt.Format(), LLM.Call(), then parses. 
   This 3-step pattern is repeated everywhere in Code-Warden.
2. **No `OutputParser` interface** — There's no standard way to parse LLM output into structured types.
   Code-Warden has its own `llm.ParseMarkdownReview()` which is hardcoded.
3. **No `MapReduceChain`** — Code-Warden manually manages goroutines, semaphores, quorum logic
   for consensus reviews. This is complex infrastructure that belongs in the framework.
4. **No `HyDERetriever`** — Code-Warden's `rag_hyde.go` has ~150 lines of channel/goroutine 
   orchestration for hypothetical document embedding. This is a standard retrieval pattern.

---

## Design Decisions & Trade-offs

### Generics: `LLMChain[T any]` vs `LLMChain` with `any` return

**Decision: Use generics (`LLMChain[T any]`)**

Go 1.25.6 supports generics. Using `LLMChain[T any]` gives type-safe output parsing:
- `LLMChain[string]` returns raw LLM output (no parser needed)  
- `LLMChain[StructuredReview]` returns a parsed struct

The alternative (returning `any` and casting) loses compile-time safety and feels un-Go-like.

**Caveat**: `LLMChain[T]` cannot implement `schema.Retriever` or be composed with other
chains that expect `string` output. This is fine — `LLMChain` is a terminal execution unit,
not a Retriever. Composition happens via function types, not interface chaining.

### OutputParser: Interface in `schema` or in `chains`?

**Decision: Define `OutputParser[T]` in `schema/`**

This allows Code-Warden to implement parsers without depending on `chains/`. The interface
is minimal:
```go
type OutputParser[T any] interface {
    Parse(ctx context.Context, text string) (T, error)
}
```

### PromptTemplate: `map[string]string` vs `any`

**Decision: Keep `map[string]string` for now**

The current `PromptTemplate.Format(map[string]string)` is simple and works. Changing to
`any` (struct-based rendering via `text/template`) is a bigger refactor that can be done
later. `LLMChain` will accept `map[string]string` as input.

### MapReduceChain: Generic vs Code-Warden-specific

**Decision: Keep it generic with function types**

Instead of hardcoding "models" into the chain, use generic `MapFunc` and `ReduceFunc` types:
```go
type MapFunc[In, Mid any]  func(ctx context.Context, input In) (Mid, error)
type ReduceFunc[Mid, Out any] func(ctx context.Context, results []Mid) (Out, error)
```
This way Code-Warden passes its own model-dispatching functions, and other consumers
can use MapReduceChain for completely different use cases.

---

## Task Breakdown

### Task 1: `OutputParser[T]` interface in `schema/`
**Files:** `schema/output_parser.go`  
**Effort:** Small  
**Branch:** `feat/output-parser-interface`

Add a generic `OutputParser[T]` interface and a `StringParser` implementation (identity parser
that returns the raw string). This is a prerequisite for LLMChain.

```go
// schema/output_parser.go
type OutputParser[T any] interface {
    Parse(ctx context.Context, text string) (T, error)
}
```

Also add a convenience `StringParser` that implements `OutputParser[string]` by returning
the input as-is (for chains that don't need structured parsing).

**Tests:** Unit test for `StringParser`.

---

### Task 2: `LLMChain[T]` in `chains/`
**Files:** `chains/llm_chain.go`, `chains/llm_chain_test.go`  
**Effort:** Medium  
**Branch:** `feat/llm-chain`  
**Depends on:** Task 1

Core struct:
```go
type LLMChain[T any] struct {
    LLM    llms.Model
    Prompt prompts.PromptTemplate
    Parser schema.OutputParser[T]
}
```

Key methods:
- `NewLLMChain[T](llm, prompt, opts...)` — Constructor with functional options
- `Call(ctx, vars map[string]string) (T, error)` — Format prompt → call LLM → parse output
- Options: `WithOutputParser[T](parser)`, `WithLLMCallOptions(...llms.CallOption)`

When `Parser` is nil and `T` is `string`, return the raw LLM output. For any other `T`,
a parser is required (enforced at construction time or with a clear runtime error).

**Tests:**
- Happy path with `StringParser` (raw string output)
- Happy path with a custom parser (mock XML parser returning a struct)
- LLM error propagation
- Parser error propagation
- Prompt variable substitution correctness

---

### Task 3: `HyDERetriever` in `vectorstores/`
**Files:** `vectorstores/hyde_retriever.go`, `vectorstores/hyde_retriever_test.go`  
**Effort:** Medium  
**Branch:** `feat/hyde-retriever`  
**Depends on:** None (independent of Task 1 & 2, but can use LLMChain internally if built first)

Core struct:
```go
type HyDERetriever struct {
    BaseRetriever schema.Retriever
    Generator     func(ctx context.Context, query string) (string, error)
    // Number of hypothetical docs to generate (default 1)
    NumGenerations int
}
```

The `Generator` is a plain function (not an `LLMChain`) — this keeps HyDERetriever
independent of `chains/` and avoids a circular dependency. Code-Warden can wrap an
`LLMChain[string].Call` into this function.

`GetRelevantDocuments` flow:
1. Call `Generator(ctx, query)` to produce a hypothetical document
2. Pass the hypothetical doc text into `BaseRetriever.GetRelevantDocuments(ctx, hypotheticalDoc)`
3. Return the results

If `NumGenerations > 1`, generate N hypothetical docs concurrently, retrieve for each,
and deduplicate results (same pattern as `MultiQueryRetriever`).

**Tests:**
- Single generation happy path
- Multiple generations with deduplication
- Generator error propagation
- Base retriever error propagation

---

### Task 4: `MapReduceChain[In, Mid, Out]` in `chains/`
**Files:** `chains/map_reduce.go`, `chains/map_reduce_test.go`  
**Effort:** Large  
**Branch:** `feat/map-reduce-chain`  
**Depends on:** None (independent, but benefits from Task 2 for map/reduce step composition)

Core struct:
```go
type MapReduceChain[In, Mid, Out any] struct {
    MapFunc       func(ctx context.Context, input In) (Mid, error)
    ReduceFunc    func(ctx context.Context, results []Mid) (Out, error)
    MaxConcurrency int            // Worker pool limit (default: len(inputs))
    Timeout        time.Duration  // Per-map-task timeout (0 = no timeout)
    QuorumFraction float64        // e.g. 0.66 = return early when 2/3 complete
}
```

Options:
- `WithMaxConcurrency(n int)`
- `WithTimeout(d time.Duration)` — per-task timeout
- `WithQuorum(fraction float64)` — return early when this fraction of map tasks succeed

`Call(ctx, inputs []In) (Out, error)` flow:
1. Fan out `MapFunc` across inputs with bounded concurrency (semaphore pattern)
2. Collect results; if quorum is set, return as soon as enough succeed
3. Pass collected `[]Mid` results to `ReduceFunc`
4. Return the final `Out`

Error handling:
- If quorum is met, partial failures are ignored (logged)
- If quorum is NOT met (too many failures), return an error with details
- Context cancellation is always respected

**Tests:**
- Happy path: all map tasks succeed
- Partial failure with quorum met (e.g., 2/3 succeed)
- Partial failure with quorum NOT met
- Concurrency limit enforcement
- Context cancellation during map phase
- Timeout per task

---

## Execution Order

```
Task 1 (OutputParser)  ──→  Task 2 (LLMChain)
                                               ──→  [Code-Warden integration]
Task 3 (HyDERetriever) ──→  [independent]
Task 4 (MapReduceChain) ──→ [independent]
```

Recommended implementation order:
1. **Task 1** — Small, unlocks Task 2
2. **Task 3** — Independent, immediate value for Code-Warden's HyDE code
3. **Task 2** — Core chain primitive, depends on Task 1
4. **Task 4** — Most complex, should be done last with full attention

Each task gets its own branch and PR per the project rules.

---

## Impact on Code-Warden

After all 4 tasks, Code-Warden can:

| Current Code-Warden pattern | Replaced by |
|--|--|
| Manual `promptMgr.Render()` → `LLM.Call()` → `ParseMarkdownReview()` | `LLMChain[StructuredReview].Call(ctx, vars)` |
| 150-line `gatherHyDEContext()` with channels/WaitGroups | `HyDERetriever.GetRelevantDocuments(ctx, query)` |
| Complex `GenerateConsensusReview()` with goroutines/semaphores/quorum | `MapReduceChain.Call(ctx, models)` |

Estimated LOC reduction in Code-Warden: **~300-400 lines** of infrastructure code 
replaced by GoFrame chain composition.
