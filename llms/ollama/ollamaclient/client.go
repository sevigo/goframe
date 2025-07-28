package ollamaclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/ollama/ollama/api"
)

const (
	DefaultOllamaURL = "http://127.0.0.1:11434"
	DefaultTimeout   = 10 * time.Minute
)

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	logger     *slog.Logger
}

func NewClient(baseURL *url.URL, httpClient *http.Client, logger *slog.Logger) (*Client, error) {
	if baseURL == nil {
		var err error
		baseURL, err = getDefaultURL()
		if err != nil {
			return nil, err
		}
	}

	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: DefaultTimeout,
			Transport: &http.Transport{
				MaxIdleConns:       100,
				IdleConnTimeout:    90 * time.Second,
				DisableCompression: false,
				MaxConnsPerHost:    100,
				ForceAttemptHTTP2:  true,
			},
		}
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
		logger:     logger,
	}, nil
}

func NewDefaultClient(logger *slog.Logger) (*Client, error) {
	return NewClient(nil, nil, logger)
}

func getDefaultURL() (*url.URL, error) {
	host := os.Getenv("OLLAMA_URL")
	if host == "" {
		host = DefaultOllamaURL
	}

	baseURL, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OLLAMA_URL: %w", err)
	}

	return baseURL, nil
}

func (c *Client) Generate(ctx context.Context, req *GenerateRequest, callback func(GenerateResponse) error) error {
	return c.streamRequest(ctx, http.MethodPost, "/api/generate", req, func(data []byte) error {
		var resp GenerateResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return fmt.Errorf("failed to unmarshal generate response: %w", err)
		}
		return callback(resp)
	})
}

func (c *Client) Embed(ctx context.Context, req *api.EmbedRequest) (*api.EmbedResponse, error) {
	var resp api.EmbedResponse
	err := c.doRequest(ctx, http.MethodPost, "/api/embed", req, &resp)
	if err != nil {
		return nil, fmt.Errorf("embed request failed: %w", err)
	}
	return &resp, nil
}

func (c *Client) List(ctx context.Context) (*api.ListResponse, error) {
	var resp api.ListResponse
	err := c.doRequest(ctx, http.MethodGet, "/api/tags", nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("list models request failed: %w", err)
	}
	return &resp, nil
}

func (c *Client) Show(ctx context.Context, req *api.ShowRequest) (*api.ShowResponse, error) {
	var resp api.ShowResponse
	err := c.doRequest(ctx, http.MethodPost, "/api/show", req, &resp)
	if err != nil {
		return nil, fmt.Errorf("show model request failed: %w", err)
	}
	return &resp, nil
}

func (c *Client) Pull(ctx context.Context, req *PullRequest, callback func(api.ProgressResponse) error) error {
	return c.streamRequest(ctx, http.MethodPost, "/api/pull", req, func(data []byte) error {
		var resp api.ProgressResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return fmt.Errorf("failed to unmarshal pull response: %w", err)
		}
		return callback(resp)
	})
}

func (c *Client) GetBaseURL() *url.URL {
	return c.baseURL
}

func (c *Client) GetHTTPClient() *http.Client {
	return c.httpClient
}

func (c *Client) doRequest(ctx context.Context, method, path string, reqData, respData any) error {
	var reqBody io.Reader
	var bodyBytes []byte
	var err error

	if reqData != nil {
		bodyBytes, err = json.Marshal(reqData)
		if err != nil {
			return fmt.Errorf("failed to encode request data: %w", err)
		}
		reqBody = bytes.NewBuffer(bodyBytes)
	}

	request, err := c.buildRequest(ctx, method, path, reqBody)
	if err != nil {
		return err
	}
	c.setJSONHeaders(request)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer response.Body.Close()

	if err := c.checkError(response); err != nil {
		return err
	}

	if respData != nil {
		if err := json.NewDecoder(response.Body).Decode(respData); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

func (c *Client) buildRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	requestURL := c.baseURL.JoinPath(path)

	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	return request, nil
}

func (c *Client) setJSONHeaders(request *http.Request) {
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
}

func (c *Client) checkError(response *http.Response) error {
	if response.StatusCode < http.StatusBadRequest {
		return nil
	}

	var apiError struct {
		Error string `json:"error"`
	}

	bodyBytes, _ := io.ReadAll(response.Body)
	response.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	if err := json.NewDecoder(bytes.NewBuffer(bodyBytes)).Decode(&apiError); err != nil {
		c.logger.Error("Ollama API request failed with non-JSON error response",
			"status", response.StatusCode,
			"method", response.Request.Method,
			"url", response.Request.URL.String(),
			"response_body", string(bodyBytes),
		)
		return fmt.Errorf("ollama API error (status %d): %s", response.StatusCode, http.StatusText(response.StatusCode))
	}

	c.logger.Error("Ollama API request failed",
		"status", response.StatusCode,
		"method", response.Request.Method,
		"url", response.Request.URL.String(),
		"ollama_error", apiError.Error,
	)

	return fmt.Errorf("ollama API error (status %d): %s", response.StatusCode, apiError.Error)
}

func ConvertEmbeddingToFloat32(embedding []float64) []float32 {
	if embedding == nil {
		return nil
	}
	embeddingF32 := make([]float32, len(embedding))
	for i, v := range embedding {
		embeddingF32[i] = float32(v)
	}
	return embeddingF32
}
