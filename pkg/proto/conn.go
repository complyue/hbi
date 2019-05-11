package proto

import (
	"fmt"
	"io"

	"github.com/complyue/hbi/pkg/errors"
	"github.com/complyue/hbi/pkg/he"
	"github.com/golang/glog"
)

// NewConnection creates the posting & hosting endpoints from a transport wire with a hosting environment
func NewConnection(he *he.HostingEnv, wire HBIWire) (PostingEnd, HostingEnd) {
	netIdent := wire.NetIdent()

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

	var (
		ok          bool
		initFunc    InitMagicFunction
		cleanupFunc CleanupMagicFunction
	)

	if initMagic := he.Get("__hbi_init__"); initMagic != nil {
		if initFunc, ok = initMagic.(InitMagicFunction); !ok {
			panic(errors.Errorf("Bad __hbi_init__() type: %T", initMagic))
		}
	}

	if cleanupMagic := he.Get("__hbi_cleanup__"); cleanupMagic != nil {
		if cleanupFunc, ok = cleanupMagic.(CleanupMagicFunction); !ok {
			panic(errors.Errorf("Bad __hbi_cleanup__() type: %T", cleanupMagic))
		}
	}

	if initFunc != nil {
		func() {
			defer func() {
				if e := recover(); e != nil {
					err := errors.RichError(e)
					errReason := fmt.Sprintf("init callback failed: %+v", err)

					ho.Disconnect(errReason, true)

					if cleanupFunc != nil {
						func() {
							defer func() {
								if e := recover(); e != nil {
									glog.Warningf("HBI %s cleanup callback failure ignored: %+v", netIdent, err)
								}
							}()

							cleanupFunc(err)
						}()
					}
				}
			}()

			initFunc(po, ho)
		}()
	}

	// run landing loop in a dedicated goroutine
	go func() {
		var (
			pkt              *Packet
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
				ho.Close()
			}

			if cleanupFunc != nil {
				func() {
					defer func() {
						if e := recover(); e != nil {
							glog.Warningf("HBI %s cleanup callback failure ignored: %+v", netIdent, err)
						}
					}()

					cleanupFunc(err)
				}()
			}
		}()

		for !ho.Cancelled() {

			pkt, err = wire.RecvPacket()
			if err != nil {
				if err == io.EOF {
					// normal case for peer closed connection
					err = nil
				}
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

				if ho.co != nil { // in case the ho co should receive an object, there should be
					// the receiving-code running nested in `ho.Exec()` calls from either of the
					// 2 cases above, and the receiving-code should call `HoCo.RecvObj()` which in
					// turn calls `co.recvObj()` where a nested landing loop will run.
					err = errors.New("an obj sent to the ho co but no prior receiving-code running")
					return
					// todo: should we use an app queue to allow objects be sent prior to the receiving-code?

					// that behavior is supported by the python/asyncio implementation as a side-effect,
					// coz reading from asyncio.transport is pushed.
					// while golang tcp transport is read as per app's decision, the app queue is not necessary.

					// for binary data/stream to be received, there's no way for data comes prior to
					// receiving-code, it's the receiving-code's responsibility to know correct data/stream
					// length. so it's natural to expect objects (landed by code) to be sent then received
					// the same way.
				}

				// no ho co means the sent object meant to be received by
				recvCo := ho.po.coPeek()
				if result, err = ho.Exec(pkt.Payload); err != nil {
					return
				}
				recvCo.respObj <- result

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

				// signal start of response to the head po co.
				// this is crucial in case the po co expects binary data/stream come
				// before any textual code packet if any at all. and in other cases, i.e.
				// no response expected at all, or receiving-code (in form of textual
				// code packet) come first, this is of no use.
				close(recvCo.respBegan)

			case "co_ack_end":

				recvCo := ho.po.coDequeue()
				if recvCo.coSeq != pkt.Payload {
					panic("mismatch co_ack_end")
				}

				close(recvCo.respObj)

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
