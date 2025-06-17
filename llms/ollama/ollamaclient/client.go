package ollamaclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/ollama/ollama/api"
)

const (
	DefaultOllamaURL = "http://127.0.0.1:11434"
	DefaultTimeout = 10 * time.Minute
	MaxBufferSize = 512 * 1000
)

// Client is a HTTP client for the Ollama API that supports both streaming and non-streaming requests.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

// NewClient creates a new Ollama client with the provided configuration.
// If baseURL is nil, it uses OLLAMA_URL environment variable or the default URL.
// If httpClient is nil, it creates a default client with a 10-minute timeout.
func NewClient(baseURL *url.URL, httpClient *http.Client) (*Client, error) {
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
		}
	}

	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
	}, nil
}

func NewDefaultClient() (*Client, error) {
	baseURL, err := getDefaultURL()
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Timeout: DefaultTimeout,
	}

	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
	}, nil
}

// getDefaultURL returns the default Ollama URL from environment or default constant.
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

// Generate makes a streaming request to the /api/generate endpoint.
// The callback function is called for each response chunk received.
func (c *Client) Generate(ctx context.Context, req *GenerateRequest, callback func(GenerateResponse) error) error {
	return c.streamRequest(ctx, http.MethodPost, "/api/generate", req, func(data []byte) error {
		var resp GenerateResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return fmt.Errorf("failed to unmarshal generate response: %w", err)
		}
		return callback(resp)
	})
}

// Embeddings makes a non-streaming request to the /api/embeddings endpoint.
func (c *Client) Embeddings(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	var resp EmbeddingResponse
	err := c.doRequest(ctx, http.MethodPost, "/api/embeddings", req, &resp)
	if err != nil {
		return nil, fmt.Errorf("embeddings request failed: %w", err)
	}
	return &resp, nil
}

// List makes a request to the /api/tags endpoint to list available models.
func (c *Client) List(ctx context.Context) (*api.ListResponse, error) {
	var resp api.ListResponse
	err := c.doRequest(ctx, http.MethodGet, "/api/tags", nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("list models request failed: %w", err)
	}
	return &resp, nil
}

// Show makes a request to the /api/show endpoint to get model information.
func (c *Client) Show(ctx context.Context, req *api.ShowRequest) (*api.ShowResponse, error) {
	var resp api.ShowResponse
	err := c.doRequest(ctx, http.MethodPost, "/api/show", req, &resp)
	if err != nil {
		return nil, fmt.Errorf("show model request failed: %w", err)
	}
	return &resp, nil
}

// Pull makes a streaming request to the /api/pull endpoint to download a model.
// The callback function is called for each progress update received.
func (c *Client) Pull(ctx context.Context, req *PullRequest, callback func(api.ProgressResponse) error) error {
	return c.streamRequest(ctx, http.MethodPost, "/api/pull", req, func(data []byte) error {
		var resp api.ProgressResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return fmt.Errorf("failed to unmarshal pull response: %w", err)
		}
		return callback(resp)
	})
}

// GetBaseURL returns the base URL of the Ollama server.
func (c *Client) GetBaseURL() *url.URL {
	return c.baseURL
}

// GetHTTPClient returns the underlying HTTP client.
func (c *Client) GetHTTPClient() *http.Client {
	return c.httpClient
}

// doRequest makes a single, non-streaming HTTP request.
func (c *Client) doRequest(ctx context.Context, method, path string, reqData, respData any) error {
	reqBody, err := c.prepareRequestBody(reqData)
	if err != nil {
		return err
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

	return c.handleResponse(response, respData)
}

// prepareRequestBody marshals the request data to JSON if provided.
func (c *Client) prepareRequestBody(reqData any) (io.Reader, error) {
	if reqData == nil {
		return nil, nil
	}

	data, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request data: %w", err)
	}

	return bytes.NewReader(data), nil
}

// buildRequest creates an HTTP request with the given parameters.
func (c *Client) buildRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	requestURL := c.baseURL.JoinPath(path)

	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	return request, nil
}

// setJSONHeaders sets headers for JSON API requests.
func (c *Client) setJSONHeaders(request *http.Request) {
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
}

// handleResponse processes a non-streaming HTTP response.
func (c *Client) handleResponse(response *http.Response, respData any) error {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if err := c.checkError(response, body); err != nil {
		return err
	}

	if len(body) > 0 && respData != nil {
		if err := json.Unmarshal(body, respData); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// checkError examines the HTTP response for API errors and returns an appropriate error.
func (c *Client) checkError(response *http.Response, body []byte) error {
	if response.StatusCode < http.StatusBadRequest {
		return nil
	}

	var apiError struct {
		Error string `json:"error"`
	}

	if err := json.Unmarshal(body, &apiError); err != nil {
		return fmt.Errorf("ollama API error (status %d): %s", response.StatusCode, string(body))
	}

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
