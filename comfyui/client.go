package comfyui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sevigo/goframe/httpclient"
)

type Client struct {
	httpClient *http.Client
	retryCfg   httpclient.RetryConfig
	baseURL    string
	wsURL      string
	logger     *slog.Logger
	clientID   string
	ownsClient bool
	connMu     sync.Mutex
	wsConn     *websocket.Conn
}

func New(opts ...Option) (*Client, error) {
	o := applyOptions(opts...)

	var ownsClient bool
	httpClient := o.httpClient
	if httpClient == nil {
		httpClient = httpclient.NewClient(httpclient.NewConfig(
			httpclient.WithTimeout(o.requestTimeout),
		))
		ownsClient = true
	}

	retryCfg := o.retry

	c := &Client{
		httpClient: httpClient,
		retryCfg:   retryCfg,
		baseURL:    o.baseURL(),
		wsURL:      o.wsURL(),
		logger:     o.logger.With("component", "comfyui_client"),
		clientID:   o.clientID,
		ownsClient: ownsClient,
	}

	return c, nil
}

func (c *Client) Close() error {
	if c.ownsClient && c.httpClient != nil {
		if tr, ok := c.httpClient.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.wsConn != nil {
		_ = c.wsConn.Close()
		c.wsConn = nil
	}
	return nil
}

func (c *Client) QueuePrompt(ctx context.Context, workflow *Workflow) (*PromptResponse, error) {
	data, err := workflow.Marshal()
	if err != nil {
		return nil, fmt.Errorf("comfyui: failed to marshal workflow: %w", err)
	}

	var promptReq PromptRequest
	if unmarshalErr := json.Unmarshal(data, &promptReq); unmarshalErr != nil {
		return nil, fmt.Errorf("comfyui: failed to unmarshal workflow: %w", unmarshalErr)
	}
	promptReq.ClientID = c.clientID

	body, err := json.Marshal(promptReq)
	if err != nil {
		return nil, fmt.Errorf("comfyui: failed to marshal prompt request: %w", err)
	}

	var promptResp PromptResponse
	err = httpclient.DoWithRetry(ctx, &c.retryCfg, "comfyui queue prompt", func() error {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/prompt", bytes.NewReader(body))
		if reqErr != nil {
			return reqErr
		}
		req.Header.Set("Content-Type", "application/json")

		resp, respErr := c.httpClient.Do(req)
		if respErr != nil {
			return respErr
		}
		defer resp.Body.Close()

		respBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return readErr
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			return ErrQueueFull
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("comfyui: server error %d: %s", resp.StatusCode, string(respBytes))
		}

		return json.Unmarshal(respBytes, &promptResp)
	})
	if err != nil {
		return nil, fmt.Errorf("comfyui: failed to queue prompt: %w", err)
	}

	return &promptResp, nil
}

func (c *Client) GetHistory(ctx context.Context, promptID string) (*HistoryEntry, error) {
	histURL := c.baseURL + "/history/" + url.PathEscape(promptID)

	var history map[string]HistoryEntry
	err := httpclient.DoWithRetry(ctx, &c.retryCfg, "comfyui get history", func() error {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, histURL, nil)
		if reqErr != nil {
			return reqErr
		}

		resp, respErr := c.httpClient.Do(req)
		if respErr != nil {
			return respErr
		}
		defer resp.Body.Close()

		respBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return readErr
		}

		return json.Unmarshal(respBytes, &history)
	})
	if err != nil {
		return nil, fmt.Errorf("comfyui: failed to get history: %w", err)
	}

	entry, ok := history[promptID]
	if !ok {
		return nil, ErrImageNotFound
	}
	return &entry, nil
}

func (c *Client) GetImage(ctx context.Context, filename string, subfolder string, imageType string) ([]byte, error) {
	params := url.Values{}
	params.Set("filename", filename)
	if subfolder != "" {
		params.Set("subfolder", subfolder)
	}
	if imageType != "" {
		params.Set("type", imageType)
	}

	imageURL := c.baseURL + "/view?" + params.Encode()

	var imageData []byte
	err := httpclient.DoWithRetry(ctx, &c.retryCfg, "comfyui get image", func() error {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
		if reqErr != nil {
			return reqErr
		}

		resp, respErr := c.httpClient.Do(req)
		if respErr != nil {
			return respErr
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return ErrImageNotFound
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("comfyui: server error %d", resp.StatusCode)
		}

		data, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return readErr
		}
		imageData = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("comfyui: failed to get image: %w", err)
	}

	return imageData, nil
}

