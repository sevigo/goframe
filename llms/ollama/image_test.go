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
