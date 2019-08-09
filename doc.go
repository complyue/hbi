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

// NewHostingEnv creates a new, empty hosting environment.
func NewHostingEnv() *HostingEnv {
	return proto.NewHostingEnv()
}

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

// HoCo is the passive, hosting conversation.
//
// A HoCo is triggered by a PoCo from peer's posting endpoint, it is automatically available to
// application, obtained by calling HostingEnd.Co()
type HoCo = proto.HoCo

// PostingEnd is the application programming interface of a posting endpoint.
type PostingEnd = proto.PostingEnd

// PoCo is the active, posting conversation.
//
// A PoCo is created from application by calling PostingEnd.NewCo()
type PoCo = proto.PoCo

// TakeSocket takes a pre-connected socket (Unix or TCP), react with specified hosting environment.
//
// The returned posting endpoint is used to create posting conversations to send code & data to remote
// site for active communication.
//
// The returned hosting endpoint is used to obtain the current hosting conversation triggered by a
// posting conversation from remote site for passive communication.
func TakeSocket(fd int, he *proto.HostingEnv) (
	po *proto.PostingEnd, ho *proto.HostingEnd, err error) {
	return sock.TakeSocket(fd, he)
}

// TakeConn takes a pre-connected socket (Unix or TCP), react with specified hosting environment.
//
// The returned posting endpoint is used to create posting conversations to send code & data to remote
// site for active communication.
//
// The returned hosting endpoint is used to obtain the current hosting conversation triggered by a
// posting conversation from remote site for passive communication.
func TakeConn(conn net.Conn, he *proto.HostingEnv) (
	po *proto.PostingEnd, ho *proto.HostingEnd, err error) {
	return sock.TakeConn(conn, he)
}

// ServeTCP listens on the specified local address (host:port), serves each incoming connection with the
// hosting environment created from the `heFactory` function.
//
// `cb` will be called with the created `*net.TCPListener`, it's handful to specify port as 0,
// and receive the actual port from the cb.
//
// This func won't return until the listener is closed.
func ServeTCP(addr string, heFactory func() *HostingEnv, cb func(*net.TCPListener) error) (err error) {
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

// ServeUnix listens on the specified file path (domain socket), serves each incoming connection with the
// hosting environment created from the `heFactory` function.
//
// `cb` will be called with the created `*net.UnixListener`.
//
// This func won't return until the listener is closed.
func ServeUnix(addr string, heFactory func() *HostingEnv, cb func(*net.UnixListener)) (err error) {
	return sock.ServeUnix(addr, heFactory, cb)
}

// DialUnix connects to specified file path (domain socket), react with specified hosting environment.
//
// The returned posting endpoint is used to create posting conversations to send code & data to remote
// site for active communication.
//
// The returned hosting endpoint is used to obtain the current hosting conversation triggered by a
// posting conversation from remote site for passive communication.
func DialUnix(addr string, he *HostingEnv) (po *PostingEnd, ho *HostingEnd, err error) {
	return sock.DialUnix(addr, he)
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

type (
	// LitIntType is the type of a literal int value from peer script
	//
	// this is decided by the underlying HBI interpreter for Go1, which is Anko
	LitIntType = int64

	// LitDecimalType is the type of a literal decimal value from peer script
	//
	// this is decided by the underlying HBI interpreter for Go1, which is Anko
	LitDecimalType = float64

	// LitListType is the type of a literal list (as called by Python, or array
	// as called by JavaScript) value from peer script
	//
	// this is decided by the underlying HBI interpreter for Go1, which is Anko
	LitListType = []interface{}

	// LitDictType is the type of a literal dict (as called by Python, or object
	// as called by JavaScript) value from peer script
	//
	// this is decided by the underlying HBI interpreter for Go1, which is Anko
	LitDictType = map[interface{}]interface{}
)
