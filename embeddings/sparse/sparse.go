package sparse

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

const (
	// modelURL is the remote location of the BGE small sparse model.
	modelURL = "https://github.com/qdrant/fast-embed/raw/main/fast_embed/models/fast-bge-small-en-v1.5.tar.gz"
	// expectedSHA256 is the verified checksum of the model archive.
	expectedSHA256 = "498720d335ad93e06cb3a4602859971001d11b47202512d53cc141f6ae7f0767"
)

// EnsureModelDownloaded downloads the model artifacts directly.
// We only need the tokenizer files for sparse vector generation.
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

	if err := downloadAndExtract(modelURL, cacheDir); err != nil {
		return "", fmt.Errorf("failed to download and extract model: %w", err)
	}

	return modelPath, nil
}

func downloadAndExtract(url, destination string) error {
	client := &http.Client{
		Timeout: 5 * time.Minute,
	}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read model body: %w", err)
	}

	if err := verifyChecksum(body, expectedSHA256); err != nil {
		return err
	}

	gzr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar: %w", err)
		}

		target := filepath.Join(destination, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("failed to create dir: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent dir: %w", err)
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to open file: %w", err)
			}
			_, copyErr := io.Copy(f, tr)
			f.Close()
			if copyErr != nil {
				return fmt.Errorf("failed to extract file: %w", copyErr)
			}
		}
	}
	return nil
}

func verifyChecksum(data []byte, expected string) error {
	h := sha256.New()
	h.Write(data)
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
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

// GenerateSparseVector converts text into a normalized SparseVector using Bag-of-Tokens.
// The resulting vector is L2-normalized to unit length for consistent similarity scoring.
// Special tokens (PAD, CLS, SEP) are filtered out to reduce noise and index size.
//
// Returns error if:
//   - Text cannot be tokenized
//   - No valid tokens remain after filtering
//   - Normalization fails (due to zero norm)
func GenerateSparseVector(ctx context.Context, text string) (*schema.SparseVector, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("text cannot be empty")
	}

	tk, err := GetTokenizer()
	if err != nil {
		return nil, fmt.Errorf("failed to get tokenizer: %w", err)
	}

	// Truncation is handled by default tokenizer settings if configured in tokenizer.json.
	encodings, err := tk.EncodeSingle(text)
	if err != nil {
		return nil, fmt.Errorf("failed to encode text: %w", err)
	}

	tokenCounts := make(map[uint32]float32)
	for _, id := range encodings.Ids {
		// Skip special tokens: 0 (PAD), 101 (CLS), 102 (SEP) are noise for BOW sparse vectors.
		if id == 0 || id == 101 || id == 102 {
			continue
		}
		tokenCounts[uint32(id)] += 1.0
	}

	if len(tokenCounts) == 0 {
		return nil, errors.New("no valid tokens generated after filtering special tokens")
	}

	var normSq float64
	for _, count := range tokenCounts {
		normSq += float64(count) * float64(count)
	}
	norm := math.Sqrt(normSq)

	if norm <= 0 {
		return nil, fmt.Errorf("invalid normalization: norm=%f for %d tokens", norm, len(tokenCounts))
	}

	indices := make([]uint32, 0, len(tokenCounts))
	values := make([]float32, 0, len(tokenCounts))
	for id, count := range tokenCounts {
		indices = append(indices, id)
		values = append(values, float32(float64(count)/norm))
	}

	return &schema.SparseVector{
		Indices: indices,
		Values:  values,
	}, nil
}
