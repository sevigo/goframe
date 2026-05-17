package comfyui

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/sevigo/goframe/httpclient"
)

type options struct {
	host           string
	httpClient     *http.Client
	requestTimeout time.Duration
	retry          httpclient.RetryConfig
	logger         *slog.Logger
	clientID       string
}

type Option func(*options)

func applyOptions(opts ...Option) options {
	o := options{
		host:           "127.0.0.1:8188",
		requestTimeout: 120 * time.Second,
		retry: httpclient.RetryConfig{
			Attempts: 3,
			Delay:    2 * time.Second,
			MaxDelay: 30 * time.Second,
			Jitter:   1 * time.Second,
		},
		logger:   slog.Default(),
		clientID: "goframe-comfyui",
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

func WithHost(host string) Option {
	return func(o *options) {
		o.host = host
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(o *options) {
		if client != nil {
			o.httpClient = client
		}
	}
}

func WithRequestTimeout(timeout time.Duration) Option {
	return func(o *options) {
		if timeout > 0 {
			o.requestTimeout = timeout
		}
	}
}

func WithRetryAttempts(attempts int) Option {
	return func(o *options) {
		if attempts >= 0 {
			o.retry.Attempts = attempts
		}
	}
}

func WithRetryDelay(delay time.Duration) Option {
	return func(o *options) {
		if delay > 0 {
			o.retry.Delay = delay
		}
	}
}

func WithMaxRetryDelay(delay time.Duration) Option {
	return func(o *options) {
		if delay > 0 {
			o.retry.MaxDelay = delay
		}
	}
}

func WithClientID(id string) Option {
	return func(o *options) {
		if id != "" {
			o.clientID = id
		}
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(o *options) {
		if logger != nil {
			o.logger = logger
		}
	}
}

func (o *options) baseURL() string {
	return fmt.Sprintf("http://%s", o.host)
}

func (o *options) wsURL() string {
	return fmt.Sprintf("ws://%s/ws?clientId=%s", o.host, o.clientID)
}
