package mp

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/complyue/hbi/pkg/sock"

	"github.com/complyue/hbi/pkg/errors"
	"github.com/complyue/hbi/pkg/proto"
	"github.com/golang/glog"
)

type tcpUpstarter struct {
	addr      string
	heFactory func() *proto.HostingEnv
	cb        func(*net.TCPListener)
}

// UpstartTCP listens on `addr` for incoming consumer connections over tcp,
// a dedicated worker subprocess will be started for each new connection,
// the consumer is served in the worker subprocess, with a hosting environment obtained
// from `heFactory`.
func UpstartTCP(addr string, heFactory func() *proto.HostingEnv, cb func(*net.TCPListener)) error {
	return Upstart(&tcpUpstarter{
		addr:      addr,
		heFactory: heFactory,
		cb:        cb,
	})
}

func (ups *tcpUpstarter) Listen() (err error) {
	var raddr *net.TCPAddr
	raddr, err = net.ResolveTCPAddr("tcp", ups.addr)
	if nil != err {
		glog.Errorf("error resolving tcp address [%s]: %+v", ups.addr, errors.RichError(err))
		return
	}
	var listener *net.TCPListener
	listener, err = net.ListenTCP("tcp", raddr)
	if err != nil {
		glog.Errorf("error listening on address [%s]: %+v", raddr, errors.RichError(err))
		return
	}
	if ups.cb != nil {
		ups.cb(listener)
	}

	for {
		var conn *net.TCPConn
		var f *os.File

		conn, err = listener.AcceptTCP()
		if err != nil {
			glog.Errorf("error accepting tcp connection: %+v", errors.RichError(err))
			return
		}

		consumerIdent := fmt.Sprintf("%s<=>%s", conn.LocalAddr(), conn.RemoteAddr())
		glog.V(1).Infof("New HBI upstart consumer connection accepted: %s", consumerIdent)

		f, err = conn.File()
		if err != nil {
			glog.Errorf("socket to file error: %+v", errors.RichError(err))
			return
		}

		upstartWorker(f, consumerIdent)
	}
}

func (ups *tcpUpstarter) ServeFD(fd int) (context.Context, error) {
	if !(fd > 2) {
		panic(fmt.Sprintf("bad fd %v for upstart", fd))
	}

	f := os.NewFile(uintptr(fd), "upstart-socket")
	if f == nil {
		panic(fmt.Sprintf("invalid fd %v for upstart", fd))
	}

	conn, err := net.FileConn(f)
	if err != nil {
		return nil, err
	}

	wire := sock.NewSocketWire(conn)
	netIdent := wire.NetIdent()
	glog.V(1).Infof("New HBI connection taken by upstart worker pid=%v: %s", os.Getpid(), netIdent)

	he := ups.heFactory()

	_, ho, err := proto.NewConnection(wire, he)
	if err != nil {
		return nil, err
	}

	return ho.Context(), nil
}
