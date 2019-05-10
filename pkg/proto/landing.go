package proto

import (
	"fmt"

	details "github.com/complyue/hbi/pkg/_details"
	"github.com/complyue/hbi/pkg/errors"
	"github.com/complyue/hbi/pkg/he"
	"github.com/golang/glog"
)

// NewConnection creates the posting & hosting endpoints from a transport wire with a hosting environment
func NewConnection(he *he.HostingEnv, wire details.HBIWire) (PostingEnd, HostingEnd) {

	// the posting endpoint always gets created, as hosting-only connections can rarely be useful.
	// even if a connection will never be used to post sth out, having a posting endpoint is no harm.
	po := &postingEnd{
		CancellableContext: NewCancellableContext(),

		wire:       wire,
		netIdent:   wire.NetIdent(),
		remoteAddr: wire.RemoteAddr(),

		nextCoSeq: minCoSeq,
	}

	if he == nil {
		// creating a posting-only connection.
		// in some cases, e.g. to replay HBI traffic recorded on disk,
		// hosting is not possible, thus a posting only connection is desirable.
		return po, nil
	}

	// most common case
	ho := &hostingEnd{
		CancellableContext: NewCancellableContext(),

		he: he,

		wire:      wire,
		netIdent:  wire.NetIdent(),
		localAddr: wire.LocalAddr(),
	}

	// will this create too much difficulty for GC ?
	po.ho, ho.po = ho, po

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

				ho.co = (*HoCo)(ho.po.coEnqueue(pkt.Payload))

				if _, err = ho.po.wire.SendPacket(pkt.Payload, "co_ack_begin"); err != nil {
					trySendPeerError = false
					return
				}

			case "co_end":

				if ho.co == nil || ho.co.coSeq != pkt.Payload {
					panic("ho co mismatch")
				}
				ho.po.coAssertSender((*coState)(ho.co))

				ho.po.coEnd((*coState)(ho.co), true)

				if _, err = ho.po.wire.SendPacket(pkt.Payload, "co_ack_end"); err != nil {
					trySendPeerError = false
					return
				}

				ho.co = nil

			case "co_ack_begin":

				recvCo := ho.po.coPeek()
				if recvCo.coSeq != pkt.Payload {
					panic("mismatch co_ack_begin")
				}

				close(recvCo.respBegan)

			case "co_ack_end":

				recvCo := ho.po.coDequeue()
				if recvCo.coSeq != pkt.Payload {
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

	return po, ho
}
