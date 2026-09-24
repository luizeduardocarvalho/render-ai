package jobs

import (
	"context"
	"fmt"
	"sync"
)

// Inline is the Queue used with the "inline" jobs.queue config (the memory
// store, local dev, and most tests): Enqueue runs the handler in its own
// goroutine against a background context instead of round-tripping through
// Cloud Tasks and a second HTTP request. A background context is
// deliberate - Enqueue itself is normally called from inside the POST
// .../render handler, and the goroutine it starts must keep running after
// that request (and its context) ends.
//
// The handler is set separately via SetHandler rather than passed to
// NewInline, because building it (a bound method on the API server) and
// building the queue (a constructor argument the server itself takes) are
// otherwise circular - see cmd/server/main.go.
type Inline struct {
	mu      sync.Mutex
	handler Handler
	wg      sync.WaitGroup
}

// NewInline creates an Inline queue with no handler set yet; call
// SetHandler before the first Enqueue.
func NewInline() *Inline {
	return &Inline{}
}

// SetHandler installs the function that processes each enqueued task. Safe
// to call concurrently with Enqueue (guarded by the same mutex), though in
// practice it's set once at startup before any request can reach Enqueue.
func (q *Inline) SetHandler(h Handler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handler = h
}

// Enqueue starts the handler for task in a new goroutine and returns
// immediately; it never fails once a handler is set.
func (q *Inline) Enqueue(_ context.Context, task Task) error {
	q.mu.Lock()
	h := q.handler
	q.mu.Unlock()
	if h == nil {
		return fmt.Errorf("jobs.Inline: Enqueue called before SetHandler")
	}
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		h(context.Background(), task)
	}()
	return nil
}

// Wait blocks until every task enqueued so far has finished running. Tests
// use this to observe a job reach its terminal state deterministically
// instead of polling.
func (q *Inline) Wait() {
	q.wg.Wait()
}
