// Package batch checks many documents with a bounded pool of workers and
// hands results back in input order.
package batch

import (
	"context"
	"sync"

	"github.com/Santaniel0/normokontrol/internal/check"
)

// Checker checks one file. The command line wraps check.File with report
// writing; tests use fakes.
type Checker interface {
	CheckFile(ctx context.Context, path string) (check.Result, error)
}

// Outcome is the result for one input path.
type Outcome struct {
	Path   string
	Result check.Result
	Err    error
}

type indexed struct {
	i int
	o Outcome
}

// Run checks paths with at most workers goroutines and calls emit in input
// order as soon as a prefix of results is complete, so a long batch prints
// progress without reordering lines. emit is called from the caller's
// goroutine only. After ctx is cancelled, paths that were not started are
// emitted with ctx.Err().
func Run(ctx context.Context, paths []string, workers int, c Checker, emit func(Outcome)) {
	workers = max(1, min(workers, len(paths)))
	jobs := make(chan int)
	results := make(chan indexed)

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				// select may hand out a job even after cancellation; parsing a
				// large file just to throw the result away is wasted work.
				if err := ctx.Err(); err != nil {
					results <- indexed{i, Outcome{Path: paths[i], Err: err}}
					continue
				}
				res, err := c.CheckFile(ctx, paths[i])
				results <- indexed{i, Outcome{Path: paths[i], Result: res, Err: err}}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i := range paths {
			select {
			case jobs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	pending := map[int]Outcome{}
	next := 0
	for r := range results {
		pending[r.i] = r.o
		for {
			o, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			emit(o)
			next++
		}
	}
	for ; next < len(paths); next++ {
		if o, ok := pending[next]; ok {
			emit(o)
			continue
		}
		emit(Outcome{Path: paths[next], Err: ctx.Err()})
	}
}
