// Package documentloaders provides document loading utilities for RAG applications.
// It includes loaders for git repositories, RSS feeds, and other document sources
// with support for streaming, batch processing, and memory protection.
package documentloaders

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/sevigo/goframe/parsers"
	"github.com/sevigo/goframe/schema"
	"golang.org/x/time/rate"
)

const (
	defaultRSSBatchSize   = 50
	defaultRSSWorkerCount = 5
	defaultTimeout        = 30 * time.Second
	defaultMaxItems       = 100
	defaultRetryAttempts  = 3
	defaultRateLimit      = 10 // requests per second
)

// Error variables for RSS loading operations.
var (
	// ErrInvalidFeedURL is returned when the feed URL is invalid.
	ErrInvalidFeedURL = errors.New("documentloaders: invalid feed URL")
	// ErrNoFeedURLs is returned when no feed URLs are provided.
	ErrNoFeedURLs = errors.New("documentloaders: no feed URLs provided")
	// ErrFeedFetchFailed is returned when a feed cannot be fetched.
	ErrFeedFetchFailed = errors.New("documentloaders: failed to fetch feed")
	// ErrTimeoutExceeded is returned when the timeout is exceeded.
	ErrTimeoutExceeded = errors.New("documentloaders: timeout exceeded")
	// ErrMaxRetriesExceeded is returned when max retries are exceeded.
	ErrMaxRetriesExceeded = errors.New("documentloaders: max retries exceeded")
)

// RSSFeedData represents a fetched RSS feed with its metadata.
type RSSFeedData struct {
	URL      string
	Feed     *gofeed.Feed
	Metadata map[string]any
	Error    error
}

// RSSLoader loads and processes documents from RSS/Atom feeds.
// It supports batch processing, parallel feed fetching, and content normalization.
type RSSLoader struct {
	feedURLs   []string
	parser     *gofeed.Parser
	registry   parsers.ParserRegistry
	normalizer *RSSNormalizer
	httpClient *http.Client
	logger     *slog.Logger
	options    rssLoaderOptions
	seenMu     sync.RWMutex // Protects options.SeenItems
	limiter    *rate.Limiter
}

type rssLoaderOptions struct {
	BatchSize      int
	WorkerCount    int
	Timeout        time.Duration
	MaxItems       int
	MaxRetries     int
	UserAgent      string
	HTTPClient     *http.Client
	Logger         *slog.Logger
	Normalization  NormalizationConfig
	SkipDuplicates bool
	SeenItems      map[string]bool
	RateLimit      int // requests per second
}

// RSSLoaderOption configures an RSSLoader.
type RSSLoaderOption func(*rssLoaderOptions)

// WithRSSBatchSize sets the batch size for document processing.
func WithRSSBatchSize(size int) RSSLoaderOption {
	return func(opts *rssLoaderOptions) {
		if size > 0 {
			opts.BatchSize = size
		}
	}
}

// WithRSSWorkerCount sets the number of parallel workers for feed fetching.
func WithRSSWorkerCount(count int) RSSLoaderOption {
	return func(opts *rssLoaderOptions) {
		if count > 0 {
			opts.WorkerCount = count
		}
	}
}

// WithRSSTimeout sets the HTTP timeout for feed requests.
func WithRSSTimeout(timeout time.Duration) RSSLoaderOption {
	return func(opts *rssLoaderOptions) {
		if timeout > 0 {
			opts.Timeout = timeout
		}
	}
}

// WithRSSMaxItems sets the maximum number of items to fetch per feed.
func WithRSSMaxItems(count int) RSSLoaderOption {
	return func(opts *rssLoaderOptions) {
		if count > 0 {
			opts.MaxItems = count
		}
	}
}

// WithRSSMaxRetries sets the number of retry attempts for failed requests.
func WithRSSMaxRetries(retries int) RSSLoaderOption {
	return func(opts *rssLoaderOptions) {
		if retries >= 0 {
			opts.MaxRetries = retries
		}
	}
}

// WithRSSUserAgent sets a custom User-Agent header for HTTP requests.
func WithRSSUserAgent(userAgent string) RSSLoaderOption {
	return func(opts *rssLoaderOptions) {
		if userAgent != "" {
			opts.UserAgent = userAgent
		}
	}
}

