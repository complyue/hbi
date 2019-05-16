package proto

import (
	"context"
	"sync"
	"time"
)

// CancellableContext defines an interface compatible with `context.Context`, and
// its cancelled status can be queried without blocking wait.
//
// This comes at a cost of an extra `sync.RWMutex` to synchronize the querying and setting of the state.
type CancellableContext interface {
	// be a context.Context
	context.Context

	// allow explicit cancel and query of cancelled state
	Cancel(err error)
	Cancelled() bool
}

// NewCancellableContext creates a new context that is cancellable.
func NewCancellableContext() CancellableContext {
	return &cancellableContext{
		done: make(chan struct{}),
	}
}

type cancellableContext struct {
	// embed to guard access to done/err
	sync.RWMutex

	// satisfying context.Context
	done chan struct{}
	err  error
}

// Done returns a closed channel if this context has already been cancelled,
// or a channel stay open atm but will be closed when this context is cancelled.
func (ctx *cancellableContext) Done() <-chan struct{} {
	// no locking is strictly needed here, even the caller reads the pre-cancelled `done`
	// channel after actually cancelled (due to dirty thread cache), that channel will
	// appear closed when selected, maintaining the correct behavior.
	return ctx.done
}

// Value is not implemented so far and always returns `nil` regardless of the `key`.
func (ctx *cancellableContext) Value(key interface{}) interface{} {
	return nil
}

// Deadline is not implemented so far and always returns zero values,
// i.e. `deadline==nil` and `ok==false`.
func (ctx *cancellableContext) Deadline() (deadline time.Time, ok bool) {
	// todo implement deadline support ?
	return
}

// Err returns the first error used to cancel this context.
func (ctx *cancellableContext) Err() error {
	ctx.RLock()
	defer ctx.RUnlock()
	return ctx.err
}

// as there's no way to check whether a chan is closed without blocking recv,
// and once cancelled, Context::Done() should never block, cancellation is hereby
// implemented by assigning cancellableContext::done to this closed chan as a marker
var closedChan = make(chan struct{})

func init() {
	// make it start out closed
	close(closedChan)
}

// Cancelled tells immediately whether this context has been cancelled.
func (ctx *cancellableContext) Cancelled() bool {
	ctx.RLock()
	defer ctx.RUnlock()
	return ctx.done == closedChan
}

// Cancel cancels this context.
func (ctx *cancellableContext) Cancel(err error) {
	ctx.Lock()
	defer ctx.Unlock()
	done := ctx.done
	if done != closedChan {
		ctx.done = closedChan
		// this signals current goroutines receiving from it
		close(done)
	}
	// admit just the first non-nil error if multiple cancellations have been requested
	if ctx.err == nil {
		ctx.err = err
	}
}
