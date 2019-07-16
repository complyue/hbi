package mp

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/complyue/hbi/pkg/errors"
	"github.com/complyue/hbi/pkg/proto"
	"github.com/complyue/hbi/pkg/sock"
	"github.com/golang/glog"
)

type tcpUpstarter struct {
	addr      string
	heFactory func() *proto.HostingEnv
	cb        func(*net.TCPListener) error
}

// UpstartTCP listens on `addr` for incoming consumer connections over tcp,
// a dedicated worker subprocess will be started for each new connection,
// the consumer is served in the worker subprocess, with a hosting environment obtained
// from `heFactory`.
func UpstartTCP(addr string, heFactory func() *proto.HostingEnv, cb func(*net.TCPListener) error) error {
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
		if err = ups.cb(listener); err != nil {
			return
		}
	}

	for {
		var conn *net.TCPConn

		conn, err = listener.AcceptTCP()
		if err != nil {
			glog.Errorf("error accepting tcp connection: %+v", errors.RichError(err))
			return
		}

		consumerIdent := fmt.Sprintf("%s<=>%s", conn.LocalAddr(), conn.RemoteAddr())
		glog.V(1).Infof("New HBI upstart consumer connection accepted: %s", consumerIdent)

		upstartWorker(conn, consumerIdent)
	}
}

func (ups *tcpUpstarter) ServeFD(fd int) (context.Context, error) {
	he := ups.heFactory()

	_, ho, err := sock.TakeSocket(fd, he)
	if err != nil {
		return nil, err
	}

	glog.V(1).Infof("New HBI connection taken by upstart worker pid=%v: %s", os.Getpid(), ho.NetIdent())

	return ho.Context(), nil
}
