package sock

import (
	"net"

	"github.com/complyue/hbi/pkg/errors"
	"github.com/complyue/hbi/pkg/proto"
	"github.com/golang/glog"
)

// ServeTCP listens on the specified local address (host:port), serves each incoming connection with
// a hosting environment produced by the `heFactory` function.
//
// `cb` will be called with the created `*net.TCPListener`, it's handful to specify port as 0,
// and receive the actual port from the cb.
//
// This func won't return until the listener is closed.
func ServeTCP(addr string, heFactory func() *proto.HostingEnv, cb func(*net.TCPListener) error) (err error) {
	var raddr *net.TCPAddr
	raddr, err = net.ResolveTCPAddr("tcp", addr)
	if nil != err {
		glog.Errorf("addr error: %+v", errors.RichError(err))
		return
	}
	var listener *net.TCPListener
	listener, err = net.ListenTCP("tcp", raddr)
	if err != nil {
		glog.Errorf("listen error: %+v", errors.RichError(err))
		return
	}
	if cb != nil {
		if err = cb(listener); err != nil {
			return
		}
	}

	for {
		var conn *net.TCPConn
		conn, err = listener.AcceptTCP()
		if nil != err {
			glog.Errorf("accept error: %+v", errors.RichError(err))
			return
		}

		// todo DoS react
		he := heFactory()

		wire := NewSocketWire(conn)
		netIdent := wire.NetIdent()
		glog.V(1).Infof("New HBI connection accepted: %s", netIdent)

		proto.NewConnection(wire, he)
	}
}

// DialTCP connects to specified remote address (host:port), react with specified hosting environment.
//
// The returned posting endpoint is used to create posting conversations to send code & data to remote
// site for active communication.
//
// The returned hosting endpoint is used to obtain the current hosting conversation triggered by a
// posting conversation from remote site for passive communication.
func DialTCP(addr string, he *proto.HostingEnv) (po *proto.PostingEnd, ho *proto.HostingEnd, err error) {
	raddr, err := net.ResolveTCPAddr("tcp", addr)
	if nil != err {
		glog.Errorf("addr error: %+v", errors.RichError(err))
		return
	}
	conn, err := net.DialTCP("tcp", nil, raddr)
	if nil != err {
		glog.Errorf("conn error: %+v", errors.RichError(err))
		return
	}

	wire := NewSocketWire(conn)
	netIdent := wire.NetIdent()
	glog.V(1).Infof("New HBI connection established: %s", netIdent)

	po, ho, err = proto.NewConnection(wire, he)

	return
}
