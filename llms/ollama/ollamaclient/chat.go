package ollamaclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ollama/ollama/api"
)

// GenerateChat makes a request to the /api/chat endpoint.
// It handles both streaming and non-streaming modes based on req.Stream.
func (c *Client) GenerateChat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error {
	// For non-streaming, accumulate the response and call the callback once at the end.
	if !*req.Stream {
		var finalResp api.ChatResponse
		var accumulatedContent strings.Builder

		err := c.streamRequest(ctx, http.MethodPost, "/api/chat", req, func(data []byte) error {
			var resp api.ChatResponse
			if err := json.Unmarshal(data, &resp); err != nil {
				return fmt.Errorf("failed to unmarshal chat response chunk: %w", err)
			}

			if resp.Message.Content != "" {
				accumulatedContent.WriteString(resp.Message.Content)
			}
			if resp.Done {
				finalResp = resp
			}
			return nil
		})

		if err != nil {
			return err
		}

		// Set the full accumulated content on the final response object
		if finalResp.Message.Content == "" {
			finalResp.Message = api.Message{}
		}
		finalResp.Message.Content = accumulatedContent.String()
		return fn(finalResp)
	}

	// For streaming, call the callback for each chunk.
	return c.streamRequest(ctx, http.MethodPost, "/api/chat", req, func(data []byte) error {
		var resp api.ChatResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return fmt.Errorf("failed to unmarshal chat response chunk: %w", err)
		}
		return fn(resp)
	})
}

// streamRequest is a generic helper to handle streaming API requests.
func (c *Client) streamRequest(ctx context.Context, method, path string, reqData any, callback func([]byte) error) error {
	reqBody, err := json.Marshal(reqData)
	if err != nil {
		return fmt.Errorf("failed to marshal request data: %w", err)
	}

	requestURL := c.baseURL.JoinPath(path)
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/x-ndjson")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("received error response (status %d): %s", response.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, MaxBufferSize), MaxBufferSize)

	for scanner.Scan() {
		if err := callback(scanner.Bytes()); err != nil {
			return err
		}
	}

	return scanner.Err()
}
