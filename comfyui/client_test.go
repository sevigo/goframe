package comfyui

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDefaultOptions(t *testing.T) {
	o := applyOptions()

	assert.Equal(t, "127.0.0.1:8188", o.host)
	assert.Equal(t, "goframe-comfyui", o.clientID)
	assert.Equal(t, 3, o.retry.Attempts)
	assert.Equal(t, 120*time.Second, o.requestTimeout)
}

func TestWithHost(t *testing.T) {
	o := applyOptions(WithHost("192.168.1.100:8188"))
	assert.Equal(t, "192.168.1.100:8188", o.host)
}

func TestWithClientID(t *testing.T) {
	o := applyOptions(WithClientID("test-client"))
	assert.Equal(t, "test-client", o.clientID)
}

func TestWithOptions(t *testing.T) {
	o := applyOptions(WithClientID(""))
	assert.Equal(t, "goframe-comfyui", o.clientID, "empty client ID should keep default")
}

func TestWithRequestTimeout(t *testing.T) {
	o := applyOptions(WithRequestTimeout(30 * time.Second))
	assert.Equal(t, 30*time.Second, o.requestTimeout)

	o = applyOptions(WithRequestTimeout(0))
	assert.Equal(t, 120*time.Second, o.requestTimeout)
}

func TestWithRetryAttempts(t *testing.T) {
	o := applyOptions(WithRetryAttempts(5))
	assert.Equal(t, 5, o.retry.Attempts)

	o = applyOptions(WithRetryAttempts(0))
	assert.Equal(t, 0, o.retry.Attempts)
}

func TestBaseURL(t *testing.T) {
	o := applyOptions(WithHost("localhost:8188"))
	assert.Equal(t, "http://localhost:8188", o.baseURL())
}

func TestWsURL(t *testing.T) {
	o := applyOptions(WithHost("localhost:8188"), WithClientID("test"))
	assert.Equal(t, "ws://localhost:8188/ws?clientId=test", o.wsURL())
}

func TestWorkflowBasics(t *testing.T) {
	w := NewWorkflow()
	assert.Equal(t, 0, w.NodeCount())

	node := w.AddNode("KSampler", 3)
	assert.Equal(t, 3, node.ID)
	assert.Equal(t, 1, w.NodeCount())
}

func TestWorkflowSetInput(t *testing.T) {
	w := NewWorkflow()
	node := w.AddNode("KSampler", 3)
	node.SetInput("prompt", "a cat").SetInput("width", 512)

	assert.Equal(t, "a cat", node.Inputs["prompt"])
	assert.Equal(t, 512, node.Inputs["width"])
}

func TestWorkflowConnect(t *testing.T) {
	w := NewWorkflow()
	loader := w.AddNode("CheckpointLoaderSimple", 4)
	loader.SetInput("class_type", "CheckpointLoaderSimple")
	loader.SetInput("ckpt_name", "model.safetensors")

	sampler := w.AddNode("KSampler", 3)
	sampler.SetInput("class_type", "KSampler")

	loader.Connect("MODEL", sampler.ID, "model")

	_, ok := sampler.Inputs["model"]
	assert.True(t, ok, "connection should exist")
}

func TestWorkflowMarshal(t *testing.T) {
	w := NewWorkflow()
	sampler := w.AddNode("KSampler", 3)
	sampler.SetInput("class_type", "KSampler")
	sampler.SetInput("prompt", "a cat in a garden")
	sampler.SetInput("width", 512)
	sampler.SetInput("height", 512)

	data, err := w.Marshal()
	require.NoError(t, err)
	assert.Contains(t, string(data), "KSampler")
	assert.Contains(t, string(data), "a cat in a garden")
}

func TestWorkflowGetNode(t *testing.T) {
	w := NewWorkflow()
	w.AddNode("KSampler", 3)

	node := w.GetNode(3)
	require.NotNil(t, node)
	assert.Equal(t, 3, node.ID)

	missing := w.GetNode(999)
	assert.Nil(t, missing)
}

func TestSentinelErrors(t *testing.T) {
	errors := []error{ErrNotConnected, ErrPromptFailed, ErrInvalidWorkflow, ErrImageNotFound, ErrQueueFull}
	for _, err := range errors {
		assert.Error(t, err)
	}
}

func TestNewClient(t *testing.T) {
	client, err := New(WithHost("localhost:8188"))
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Equal(t, "http://localhost:8188", client.baseURL)
	assert.Equal(t, "ws://localhost:8188/ws?clientId=goframe-comfyui", client.wsURL)

	err = client.Close()
	assert.NoError(t, err)
}

func TestNewClientWithHTTPClient(t *testing.T) {
	customClient := &http.Client{Timeout: 30 * time.Second}
	client, err := New(WithHTTPClient(customClient))
	require.NoError(t, err)
	assert.False(t, client.ownsClient, "custom HTTP client should not be owned")

	err = client.Close()
	assert.NoError(t, err)
}
