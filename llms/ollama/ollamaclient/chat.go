package ollamaclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/ollama/ollama/api"
)

const (
	MaxBufferSize = 512 * 1024
)

var bufferPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

func (c *Client) GenerateChat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error {
	if req.Stream == nil || !*req.Stream {
		var finalResp api.ChatResponse
		var accumulatedContent strings.Builder

		err := c.streamRequest(ctx, http.MethodPost, "/api/chat", req, func(data []byte) error {
			var resp api.ChatResponse
			if err := json.Unmarshal(data, &resp); err != nil {
				return fmt.Errorf("unmarshal chat chunk: %w", err)
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

		finalResp.Message.Content = accumulatedContent.String()
		return fn(finalResp)
	}

	// Streaming mode: pass each chunk to callback.
	return c.streamRequest(ctx, http.MethodPost, "/api/chat", req, func(data []byte) error {
		var resp api.ChatResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return fmt.Errorf("unmarshal streaming chat chunk: %w", err)
		}
		return fn(resp)
	})
}

func (c *Client) streamRequest(ctx context.Context, method, path string, reqData any, callback func([]byte) error) error {
	buf, ok := bufferPool.Get().(*bytes.Buffer)
	if !ok {
		return errors.New("failed get data from buffer")
	}
	buf.Reset()
	defer bufferPool.Put(buf)

	encoder := json.NewEncoder(buf)
	if reqData != nil {
		if err := encoder.Encode(reqData); err != nil {
			c.logger.Debug("error marshalling request body", "error", err)
			return fmt.Errorf("marshal request body: %w", err)
		}
	}

	requestURL := c.baseURL.JoinPath(path)
	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), buf)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		c.logger.Error("Ollama API stream request failed",
			"status", resp.StatusCode,
			"method", method,
			"url", requestURL.String(),
			"response_body", string(body),
		)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, MaxBufferSize), MaxBufferSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if err := callback(line); err != nil {
			return fmt.Errorf("callback error: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream read error: %w", err)
	}

	return nil
}
