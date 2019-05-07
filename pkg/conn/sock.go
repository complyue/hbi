package conn

import (
	"net"

	details "github.com/complyue/hbi/pkg/_details"

	"github.com/complyue/hbi/pkg/ctx"
	"github.com/complyue/hbi/pkg/errors"
	"github.com/complyue/hbi/pkg/proto"
	"github.com/golang/glog"
)

// ServeTCP listens on the specified local address (host:port), serves each incoming connection with the
// context created from the `ctxFact` function.
//
// `cb` will be called with the created `*net.TCPListener`, it's handful to specify port as 0,
// and receive the actual port from the cb.
//
// This func won't return until the listener is closed.
func ServeTCP(ctxFact func() ctx.HostingCtx, addr string, cb func(*net.TCPListener)) (err error) {
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
		cb(listener)
	}

	for {
		var conn *net.TCPConn
		conn, err = listener.AcceptTCP()
		if nil != err {
			glog.Errorf("accept error: %+v", errors.RichError(err))
			return
		}

		// todo DoS react
		context := ctxFact()

		wire := details.NewTCPWire(conn)
		netIdent := wire.NetIdent()
		glog.V(1).Infof("New HBI connection accepted: %s", netIdent)

		po := proto.NewPostingEnd(wire)
		ho := proto.NewHostingEnd(context, po, wire)

		if initMagic, ok := context["__hbi_init__"].(proto.InitMagicFunction); ok {
			initMagic(po, ho)
		} else {
			context["po"], context["ho"] = po, ho
		}
	}
}

// DialTCP connects to specified remote address (host:port), react with specified context.
//
// The returned `po` is used to send code & data to remote peer for hosted landing.
func DialTCP(context ctx.HostingCtx, addr string) (po proto.PostingEnd, ho proto.HostingEnd, err error) {
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

	wire := details.NewTCPWire(conn)
	netIdent := wire.NetIdent()
	glog.V(1).Infof("New HBI connection established: %s", netIdent)

	po = proto.NewPostingEnd(wire)
	ho = proto.NewHostingEnd(context, po, wire)

	if initMagic, ok := context["__hbi_init__"].(proto.InitMagicFunction); ok {
		initMagic(po, ho)
	} else {
		context["po"], context["ho"] = po, ho
	}

	return
}
