package hbi

import (
	"fmt"
	"strings"
)

// Repr converts a value object to its textual representation, that can be used to
// reconstruct the object by Anko (https://github.com/mattn/anko), or an HBI HostingEnv
// implemented in other programming languages / runtimes.
//
// The syntax is very much JSON like, with `[]interface{}` maps to JSON array,
// and `map[interface{}]interface{}` maps to JSON object.
// But note it's not JSON compatible when a non-string key is present.
//
// Despite those few special types, the representation of a value is majorly obtained via
// `Sprintf("%#v", v)`, which can be customized by overriding `Format(fmt.State, rune)` method
// of its type, like the example shows. For desired result:
//
//   fmt.Printf("(%6s) %s\n", "Repr", hbi.Repr(msg))
//   fmt.Printf("(%6s) %#v\n", "Repr", msg)
//   fmt.Printf("(%6s) %+v\n", "Long", msg)
//   fmt.Printf("(%6s) %v\n", "Short", msg)
//   fmt.Printf("(%6s) %s\n", "String", msg)
//
//   // Output:
//   // (  Repr) Msg("Compl","Hello, HBI world!",1557998919)
//   // (  Repr) Msg("Compl","Hello, HBI world!",1557998919)
//   // (  Long) [May 16 17:28:39+08] @Compl: Hello, HBI world!
//   // ( Short) @Compl: Hello, HBI world!
//   // (String) Msg<@Compl
//
// Implement the `Format(fmt.State, rune)` method like this:
//
// 	 func (msg *Msg) Format(s fmt.State, verb rune) {
// 	 	switch verb {
// 	 	case 's': // string form
// 	 		io.WriteString(s, "Msg<@")
// 	 		io.WriteString(s, msg.From)
// 	 	case 'v':
// 	 		if s.Flag('#') { // repr form
// 	 			io.WriteString(s, "Msg(")
// 	 			io.WriteString(s, fmt.Sprintf("%#v", msg.From))
// 	 			io.WriteString(s, ",")
// 	 			io.WriteString(s, fmt.Sprintf("%#v", msg.Content))
// 	 			io.WriteString(s, ",")
// 	 			io.WriteString(s, fmt.Sprintf("%d", msg.Time.Unix()))
// 	 			io.WriteString(s, ")")
// 	 		} else { // value form
// 	 			if s.Flag('+') {
// 	 				io.WriteString(s, "[")
// 	 				io.WriteString(s, msg.Time.Format("Jan 02 15:04:05Z07"))
// 	 				io.WriteString(s, "] ")
// 	 			}
// 	 			io.WriteString(s, "@")
// 	 			io.WriteString(s, msg.From)
// 	 			io.WriteString(s, ": ")
// 	 			io.WriteString(s, msg.Content)
// 	 		}
// 	 	}
// 	 }
//
// See:
// https://docs.python.org/3/library/functions.html#repr
// and
// https://docs.python.org/3/reference/datamodel.html#object.__repr__
// for a similar construct in Python.
//
// Expand the `Example` section below to see full source.
//
func Repr(val interface{}) string {
	var r strings.Builder
	if err := repr(val, &r); err != nil {
		panic(err)
	}
	return r.String()
}

func repr(val interface{}, r *strings.Builder) error {
	if val == nil {
		// Anko understands literal `nil` and handles typed/untyped nils properly.
		//
		// Any other language/runtime that intends to interop with Golang peers should map nil
		// to a value of its native env, e.g. `None` for Python, `null` for JavaScript.
		r.WriteString("nil")
	}

	switch v := val.(type) {
	case []interface{}:
		var first = true
		r.WriteString("[")
		for _, e := range v {
			if first {
				first = false
			} else {
				r.WriteString(", ")
			}
			r.WriteString(Repr(e))
		}
		r.WriteString("]")
	case map[interface{}]interface{}:
		var first = true
		r.WriteString("{")
		for k, vv := range v {
			if first {
				first = false
			} else {
				r.WriteString(", ")
			}
			if err := repr(k, r); err != nil {
				return err
			}
			r.WriteString(": ")
			if err := repr(vv, r); err != nil {
				return err
			}
		}
		r.WriteString("}")
	default:
		fmt.Fprintf(r, "%#v", val)
	}

	return nil
}
