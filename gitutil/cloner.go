package gitutil

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/go-git/go-git/v5"
)

// Cloner handles the temporary cloning of remote Git repositories.
type Cloner struct {
	Logger *slog.Logger
}

// NewCloner creates a new GitCloner.
func NewCloner(logger *slog.Logger) *Cloner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Cloner{Logger: logger}
}

// Clone checks out a remote repository to a temporary local directory.
func (c *Cloner) Clone(ctx context.Context, repoURL string) (string, func(), error) {
	tempPath, err := os.MkdirTemp("", "goframe-repo-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	c.Logger.InfoContext(ctx, "Cloning repository", "url", repoURL, "path", tempPath)

	cleanupFunc := func() {
		c.Logger.Info("Cleaning up temporary repository", "path", tempPath)
		_ = os.RemoveAll(tempPath)
	}

	_, err = git.PlainCloneContext(ctx, tempPath, false, &git.CloneOptions{
		URL:      repoURL,
		Progress: nil,
		Depth:    1,
	})

	if err != nil {
		cleanupFunc()
		return "", nil, fmt.Errorf("failed to clone repo '%s': %w", repoURL, err)
	}

	c.Logger.InfoContext(ctx, "Repository cloned successfully")
	return tempPath, cleanupFunc, nil
}
