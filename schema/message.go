package schema

import "strings"

// ChatMessageType represents the role of a message in a conversation.
type ChatMessageType string

// Chat message type constants.
const (
	// ChatMessageTypeSystem represents a system message that sets behavior.
	ChatMessageTypeSystem ChatMessageType = "system"
	// ChatMessageTypeHuman represents a user message.
	ChatMessageTypeHuman ChatMessageType = "human"
	// ChatMessageTypeAI represents an assistant/AI message.
	ChatMessageTypeAI ChatMessageType = "ai"
	// ChatMessageTypeGeneric represents a generic message type.
	ChatMessageTypeGeneric ChatMessageType = "generic"
)

// ContentPart represents a part of a message content.
// Content parts can be text, images, or other multimodal content.
type ContentPart interface {
	String() string
	isPart()
}

// TextContent represents text content in a message.
type TextContent struct {
	// Text is the text content.
	Text string
}

// String returns the text content.
func (tc TextContent) String() string {
	return tc.Text
}

// isPart marks TextContent as implementing ContentPart.
func (TextContent) isPart() {}

// MessageContent represents a message in a conversation with a role and content parts.
type MessageContent struct {
	// Role is the role of the message sender.
	Role ChatMessageType
	// Parts contains the content parts of the message.
	Parts []ContentPart
}

// NewTextMessage creates a new message with text content.
func NewTextMessage(role ChatMessageType, text string) MessageContent {
	return MessageContent{
		Role:  role,
		Parts: []ContentPart{TextContent{Text: text}},
	}
}

// NewSystemMessage creates a new system message with the given text.
func NewSystemMessage(text string) MessageContent {
	return NewTextMessage(ChatMessageTypeSystem, text)
}

// NewHumanMessage creates a new human/user message with the given text.
func NewHumanMessage(text string) MessageContent {
	return NewTextMessage(ChatMessageTypeHuman, text)
}

// NewAIMessage creates a new AI/assistant message with the given text.
func NewAIMessage(text string) MessageContent {
	return NewTextMessage(ChatMessageTypeAI, text)
}

// String returns the concatenated text of all content parts.
func (mc MessageContent) String() string {
	if len(mc.Parts) == 0 {
		return ""
	}

	parts := make([]string, 0, len(mc.Parts))
	for _, part := range mc.Parts {
		if s := part.String(); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

// GetTextContent returns the text content of the message.
func (mc MessageContent) GetTextContent() string {
	return mc.String()
}