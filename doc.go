// Package hbi - Hosting Based Interface - Go1
package hbi

import (
	"net"

	"github.com/complyue/hbi/pkg/proto"
	"github.com/complyue/hbi/pkg/sock"
)

// HostingEnv is the container of hbi artifacts, including:
//   * functions
//   * object constructors (special functions taking n args, returning 1 object)
//   * value objects
//   * reactor methods
// These artifacts need to be explicitly exposed to a hosting environment,
// to accomodate landing of peer scripting code.
type HostingEnv = proto.HostingEnv

// HostingEnd is the application programming interface of an HBI hosting endpoint.
type HostingEnd = proto.HostingEnd

// PostingEnd is the application programming interface of an HBI posting endpoint.
type PostingEnd = proto.PostingEnd

// Conver is the conversation interface.
//  there're basically 2 types of conversations:
//   * the active, posting conversation
//   * the passive, hosting conversation
type Conver = proto.Conver

// NewHostingEnv creates a new hosting environment
func NewHostingEnv() *proto.HostingEnv {
	return proto.NewHostingEnv()
}

// ServeTCP listens on the specified local address (host:port), serves each incoming connection with the
// hosting env created from the `heFactory` function.
//
// `cb` will be called with the created `*net.TCPListener`, it's handful to specify port as 0,
// and receive the actual port from the cb.
//
// This func won't return until the listener is closed.
func ServeTCP(addr string, heFactory func() *proto.HostingEnv, cb func(*net.TCPListener)) (err error) {
	return sock.ServeTCP(addr, heFactory, cb)
}

// DialTCP connects to specified remote address (host:port), react with specified hosting env.
//
// The returned `po` is used to send code & data to remote peer for hosted landing.
func DialTCP(addr string, he *proto.HostingEnv) (po *proto.PostingEnd, ho *proto.HostingEnd, err error) {
	return sock.DialTCP(addr, he)
}
