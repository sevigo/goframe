package chains

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MapReduceChain runs a map function concurrently over inputs, then reduces
// the collected results into a final output. Supports concurrency limits,
// per-task timeouts, and quorum-based early return.
type MapReduceChain[In, Mid, Out any] struct {
	MapFunc        func(ctx context.Context, input In) (Mid, error)
	ReduceFunc     func(ctx context.Context, results []Mid) (Out, error)
	MaxConcurrency int
	Timeout        time.Duration // per-task timeout (0 = no timeout)
	QuorumFraction float64       // e.g. 0.66 = return when 2/3 succeed
}

type MapReduceOption[In, Mid, Out any] func(*MapReduceChain[In, Mid, Out])

// WithMaxConcurrency limits the number of map tasks running in parallel.
func WithMaxConcurrency[In, Mid, Out any](n int) MapReduceOption[In, Mid, Out] {
	return func(c *MapReduceChain[In, Mid, Out]) {
		c.MaxConcurrency = n
	}
}

// WithMapTimeout sets a timeout for each individual map task.
func WithMapTimeout[In, Mid, Out any](d time.Duration) MapReduceOption[In, Mid, Out] {
	return func(c *MapReduceChain[In, Mid, Out]) {
		c.Timeout = d
	}
}

// WithQuorum sets the fraction of map tasks that must succeed before
// the chain proceeds to the reduce step. For example, 0.66 means
// at least 2 out of 3 tasks must succeed.
func WithQuorum[In, Mid, Out any](fraction float64) MapReduceOption[In, Mid, Out] {
	return func(c *MapReduceChain[In, Mid, Out]) {
		c.QuorumFraction = fraction
	}
}

func NewMapReduceChain[In, Mid, Out any](
	mapFn func(ctx context.Context, input In) (Mid, error),
	reduceFn func(ctx context.Context, results []Mid) (Out, error),
	opts ...MapReduceOption[In, Mid, Out],
) *MapReduceChain[In, Mid, Out] {
	c := &MapReduceChain[In, Mid, Out]{
		MapFunc:    mapFn,
		ReduceFunc: reduceFn,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Call runs the map function over all inputs concurrently (with bounded
// concurrency), waits for quorum, then calls ReduceFunc with the results.
func (c *MapReduceChain[In, Mid, Out]) Call(ctx context.Context, inputs []In) (Out, error) {
	var zero Out
	if len(inputs) == 0 {
		return c.ReduceFunc(ctx, nil)
	}

	total := len(inputs)
	quorumNeeded := total
	if c.QuorumFraction > 0 && c.QuorumFraction < 1.0 {
		quorumNeeded = int(float64(total)*c.QuorumFraction) + 1
		if quorumNeeded > total {
			quorumNeeded = total
		}
	}

	type mapResult struct {
		value Mid
		err   error
	}

	resultCh := make(chan mapResult, total)

	// Semaphore for concurrency limiting
	maxWorkers := total
	if c.MaxConcurrency > 0 && c.MaxConcurrency < total {
		maxWorkers = c.MaxConcurrency
	}
	sem := make(chan struct{}, maxWorkers)

	var wg sync.WaitGroup
	for _, input := range inputs {
		wg.Add(1)
		go func(in In) {
			defer wg.Done()

			// Acquire semaphore slot
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				resultCh <- mapResult{err: ctx.Err()}
				return
			}

			// Apply per-task timeout if configured
			taskCtx := ctx
			if c.Timeout > 0 {
				var cancel context.CancelFunc
				taskCtx, cancel = context.WithTimeout(ctx, c.Timeout)
				defer cancel()
			}

			val, err := c.MapFunc(taskCtx, in)
			resultCh <- mapResult{value: val, err: err}
		}(input)
	}

	// Close resultCh after all goroutines finish
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results, stop early on quorum
	var collected []Mid
	var failures int

	for res := range resultCh {
		if res.err != nil {
			failures++
			// Check if quorum is no longer achievable
			remaining := total - len(collected) - failures
			if len(collected)+remaining < quorumNeeded {
				return zero, fmt.Errorf("quorum not achievable: %d/%d succeeded, need %d", len(collected), total, quorumNeeded)
			}
			continue
		}

		collected = append(collected, res.value)
		if len(collected) >= quorumNeeded {
			break
		}
	}

	if len(collected) < quorumNeeded {
		return zero, fmt.Errorf("quorum not met: %d/%d succeeded, need %d", len(collected), total, quorumNeeded)
	}

	return c.ReduceFunc(ctx, collected)
}
