package proto

import (
	"context"
	"fmt"
	"io"

	"github.com/complyue/hbi/pkg/errors"
	"github.com/golang/glog"
)

// HostingEnd is the application programming interface of a hosting endpoint.
type HostingEnd struct {
	hbic *HBIC

	env *HostingEnv

	localAddr string
}

// Env returns the hosting environment that this hosting endpoint is attached to.
func (ho *HostingEnd) Env() *HostingEnv {
	return ho.env
}

// LocalAddr returns the local network address of the underlying wire.
func (ho *HostingEnd) LocalAddr() string {
	return ho.localAddr
}

// NetIdent returns the network identification string of the underlying wire.
func (ho *HostingEnd) NetIdent() string {
	return ho.hbic.netIdent
}

// Co returns the current hosting conversation in `recv` stage.
func (ho *HostingEnd) Co() *HoCo {
	hbic := ho.hbic
	hbic.recvMutex.Lock()
	defer hbic.recvMutex.Unlock()

	if hoCo, ok := hbic.recver.(*HoCo); ok {
		return hoCo
	}

	return nil
}

// Disconnect disconnects the underlying wire of the HBI connection, optionally with
// a error message sent to peer site for information purpose.
func (ho *HostingEnd) Disconnect(errReason string, trySendPeerError bool) {
	ho.hbic.Disconnect(errReason, trySendPeerError)
}

// Close disconnects the underlying wire of the HBI connection.
func (ho *HostingEnd) Close() {
	ho.hbic.Disconnect("", false)
}

// Context returns the conttext associated with the underlying HBI connection.
func (ho *HostingEnd) Context() context.Context {
	return ho.hbic
}

// Disconnected tells whether the underlying HBI connection has been disconnected.
func (ho *HostingEnd) Disconnected() bool {
	return ho.hbic.CancellableContext.Cancelled()
}

// HoCo is the passive, hosting conversation.
//
// A HoCo is triggered by a PoCo from peer's posting endpoint, it is automatically available to
// application, obtained by calling HostingEnd.Co()
type HoCo struct {
	hbic  *HBIC
	coSeq string

	recvDone chan struct{}
	sendDone chan struct{}
}

// CoSeq returns the sequence number of this conversation.
//
// The sequence of a hosting conversation is always the same as the peer's posting conversation
// that triggered it.
func (co *HoCo) CoSeq() string {
	return co.coSeq
}

// RecvObj returns the landed result of a piece of peer-scripting-code sent by calling
// PoCo.SendObj() with the remote posting conversation which triggered this ho co.
//
// Note this can only be called in `recv` stage, and from the dedicated hosting goroutine, i.e.
// from functions exposed to the hosting environment and called by the peer-scripting-code from
// the remote posting conversation which triggered this ho co.
func (co *HoCo) RecvObj() (interface{}, error) {
	if co.recvDone == nil {
		panic(errors.New("ho co not in recv stage"))
	}
	if co != co.hbic.recver {
		panic(errors.New("ho co not current recver"))
	}

	return co.hbic.recvOneObj()
}

// RecvData receives the binary data/stream sent by calling PoCo.SendData() or PoCo.SendStream()
// with the remote posting conversation which triggered this ho co.
//
// Note this can only be called in `recv` stage, and from the dedicated hosting goroutine, i.e.
// from functions exposed to the hosting environment and called by the peer-scripting-code from
// the remote posting conversation which triggered this ho co.
func (co *HoCo) RecvData(d []byte) error {
	if co.recvDone == nil {
		panic(errors.New("ho co not in recv stage"))
	}
	if co != co.hbic.recver {
		panic(errors.New("ho co not current recver"))
	}

	return co.hbic.recvData(d)
}

// RecvStream receives the binary data/stream sent by calling PoCo.SendData() or PoCo.SendStream()
// with the remote posting conversation which triggered this ho co.
//
// Note this can only be called in `recv` stage, and from the dedicated hosting goroutine, i.e.
// from functions exposed to the hosting environment and called by the peer-scripting-code from
// the remote posting conversation which triggered this ho co.
func (co *HoCo) RecvStream(ds func() ([]byte, error)) error {
	if co.recvDone == nil {
		panic(errors.New("ho co not in recv stage"))
	}
	if co != co.hbic.recver {
		panic(errors.New("ho co not current recver"))
	}

	return co.hbic.recvStream(ds)
}