// WithRSSHTTPClient sets a custom HTTP client for feed requests.
func WithRSSHTTPClient(client *http.Client) RSSLoaderOption {
	return func(opts *rssLoaderOptions) {
		if client != nil {
			opts.HTTPClient = client
		}
	}
}

// WithRSSLogger sets a custom logger for the loader.
func WithRSSLogger(logger *slog.Logger) RSSLoaderOption {
	return func(opts *rssLoaderOptions) {
		if logger != nil {
			opts.Logger = logger
		}
	}
}

// WithRSSNormalization sets the content normalization configuration.
func WithRSSNormalization(config NormalizationConfig) RSSLoaderOption {
	return func(opts *rssLoaderOptions) {
		opts.Normalization = config
	}
}

// WithRSSSkipDuplicates enables deduplication of feed items by GUID/link.
func WithRSSSkipDuplicates(skip bool) RSSLoaderOption {
	return func(opts *rssLoaderOptions) {
		opts.SkipDuplicates = skip
	}
}

// WithRSSSeenItems provides a pre-populated map of seen item GUIDs for deduplication.
func WithRSSSeenItems(seen map[string]bool) RSSLoaderOption {
	return func(opts *rssLoaderOptions) {
		opts.SeenItems = seen
	}
}

// WithRSSRateLimit sets the maximum number of requests per second.
// This helps prevent overwhelming RSS servers with too many concurrent requests.
// Default is 10 requests per second.
func WithRSSRateLimit(requestsPerSecond int) RSSLoaderOption {
	return func(opts *rssLoaderOptions) {
		if requestsPerSecond > 0 {
			opts.RateLimit = requestsPerSecond
		}
	}
}

// NewRSS creates a new RSSLoader for the specified feed URLs.
// Returns an error if no URLs are provided or the registry is nil.
func NewRSS(feedURLs []string, registry parsers.ParserRegistry, opts ...RSSLoaderOption) (*RSSLoader, error) {
	if len(feedURLs) == 0 {
		return nil, ErrNoFeedURLs
	}

	if registry == nil {
		return nil, ErrNilRegistry
	}

	loaderOpts := rssLoaderOptions{
		BatchSize:      defaultRSSBatchSize,
		WorkerCount:    defaultRSSWorkerCount,
		Timeout:        defaultTimeout,
		MaxItems:       defaultMaxItems,
		MaxRetries:     defaultRetryAttempts,
		UserAgent:      "goframe-rss-loader/1.0",
		SeenItems:      make(map[string]bool),
		SkipDuplicates: false,
		Logger:         slog.Default(),
		RateLimit:      defaultRateLimit,
	}

	for _, opt := range opts {
		opt(&loaderOpts)
	}

	if loaderOpts.HTTPClient == nil {
		loaderOpts.HTTPClient = &http.Client{
			Timeout: loaderOpts.Timeout,
		}
	}

	normalizer := NewRSSNormalizer(loaderOpts.Normalization)
	parser := gofeed.NewParser()
	parser.Client = loaderOpts.HTTPClient
	parser.UserAgent = loaderOpts.UserAgent

	// Create rate limiter
	limiter := rate.NewLimiter(rate.Limit(loaderOpts.RateLimit), loaderOpts.RateLimit)

	return &RSSLoader{
		feedURLs:   feedURLs,
		parser:     parser,
		registry:   registry,
		normalizer: normalizer,
		httpClient: loaderOpts.HTTPClient,
		logger:     loaderOpts.Logger.With("component", "rss_loader"),
		options:    loaderOpts,
		limiter:    limiter,
	}, nil
}

// Load fetches all feeds and returns all documents.
// Warning: This method loads all documents into memory. For large feeds,
// use LoadAndProcessStream instead for better memory efficiency.
func (r *RSSLoader) Load(ctx context.Context) ([]schema.Document, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	r.logger.InfoContext(ctx, "Starting RSS feed load", "feeds", len(r.feedURLs))

	var documents []schema.Document
	var mu sync.Mutex

	err := r.LoadAndProcessStream(ctx, func(ctx context.Context, docs []schema.Document) error {
		mu.Lock()
		documents = append(documents, docs...)
		mu.Unlock()
		return nil
	})

	if err != nil {
		return nil, err
	}

	r.logger.InfoContext(ctx, "RSS feed load completed", "total_documents", len(documents))
	return documents, nil
}

