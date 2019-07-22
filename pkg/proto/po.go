package proto

import (
	"context"

	"github.com/complyue/hbi/pkg/errors"
)

// PostingEnd is the application programming interface of a posting endpoint.
type PostingEnd struct {
	hbic *HBIC

	remoteAddr string
}

// RemoteAddr returns the remote network address of the underlying wire.
func (po *PostingEnd) RemoteAddr() string {
	return po.remoteAddr
}

// NetIdent returns the network identification string of the underlying wire.
func (po *PostingEnd) NetIdent() string {
	return po.hbic.netIdent
}

// NewCo starts a new posting conversation.
func (po *PostingEnd) NewCo(he *HostingEnv) (*PoCo, error) {
	return po.hbic.newPoCo(he)
}

// Notif is shorthand to (implicitly) create a posting conversation, which is closed
// immediately after `code` is sent with it.
//
// the `completed` channel returned can be received from, to wait acknowledgement
// for end of conversation has been received from remote peer.
func (po *PostingEnd) Notif(code string) (completed <-chan struct{}, err error) {
	var co *PoCo
	if co, err = po.hbic.newPoCo(nil); err != nil {
		return
	}
	defer co.Close()
	completed = co.endAcked

	if _, err = po, po.hbic.sendPacket(code, ""); err != nil {
		return
	}
	return
}

// NotifData is shorthand to (implicitly) create a posting conversation, which is closed
// immediately after `code` and `d` are sent with it.
//
// the `completed` channel returned can be received from, to wait acknowledgement
// for end of conversation has been received from remote peer.
func (po *PostingEnd) NotifData(code string, d []byte) (completed <-chan struct{}, err error) {
	var co *PoCo
	if co, err = po.hbic.newPoCo(nil); err != nil {
		return
	}
	defer co.Close()
	completed = co.endAcked

	if err = po.hbic.sendPacket(code, ""); err != nil {
		return
	}
	if err = po.hbic.sendData(d); err != nil {
		return
	}
	return
}

// Disconnect disconnects the underlying wire of the HBI connection, optionally with
// a error message sent to peer site for information purpose.
func (po *PostingEnd) Disconnect(errReason string, trySendPeerError bool) {
	po.hbic.Disconnect(errReason, trySendPeerError)
}

// Close disconnects the underlying wire of the HBI connection.
func (po *PostingEnd) Close() {
	po.hbic.Disconnect("", false)
}

// Context returns the conttext associated with the underlying HBI connection.
func (po *PostingEnd) Context() context.Context {
	return po.hbic
}

// Disconnected tells whether the underlying HBI connection has been disconnected.
func (po *PostingEnd) Disconnected() bool {
	return po.hbic.CancellableContext.Cancelled()
}

// PoCo is the active, posting conversation.
//
// A PoCo is created from application by calling PostingEnd.NewCo()
type PoCo struct {
	// hosting env to use for this conversation, if not nil
	HE *HostingEnv

	hbic  *HBIC
	coSeq string

	sendDone chan struct{}

	beginAcked chan struct{}
	recvDone   chan struct{}
	endAcked   chan struct{}
}

// CoSeq returns the sequence number of this conversation.
//
// The sequence number of a posting conversation is assigned by the posting endpoint created
// it, the value does not necessarily be unique across a long time period, but won't repeat
// among a lot of conversations per sent over a wire in line.
func (co *PoCo) CoSeq() string {
	return co.coSeq
}

// SendCode sends `code` to peer's hosting endpoint for landing by its hosting environment.
//
// Only side effects are expected from landing of `code` at peer site.
//
// Note this can only be called in `send` stage, and from the goroutine which created this
// conversation.
func (co *PoCo) SendCode(code string) error {
	if co.sendDone == nil {
		panic(errors.New("po co not in send stage"))
	}
	if co != co.hbic.sender {
		panic(errors.New("po co not current sender ?!"))
	}

	return co.hbic.sendPacket(code, "")
}

// SendObj sends `code` to peer's hosting endpoint for landing by its hosting environment.
//
// The respective hosting conversation at peer site is expected to receive the result value
// from landing of `code`, by calling HoCo.RecvObj()
//
// Note this can only be called in `send` stage, and from the goroutine which created this
// conversation.
func (co *PoCo) SendObj(code string) error {
	if co.sendDone == nil {
		panic(errors.New("po co not in send stage"))
	}
	if co != co.hbic.sender {
		panic(errors.New("po co not current sender ?!"))
	}

	return co.hbic.sendPacket(code, "co_recv")
}

// SendData sends a single chunk of binary data to peer site.
//
// The respective hosting conversation at peer site is expected to receive the data by
// calling HoCo.RecvData() or HoCo.RecvStream()
//
// Note this can only be called in `send` stage, and from the goroutine which created this
// conversation.
func (co *PoCo) SendData(d []byte) error {
	if co.sendDone == nil {
		panic(errors.New("po co not in send stage"))
	}
	if co != co.hbic.sender {
		panic(errors.New("po co not current sender ?!"))
	}

	return co.hbic.sendData(d)
}

