package chains_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/chains"
)

func TestMapReduceChain_Call(t *testing.T) {
	ctx := context.Background()

	// Helper map/reduce functions for int->string->string pipeline
	toUpper := func(_ context.Context, s string) (string, error) {
		return strings.ToUpper(s), nil
	}
	join := func(_ context.Context, results []string) (string, error) {
		return strings.Join(results, ", "), nil
	}

	t.Run("all tasks succeed", func(t *testing.T) {
		chain := chains.NewMapReduceChain[string, string, string](toUpper, join)

		result, err := chain.Call(ctx, []string{"hello", "world"})
		require.NoError(t, err)
		assert.Contains(t, result, "HELLO")
		assert.Contains(t, result, "WORLD")
	})

	t.Run("empty input", func(t *testing.T) {
		called := false
		reduce := func(_ context.Context, results []string) (string, error) {
			called = true
			return "empty", nil
		}
		chain := chains.NewMapReduceChain[string, string, string](toUpper, reduce)

		result, err := chain.Call(ctx, []string{})
		require.NoError(t, err)
		assert.Equal(t, "empty", result)
		assert.True(t, called, "reduce should be called even with empty input")
	})

	t.Run("quorum met with partial failures", func(t *testing.T) {
		callIdx := atomic.Int32{}
		mapFn := func(_ context.Context, _ string) (string, error) {
			idx := callIdx.Add(1)
			if idx == 2 {
				return "", errors.New("intentional failure")
			}
			return fmt.Sprintf("result-%d", idx), nil
		}

		chain := chains.NewMapReduceChain[string, string, string](
			mapFn, join,
			chains.WithQuorum[string, string, string](0.66),
		)

		result, err := chain.Call(ctx, []string{"a", "b", "c"})
		require.NoError(t, err)
		// At least 2 results (quorum = ceil(0.66 * 3) = 2+1 = 3? No, 0.66*3 = 1.98 + 1 = 2)
		assert.NotEmpty(t, result)
	})

	t.Run("quorum not met", func(t *testing.T) {
		failingMap := func(_ context.Context, _ string) (string, error) {
			return "", errors.New("fail")
		}

		chain := chains.NewMapReduceChain[string, string, string](
			failingMap, join,
			chains.WithQuorum[string, string, string](0.66),
		)

		_, err := chain.Call(ctx, []string{"a", "b", "c"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quorum not")
	})

	t.Run("concurrency limit is respected", func(t *testing.T) {
		var peak atomic.Int32
		var current atomic.Int32

		mapFn := func(_ context.Context, s string) (string, error) {
			val := current.Add(1)
			// Track peak concurrency
			for {
				p := peak.Load()
				if val <= p || peak.CompareAndSwap(p, val) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
			current.Add(-1)
			return strings.ToUpper(s), nil
		}

		chain := chains.NewMapReduceChain[string, string, string](
			mapFn, join,
			chains.WithMaxConcurrency[string, string, string](2),
		)

		_, err := chain.Call(ctx, []string{"a", "b", "c", "d"})
		require.NoError(t, err)
		assert.LessOrEqual(t, int(peak.Load()), 2, "peak concurrency should not exceed limit")
	})

	t.Run("context cancellation stops execution", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(context.Background())

		slowMap := func(ctx context.Context, _ string) (string, error) {
			select {
			case <-time.After(5 * time.Second):
				return "done", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}

		chain := chains.NewMapReduceChain[string, string, string](slowMap, join)

		cancel() // Cancel immediately
		_, err := chain.Call(cancelCtx, []string{"a", "b"})
		require.Error(t, err)
	})

	t.Run("per-task timeout", func(t *testing.T) {
		slowMap := func(ctx context.Context, _ string) (string, error) {
			select {
			case <-time.After(5 * time.Second):
				return "done", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}

		chain := chains.NewMapReduceChain[string, string, string](
			slowMap, join,
			chains.WithMapTimeout[string, string, string](50*time.Millisecond),
		)

		_, err := chain.Call(ctx, []string{"a"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quorum not")
	})
}
