package ctx

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/complyue/goScript"
	"github.com/complyue/hbi/pkg/errors"
)

type HostingCtx map[string]interface{}

func NewHostingCtx() HostingCtx {
	return HostingCtx{
		// simulate `new` of Go language
		"new": func(rt reflect.Type) interface{} {
			return reflect.New(rt).Interface()
		},
	}
}

func (ctx HostingCtx) ExposeReactor(reactor interface{}) (err error) {

	pv := reflect.ValueOf(reactor)
	pt := pv.Type()
	if pt.Kind() != reflect.Ptr {
		panic(errors.Errorf("pointer implementation expected for reactor, found type: %T", reactor))
	}
	cv := pv.Elem()
	ct := cv.Type()

	// expose exported fields of the context struct
	for fi, nf := 0, ct.NumField(); fi < nf; fi++ {
		sf := ct.Field(fi)
		fv := cv.Field(fi)
		if sf.PkgPath != "" {
			continue // ignore unexported field
		}
		if sf.Anonymous { // expose methods of embeded field struct
			ctx.ExposeReactor(fv.Interface())
			continue
		}
		// expose field getter/setter func
		ctx[sf.Name] = func() interface{} {
			return fv.Interface()
		}
		ctx["Set"+sf.Name] = reflect.MakeFunc(
			reflect.FuncOf([]reflect.Type{sf.Type}, []reflect.Type{}, false),
			func(args []reflect.Value) (results []reflect.Value) {
				fv.Set(args[0])
				return
			},
		).Interface()
	}

	// collect exported methods of the context struct
	for mi, nm := 0, pv.NumMethod(); mi < nm; mi++ {
		mt := pt.Method(mi)
		if mt.PkgPath != "" {
			continue // ignore unexported method
		}
		mv := pv.Method(mi)
		ctx[mt.Name] = mv.Interface()
	}

	ctx["reactor"] = reactor
	return
}

func (ctx HostingCtx) ExposeTypes(typedPtrs []interface{}) (err error) {
	for _, v := range typedPtrs {
		vt := reflect.TypeOf(v)
		t := vt
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		ctx[t.Name()] = t
	}
	return
}

func (ctx HostingCtx) ExposeTypeAlias(typedPtrs map[string]interface{}) (err error) {
	for k, v := range typedPtrs {
		vt := reflect.TypeOf(v)
		t := vt
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		ctx[k] = t
	}
	return
}

// this needs not to be thread safe, should only be called from a single hosting goroutine
func (ctx HostingCtx) Exec(code string, sourceName string) (result interface{}, err error) {
	result, err = goScript.RunInContext(sourceName, code, (map[string]interface{})(ctx))
	return
}

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
