package documentloaders

import (
	"context"
	"log/slog"

	"github.com/sevigo/goframe/gitutil"
	"github.com/sevigo/goframe/parsers"
	"github.com/sevigo/goframe/schema"
)

type RemoteGitRepoLoader struct {
	RepoURL        string
	ParserRegistry parsers.ParserRegistry
	Logger         *slog.Logger
}

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

func (l *RemoteGitRepoLoader) Load(ctx context.Context) ([]schema.Document, error) {
	cloner := gitutil.NewCloner(l.Logger)
	tempPath, cleanup, err := cloner.Clone(ctx, l.RepoURL)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	localGitLoader, err := NewGit(tempPath, l.ParserRegistry)
	if err != nil {
		return nil, err
	}

	documents, err := localGitLoader.Load(ctx)
	if err != nil {
		return nil, err
	}

	for i := range documents {
		documents[i].Metadata["original_source_url"] = l.RepoURL
	}

	return documents, nil
}
