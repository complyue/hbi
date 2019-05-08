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

func NewHostingEnd(he *he.HostingEnv, po PostingEnd, wire details.HBIWire) HostingEnd {
	ho := &hostingEnd{
		CancellableContext: NewCancellableContext(),

		he: he,

		wire:      wire,
		netIdent:  wire.NetIdent(),
		localAddr: wire.LocalAddr(),

		po: po.(*postingEnd),
	}

	// run landing loop in a dedicated goroutine
	go func() {
		var (
			pkt              *details.Packet
			err              error
			trySendPeerError = true
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
				ho.Disconnect(errReason, trySendPeerError)
			} else {
				// normal disconnection
				if glog.V(1) {
					glog.Infof("HBI %s landing loop stopped.", ho.netIdent)
				}
			}
		}()

		for !ho.Cancelled() {

			pkt, err = wire.RecvPacket()
			if err != nil {
				return
			}
			var result interface{}

			switch pkt.WireDir {
			case "":

				if _, err = ho.Exec(pkt.Payload); err != nil {
					return
				}

			case "co_send":

				if result, err = ho.Exec(pkt.Payload); err != nil {
					return
				}
				if _, err = wire.SendPacket(fmt.Sprintf("%#v", result), "co_recv"); err != nil {
					trySendPeerError = false
					return
				}

			case "co_recv":

				panic("co_recv invaded landing loop")

			case "co_begin":

				co := &HoCo{
					ho: ho, coID: pkt.Payload,
					ended: make(chan struct{}),
				}
				ho.po.coEnqueue(co)
				ho.co = co

				if _, err = ho.po.wire.SendPacket(pkt.Payload, "co_ack_begin"); err != nil {
					trySendPeerError = false
					return
				}

			case "co_end":

				if ho.co == nil || ho.co.coID != pkt.Payload {
					panic("ho co mismatch")
				}
				ho.po.coAssertSender(ho.co)

				ho.po.coEnd(ho.co, true)
				ho.co = nil

			case "co_ack_begin":

				recvCo := ho.po.coPeek()
				if recvCo.CoID() != pkt.Payload {
					panic("mismatch co_ack_begin")
				}
				close(recvCo.(*PoCo).respBegin)

			case "co_ack_end":

				recvCo := ho.po.coDequeue()
				if recvCo.CoID() != pkt.Payload {
					panic("mismatch co_ack_end")
				}

			case "err":

				ho.Disconnect(fmt.Sprintf("peer error: %s", pkt.Payload), false)
				return

			default:

				panic("Unexpected packet")

			}
		}
	}()

	return ho
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
