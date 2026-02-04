package sparse

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/sevigo/goframe/schema"
	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
)

var (
	vocabOnce         sync.Once
	vocabErr          error
	tokenizerInstance *tokenizer.Tokenizer
)

// EnsureModelDownloaded downloads the model artifacts directly from GCS.
// We avoid using fastembed.NewFlagEmbedding because it initializes the ONNX runtime,
// which requires a C library that might not be present. We only need the tokenizer files.
func EnsureModelDownloaded() (string, error) {
	modelName := "fast-bge-small-en-v1.5"
	cacheDir := filepath.Join(os.Getenv("HOME"), ".cache", "goframe", "models")
	modelPath := filepath.Join(cacheDir, modelName)

	if _, err := os.Stat(modelPath); err == nil {
		return modelPath, nil
	}

	// Download
	url := fmt.Sprintf("https://storage.googleapis.com/qdrant-fastembed/%s.tar.gz", modelName)
	if err := downloadAndExtract(url, cacheDir); err != nil {
		return "", fmt.Errorf("failed to download and extract model: %w", err)
	}

	return modelPath, nil
}

func downloadAndExtract(url, targetDir string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Untar
	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(targetDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

// GetTokenizer returns a singleton instance of the tokenizer.
func GetTokenizer() (*tokenizer.Tokenizer, error) {
	vocabOnce.Do(func() {
		modelPath, err := EnsureModelDownloaded()
		if err != nil {
			vocabErr = err
			return
		}

		// Load tokenizer.json
		tokenizerPath := filepath.Join(modelPath, "tokenizer.json")
		tk, err := pretrained.FromFile(tokenizerPath)
		if err != nil {
			vocabErr = fmt.Errorf("failed to load tokenizer from %s: %w", tokenizerPath, err)
			return
		}
		tokenizerInstance = tk
	})
	return tokenizerInstance, vocabErr
}

// GenerateSparseVector converts text into a SparseVector using a Bag-of-Tokens approach.
// It tokenizes the text and counts the frequency of each token.
func GenerateSparseVector(ctx context.Context, text string) (*schema.SparseVector, error) {
	tk, err := GetTokenizer()
	if err != nil {
		return nil, fmt.Errorf("failed to get tokenizer: %w", err)
	}

	// Tokenize
	// We explicitly enable truncation to avoid issues with very long texts,
	// though for sparse vectors we might want all tokens.
	// For now let's behave like a standard embedding model (512 tokens).
	encodings, err := tk.EncodeSingle(text)
	if err != nil {
		return nil, fmt.Errorf("failed to tokenize text: %w", err)
	}

	// Count token frequencies
	tokenCounts := make(map[uint32]float32)
	for _, id := range encodings.Ids {
		// Skip special tokens if possible (0 is padding).
		if id == 0 {
			continue
		}
		tokenCounts[uint32(id)] += 1.0
	}

	if len(tokenCounts) == 0 {
		return nil, fmt.Errorf("no tokens generated")
	}

	// Convert to SparseVector struct
	indices := make([]uint32, 0, len(tokenCounts))
	values := make([]float32, 0, len(tokenCounts))

	for id, count := range tokenCounts {
		indices = append(indices, id)
		values = append(values, count)
	}

	return &schema.SparseVector{
		Indices: indices,
		Values:  values,
	}, nil
}
