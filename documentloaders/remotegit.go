package documentloaders

import (
	"context"
	"log/slog"

	"github.com/sevigo/goframe/gitutil"
	"github.com/sevigo/goframe/parsers"
	"github.com/sevigo/goframe/schema"
)

// RemoteGitRepoLoader clones a remote repository and then uses a GitLoader
// to load all parsable files within it into documents.
type RemoteGitRepoLoader struct {
	RepoURL        string
	ParserRegistry parsers.ParserRegistry
	Logger         *slog.Logger
}

// NewRemoteGitRepoLoader creates a loader for remote Git repositories.
func NewRemoteGitRepoLoader(repoURL string, registry parsers.ParserRegistry, logger *slog.Logger) *RemoteGitRepoLoader {
	if logger == nil {
		logger = slog.Default()
	}
	return &RemoteGitRepoLoader{
		RepoURL:        repoURL,
		ParserRegistry: registry,
		Logger:         logger,
	}
}

// Load implements the Loader interface. It clones the repo, loads the documents,
// and cleans up the temporary directory.
func (l *RemoteGitRepoLoader) Load(ctx context.Context) ([]schema.Document, error) {
	cloner := gitutil.NewCloner(l.Logger)
	tempPath, cleanup, err := cloner.Clone(ctx, l.RepoURL)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	localGitLoader := NewGit(tempPath, l.ParserRegistry)

	documents, err := localGitLoader.Load(ctx)
	if err != nil {
		return nil, err
	}

	for i := range documents {
		documents[i].Metadata["original_source_url"] = l.RepoURL
	}

	return documents, nil
}