func (c *Client) UploadImage(ctx context.Context, filename string, imageData []byte, overwrite bool, imageType string) (*UploadResponse, error) {
	var uploadResp UploadResponse

	err := httpclient.DoWithRetry(ctx, &c.retryCfg, "comfyui upload image", func() error {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)

		part, formErr := writer.CreateFormFile("image", filename)
		if formErr != nil {
			return fmt.Errorf("comfyui: failed to create form file: %w", formErr)
		}
		if _, writeErr := part.Write(imageData); writeErr != nil {
			return fmt.Errorf("comfyui: failed to write image data: %w", writeErr)
		}

		if overwrite {
			if fieldErr := writer.WriteField("overwrite", "true"); fieldErr != nil {
				return fmt.Errorf("comfyui: failed to write overwrite field: %w", fieldErr)
			}
		}
		if imageType != "" {
			if fieldErr := writer.WriteField("type", imageType); fieldErr != nil {
				return fmt.Errorf("comfyui: failed to write type field: %w", fieldErr)
			}
		}

		if closeErr := writer.Close(); closeErr != nil {
			return fmt.Errorf("comfyui: failed to close multipart writer: %w", closeErr)
		}

		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/upload/image", &buf)
		if reqErr != nil {
			return reqErr
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())

		resp, respErr := c.httpClient.Do(req)
		if respErr != nil {
			return respErr
		}
		defer resp.Body.Close()

		respBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return readErr
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("comfyui: upload failed %d: %s", resp.StatusCode, string(respBytes))
		}

		return json.Unmarshal(respBytes, &uploadResp)
	})
	if err != nil {
		return nil, fmt.Errorf("comfyui: failed to upload image: %w", err)
	}
	return &uploadResp, nil
}

func (c *Client) GetQueue(ctx context.Context) (*QueueInfo, error) {
	var queue QueueInfo
	err := httpclient.DoWithRetry(ctx, &c.retryCfg, "comfyui get queue", func() error {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/queue", nil)
		if reqErr != nil {
			return reqErr
		}

		resp, respErr := c.httpClient.Do(req)
		if respErr != nil {
			return respErr
		}
		defer resp.Body.Close()

		respBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return readErr
		}

		return json.Unmarshal(respBytes, &queue)
	})
	if err != nil {
		return nil, fmt.Errorf("comfyui: failed to get queue: %w", err)
	}
	return &queue, nil
}

func (c *Client) Interrupt(ctx context.Context) error {
	body, _ := json.Marshal(map[string]any{})
	return httpclient.DoWithRetry(ctx, &c.retryCfg, "comfyui interrupt", func() error {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/interrupt", bytes.NewReader(body))
		if reqErr != nil {
			return reqErr
		}
		req.Header.Set("Content-Type", "application/json")

		resp, respErr := c.httpClient.Do(req)
		if respErr != nil {
			return respErr
		}
		defer resp.Body.Close()
		return nil
	})
}

func (c *Client) StreamProgress(ctx context.Context) (<-chan ProgressEvent, error) {
	ch := make(chan ProgressEvent, 64)

	c.connMu.Lock()
	if c.wsConn != nil {
		_ = c.wsConn.Close()
	}
	c.connMu.Unlock()

	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, c.wsURL, nil)
	if err != nil {
		if resp != nil {
			resp.Body.Close() //nolint:gosec // best-effort cleanup on error path
		}
		return nil, fmt.Errorf("comfyui: failed to connect websocket: %w", err)
	}
	resp.Body.Close() //nolint:gosec // best-effort cleanup, connection succeeded

	c.connMu.Lock()
	c.wsConn = conn
	c.connMu.Unlock()

	go c.readWebSocket(ctx, conn, ch)

	return ch, nil
}

func (c *Client) readWebSocket(ctx context.Context, conn *websocket.Conn, ch chan<- ProgressEvent) {
	defer close(ch)
	defer func() {
		c.connMu.Lock()
		if c.wsConn == conn {
			c.wsConn = nil
		}
		c.connMu.Unlock()
		_ = conn.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			c.logger.DebugContext(ctx, "WebSocket read error", "error", err)
			return
		}

		var msg map[string]any
		if unmarshalErr := json.Unmarshal(message, &msg); unmarshalErr != nil {
			continue
		}

		msgType, ok := msg["type"].(string)
		if !ok {
			continue
		}

		data, ok := msg["data"].(map[string]any)
		if !ok {
			continue
		}

		c.handleWSMessage(ctx, msgType, data, ch)
	}
}

