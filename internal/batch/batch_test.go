package batch

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Santaniel0/normokontrol/internal/check"
)

// fake sleeps longer for earlier files so they finish out of order.
type fake struct {
	running, peak atomic.Int32
	n             int
	block         chan struct{}
}

func (f *fake) CheckFile(ctx context.Context, path string) (check.Result, error) {
	cur := f.running.Add(1)
	defer f.running.Add(-1)
	for {
		p := f.peak.Load()
		if cur <= p || f.peak.CompareAndSwap(p, cur) {
			break
		}
	}
	i, _ := strconv.Atoi(path)
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return check.Result{}, ctx.Err()
		}
	}
	time.Sleep(time.Duration(f.n-i) * time.Millisecond)
	if i%5 == 0 {
		return check.Result{}, fmt.Errorf("file %d: broken", i)
	}
	return check.Result{File: path}, nil
}

func paths(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = strconv.Itoa(i)
	}
	return out
}

func TestOrderAndBound(t *testing.T) {
	const n, workers = 30, 4
	f := &fake{n: n}
	var got []Outcome
	Run(context.Background(), paths(n), workers, f, func(o Outcome) { got = append(got, o) })
	if len(got) != n {
		t.Fatalf("got %d outcomes", len(got))
	}
	for i, o := range got {
		if o.Path != strconv.Itoa(i) {
			t.Fatalf("outcome %d is for %s", i, o.Path)
		}
		if (i%5 == 0) != (o.Err != nil) {
			t.Errorf("outcome %d: err = %v", i, o.Err)
		}
	}
	if p := f.peak.Load(); p > workers || p < 2 {
		t.Errorf("peak concurrency %d, want 2..%d", p, workers)
	}
}

func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fake{n: 10, block: make(chan struct{})}
	var got []Outcome
	done := make(chan struct{})
	go func() {
		Run(ctx, paths(10), 2, f, func(o Outcome) { got = append(got, o) })
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
	if len(got) != 10 {
		t.Fatalf("got %d outcomes, want every path reported", len(got))
	}
	for i, o := range got {
		if o.Path != strconv.Itoa(i) || !errors.Is(o.Err, context.Canceled) {
			t.Errorf("outcome %d = %+v", i, o)
		}
	}
}

func TestEmpty(t *testing.T) {
	called := false
	Run(context.Background(), nil, 4, &fake{}, func(Outcome) { called = true })
	if called {
		t.Error("emit called for an empty batch")
	}
}
