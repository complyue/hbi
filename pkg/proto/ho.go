package proto

import (
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

// HoCo returns the current hosting conversation in `recv` stage.
func (ho *HostingEnd) HoCo() *HoCo {
	return ho.hbic.recverHoCo()
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

// Done implements the ctx.context interface by returning a channel closed after the
// underlying HBI connection is disconnected.
func (ho *HostingEnd) Done() <-chan struct{} {
	return ho.hbic.Done()
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
	if co.sendDone == nil {
		panic(errors.New("ho co closed"))
	}
	if co != co.hbic.recver {
		panic(errors.New("ho co not in recv stage"))
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
	if co.sendDone == nil {
		panic(errors.New("ho co closed"))
	}
	if co != co.hbic.recver {
		panic(errors.New("ho co not in recv stage"))
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
	if co.sendDone == nil {
		panic(errors.New("ho co closed"))
	}
	if co != co.hbic.recver {
		panic(errors.New("ho co not in recv stage"))
	}

	return co.hbic.recvStream(ds)
}

// StartSend transits this hosting conversation from `recv` stage to `send` stage.
//
// A hosting conversation starts out in `recv` stage, within which SendXXX() operations can not
// be performed with it; until its StartSend() is called to transit it to `send` stage, after
// then SendXXX() operations are allowed but RecvXXX() operations can no longer be performed.
func (co *HoCo) StartSend() error {
	if co.recvDone == nil {
		return errors.New("ho co not in recv stage")
	}

	hbic := co.hbic

	pkt, err := hbic.wire.RecvPacket()
	if err != nil {
		if err == io.EOF {
			// wire disconnected by peer
			err = nil    // not considered an error
			hbic.Close() // disconnect normally
		} else if hbic.CancellableContext.Cancelled() {
			// active disconnection caused wire reading
			err = nil // not considered an error
		}
		return err
	}

	if pkt.WireDir != "co_end" {
		return errors.Errorf("More packet not received by ho co before starting send: %+v", pkt)
	}
	if pkt.Payload != co.coSeq {
		return errors.New("co seq mismatch on co_end")
	}

	// signal coKeeper to start receiving next co
	close(co.recvDone)
	co.recvDone = nil
	hbic.recvMutex.Unlock()

	if err := co.hbic.hoCoStartSend(co); err != nil {
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
	if co.recvDone != nil {
		panic(errors.New("ho co not in send stage"))
	}
	if co.sendDone == nil {
		panic(errors.New("ho co closed"))
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
	if co.recvDone != nil {
		panic(errors.New("ho co not in send stage"))
	}
	if co.sendDone == nil {
		panic(errors.New("ho co closed"))
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
	if co.recvDone != nil {
		panic(errors.New("ho co not in send stage"))
	}
	if co.sendDone == nil {
		panic(errors.New("ho co closed"))
	}
	if co != co.hbic.sender {
		panic(errors.New("ho co not current sender"))
	}

	if err := co.hbic.sendPacket(co.coSeq, "po_data"); err != nil {
		return err
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
	if co.recvDone != nil {
		panic(errors.New("ho co not in send stage"))
	}
	if co.sendDone == nil {
		panic(errors.New("ho co closed"))
	}
	if co != co.hbic.sender {
		panic(errors.New("ho co not current sender"))
	}

	if err := co.hbic.sendPacket(co.coSeq, "po_data"); err != nil {
		return err
	}
	return co.hbic.sendStream(ds)
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

	defer func() {
		// notify peer the completion of hosting of its po co

		if _, err = wire.SendPacket(co.coSeq, "co_ack_end"); err != nil {
			return
		}

		close(co.sendDone)
		co.sendDone = nil

	}()

	// lock recvMutex during receiving
	hbic.recvMutex.Lock()

	for !hbic.Cancelled() && err == nil && len(discReason) <= 0 {

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

			if _, err = env.RunInEnv(pkt.Payload, hbic); err != nil {
				return
			}

		case "co_send":

			// peer is requesting this end to push landed result (in textual repr code) back

			if result, err = env.RunInEnv(pkt.Payload, hbic); err != nil {
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
				panic("StartSend() called ")
			}

			// signal coKeeper to start receiving next co
			close(co.recvDone)
			co.recvDone = nil
			hbic.recvMutex.Unlock()

			return

		case "co_recv":

			// pushing obj to a ho co

			discReason = "co_recv without priori receiving code under landing"
			return

		case "po_data":

			// pushing data/stream to a ho co
			discReason = "po_data to a ho co"
			return

		case "err":

			discReason = fmt.Sprintf("peer error: %s", pkt.Payload)
			trySendPeerError = false
			return

		default:

			discReason = fmt.Sprintf("HostingEnd unexpected packet: %+v", pkt)
			return

		}
	}
}
