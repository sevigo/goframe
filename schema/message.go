package schema

import "strings"

type ChatMessageType string

const (
	ChatMessageTypeSystem  ChatMessageType = "system"
	ChatMessageTypeHuman   ChatMessageType = "human"
	ChatMessageTypeAI      ChatMessageType = "ai"
	ChatMessageTypeGeneric ChatMessageType = "generic"
)

type ContentPart interface {
	String() string
	isPart()
}

type TextContent struct {
	Text string
}

func (tc TextContent) String() string {
	return tc.Text
}

func (TextContent) isPart() {}

type MessageContent struct {
	Role  ChatMessageType
	Parts []ContentPart
}

func (mc MessageContent) String() string {
	if len(mc.Parts) == 0 {
		return ""
	}

	var parts []string
	for _, part := range mc.Parts {
		if s := part.String(); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

func (mc MessageContent) GetTextContent() string {
	return mc.String()
}

func NewTextMessage(role ChatMessageType, text string) MessageContent {
	return MessageContent{
		Role:  role,
		Parts: []ContentPart{TextContent{Text: text}},
	}
}

func NewSystemMessage(text string) MessageContent {
	return NewTextMessage(ChatMessageTypeSystem, text)
}

func NewHumanMessage(text string) MessageContent {
	return NewTextMessage(ChatMessageTypeHuman, text)
}

func NewAIMessage(text string) MessageContent {
	return NewTextMessage(ChatMessageTypeAI, text)
}