// SendStream polls callback function `ds()` until it returns a nil []byte or non-nil error,
// and send each chunk to peer site in line. `ds()` will be called another time after the
// chunk returned from the previous call has been sent out.
//
// The respective hosting conversation at peer site is expected to receive the data by
// calling HoCo.RecvData() or HoCo.RecvStream()
//
// Note this can only be called in `send` stage, and from the goroutine which created this
// conversation.
func (co *PoCo) SendStream(ds func() ([]byte, error)) error {
	if co.sendDone == nil {
		panic(errors.New("po co not in send stage"))
	}
	if co != co.hbic.sender {
		panic(errors.New("po co not current sender ?!"))
	}

	return co.hbic.sendStream(ds)
}

// StartRecv transits this posting conversation from `send` stage to `recv` stage.
//
// Once in `recv` stage, no `send` operation can be performed any more with this conversation,
// the underlying wire is released for other posting conversation to start off.
//
// Note this can only be called in `send` stage, and from the goroutine which created this
// conversation.
func (co *PoCo) StartRecv() error {
	hbic := co.hbic

	if co.sendDone == nil {
		panic(errors.New("po co not in send stage"))
	}

	if err := hbic.poCoFinishSend(co); err != nil {
		return err
	}

	// wait begin of ho co ack
	select {
	case <-hbic.Done():
		err := hbic.Err()
		if err == nil {
			err = errors.New("hbic disconnected")
		}
		return err
	case <-co.beginAcked:
		// normal case, be current recver now
	}

	hbic.recvMutex.Lock()
	defer hbic.recvMutex.Unlock()

	if co != hbic.recver {
		panic(errors.New("po co not current recver ?!"))
	}

	return nil
}

// RecvObj returns the landed result of a piece of back-script `code` sent with the triggered
// hosting conversation at remote site via HoCo.SendObj(code)
//
// Note this can only be called in `recv` stage, and from the goroutine which created this
// conversation.
func (co *PoCo) RecvObj() (obj interface{}, err error) {
	if co.sendDone != nil {
		panic(errors.New("po co not in recv stage"))
	}

	hbic := co.hbic
	if co != hbic.recver {
		panic(errors.New("po co not current recver ?!"))
	}

	return hbic.recvOneObj(co.HE)
}

// RecvData receives the binary data/stream sent with the triggered hosting conversation at
// remote site via HoCo.SendData() or HoCo.SendStream()
//
// Note this can only be called in `recv` stage, and from the goroutine which created this
// conversation.
func (co *PoCo) RecvData(d []byte) error {
	if co.sendDone != nil {
		panic(errors.New("po co not in recv stage"))
	}

	hbic := co.hbic
	if co != hbic.recver {
		panic(errors.New("po co not current recver ?!"))
	}

	return hbic.recvData(d)
}

// RecvStream receives the binary data/stream sent with the triggered hosting conversation at
// remote site via HoCo.SendData() or HoCo.SendStream()
//
// Note this can only be called in `recv` stage, and from the goroutine which created this
// conversation.
func (co *PoCo) RecvStream(ds func() ([]byte, error)) error {
	if co.sendDone != nil {
		panic(errors.New("po co not in recv stage"))
	}

	hbic := co.hbic
	if co != hbic.recver {
		panic(errors.New("po co not current recver ?!"))
	}

	return hbic.recvStream(ds)
}

// Close closes this posting conversation, neither send nor recv operation can be performed
// with a closed posting conversation.
//
// Note this can only be called from the goroutine which created this conversation.
func (co *PoCo) Close() error {
	hbic := co.hbic
	if hbic.CancellableContext.Cancelled() {
		return hbic.Err() // already disconnected
	}

	if co.sendDone != nil {
		hbic.poCoFinishSend(co)
	}

	if co.recvDone != closedChan {
		close(co.recvDone)
		co.recvDone = closedChan
	}

	return nil
}

// Completed returns the channel that get closed when this posting conversation has been
// fully processed with the triggered hosting conversation at remote site done.
//
// Subsequent processes depending on the success of this conversation's completion can
// receive from the returned channel to wait the signal of proceed, with the backing hbic's
// Done() channel selected together.
//
// Closing of this channel before its backing hbic is closed can confirm the final success of
// this conversation, as well its `recv` stage. i.e. all peer-scripting-code and data/stream
// sent with this conversation has been landed by peer's hosting endpoint, with a triggered
// hosting conversation, and all back-scripts (plus data/stream if any) as the response
// from that hosting conversation has been landed by local hosting endpoint, and received
// with this posting conversation (if any recv ops involved).
func (co *PoCo) Completed() <-chan struct{} {
	return co.endAcked
}
