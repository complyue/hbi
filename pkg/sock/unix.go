package sock

import (
	"net"

	"github.com/complyue/hbi/pkg/errors"
	"github.com/complyue/hbi/pkg/proto"
	"github.com/golang/glog"
)

// ServeUnix listens on the specified file path (domain socket), serves each incoming connection with the
// hosting environment created from the `heFactory` function.
//
// `cb` will be called with the created `*net.UnixListener`.
//
// This func won't return until the listener is closed.
func ServeUnix(addr string, heFactory func() *proto.HostingEnv, cb func(*net.UnixListener)) (err error) {
	var raddr *net.UnixAddr
	raddr, err = net.ResolveUnixAddr("unix", addr)
	if nil != err {
		glog.Errorf("addr error: %+v", errors.RichError(err))
		return
	}
	var listener *net.UnixListener
	listener, err = net.ListenUnix("unix", raddr)
	if err != nil {
		glog.Errorf("listen error: %+v", errors.RichError(err))
		return
	}
	if cb != nil {
		cb(listener)
	}

	for {
		var conn *net.UnixConn
		conn, err = listener.AcceptUnix()
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

// DialUnix connects to specified file path (domain socket), react with specified hosting environment.
//
// The returned posting endpoint is used to create posting conversations to send code & data to remote
// site for active communication.
//
// The returned hosting endpoint is used to obtain the current hosting conversation triggered by a
// posting conversation from remote site for passive communication.
func DialUnix(addr string, he *proto.HostingEnv) (po *proto.PostingEnd, ho *proto.HostingEnd, err error) {
	raddr, err := net.ResolveUnixAddr("Unix", addr)
	if nil != err {
		glog.Errorf("addr error: %+v", errors.RichError(err))
		return
	}
	conn, err := net.DialUnix("Unix", nil, raddr)
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
