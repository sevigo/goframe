package agent

import (
	"context"
	"sync"
)

type EventType string

const (
	EventTypeMessage    EventType = "message"
	EventTypePart       EventType = "part"
	EventTypeTool       EventType = "tool"
	EventTypeFile       EventType = "file"
	EventTypeError      EventType = "error"
	EventTypeComplete   EventType = "complete"
	EventTypePermission EventType = "permission"
)

type Event struct {
	Type      EventType
	SessionID string
	MessageID string
	PartID    string
	Data      interface{}
	Error     error
}

type EventHandlerFunc func(ctx context.Context, event Event) error

type EventHandler struct {
	handlers map[EventType][]EventHandlerFunc
	mu       sync.RWMutex
}

func NewEventHandler() *EventHandler {
	return &EventHandler{
		handlers: make(map[EventType][]EventHandlerFunc),
	}
}

func (h *EventHandler) On(eventType EventType, handler EventHandlerFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[eventType] = append(h.handlers[eventType], handler)
}

func (h *EventHandler) OnMessage(handler func(ctx context.Context, msg Message) error) {
	h.On(EventTypeMessage, func(ctx context.Context, event Event) error {
		if msg, ok := event.Data.(Message); ok {
			return handler(ctx, msg)
		}
		return nil
	})
}

func (h *EventHandler) OnTextPart(handler func(ctx context.Context, text string) error) {
	h.On(EventTypePart, func(ctx context.Context, event Event) error {
		if part, ok := event.Data.(TextPartData); ok {
			return handler(ctx, part.Text)
		}
		return nil
	})
}

func (h *EventHandler) OnToolCall(handler func(ctx context.Context, tool ToolCall) error) {
	h.On(EventTypeTool, func(ctx context.Context, event Event) error {
		if tool, ok := event.Data.(ToolCall); ok {
			return handler(ctx, tool)
		}
		return nil
	})
}

func (h *EventHandler) OnFile(handler func(ctx context.Context, file FileInfo) error) {
	h.On(EventTypeFile, func(ctx context.Context, event Event) error {
		if file, ok := event.Data.(FileInfo); ok {
			return handler(ctx, file)
		}
		return nil
	})
}

func (h *EventHandler) OnError(handler func(ctx context.Context, err error) error) {
	h.On(EventTypeError, func(ctx context.Context, event Event) error {
		if event.Error != nil {
			return handler(ctx, event.Error)
		}
		return nil
	})
}

func (h *EventHandler) OnComplete(handler func(ctx context.Context, resp Response) error) {
	h.On(EventTypeComplete, func(ctx context.Context, event Event) error {
		if resp, ok := event.Data.(Response); ok {
			return handler(ctx, resp)
		}
		return nil
	})
}

func (h *EventHandler) OnPermission(handler func(ctx context.Context, req PermissionRequest) error) {
	h.On(EventTypePermission, func(ctx context.Context, event Event) error {
		if req, ok := event.Data.(PermissionRequest); ok {
			return handler(ctx, req)
		}
		return nil
	})
}

func (h *EventHandler) Handle(ctx context.Context, event Event) error {
	h.mu.RLock()
	handlers := h.handlers[event.Type]
	h.mu.RUnlock()

	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (h *EventHandler) Clear(eventType EventType) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.handlers, eventType)
}

func (h *EventHandler) ClearAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers = make(map[EventType][]EventHandlerFunc)
}

type TextPartData struct {
	Text string
}

type ToolCall struct {
	ID     string
	Name   string
	Input  map[string]interface{}
	State  string
	Output interface{}
	Error  string
}

type FileInfo struct {
	Path     string
	Content  []byte
	MimeType string
}

type Message struct {
	ID        string
	Role      string
	SessionID string
	Content   string
	Parts     []Part
	CreatedAt int64
}

type Part struct {
	ID   string
	Type string
	Text string
	Tool *ToolCall
	File *FileInfo
}

type Response struct {
	SessionID string
	MessageID string
	Content   string
	Parts     []Part
	Tokens    TokenUsage
	Cost      float64
	Error     error
}

type TokenUsage struct {
	Input      float64
	Output     float64
	Reasoning  float64
	CacheRead  float64
	CacheWrite float64
}