func (c *Client) handleWSMessage(ctx context.Context, msgType string, data map[string]any, ch chan<- ProgressEvent) {
	switch msgType {
	case "progress":
		value, _ := data["value"].(float64)
		maxVal, _ := data["max"].(float64)
		promptID, _ := data["prompt_id"].(string)

		select {
		case ch <- ProgressEvent{
			Step:     int(value),
			MaxSteps: int(maxVal),
			Value:    value,
			Max:      maxVal,
			Phase:    "sampling",
			PromptID: promptID,
		}:
		default:
		}

	case "execution_start":
		promptID, _ := data["prompt_id"].(string)
		select {
		case ch <- ProgressEvent{Phase: "start", PromptID: promptID}:
		default:
		}

	case "execution_error":
		promptID, _ := data["prompt_id"].(string)
		select {
		case ch <- ProgressEvent{Phase: "error", PromptID: promptID}:
		default:
		}

	case "executing":
		promptID, _ := data["prompt_id"].(string)
		nodeID, _ := data["node"].(string)

		if nodeID == "" {
			step := 0
			if value, ok := data["value"].(float64); ok {
				step = int(value)
			}
			maxSteps := 0
			if maxVal, ok := data["max"].(float64); ok {
				maxSteps = int(maxVal)
			}
			select {
			case ch <- ProgressEvent{
				Phase:    "complete",
				PromptID: promptID,
				Step:     step,
				MaxSteps: maxSteps,
			}:
			default:
			}
		}
	}
}

func (c *Client) Generate(ctx context.Context, workflow *Workflow) (*ImageResult, error) {
	start := time.Now()

	promptResp, err := c.QueuePrompt(ctx, workflow)
	if err != nil {
		return nil, err
	}
	promptID := promptResp.PromptID

	progressCh, err := c.StreamProgress(ctx)
	if err != nil {
		return nil, fmt.Errorf("comfyui: failed to stream progress: %w", err)
	}

	for event := range progressCh {
		if event.PromptID != "" && event.PromptID != promptID {
			continue
		}

		if event.Phase == "complete" {
			break
		}
		if event.Phase == "error" {
			return nil, ErrPromptFailed
		}

		c.logger.DebugContext(ctx, "Generation progress",
			"step", event.Step, "max_steps", event.MaxSteps,
			"phase", event.Phase, "prompt_id", event.PromptID,
		)
	}

	history, err := c.GetHistory(ctx, promptID)
	if err != nil {
		return nil, fmt.Errorf("comfyui: failed to get history: %w", err)
	}

	result := &ImageResult{
		PromptID: promptID,
		Duration: time.Since(start),
	}

	c.collectImages(ctx, history, result)

	return result, nil
}

func (c *Client) collectImages(ctx context.Context, history *HistoryEntry, result *ImageResult) {
	for _, output := range history.Outputs {
		nodeOutput, ok := output.(map[string]any)
		if !ok {
			continue
		}

		imagesList, ok := nodeOutput["images"].([]any)
		if !ok {
			continue
		}

		for _, img := range imagesList {
			imgMap, ok := img.(map[string]any)
			if !ok {
				continue
			}

			filename, _ := imgMap["filename"].(string)
			subfolder, _ := imgMap["subfolder"].(string)
			imgType, _ := imgMap["type"].(string)
			result.Filenames = append(result.Filenames, filename)

			imageData, imgErr := c.GetImage(ctx, filename, subfolder, imgType)
			if imgErr != nil {
				c.logger.WarnContext(ctx, "Failed to download image",
					"filename", filename, "error", imgErr)
				continue
			}
			result.Images = append(result.Images, imageData)
		}
	}
}

func (c *Client) GenerateAsync(ctx context.Context, workflow *Workflow) (<-chan ProgressEvent, string, error) {
	promptResp, err := c.QueuePrompt(ctx, workflow)
	if err != nil {
		return nil, "", err
	}

	progressCh, err := c.StreamProgress(ctx)
	if err != nil {
		return nil, promptResp.PromptID, fmt.Errorf("comfyui: failed to stream progress: %w", err)
	}

	return progressCh, promptResp.PromptID, nil
}
