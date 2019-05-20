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

// Conver is the application programming interface of a conversation, common to PoCo and HoCo.
//
// There're 2 types of conversation:
//  * the active, posting conversation
//    which is created by application by calling PostingEnd.Po()
//  * the passive, hosting conversation
//    which is automatically available to application, and obtained by calling HostingEnv.Co()
type Conver = proto.Conver

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
