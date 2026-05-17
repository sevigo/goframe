// Package gemini provides a Go client for Google's Gemini generative models
// via the google.golang.org/genai SDK.
//
// The LLM type implements [llms.Model], [embeddings.Embedder], and
// [embeddings.ImageEmbedder], enabling it to serve as a drop-in provider
// for GoFrame's RAG pipeline, chains, and agent loop.
//
// # Quick Start
//
//	model, err := gemini.New(ctx,
//	    gemini.WithAPIKey("AIza..."),
//	    gemini.WithModel("gemini-2.5-flash"),
//	)
//
// # Text Embedding
//
//	embedder, _ := embeddings.NewEmbedder(model)
//	vecs, _ := embedder.EmbedDocuments(ctx, []string{"hello", "world"})
//
// # Image Embedding
//
//	imageData, _ := os.ReadFile("photo.png")
//	vec, _ := model.EmbedImage(ctx, embeddings.ImageData{
//	    Data:     imageData,
//	    MimeType: "image/png",
//	})
//
// # Retry and Timeouts
//
// The client uses [httpclient.DoWithRetry] for non-streaming calls with
// configurable exponential backoff. Streaming calls are not retried.
// Default: 3 retries, 2s initial delay, 30s max delay, 1s jitter.
//
// # Error Handling
//
// Gemini-specific errors (API_KEY_INVALID, PERMISSION_DENIED, etc.) are
// classified as non-retryable. Transient errors (RESOURCE_EXHAUSTED, 429,
// 500, 503) trigger automatic retries.
package gemini
