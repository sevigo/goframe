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
	// ChatMessageTypeTool represents a tool result message.
	ChatMessageTypeTool ChatMessageType = "tool"
)

// ContentPart represents a part of a message content.
// Content parts can be text, images, tool calls, or tool results.
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

// ImageContent represents image content in a message (base64-encoded).
type ImageContent struct {
	// Data is the base64-encoded image data.
	Data string
	// MimeType is the MIME type of the image (e.g., "image/png", "image/jpeg").
	MimeType string
}

// String returns a placeholder for image content.
func (ic ImageContent) String() string {
	if ic.MimeType != "" {
		return "[image:" + ic.MimeType + "]"
	}
	return "[image]"
}

// isPart marks ImageContent as implementing ContentPart.
func (ImageContent) isPart() {}

// ToolCallContent represents a tool call request from the AI within a message.
// This is used to preserve tool call information (including IDs) when
// replaying conversation history back to LLM providers like OpenAI.
type ToolCallContent struct {
	// ID is the unique identifier for the tool call.
	ID string
	// FunctionName is the name of the function to call.
	FunctionName string
	// Arguments is the function call arguments.
	Arguments map[string]any
}

// String returns the function name of the tool call.
func (tcc ToolCallContent) String() string {
	return tcc.FunctionName
}

// isPart marks ToolCallContent as implementing ContentPart.
func (ToolCallContent) isPart() {}

// ToolResultContent represents a tool execution result in a message.
type ToolResultContent struct {
	// ToolName is the name of the tool that was executed.
	ToolName string
	// ToolCallID is the unique identifier for the tool call (required by some providers like OpenAI).
	ToolCallID string
	// Content is the result of the tool execution.
	Content string
}

// String returns the tool result content.
func (trc ToolResultContent) String() string {
	return trc.Content
}

// isPart marks ToolResultContent as implementing ContentPart.
func (ToolResultContent) isPart() {}

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

// NewAIMessageWithToolCalls creates an AI message with text content and tool calls.
// The text is optional; if empty, only tool call parts are included.
func NewAIMessageWithToolCalls(text string, toolCalls []ToolCallContent) MessageContent {
	parts := make([]ContentPart, 0, 1+len(toolCalls))
	if text != "" {
		parts = append(parts, TextContent{Text: text})
	}
	for _, tc := range toolCalls {
		parts = append(parts, tc)
	}
	return MessageContent{
		Role:  ChatMessageTypeAI,
		Parts: parts,
	}
}

// NewToolResultMessage creates a tool result message with the given tool name and content.
//
// Deprecated: Use NewToolResultMessageWithID instead. Providers like OpenAI require
// a tool call ID to match results with requests; messages created with this function
// have an empty ToolCallID, which will cause errors with OpenAI's API.
func NewToolResultMessage(toolName, content string) MessageContent {
	return MessageContent{
		Role: ChatMessageTypeTool,
		Parts: []ContentPart{ToolResultContent{
			ToolName: toolName,
			Content:  content,
		}},
	}
}

// NewToolResultMessageWithID creates a tool result message with tool name, content, and tool call ID.
// The tool call ID is required by providers like OpenAI to match the result with the original call.
func NewToolResultMessageWithID(toolName, toolCallID, content string) MessageContent {
	return MessageContent{
		Role: ChatMessageTypeTool,
		Parts: []ContentPart{ToolResultContent{
			ToolName:   toolName,
			ToolCallID: toolCallID,
			Content:    content,
		}},
	}
}

// NewHumanMessageWithImage creates a human message with text and an image.
func NewHumanMessageWithImage(text string, imageData, mimeType string) MessageContent {
	return MessageContent{
		Role: ChatMessageTypeHuman,
		Parts: []ContentPart{
			TextContent{Text: text},
			ImageContent{Data: imageData, MimeType: mimeType},
		},
	}
}

// GetImages extracts all images from the message parts.
func (mc MessageContent) GetImages() []ImageContent {
	var images []ImageContent
	for _, part := range mc.Parts {
		if img, ok := part.(ImageContent); ok {
			images = append(images, img)
		}
	}
	return images
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
