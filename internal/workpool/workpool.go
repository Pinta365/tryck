// Package workpool runs indexed work on a bounded number of goroutines.
package workpool

import (
	"context"
	"sync"
	"sync/atomic"
)

// Run calls fn(i) for i in [0, n) on up to workers goroutines and waits
// for all of them. Once ctx is done no new items start, and Run returns
// ctx.Err(). fn must be safe for concurrent use; results are usually
// written to index i of a preallocated slice.
func Run(ctx context.Context, n, workers int, fn func(i int)) error {
	workers = max(1, min(workers, n))
	var next atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				i := int(next.Add(1) - 1)
				if i >= n {
					return
				}
				fn(i)
			}
		}()
	}
	wg.Wait()
	return ctx.Err()
}
