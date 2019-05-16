package hbi

import (
	"fmt"
)

// Repr converts a value object to its string representation,
// as much programming-language neutral as possible.
//
// This is majorly implemented with `fmt.Sprintf("%#v", val)`, which produces Golang native
// representation in string forms.
//
// Such representation can be customized by overriding a type's `Format(fmt.State, rune)` method,
// like the example shows that desired result:
//
//   fmt.Printf("(%6s) %#v\n", "Repr", msg)
//   fmt.Printf("(%6s) %+v\n", "Long", msg)
//   fmt.Printf("(%6s) %v\n", "Short", msg)
//   fmt.Printf("(%6s) %s\n", "String", msg)
//
//   // Output:
//   // (  Repr) Msg("Compl","Hello, HBI world!",1557998919)
//   // (  Long) [May 16 17:28:39+08] @Compl: Hello, HBI world!
//   // ( Short) @Compl: Hello, HBI world!
//   // (String) Msg<@Compl
//
// Can be customized from implementation:
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
// https://docs.python.org/3/library/string.html#format-string-syntax
// for a similar construct in Python.
//
// Expand the `Example` section below to see full source.
//
func Repr(val interface{}) string {

	if val == nil {
		// Anko understands literal `nil` and handles typed/untyped nils properly.
		//
		// Any other language/runtime that intends to interop with Golang peers should map nil
		// to a value of its native env, e.g. `None` for Python, `null` for JavaScript.
		return "nil"
	}
	// TODO handle more quirks

	return fmt.Sprintf("%#v", val)
}