func (r *RSSLoader) LoadAndProcessStream(ctx context.Context, processFn func(ctx context.Context, docs []schema.Document) error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	r.logger.InfoContext(ctx, "Starting streaming RSS load")

	feedChan := make(chan RSSFeedData, r.options.WorkerCount*2)
	docChan := make(chan schema.Document, r.options.BatchSize*2)
	errChan := make(chan error, 1)

	pipelineCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup

	wg.Go(func() {
		defer close(feedChan)
		r.fetchFeedsWorkers(pipelineCtx, feedChan)
	})

	var processorWg sync.WaitGroup
	for range r.options.WorkerCount {
		processorWg.Go(func() {
			r.processFeedWorker(pipelineCtx, feedChan, docChan)
		})
	}

	go func() {
		processorWg.Wait()
		close(docChan)
	}()

	wg.Go(func() {
		if err := r.batchAndProcess(pipelineCtx, docChan, processFn); err != nil {
			select {
			case errChan <- fmt.Errorf("batch processing failed: %w", err):
			default:
			}
			cancel()
		}
	})

	wg.Wait()

	select {
	case err := <-errChan:
		return err
	default:
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	r.logger.InfoContext(ctx, "Streaming RSS load completed")
	return nil
}
func (r *RSSLoader) fetchFeedsWorkers(ctx context.Context, feedChan chan<- RSSFeedData) {
	var wg sync.WaitGroup
	urlChan := make(chan string, len(r.feedURLs))

	for _, url := range r.feedURLs {
		urlChan <- url
	}
	close(urlChan)

	for range r.options.WorkerCount {
		wg.Go(func() {
			for feedURL := range urlChan {
				if ctx.Err() != nil {
					return
				}

				feedData := r.fetchFeedWithRetry(ctx, feedURL)
				select {
				case feedChan <- feedData:
				case <-ctx.Done():
					return
				}
			}
		})
	}

	wg.Wait()
}

func (r *RSSLoader) fetchFeedWithRetry(ctx context.Context, feedURL string) RSSFeedData {
	var lastErr error

	for attempt := 0; attempt <= r.options.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return RSSFeedData{URL: feedURL, Error: ctx.Err()}
		}

		// Apply rate limiting
		if err := r.limiter.Wait(ctx); err != nil {
			return RSSFeedData{URL: feedURL, Error: err}
		}

		feed, err := r.fetchFeed(ctx, feedURL)
		if err == nil {
			r.logger.DebugContext(ctx, "Feed fetched successfully", "url", feedURL, "items", len(feed.Items))
			return RSSFeedData{
				URL:      feedURL,
				Feed:     feed,
				Metadata: r.extractFeedMetadata(feed),
			}
		}

		lastErr = err
		r.logger.WarnContext(ctx, "Feed fetch attempt failed",
			"url", feedURL,
			"attempt", attempt+1,
			"max_retries", r.options.MaxRetries,
			"error", err)

		if attempt < r.options.MaxRetries {
			// Use a timer with proper cleanup
			timer := time.NewTimer(time.Second * time.Duration(attempt+1))
			select {
			case <-ctx.Done():
				timer.Stop()
				return RSSFeedData{URL: feedURL, Error: ctx.Err()}
			case <-timer.C:
				timer.Stop()
			}
		}
	}

	r.logger.ErrorContext(ctx, "Failed to fetch feed after all retries", "url", feedURL, "error", lastErr)
	return RSSFeedData{URL: feedURL, Error: lastErr}
}

func (r *RSSLoader) fetchFeed(ctx context.Context, feedURL string) (*gofeed.Feed, error) {
	reqCtx, cancel := context.WithTimeout(ctx, r.options.Timeout)
	defer cancel()

	feed, err := r.parser.ParseURLWithContext(feedURL, reqCtx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFeedFetchFailed, err)
	}

	return feed, nil
}