// FinishRecv transits this hosting conversation from `recv` to `work` stage.
//
// As soon as all recv operations done, if some time-consuming work should be carried out to
// prepare the response to be sent back, a hosting conversation should transit to `work` stage
// by calling `FinishRecv()`; a hosting conversion should be closed directly, if nothing is
// supposed to be sent back.
//
// Note this can only be called from the dedicated hosting goroutine, i.e. from functions
// exposed to the hosting environment and called by the peer-scripting-code from the remote
// posting conversation which triggered this ho co.
func (co *HoCo) FinishRecv() error {
	hbic := co.hbic
	if co.recvDone != nil {
		if err := hbic.hoCoFinishRecv(co); err != nil {
			return err
		}
	}
	return nil
}

// StartSend transits this hosting conversation from `recv` or `work` stage to `send` stage.
//
// As soon as all recv operations done, if no time-consuming work needs to be carried out to
// prepare the response to be sent back, a hosting conversation should transit to `send` stage
// by calling `StartSend()`; a hosting conversion should be closed directly, if nothing is
// supposed to be sent back.
//
// As soon as no further back-script and/or data/stream is to be sent with a hosting conversation,
// it should close to release the underlying HBI transport wire for the next posting conversation
// to start off or next send-ready hosting conversation to start sending.
//
// Note this can only be called from the dedicated hosting goroutine, i.e. from functions
// exposed to the hosting environment and called by the peer-scripting-code from the remote
// posting conversation which triggered this ho co.
func (co *HoCo) StartSend() error {
	hbic := co.hbic
	if co.recvDone != nil {
		if err := hbic.hoCoFinishRecv(co); err != nil {
			return err
		}
	}
	if err := hbic.hoCoStartSend(co); err != nil {
		return err
	}
	return nil
}

// SendCode sends `code` as back-script to peer's hosting endpoint for landing by its hosting
// environment. Only side effects are expected from landing of `code` at peer site.
//
// Note this can only be called in `send` stage, and from the dedicated hosting goroutine, i.e.
// from functions exposed to the hosting environment and called by the peer-scripting-code from
// the remote posting conversation which triggered this ho co.
func (co *HoCo) SendCode(code string) error {
	if co.sendDone == nil {
		panic(errors.New("ho co not in send stage"))
	}
	if co != co.hbic.sender {
		panic(errors.New("ho co not current sender"))
	}

	return co.hbic.sendPacket(code, "")
}

// SendObj sends `code` to peer's hosting endpoint for landing by its hosting environment, and
// the landed value to be received by calling PoCo.RecvObj() with the remote posting conversation
// which triggered this ho co.
//
// Note this can only be called in `send` stage, and from the dedicated hosting goroutine, i.e.
// from functions exposed to the hosting environment and called by the peer-scripting-code from
// the remote posting conversation which triggered this ho co.
func (co *HoCo) SendObj(code string) error {
	if co.sendDone == nil {
		panic(errors.New("ho co not in send stage"))
	}
	if co != co.hbic.sender {
		panic(errors.New("ho co not current sender"))
	}

	return co.hbic.sendPacket(code, "co_recv")
}

// SendData sends a single chunk of binary data to peer site, to be received with the remote
// posting conversation which triggered this ho co, by calling PoCo.RecvData() or
// PoCo.RecvStream()
//
// Note this can only be called in `send` stage, and from the dedicated hosting goroutine, i.e.
// from functions exposed to the hosting environment and called by the peer-scripting-code from
// the remote posting conversation which triggered this ho co.
func (co *HoCo) SendData(d []byte) error {
	if co.sendDone == nil {
		panic(errors.New("ho co not in send stage"))
	}
	if co != co.hbic.sender {
		panic(errors.New("ho co not current sender"))
	}
	return co.hbic.sendData(d)
}

