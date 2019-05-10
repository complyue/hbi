package proto

import (
	"fmt"
	"net"

	details "github.com/complyue/hbi/pkg/_details"
	"github.com/complyue/hbi/pkg/errors"
	"github.com/complyue/hbi/pkg/he"
	"github.com/golang/glog"
)

type HostingEnd interface {
	// be a cancellable context
	CancellableContext

	Env() *he.HostingEnv

	NetIdent() string
	LocalAddr() net.Addr

	Po() PostingEnd

	// Co returns current hosting conversation, or nil if no one open.
	Co() *HoCo

	Disconnect(errReason string, trySendPeerError bool)

	Close()
}

type hostingEnd struct {
	// embed a cancellable context
	CancellableContext

	he *he.HostingEnv

	wire details.HBIWire

	netIdent  string
	localAddr net.Addr

	po *postingEnd

	co *HoCo
}

func (ho *hostingEnd) Env() *he.HostingEnv {
	return ho.he
}

func (ho *hostingEnd) Exec(code string) (result interface{}, err error) {
	result, err = ho.he.RunInEnv(code, ho)
	return
}

func (ho *hostingEnd) NetIdent() string {
	return ho.netIdent
}

func (ho *hostingEnd) LocalAddr() net.Addr {
	return ho.localAddr
}

func (ho *hostingEnd) Po() PostingEnd {
	return ho.po
}

func (ho *hostingEnd) Co() *HoCo {
	return ho.co
}

func (ho *hostingEnd) recvObj() (obj interface{}, err error) {
	var (
		pkt *details.Packet
	)

	defer func() {
		if e := recover(); e != nil {
			err = errors.RichError(e)
		} else if err != nil {
			err = errors.RichError(err)
		}

		if err != nil {
			// error occurred, log & disconnect
			errReason := fmt.Sprintf("HBI landing error:\n%+v\nHBI Packet: %+v", err, pkt)
			glog.Error(errReason)
			ho.Disconnect(errReason, true)
		}
	}()

	for {

		if ho.Cancelled() {
			err = errors.New("hosting endpoint closed")
			return
		}

		pkt, err = ho.wire.RecvPacket()
		if err != nil {
			return
		}

		switch pkt.WireDir {
		case "":

			if _, err = ho.Exec(pkt.Payload); err != nil {
				return
			}

		case "co_send":

			panic("co_send invaded recv loop")

		case "co_recv":

			obj, err = ho.Exec(pkt.Payload)
			return

		default:

			panic("Unexpected packet")

		}
	}
}

func (ho *hostingEnd) Cancel(err error) {
	if ho.CancellableContext.Cancelled() {
		// already cancelled
		return
	}

	var errReason string
	if err != nil {
		errReason = fmt.Sprintf("%+v", errors.RichError(err))
	}

	ho.Disconnect(errReason, true)
}

func (ho *hostingEnd) Disconnect(errReason string, trySendPeerError bool) {
	defer ho.wire.Disconnect()

	ho.CancellableContext.Cancel(errors.New(errReason))

	ho.po.Disconnect(errReason, trySendPeerError)
}

func (ho *hostingEnd) Close() {
	ho.Disconnect("", false)
}
