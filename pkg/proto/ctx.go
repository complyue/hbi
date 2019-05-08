package proto

import (
	"context"
	"sync"
	"time"
)

// as there's no way to check whether a chan is closed without blocking recv,
// and once cancelled, Context::Done() should never block, cancellation is hereby
// implemented by assigning cancellableContext::done to this closed chan as a marker
var closedChan = make(chan struct{})

func init() {
	// make it start out closed
	close(closedChan)
}

// CancellableContext defines an interface compatible with `context.Context`, and
// at the same time its cancelled status can be queried without blocking wait.
type CancellableContext interface {
	// be a context.Context
	context.Context

	// allow explicit cancel and query of cancelled state
	Cancel(err error)
	Cancelled() bool

	// be a RWLocker interface
	Lock()
	Unlock()
	RLock()
	RUnlock()
}

// NewCancellableContext creates a new context that is cancellable.
func NewCancellableContext() CancellableContext {
	return &cancellableContext{
		done: make(chan struct{}),
	}
}

type cancellableContext struct {
	// embed a RW lock
	sync.RWMutex

	// satisfying context.Context
	done chan struct{}
	err  error
}

func (ctx *cancellableContext) Cancelled() bool {
	ctx.RLock()
	defer ctx.RUnlock()
	return ctx.done == closedChan
}

func (ctx *cancellableContext) Cancel(err error) {
	ctx.Lock()
	defer ctx.Unlock()
	if err != nil { // preserve last non-nil error if multiple cancellations have been requested
		ctx.err = err
	}
	if done := ctx.done; done != closedChan {
		ctx.done = closedChan
		// signal current goroutines receiving from it
		close(done)
	}
}

func (ctx *cancellableContext) Done() <-chan struct{} {
	ctx.RLock()
	defer ctx.RUnlock()
	return ctx.done
}

func (ctx *cancellableContext) Err() error {
	ctx.RLock()
	defer ctx.RUnlock()
	return ctx.err
}

func (ctx *cancellableContext) Value(key interface{}) interface{} {
	return nil
}

func (ctx *cancellableContext) Deadline() (deadline time.Time, ok bool) {
	// todo implement deadline support ?
	return
}
