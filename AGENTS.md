# GoFrame Agent Development Guide

Module: `github.com/sevigo/goframe`  
Go version: 1.26+

## Commands

```bash
make lint          # golangci-lint (installs to ./bin if missing)
make lint-fix      # auto-fix lint issues
make test          # all tests (Docker env vars set automatically for testcontainers)
make test-race     # tests with race detector
make pre-push      # lint + test (run before pushing)
go test ./agent/... -v -run TestRegistry    # single test
go test ./vectorstores/qdrant/... -v       # single package
make build-examples                          # verify all examples compile
```

The Makefile wraps `go test` with Docker environment variables for testcontainers-go. Running `go test` directly may still work but `make test` is safer.

## Architecture

Go RAG library for code understanding. Core pipeline:

```
GitLoader → ParserRegistry → CodeAwareTextSplitter → Embedder + SparseProvider → Qdrant
```

Key packages and their roles:

- `schema/` — core types: `Document`, `SparseVector`, `Retriever`, `Reranker`
- `llms/` — LLM clients: `ollama`, `openai`, `gemini` (each subpackage)
- `embeddings/` — embedder interface + batch embedding; `embeddings/sparse/` (BoW), `embeddings/sparse/code/` (code-aware tokenizer)
- `vectorstores/qdrant/` — Qdrant store with hybrid search, metadata filtering
- `vectorstores/` — `DependencyRetriever`, `DefinitionRetriever`, `ToRetriever`
- `parsers/` — language parser plugins (Go, TypeScript, Markdown, JSON, YAML, Terraform, Protobuf, PDF, CSV, HTML, RSS)
- `textsplitter/` — `CodeAwareTextSplitter` (AST-boundary splitting)
- `documentloaders/` — `GitLoader` for git repo ingestion
- `chains/` — `LLMChain[T]`, `RetrievalQA`, `MapReduceChain`
- `agent/` — agent SDK: session management, MCP server config, streaming
- `voice/` — TTS (Kokoro, ElevenLabs)

Examples in `examples/` are standalone programs, each with just a `main.go`. No per-example `go.mod`. `examples/vision-example/` has `//go:build ignore`.

## Linter

Uses golangci-lint v2.11.3. Notable rules:

- **Error wrapping**: `%w` only (enforced by `errorlint`); never `%v`
- **Named returns**: banned (`nonamedreturns`)
- **Function limits**: 150 lines, 60 statements (`funlen`)
- **Cyclomatic complexity**: max 30 (`cyclop`)
- **No `init()`**: `gochecknoinits`
- **No naked returns**: `nakedret` with `max-func-lines: 0`
- **nolint directives**: must name specific linter AND include explanation (`nolintlint: require-specific + require-explanation`)
- **Global logger banned in non-test**: `sloglint: no-global=all, context=scope` — use `slog.WithContext`-style or pass logger explicitly
- **Import grouping**: `goimports` with local-prefixes (currently set to `github.com/my/project` in `.golangci.yaml` — **bug**, should be `github.com/sevigo/goframe`)
- **`log` package banned** in non-main files: use `log/slog` (`depguard`)
- **`math/rand` banned** in non-test: use `math/rand/v2` (`depguard`)
- `examples/` path excluded from all lint analysis

## Testing

- Uses `stretchr/testify` — `require` for critical setup (stops on failure), `assert` for verifications
- Subtest pattern: `t.Run("scenario", func(t *testing.T) { ... })` with arrange/act/assert
- `testdata/` at project root only (MSMARCO CSV for RAG evaluation example)
- Integration tests: `voice/integration_test.go` uses `//go:build integration` tag — run with `go test -tags=integration ./voice/...`

## External Services

Tests use testcontainers-go. Docker must be running for `make test`.  
`docker-compose.yml` provides Qdrant + Ollama for local development: `docker compose up -d`

## Code Conventions

- Constructor with functional options: `NewXxx(opts ...Option)` + `WithXxx()` pattern
- Sentinel errors: `var ErrXxx = errors.New(...)` in `errors.go` files (found in `agent/`, `contextpacker/`, `llms/openai/`)
- Package doc comments in `doc.go` files (found in `agent/`, `chains/`, `documentloaders/`, `gitutil/`, `output/`, and many subpackages)
- Context cancellation in long-running operations
- `sync.RWMutex` for concurrent access to shared state