func (r *RSSLoader) processFeedWorker(ctx context.Context, feedChan <-chan RSSFeedData, docChan chan<- schema.Document) {
	for {
		select {
		case <-ctx.Done():
			return
		case feedData, ok := <-feedChan:
			if !ok {
				return
			}

			if feedData.Error != nil {
				r.logger.ErrorContext(ctx, "Skipping feed due to error",
					"url", feedData.URL,
					"error", feedData.Error)
				continue
			}

			docs := r.processFeedItems(feedData)
			for _, doc := range docs {
				select {
				case docChan <- doc:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

func (r *RSSLoader) processFeedItems(feedData RSSFeedData) []schema.Document {
	if feedData.Feed == nil {
		return nil
	}

	items := feedData.Feed.Items
	if r.options.MaxItems > 0 && len(items) > r.options.MaxItems {
		items = items[:r.options.MaxItems]
	}

	var documents []schema.Document

	for _, item := range items {
		doc := r.createDocument(item, feedData)
		if doc != nil {
			documents = append(documents, *doc)
		}
	}

	return documents
}

func (r *RSSLoader) createDocument(item *gofeed.Item, feedData RSSFeedData) *schema.Document {
	// Get GUID for deduplication
	guid := item.GUID
	if guid == "" {
		guid = item.Link
	}

	// Deduplication check
	if r.options.SkipDuplicates {
		r.seenMu.RLock()
		if r.options.SeenItems[guid] {
			r.seenMu.RUnlock()
			r.logger.Debug("Skipping duplicate item", "guid", guid)
			return nil
		}
		r.seenMu.RUnlock()
	}

	// Normalize content
	title := r.normalizer.NormalizeTitle(item.Title, item.Link)
	content := item.Content
	if content == "" {
		content = item.Description
	}
	content = r.normalizer.NormalizeContent(content)

	// Skip low-quality items
	if r.normalizer.ShouldSkipItem(title, content) {
		r.logger.Debug("Skipping item: insufficient content", "title", title)
		return nil
	}

	// Build metadata
	link := r.normalizer.NormalizeURL(item.Link)
	link = r.normalizer.ResolveURL(feedData.Feed.Link, link)

	pubDate := r.normalizer.ParseDate(item.Published)
	if pubDate.IsZero() {
		pubDate = r.normalizer.ParseDate(item.Updated)
	}

	author := ""
	if item.Author != nil {
		author = r.normalizer.NormalizeAuthor(item.Author.Name)
	}

	categories := r.normalizer.NormalizeCategories(item.Categories)

	metadata := map[string]any{
		"source":        feedData.URL,
		"feed_title":    feedData.Feed.Title,
		"feed_link":     feedData.Feed.Link,
		"title":         title,
		"link":          link,
		"pub_date":      pubDate,
		"author":        author,
		"categories":    categories,
		"guid":          guid,
		"content_type":  "rss_item",
		"chunk_type":    "rss_content",
		"identifier":    guid,
		"is_definition": false,
	}

	// Mark as seen after successful processing
	if r.options.SkipDuplicates {
		r.seenMu.Lock()
		r.options.SeenItems[guid] = true
		r.seenMu.Unlock()
	}

	return &schema.Document{
		PageContent: content,
		Metadata:    metadata,
	}
}

func (r *RSSLoader) extractFeedMetadata(feed *gofeed.Feed) map[string]any {
	metadata := map[string]any{
		"feed_title":       feed.Title,
		"feed_link":        feed.Link,
		"feed_description": feed.Description,
	}

	if feed.Language != "" {
		metadata["feed_language"] = feed.Language
	}

	if feed.Published != "" {
		if pubDate := r.normalizer.ParseDate(feed.Published); !pubDate.IsZero() {
			metadata["feed_published"] = pubDate
		}
	}

	if feed.Updated != "" {
		if updated := r.normalizer.ParseDate(feed.Updated); !updated.IsZero() {
			metadata["feed_updated"] = updated
		}
	}

	return metadata
}

func (r *RSSLoader) batchAndProcess(ctx context.Context, docChan <-chan schema.Document, processFn func(ctx context.Context, docs []schema.Document) error) error {
	batch := make([]schema.Document, 0, r.options.BatchSize)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case doc, ok := <-docChan:
			if !ok {
				if len(batch) > 0 {
					if err := processFn(ctx, batch); err != nil {
						return fmt.Errorf("final batch processing failed: %w", err)
					}
				}
				return nil
			}

			batch = append(batch, doc)

			if len(batch) >= r.options.BatchSize {
				if err := processFn(ctx, batch); err != nil {
					return fmt.Errorf("batch processing failed: %w", err)
				}
				batch = make([]schema.Document, 0, r.options.BatchSize)
			}
		}
	}
}
