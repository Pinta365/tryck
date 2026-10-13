package workpool

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestRunVisitsEachIndexOnce(t *testing.T) {
	for _, workers := range []int{0, 1, 3, 64} {
		seen := make([]atomic.Int32, 100)
		if err := Run(context.Background(), len(seen), workers, func(i int) { seen[i].Add(1) }); err != nil {
			t.Fatal(err)
		}
		for i := range seen {
			if seen[i].Load() != 1 {
				t.Fatalf("workers=%d: index %d visited %d times", workers, i, seen[i].Load())
			}
		}
	}
}

func TestRunEmpty(t *testing.T) {
	if err := Run(context.Background(), 0, 4, func(int) { t.Error("called") }); err != nil {
		t.Fatal(err)
	}
}

func TestRunConcurrencyLimit(t *testing.T) {
	var cur, peak atomic.Int32
	block := make(chan struct{})
	done := make(chan error)
	go func() {
		done <- Run(context.Background(), 20, 3, func(int) {
			n := cur.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			<-block
			cur.Add(-1)
		})
	}()
	for range 20 {
		block <- struct{}{}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if peak.Load() > 3 {
		t.Errorf("peak concurrency %d > 3", peak.Load())
	}
}

func TestRunCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	err := Run(ctx, 1000, 2, func(i int) {
		if calls.Add(1) == 5 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
	if calls.Load() > 10 {
		t.Errorf("%d items ran after cancel", calls.Load())
	}
}
