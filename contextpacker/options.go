package contextpacker

import (
	"log/slog"

	"github.com/sevigo/goframe/llms"
)

// options configures the Packer behavior.
type options struct {
	template string
	strategy PackingStrategy
	logger   *slog.Logger
}

// Option configures a Packer.
type Option func(*options)

// WithTemplate sets the document template.
// Use {{.content}} for document content and {{.metadata}} for metadata.
func WithTemplate(tmpl string) Option {
	return func(opts *options) {
		opts.template = tmpl
	}
}

// WithStrategy sets the packing strategy.
func WithStrategy(strategy PackingStrategy) Option {
	return func(opts *options) {
		opts.strategy = strategy
	}
}

// WithLogger sets a custom logger.
func WithLogger(logger *slog.Logger) Option {
	return func(opts *options) {
		opts.logger = logger
	}
}

// validateOptions returns default options applied with user options.
func validateOptions(opts ...Option) options {
	result := options{
		template: DefaultTemplate,
		strategy: GreedyStrategy{},
		logger:   slog.Default(),
	}
	for _, opt := range opts {
		opt(&result)
	}
	return result
}

// New creates a new Packer with the given tokenizer and token limit.
func New(tokenizer llms.Tokenizer, maxTokens int, opts ...Option) (*Packer, error) {
	if tokenizer == nil {
		return nil, ErrNilTokenizer
	}
	if maxTokens <= 0 {
		return nil, ErrInvalidMaxTokens
	}

	cfg := validateOptions(opts...)

	tmpl, err := parseTemplate(cfg.template)
	if err != nil {
		return nil, err
	}

	return &Packer{
		tokenizer: tokenizer,
		maxTokens: maxTokens,
		template:  tmpl,
		strategy:  cfg.strategy,
		logger:    cfg.logger,
	}, nil
}
