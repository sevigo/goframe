package documentloaders

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/sevigo/goframe/schema"
)

// CLICommandLoader runs a command and uses its stdout as the document content.
type CLICommandLoader struct {
	Command string
	Args    []string
}

func NewCLICommandLoader(command string, args ...string) *CLICommandLoader {
	return &CLICommandLoader{Command: command, Args: args}
}

func (l *CLICommandLoader) Load(ctx context.Context) ([]schema.Document, error) {
	command := filepath.Base(l.Command)
	cmd := exec.CommandContext(ctx, command, l.Args...)
	output, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("command '%s' failed: %w\nstderr: %s", l.Command, err, string(ee.Stderr))
		}
		return nil, err
	}
	sourceName := fmt.Sprintf("output of command '%s'", l.Command)
	doc := schema.NewDocument(string(output), map[string]any{"source": sourceName})
	return []schema.Document{doc}, nil
}