// SendStream polls callback function `ds()` until it returns a nil []byte or non-nil error,
// and send each chunk to peer site in order to be received with the remote
// posting conversation which triggered this ho co, by calling PoCo.RecvData() or
// PoCo.RecvStream()
//
// `ds()` will be called each time after the chunk returned from the previous call has been
// sent out.
//
// Note this can only be called in `send` stage, and from the dedicated hosting goroutine, i.e.
// from functions exposed to the hosting environment and called by the peer-scripting-code from
// the remote posting conversation which triggered this ho co.
func (co *HoCo) SendStream(ds func() ([]byte, error)) error {
	if co.sendDone == nil {
		panic(errors.New("ho co not in send stage"))
	}
	if co != co.hbic.sender {
		panic(errors.New("ho co not current sender"))
	}
	return co.hbic.sendStream(ds)
}

// Close closes this hosting conversation, neither send nor recv operation can be performed
// with a closed hosting conversation.
//
// Note this can only be called from the dedicated hosting goroutine, i.e. from functions
// exposed to the hosting environment and called by the peer-scripting-code from the remote
// posting conversation which triggered this ho co.
func (co *HoCo) Close() error {
	hbic := co.hbic
	if co.recvDone != nil {
		if err := hbic.hoCoFinishRecv(co); err != nil {
			return err
		}
	}
	if err := hbic.hoCoFinishSend(co); err != nil {
		return err
	}
	return nil
}

// this must be run as a dedicated goroutine
func (co *HoCo) hostingThread() {
	var (
		hbic = co.hbic

		err error

		wire = hbic.wire
		env  = hbic.ho.env

		pkt *Packet

		discReason       string
		trySendPeerError = true
	)

	if co != hbic.recver {
		panic(errors.New("ho co not start out as current recver ?!"))
	}
	if co.recvDone == nil {
		panic(errors.New("ho co started out w/ recv done ?!"))
	}

	defer func() {
		if e := recover(); e != nil {
			err = errors.RichError(e)
		} else if err != nil {
			err = errors.RichError(err)
		}

		if len(discReason) > 0 {
			if err == nil {
				err = errors.New(discReason)
			} else {
				glog.Errorf("Detail error for disconnection: %+v", err)
			}
		} else if err != nil {
			discReason = fmt.Sprintf("landing error: %+v", err)
		}

		// disconnect wire if there's a reason
		if len(discReason) > 0 {
			hbic.Disconnect(discReason, trySendPeerError)
			glog.Errorf("Last HBI packet (possibly responsible for failure): %+v", pkt)
			return
		}
	}()

	for {
		if hbic.Cancelled() || err != nil || len(discReason) > 0 || co.recvDone == nil {
			panic(errors.New("?!"))
		}

		pkt, err = wire.RecvPacket()
		if err != nil {
			if err == io.EOF {
				// wire disconnected by peer
				err = nil    // not considered an error
				hbic.Close() // disconnect normally
			} else if hbic.CancellableContext.Cancelled() {
				// active disconnection caused wire reading
				err = nil // not considered an error
			}
			return
		}
		var result interface{}

		switch pkt.WireDir {

		case "":
			// peer is pushing the textual code for side-effect of its landing

			if _, err = env.RunInEnv(hbic, pkt.Payload); err != nil {
				return
			}

			if co.recvDone == nil {
				// recv actively finished by the exposed reacting function

				// finish send anyway
				if err = hbic.hoCoFinishSend(co); err != nil {
					return
				}

				// terminate this hosting thread anyway
				return
			}

		case "co_send":
			// peer is requesting this end to push landed result (in textual repr code) back

			if result, err = env.RunInEnv(hbic, pkt.Payload); err != nil {
				return
			}
			if _, err = wire.SendPacket(Repr(result), "co_recv"); err != nil {
				trySendPeerError = false
				return
			}

		case "co_end":
			// done with this hosting conversation

			if pkt.Payload != co.coSeq {
				discReason = "co seq mismatch on co_end"
			}

			if co.recvDone == nil {
				panic(errors.New("recv finished by reacting func without co_end swallowed ?!"))
			}

			// signal coKeeper to start receiving next co
			close(co.recvDone)
			co.recvDone = nil

			if err = hbic.hoCoFinishSend(co); err != nil {
				return
			}

			return

		case "co_recv":
			// pushing obj to a ho co

			discReason = "co_recv without priori receiving code under landing"
			return

		case "err":

			discReason = fmt.Sprintf("peer error: %s", pkt.Payload)
			trySendPeerError = false
			return

		default:

			discReason = fmt.Sprintf("HO unexpected packet: %+v", pkt)
			return

		}
	}
}
