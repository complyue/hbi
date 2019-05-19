package proto

import (
	"context"
	"reflect"

	"github.com/complyue/anko/vm"
	"github.com/complyue/hbi/pkg/errors"
)

// HostingEnv is the container of hbi artifacts, including:
//   * functions
//   * object constructors (special functions taking n args, returning 1 object)
//   * value objects
//   * reactor methods
// These artifacts need to be explicitly exposed to a hosting environment,
// to accomodate landing of peer scripting code.
type HostingEnv struct {
	ve       *vm.Env
	po       *PostingEnd
	ho       *HostingEnd
	exposure []string
}

func NewHostingEnv() *HostingEnv {
	he := &HostingEnv{
		ve:       vm.NewEnv(),
		exposure: make([]string, 0, 50),
	}
	return he
}

func (he *HostingEnv) AnkoEnv() *vm.Env {
	return he.ve
}

func (he *HostingEnv) Po() *PostingEnd {
	return he.po
}

func (he *HostingEnv) Ho() *HostingEnd {
	return he.ho
}

func (he *HostingEnv) ExposedNames() []string {
	return append([]string(nil), he.exposure...)
}

// this needs not to be thread safe, should only be called from a single hosting goroutine
func (he *HostingEnv) RunInEnv(code string, ctx context.Context) (result interface{}, err error) {
	result, err = he.ve.ExecuteContext(ctx, code)
	return
}

func (he *HostingEnv) NameExposed(name string) bool {
	// list of exposed names should not be quite long, linear search is okay
	for _, expName := range he.exposure {
		if expName == name {
			return true
		}
	}
	return false
}

func (he *HostingEnv) validateExposeName(name string) {
	if he.NameExposed(name) {
		panic(errors.Errorf("exposure name %s already used", name))
	}
}

func (he *HostingEnv) ExposeFunction(name string, fun interface{}) {
	if reflect.TypeOf(fun).Kind() != reflect.Func {
		panic(errors.Errorf("not a function: %T = %#v", fun, fun))
	}
	he.validateExposeName(name)
	if err := he.ve.Define(name, fun); err != nil {
		panic(errors.RichError(err))
	}
	he.exposure = append(he.exposure, name)
}

// ExposeCtor exposes the specified constructor function,
// with expose name as the function's return type name,
// or `typeAlias` if it's not empty.
//
// note a constructor function should return one and only one value.
func (he *HostingEnv) ExposeCtor(ctorFunc interface{}, typeAlias string) {
	ft := reflect.TypeOf(ctorFunc)
	if ft.Kind() != reflect.Func {
		panic(errors.Errorf("not a function: %T = %#v", ctorFunc, ctorFunc))
	}
	if ft.NumOut() != 1 {
		panic(errors.Errorf("ctor function should return just 1 value, not %v", ft.NumOut()))
	}
	frt := ft.Out(0)
	if frt.Kind() == reflect.Ptr {
		frt = frt.Elem()
	}
	if typeAlias == "" {
		typeAlias = frt.Name()
	}
	he.validateExposeName(typeAlias)
	if err := he.ve.Define(typeAlias, ctorFunc); err != nil {
		panic(errors.RichError(err))
	}
	he.exposure = append(he.exposure, typeAlias)
}

func (he *HostingEnv) ExposeValue(name string, fun interface{}) {
	he.validateExposeName(name)
	if err := he.ve.Define(name, fun); err != nil {
		panic(errors.RichError(err))
	}
	he.exposure = append(he.exposure, name)
}

func (he *HostingEnv) Get(name string) interface{} {
	if val, err := he.ve.Get(name); err != nil {
		// undefined symbol, return nil instead of panic here
		return nil
	} else {
		return val
	}
}

func (he *HostingEnv) ExposeReactor(reactor interface{}) {
	var expNameWhiteList map[string]struct{}
	if reactorObj, ok := reactor.(Reactor); ok {
		wlNames := reactorObj.NamesToExpose()
		expNameWhiteList = make(map[string]struct{}, len(wlNames))
		for i := range wlNames {
			expNameWhiteList[wlNames[i]] = struct{}{}
		}
	}

	var expName string

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
			he.ExposeReactor(fv.Addr().Interface())
			continue
		}
		if expNameWhiteList != nil {
			if _, ok := expNameWhiteList[sf.Name]; !ok {
				// not in declared exposure list
				continue
			}
		}
		// expose field getter/setter func
		expName = "Get" + sf.Name
		if err := he.ve.Define(expName, func() interface{} {
			return fv.Interface()
		}); err != nil {
			panic(errors.RichError(err))
		}
		if !he.NameExposed(expName) {
			he.exposure = append(he.exposure, expName)
		}
		expName = "Set" + sf.Name
		if err := he.ve.Define(expName, reflect.MakeFunc(
			reflect.FuncOf([]reflect.Type{sf.Type}, []reflect.Type{}, false),
			func(args []reflect.Value) (results []reflect.Value) {
				fv.Set(args[0])
				return
			},
		).Interface()); err != nil {
			panic(errors.RichError(err))
		}
		if !he.NameExposed(expName) {
			he.exposure = append(he.exposure, expName)
		}
	}

	// collect exported methods of the context struct
	for mi, nm := 0, pv.NumMethod(); mi < nm; mi++ {
		mt := pt.Method(mi)
		if mt.PkgPath != "" {
			continue // ignore unexported method
		}
		mv := pv.Method(mi)
		if expNameWhiteList != nil {
			if _, ok := expNameWhiteList[mt.Name]; !ok {
				// not in declared exposure list
				continue
			}
		}
		expName = mt.Name
		if err := he.ve.Define(expName, mv.Interface()); err != nil {
			panic(errors.RichError(err))
		}
		if !he.NameExposed(expName) {
			he.exposure = append(he.exposure, expName)
		}
	}

}

// Reactor is the interface optionally implemented by a type whose instances are
// to be exposed to a `HostingEnv` by calling `he.ExposeReactor()`.
//
// If a reactor object does not implement this interface, all its exported
// fields and methods will be exposed.
type Reactor interface {
	NamesToExpose() []string
}
