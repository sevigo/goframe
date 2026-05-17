package comfyui

import (
	"encoding/json"
	"fmt"
	"sync"
)

type Workflow struct {
	nodes map[int]*Node
	mu    sync.RWMutex
}

func NewWorkflow() *Workflow {
	return &Workflow{
		nodes: make(map[int]*Node),
	}
}

type Node struct {
	ID       int                    `json:"class_type"`
	Inputs   map[string]any         `json:"inputs"`
	Meta     map[string]any         `json:"_meta,omitempty"`
	Outputs  map[string][]LinkInput `json:"-"` // internal routing, not serialized
	workflow *Workflow
}

type LinkInput struct {
	NodeID       int    `json:"node_id"`
	OutputSocket string `json:"output_socket"`
	InputSocket  string `json:"input_socket"`
}

func (w *Workflow) AddNode(classType string, id int) *Node {
	w.mu.Lock()
	defer w.mu.Unlock()

	node := &Node{
		ID:       id,
		Inputs:   make(map[string]any),
		Outputs:  make(map[string][]LinkInput),
		workflow: w,
	}
	node.Inputs["class_type"] = classType
	w.nodes[id] = node
	return node
}

func (n *Node) SetInput(key string, value any) *Node {
	n.Inputs[key] = value
	return n
}

func (n *Node) Connect(outputName string, targetID int, inputName string) *Node {
	n.workflow.mu.RLock()
	target, ok := n.workflow.nodes[targetID]
	n.workflow.mu.RUnlock()

	if !ok {
		return n
	}

	n.workflow.mu.Lock()
	target.Inputs[inputName] = []any{targetID, outputName}
	n.workflow.mu.Unlock()

	return n
}

func (w *Workflow) Marshal() ([]byte, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	prompt := map[string]any{}
	for id, node := range w.nodes {
		prompt[fmt.Sprintf("%d", id)] = map[string]any{
			"class_type": node.Inputs["class_type"],
			"inputs":     node.sanitizeInputs(),
		}
	}

	return json.Marshal(map[string]any{"prompt": prompt})
}

func (n *Node) sanitizeInputs() map[string]any {
	clean := make(map[string]any)
	for k, v := range n.Inputs {
		if k == "class_type" {
			continue
		}
		clean[k] = v
	}
	return clean
}

func (w *Workflow) GetNode(id int) *Node {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.nodes[id]
}

func (w *Workflow) NodeCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.nodes)
}
