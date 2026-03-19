// Package sparse provides utilities for generating sparse vectors for hybrid search.
//
// Sparse vectors enable exact term matching combined with semantic similarity,
// improving retrieval accuracy for queries that require precise term matching.
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

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"

	"github.com/sevigo/goframe/httpclient"
	"github.com/sevigo/goframe/schema"
)

// Provider defines the interface for generating sparse vectors.
type Provider interface {
	GenerateSparseVector(ctx context.Context, text string) (*schema.SparseVector, error)
}

var (
	providerMu sync.RWMutex
	provider   Provider
)

// RegisterProvider registers a sparse vector provider, replacing the default.
func RegisterProvider(p Provider) {
	providerMu.Lock()
	defer providerMu.Unlock()
	provider = p
}

// GenerateSparseVector builds a normalized sparse vector from text using the registered provider.
// If no provider is registered, it uses the default BoWProvider for backward compatibility.
func GenerateSparseVector(ctx context.Context, text string) (*schema.SparseVector, error) {
	p := getProvider()
	return p.GenerateSparseVector(ctx, text)
}

// getProvider returns the registered provider, or creates a default BoWProvider if none is set.
func getProvider() Provider {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()

	if p != nil {
		return p
	}

	// Lazy initialization of default provider
	providerMu.Lock()
	defer providerMu.Unlock()

	// Double-check after acquiring write lock
	if provider == nil {
		provider = NewBoWProvider()
	}
	return provider
}

// BoWProvider implements the Provider interface using a Bag-of-Words approach
// with a pretrained tokenizer.
type BoWProvider struct {
	mu                sync.RWMutex
	tokenizerInstance *tokenizer.Tokenizer
}

// NewBoWProvider creates a new Bag-of-Words sparse provider.
func NewBoWProvider() *BoWProvider {
	return &BoWProvider{}
}

// GenerateSparseVector builds a normalized BOW sparse vector from text.
// Special tokens (PAD, CLS, SEP) are filtered to reduce noise.
func (p *BoWProvider) GenerateSparseVector(ctx context.Context, text string) (*schema.SparseVector, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("text cannot be empty")
	}

	tk, err := p.getTokenizer(ctx)
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

func (p *BoWProvider) getTokenizer(ctx context.Context) (*tokenizer.Tokenizer, error) {
	p.mu.RLock()
	if p.tokenizerInstance != nil {
		defer p.mu.RUnlock()
		return p.tokenizerInstance, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.tokenizerInstance != nil {
		return p.tokenizerInstance, nil
	}

	modelPath, err := EnsureModelDownloaded(ctx)
	if err != nil {
		return nil, err
	}

	tokenizerPath := filepath.Join(modelPath, "tokenizer.json")
	tk, err := pretrained.FromFile(tokenizerPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load tokenizer from %s: %w", tokenizerPath, err)
	}
	p.tokenizerInstance = tk
	return p.tokenizerInstance, nil
}

const (
	// modelURL is the remote location of the BGE small sparse model.
	modelURL = "https://storage.googleapis.com/qdrant-fastembed/fast-bge-small-en-v1.5.tar.gz"
	// expectedSHA256 is the verified checksum of the model archive.
	// Verified against official Qdrant fastembed releases.
	expectedSHA256 = "3858004b3822f64f940280874b8f2d2dc25b34a4f3eb3cdf617bdceeb21ed9ed"
)

// EnsureModelDownloaded pulls the model files into the local cache if missing.
// We only need the tokenizers for sparse vector generation.
func EnsureModelDownloaded(ctx context.Context) (string, error) {
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

	if _, err := os.Stat(modelPath); err == nil { //nolint:gosec // path constructed from trusted model names
		return modelPath, nil
	}

	if err := downloadAndExtract(ctx, modelURL, cacheDir); err != nil {
		return "", fmt.Errorf("failed to download and extract model: %w", err)
	}

	return modelPath, nil
}

func downloadAndExtract(ctx context.Context, url, destination string) error {
	// Use shared download client optimized for large file transfers
	client := httpclient.DownloadClient()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
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

	if bodyErr := verifyChecksum(body, expectedSHA256); bodyErr != nil {
		return bodyErr
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

		// Guard against Zip-Slip
		//nolint:gosec // Protection implemented below
		target := filepath.Join(destination, header.Name)
		if !strings.HasPrefix(target, filepath.Clean(destination)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path in tar: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0750); err != nil { //nolint:gosec // target is within user's cache directory
				return fmt.Errorf("failed to create dir: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil { //nolint:gosec // target is within user's cache directory
				return fmt.Errorf("failed to create parent dir: %w", err)
			}
			// Use restricted permissions (0600) for cached model files
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, 0600) //nolint:gosec // target is within user's cache directory
			if err != nil {
				return fmt.Errorf("failed to open file: %w", err)
			}
			limit := int64(100 * 1024 * 1024)
			if _, copyErr := io.CopyN(f, tr, limit); copyErr == nil {
				var buf [1]byte
				if n, err := tr.Read(buf[:]); err != io.EOF && n > 0 {
					_ = f.Close()
					return fmt.Errorf("file %s exceeds decompression limit of %d bytes", header.Name, limit)
				}
			} else if !errors.Is(copyErr, io.EOF) {
				_ = f.Close()
				return fmt.Errorf("failed to extract file %s: %w", header.Name, copyErr)
			}
			if closeErr := f.Close(); closeErr != nil {
				return fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}
	return nil
}

func verifyChecksum(data []byte, expected string) error {
	if expected == "" {
		return nil
	}
	h := sha256.New()
	h.Write(data)
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}
