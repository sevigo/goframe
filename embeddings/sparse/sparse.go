package sparse

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

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
	cacheDir := os.Getenv("GOFRAME_CACHE_DIR")
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home dir: %w", err)
		}
		cacheDir = filepath.Join(home, ".cache", "goframe", "models")
	}
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
	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	resp, err := client.Get(url)
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

			// Copy content to file
			_, copyErr := io.Copy(f, tr)
			f.Close() // Explicit close
			if copyErr != nil {
				return copyErr
			}
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
// The resulting vector is L2 normalized.
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
		// 101/102 are usually CLS/SEP in BERT models, which are noise for sparse vectors.
		if id == 0 || id == 101 || id == 102 {
			continue
		}
		tokenCounts[uint32(id)] += 1.0
	}

	if len(tokenCounts) == 0 {
		return nil, fmt.Errorf("no tokens generated")
	}

	// Calculate L2 norm for normalization
	var norm float64
	for _, count := range tokenCounts {
		norm += float64(count * count)
	}
	norm = math.Sqrt(norm)

	// Convert to SparseVector struct with normalized values
	indices := make([]uint32, 0, len(tokenCounts))
	values := make([]float32, 0, len(tokenCounts))

	for id, count := range tokenCounts {
		indices = append(indices, id)
		if norm > 0 {
			values = append(values, float32(float64(count)/norm))
		} else {
			values = append(values, count)
		}
	}

	return &schema.SparseVector{
		Indices: indices,
		Values:  values,
	}, nil
}
