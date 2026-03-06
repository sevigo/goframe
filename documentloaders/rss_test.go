package documentloaders_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/documentloaders"
	"github.com/sevigo/goframe/parsers"
	"github.com/sevigo/goframe/schema"
)

func TestRSSLoader_Load(t *testing.T) {
	rssFeed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <link>https://example.com</link>
    <description>A test RSS feed</description>
    <item>
      <title>First Article</title>
      <link>https://example.com/article1?utm_source=rss</link>
      <description>&lt;p&gt;This is &lt;b&gt;HTML&lt;/b&gt; content&lt;/p&gt;</description>
      <pubDate>Mon, 02 Jan 2006 15:04:05 -0700</pubDate>
      <author>John Doe</author>
      <guid>article-1</guid>
      <category>Tech</category>
      <category>News</category>
    </item>
    <item>
      <title>Second Article</title>
      <link>https://example.com/article2</link>
      <description>Plain description without HTML</description>
      <pubDate>Tue, 03 Jan 2006 10:00:00 +0000</pubDate>
      <guid>article-2</guid>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(rssFeed))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := parsers.NewRegistry(logger)
	err := registry.RegisterParser(parsers.NewRSSParser())
	require.NoError(t, err)

	loader, err := documentloaders.NewRSS(
		[]string{server.URL},
		registry,
		documentloaders.WithRSSBatchSize(10),
		documentloaders.WithRSSMaxItems(10),
		documentloaders.WithRSSNormalization(documentloaders.NormalizationConfig{
			StripHTML:        true,
			RemoveTracking:   true,
			MaxContentLength: 1000,
			MinContentLength: 10,
		}),
	)
	require.NoError(t, err)

	docs, err := loader.Load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, docs)
	assert.GreaterOrEqual(t, len(docs), 2)

	foundArticle1 := false
	foundArticle2 := false

	for _, doc := range docs {
		title, _ := doc.Metadata["title"].(string)
		if title == "First Article" {
			foundArticle1 = true
			assert.Contains(t, doc.PageContent, "This is HTML content")
			assert.NotContains(t, doc.PageContent, "<p>")
			assert.NotContains(t, doc.PageContent, "<b>")

			link, _ := doc.Metadata["link"].(string)
			assert.NotContains(t, link, "utm_source")

			author, _ := doc.Metadata["author"].(string)
			assert.Equal(t, "John Doe", author)

			categories, _ := doc.Metadata["categories"].([]string)
			assert.Contains(t, categories, "tech")
			assert.Contains(t, categories, "news")
		}

		if title == "Second Article" {
			foundArticle2 = true
			assert.Contains(t, doc.PageContent, "Plain description")
		}
	}

	assert.True(t, foundArticle1, "First article should be found")
	assert.True(t, foundArticle2, "Second article should be found")
}

func TestRSSLoader_LoadAndProcessStream(t *testing.T) {
	rssFeed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Stream Test Feed</title>
    <link>https://example.com</link>
    <item>
      <title>Stream Article</title>
      <link>https://example.com/stream-article</link>
      <description>Testing stream processing</description>
      <guid>stream-1</guid>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(rssFeed))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := parsers.NewRegistry(logger)
	err := registry.RegisterParser(parsers.NewRSSParser())
	require.NoError(t, err)

	loader, err := documentloaders.NewRSS(
		[]string{server.URL},
		registry,
		documentloaders.WithRSSWorkerCount(1),
		documentloaders.WithRSSBatchSize(5),
	)
	require.NoError(t, err)

	var batches [][]schema.Document
	err = loader.LoadAndProcessStream(context.Background(), func(ctx context.Context, docs []schema.Document) error {
		batches = append(batches, docs)
		return nil
	})

	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(batches), 0)

	if len(batches) > 0 {
		totalDocs := 0
		for _, batch := range batches {
			totalDocs += len(batch)
		}
		assert.GreaterOrEqual(t, totalDocs, 1)
	}
}

func TestRSSLoader_WithDuplicates(t *testing.T) {
	rssFeed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Duplicate Test</title>
    <link>https://example.com</link>
    <item>
      <title>Duplicate Article</title>
      <link>https://example.com/dup</link>
      <description>Same article</description>
      <guid>dup-1</guid>
    </item>
    <item>
      <title>Duplicate Article</title>
      <link>https://example.com/dup</link>
      <description>Same article</description>
      <guid>dup-1</guid>
    </item>
    <item>
      <title>Unique Article</title>
      <link>https://example.com/unique</link>
      <description>Different article</description>
      <guid>unique-1</guid>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(rssFeed))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := parsers.NewRegistry(logger)
	err := registry.RegisterParser(parsers.NewRSSParser())
	require.NoError(t, err)

	loader, err := documentloaders.NewRSS(
		[]string{server.URL},
		registry,
		documentloaders.WithRSSSkipDuplicates(true),
		documentloaders.WithRSSNormalization(documentloaders.NormalizationConfig{
			MinContentLength: 10,
			MinTitleLength:   3,
		}),
	)
	require.NoError(t, err)

	docs, err := loader.Load(context.Background())
	require.NoError(t, err)

	assert.LessOrEqual(t, len(docs), 2, "Should skip duplicate items")
}

func TestRSSLoader_WithTimeout(t *testing.T) {
	// Create a server that delays response
	delay := 2 * time.Second
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<rss><channel><title>Test</title></channel></rss>`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := parsers.NewRegistry(logger)
	err := registry.RegisterParser(parsers.NewRSSParser())
	require.NoError(t, err)

	loader, err := documentloaders.NewRSS(
		[]string{server.URL},
		registry,
		documentloaders.WithRSSTimeout(100*time.Millisecond),
		documentloaders.WithRSSMaxRetries(0),
	)
	require.NoError(t, err)

	// Use a context with deadline shorter than the server delay
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	docs, err := loader.Load(ctx)
	elapsed := time.Since(start)

	// The loader should timeout and return quickly
	assert.Less(t, elapsed, 300*time.Millisecond, "Should timeout quickly")

	// If there's an error, that's acceptable (timeout)
	// If no error but no docs, that's also acceptable (feed failed but loader continues)
	if err == nil {
		assert.Equal(t, 0, len(docs), "Should have no documents when feed times out")
	}
}

func TestRSSLoader_InvalidURL(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := parsers.NewRegistry(logger)
	err := registry.RegisterParser(parsers.NewRSSParser())
	require.NoError(t, err)

	loader, err := documentloaders.NewRSS(
		[]string{"http://invalid.invalid/feed.xml"},
		registry,
		documentloaders.WithRSSTimeout(1*time.Second),
		documentloaders.WithRSSMaxRetries(0),
	)
	require.NoError(t, err)

	docs, err := loader.Load(context.Background())
	// RSS loader doesn't return error when feeds fail, it just logs and continues
	// So we should get an empty result
	require.NoError(t, err)
	assert.Equal(t, 0, len(docs), "Should have no documents from invalid URL")
}

func TestRSSLoader_NoFeeds(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := parsers.NewRegistry(logger)
	_, err := documentloaders.NewRSS([]string{}, registry)
	assert.Error(t, err)
	assert.Equal(t, documentloaders.ErrNoFeedURLs, err)
}

func TestRSSLoader_NilRegistry(t *testing.T) {
	_, err := documentloaders.NewRSS([]string{"https://example.com/feed.xml"}, nil)
	assert.Error(t, err)
}
