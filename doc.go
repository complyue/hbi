// Package hbi - Hosting Based Interface - Go1
//
// `HBI` is a `meta protocol` for application systems (read `service` software components),
// possibly implemented in different programming languages and/or base runtimes,
// to establish communication channels between os processes
// (may or may not across computing nodes), as to communicate with `peer-scripting-code` posted
// to eachother's `hosting environment`.
//
// By providing a `hosting environment` which exposes necessary artifacts (various
// `functions` in essense, see Mechanism) (https://github.com/complyue/hbi#mechanism)
// to accommodate the `landing` of the `peer-scripting-code` from the other end,
// a service process defines both its API and the effect network protocol to access the API,
// at granted efficience.
//
// Such network protocols are called `API defined protocol`s.
package hbi

import (
	"net"

	"github.com/complyue/hbi/pkg/proto"
	"github.com/complyue/hbi/pkg/sock"
)

// HostingEnv is the container of HBI artifacts, including:
//   * functions
//   * object constructors (special functions taking n args, returning 1 object)
//   * reactor methods
//   * value objects
// These artifacts need to be explicitly exposed to a hosting environment,
// to accomodate landing of `peer-scripting-code` from the other end.
type HostingEnv = proto.HostingEnv

// HostingEnd is the application programming interface of a hosting endpoint.
type HostingEnd = proto.HostingEnd

// PostingEnd is the application programming interface of a posting endpoint.
type PostingEnd = proto.PostingEnd

// NewHostingEnv creates a new, empty hosting environment.
func NewHostingEnv() *HostingEnv {
	return proto.NewHostingEnv()
}

// ServeTCP listens on the specified local address (host:port), serves each incoming connection with the
// hosting environment created from the `heFactory` function.
//
// `cb` will be called with the created `*net.TCPListener`, it's handful to specify port as 0,
// and receive the actual port from the cb.
//
// This func won't return until the listener is closed.
func ServeTCP(addr string, heFactory func() *HostingEnv, cb func(*net.TCPListener)) (err error) {
	return sock.ServeTCP(addr, heFactory, cb)
}

// DialTCP connects to specified remote address (host:port), react with specified hosting environment.
//
// The returned posting endpoint is used to create posting conversations to send code & data to remote
// site for active communication.
//
// The returned hosting endpoint is used to obtain the current hosting conversation triggered by a
// posting conversation from remote site for passive communication.
func DialTCP(addr string, he *HostingEnv) (po *PostingEnd, ho *HostingEnd, err error) {
	return sock.DialTCP(addr, he)
}

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
	return proto.Repr(val)
}
