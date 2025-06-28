package ollamaclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
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
}

var jsonBufferPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

type closerFunc func()

func (f closerFunc) Close() error {
	f()
	return nil
}

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
			Transport: &http.Transport{
				MaxIdleConns:       100,
				IdleConnTimeout:    90 * time.Second,
				DisableCompression: false,
				MaxConnsPerHost:    100,
				ForceAttemptHTTP2:  true,
			},
		}
	}

	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
	}, nil
}

func NewDefaultClient() (*Client, error) {
	return NewClient(nil, nil)
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
	reqBody, err := c.prepareRequestBody(reqData)
	if err != nil {
		return err
	}
	defer func() {
		if closer, ok := reqBody.(io.Closer); ok {
			_ = closer.Close()
		}
	}()

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

func (c *Client) prepareRequestBody(reqData any) (io.Reader, error) {
	buf, ok := jsonBufferPool.Get().(*bytes.Buffer)
	if !ok {
		return nil, errors.New("failed get data from buffer")
	}
	buf.Reset()

	if err := json.NewEncoder(buf).Encode(reqData); err != nil {
		jsonBufferPool.Put(buf)
		return nil, fmt.Errorf("failed to encode request data: %w", err)
	}

	return struct {
		io.Reader
		io.Closer
	}{
		Reader: buf,
		Closer: closerFunc(func() {
			jsonBufferPool.Put(buf)
		}),
	}, nil
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

	if err := json.NewDecoder(response.Body).Decode(&apiError); err != nil {
		return fmt.Errorf("ollama API error (status %d): %s", response.StatusCode, http.StatusText(response.StatusCode))
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
