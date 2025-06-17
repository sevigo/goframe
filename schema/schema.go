package schema

import (
	"context"
	"fmt"
)

type Document struct {
	PageContent string
	Metadata    map[string]any
}

func (d Document) String() string {
	return d.PageContent
}

func NewDocument(content string, metadata map[string]any) Document {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	return Document{
		PageContent: content,
		Metadata:    metadata,
	}
}

type ModelDetails struct {
	Family        string
	ParameterSize string
	Quantization  string
	Dimension     int64
}

func (md ModelDetails) String() string {
	return fmt.Sprintf("%s (%s, %s, dim: %d)",
		md.Family, md.ParameterSize, md.Quantization, md.Dimension)
}

type Retriever interface {
	GetRelevantDocuments(ctx context.Context, query string) ([]Document, error)
}

type ChatMessage interface {
	GetType() ChatMessageType
	GetContent() string
}

type SystemChatMessage struct {
	Content string
}

func (m SystemChatMessage) GetType() ChatMessageType {
	return ChatMessageTypeSystem
}

func (m SystemChatMessage) GetContent() string {
	return m.Content
}

type HumanChatMessage struct {
	Content string
}

func (m HumanChatMessage) GetType() ChatMessageType {
	return ChatMessageTypeHuman
}

func (m HumanChatMessage) GetContent() string {
	return m.Content
}

type AIChatMessage struct {
	Content string
}

func (m AIChatMessage) GetType() ChatMessageType {
	return ChatMessageTypeAI
}

func (m AIChatMessage) GetContent() string {
	return m.Content
}

// Convenience constructors
func NewSystemChatMessage(content string) SystemChatMessage {
	return SystemChatMessage{Content: content}
}

func NewHumanChatMessage(content string) HumanChatMessage {
	return HumanChatMessage{Content: content}
}

func NewAIChatMessage(content string) AIChatMessage {
	return AIChatMessage{Content: content}
}

func ChatMessageToString(msg ChatMessage) string {
	return fmt.Sprintf("[%s]: %s", msg.GetType(), msg.GetContent())
}

type CollectionInfo struct {
	Name           string `json:"name"`            // Name of the collection.
	PointsCount    uint64 `json:"points_count"`    // Number of points (vectors) in the collection.
	VectorSize     uint64 `json:"vector_size"`     // Dimensionality of the vectors in this collection.
	VectorDistance string `json:"vector_distance"` // Distance metric used by the collection (e.g., "Cosine").
}
