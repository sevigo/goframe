package ollama

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/schema"
)

func TestUnit_BuildChatMessagesWithImages(t *testing.T) {
	testImageBase64 := "dGVzdF9pbWFnZV9kYXRh"

	messages := []schema.MessageContent{
		{
			Role: schema.ChatMessageTypeHuman,
			Parts: []schema.ContentPart{
				schema.TextContent{Text: "What is this?"},
				schema.ImageContent{Data: testImageBase64, MimeType: "image/png"},
			},
		},
	}

	require.Len(t, messages, 1, "Should have exactly one message")
	require.Len(t, messages[0].Parts, 2, "Should have exactly two content parts")

	images := messages[0].GetImages()
	assert.Len(t, images, 1, "Should extract exactly one image")
	assert.Equal(t, testImageBase64, images[0].Data, "Image data should match")
	assert.Equal(t, "image/png", images[0].MimeType, "MIME type should match")
}

func TestUnit_Base64Decoding(t *testing.T) {
	sampleImageBase64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	_, err := base64.StdEncoding.DecodeString(sampleImageBase64)
	require.NoError(t, err, "Sample image should be valid base64")
}

func TestUnit_BuildChatMessagesWithToolCalls(t *testing.T) {
	llm, _ := New(WithModel("test-model"))

	messages := []schema.MessageContent{
		schema.NewHumanMessage("What's the weather?"),
		schema.NewAIMessageWithToolCalls("Let me check.", []schema.ToolCallContent{
			{
				ID:           "call_001",
				FunctionName: "get_weather",
				Arguments:    map[string]any{"location": "Paris"},
			},
		}),
		schema.NewToolResultMessageWithID("get_weather", "call_001", `{"temp": 18}`),
	}

	result := llm.buildChatMessages(messages)
	assert.Len(t, result, 3)

	assert.Equal(t, "user", result[0].Role)
	assert.Equal(t, "What's the weather?", result[0].Content)

	assert.Equal(t, "assistant", result[1].Role)
	assert.Equal(t, "Let me check.", result[1].Content)
	assert.Len(t, result[1].ToolCalls, 1)
	assert.Equal(t, "call_001", result[1].ToolCalls[0].ID)
	assert.Equal(t, "get_weather", result[1].ToolCalls[0].Function.Name)

	assert.Equal(t, "tool", result[2].Role)
	assert.Equal(t, `{"temp": 18}`, result[2].Content)
	assert.Equal(t, "get_weather", result[2].ToolName)
	assert.Equal(t, "call_001", result[2].ToolCallID)
}

func TestUnit_BuildChatMessagesPlain(t *testing.T) {
	llm, _ := New(WithModel("test-model"))

	messages := []schema.MessageContent{
		schema.NewSystemMessage("You are helpful."),
		schema.NewHumanMessage("Hello!"),
		schema.NewAIMessage("Hi there!"),
	}

	result := llm.buildChatMessages(messages)
	assert.Len(t, result, 3)
	assert.Equal(t, "system", result[0].Role)
	assert.Equal(t, "You are helpful.", result[0].Content)
	assert.Equal(t, "user", result[1].Role)
	assert.Equal(t, "Hello!", result[1].Content)
	assert.Equal(t, "assistant", result[2].Role)
	assert.Equal(t, "Hi there!", result[2].Content)
}
func TestUnit_NewHumanMessageWithImage(t *testing.T) {
	imageData := "aGVsbG8gd29ybGQ="
	msg := schema.NewHumanMessageWithImage("Describe this image", imageData, "image/png")

	assert.Equal(t, schema.ChatMessageTypeHuman, msg.Role)
	require.Len(t, msg.Parts, 2)

	textPart, ok := msg.Parts[0].(schema.TextContent)
	require.True(t, ok)
	assert.Equal(t, "Describe this image", textPart.Text)

	imagePart, ok := msg.Parts[1].(schema.ImageContent)
	require.True(t, ok)
	assert.Equal(t, imageData, imagePart.Data)
	assert.Equal(t, "image/png", imagePart.MimeType)
}
