package proto

import (
	"github.com/complyue/hbi/pkg/errors"
)

// PoCo is the active, posting conversation.
//
// A PoCo is created from application by calling PostingEnd.NewCo()
type PoCo struct {
	hbic  *HBIC
	coSeq string

	sendDone chan struct{}

	beginAcked chan struct{}
	roq        chan interface{}
	rdq        chan []byte
	rddq       chan *byte
	endAcked   chan struct{}
}

// CoSeq returns the sequence number of this conversation.
//
// The sequence number of a posting conversation is assigned by the posting endpoint created
// it, the value does not necessarily be unique within a long time period, but won't repeat
// among millons of conversations per sent over a wire in line.
func (co *PoCo) CoSeq() string {
	return co.coSeq
}

// SendCode sends `code` to peer's hosting endpoint for landing by its hosting environment.
//
// Only side effects are expected from landing of `code` at peer site.
//
// It is prohibited to `send` after the posting conversation has been closed, i.e. can only
// do within `send` phase.
//
// Note this is not thread safe, can only be called from the goroutine which created this
// conversation.
func (co *PoCo) SendCode(code string) error {
	if co.sendDone == nil {
		panic(errors.New("PoCo already closed"))
	}

	return co.hbic.sendPacket(code, "")
}

// SendObj sends `code` to peer's hosting endpoint for landing by its hosting environment.
//
// The respective hosting conversation at peer site is expected to receive the result value
// from landing of `code`, by calling Ho().RecvObj()
//
// It is prohibited to `send` after the posting conversation has been closed, i.e. can only
// do within `send` phase.
//
// Note this is not thread safe, can only be called from the goroutine which created this
// conversation.
func (co *PoCo) SendObj(code string) error {
	if co.sendDone == nil {
		panic(errors.New("PoCo already closed"))
	}

	return co.hbic.sendPacket(code, "co_recv")
}

// SendData sends a single chunk of binary data to peer site.
//
// The respective hosting conversation at peer site is expected to receive the data by
// calling Ho().RecvData() or Ho().RecvStream()
//
// It is prohibited to `send` after the posting conversation has been closed, i.e. can only
// do within `send` phase.
//
// Note this is not thread safe, can only be called from the goroutine which created this
// conversation.
func (co *PoCo) SendData(d []byte) error {
	if co.sendDone == nil {
		panic(errors.New("PoCo already closed"))
	}

	return co.hbic.sendData(d)
}

// SendStream polls callback function `ds()` until it returns a nil []byte or non-nil error,
// and send each chunk to peer site in line. `ds()` will be called another time after the
// chunk returned from the previous call has been sent out.
//
// The respective hosting conversation at peer site is expected to receive the data by
// calling Ho().RecvData() or Ho().RecvStream()
//
// It is prohibited to `send` after the posting conversation has been closed, i.e. can only
// do within `send` phase.
//
// Note this is not thread safe, can only be called from the goroutine which created this
// conversation.
func (co *PoCo) SendStream(ds func() ([]byte, error)) error {
	if co.sendDone == nil {
		panic(errors.New("PoCo already closed"))
	}

	return co.hbic.sendStream(ds)
}

// Close closes this posting conversation to transit it from `send` phase to `recv` phase.
//
// Once closed, no out sending can be performed with this conversation, as the underlying
// wire is released for other posting conversation to start off, so as to keep ideal
// efficiency of the HBI connection in pipeline.
//
// Note this is not thread safe, can only be called from the goroutine which created this
// conversation.
func (co *PoCo) Close() error {
	if co.hbic.Cancelled() {
		err := co.hbic.Err()
		if err == nil {
			err = errors.New("disconnected")
		}
		return err
	}

	if co.sendDone == nil {
		panic(errors.New("PoCo already closed"))
	}

	return co.hbic.endPoCo(co)
}

// IsClosed tells whether this posting conversation has been closed, i.e. in its `recv` phase
// instead of `send` phase.
func (co *PoCo) IsClosed() bool {
	co.hbic.muCo.Lock()
	sendDone := co.sendDone
	co.hbic.muCo.Unlock()

	return sendDone == nil
}

// RecvDone returns the channel that get closed when this posting conversation has been
// fully processed.
//
// Subsequent processes depending on the success of this conversation's completion can
// receive from the returned channel to wait the signal of proceed, with the backing hbic's
// Done() channel selected together.
//
// Closing of this channel before its backing hbic is closed can confirm the final success of
// this conversation, as well its `recv` phase. i.e. all peer-scripting-code and data/stream
// sent with this conversation has been landed by peer's hosting endpoint, with a triggered
// hosting conversation, and all back-scripts (plus data/stream if any) as the response
// from that hosting conversation has been landed by local hosting endpoint, and received
// with this posting conversation (if any recv ops involved).
func (co *PoCo) RecvDone() <-chan struct{} {
	return co.endAcked
}

// RecvObj returns the landed result of a piece of back-script `code` sent with the triggered
// hosting conversation at remote site via HoCo().SendObj(code)
//
// It is prohibited to `recv` before the posting conversation is closed, i.e. can only do
// within the `recv` phase. As the landing of back-scripts from triggered hosting conversation
// at remote site can be queue up behind other hosting conversations triggered by other remote
// posting conversations, those hosting conversations can not start landing before this posting
// conversation is closed, `recv` before this po co closed will deadlock in such cases.
func (co *PoCo) RecvObj() (obj interface{}, err error) {
	co.hbic.muCo.Lock()
	sendDone := co.sendDone
	co.hbic.muCo.Unlock()
	if sendDone != nil {
		panic(errors.New("PoCo still open, must close it before doing recv"))
	}

	// wait co_recv from peer, the payload will be landed and sent via roq
	select {
	case <-co.hbic.Done():
		err = co.hbic.Err()
		if err == nil {
			err = errors.New("disconnected")
		}
		return
	case result, ok := <-co.roq:
		if !ok {
			err = errors.New("no obj sent from triggered hosting conversation")
			return
		}
		obj = result
	}
	return
}

// RecvData receives the binary data/stream sent with the triggered hosting conversation at
// remote site via HoCo().SendData() or HoCo().SendStream()
//
// It is prohibited to `recv` before the posting conversation is closed, i.e. can only do
// within the `recv` phase. As the landing of back-scripts from triggered hosting conversation
// at remote site can be queue up behind other hosting conversations triggered by other remote
// posting conversations, those hosting conversations can not start landing before this posting
// conversation is closed, `recv` before this po co closed will deadlock in such cases.
func (co *PoCo) RecvData(d []byte) error {
	co.hbic.muCo.Lock()
	sendDone := co.sendDone
	co.hbic.muCo.Unlock()
	if sendDone != nil {
		panic(errors.New("PoCo still open, must close it before doing recv"))
	}

	select {
	case <-co.hbic.Done():
		err := co.hbic.Err()
		if err == nil {
			err = errors.New("disconnected")
		}
		return err
	case co.rdq <- d:
		// normal case
	}

	select {
	case <-co.hbic.Done():
		err := co.hbic.Err()
		if err == nil {
			err = errors.New("disconnected")
		}
		return err
	case pd := <-co.rddq:
		// normal case
		if pd != &d[0] {
			panic("?!")
		}
	}

	return nil
}

// RecvStream receives the binary data/stream sent with the triggered hosting conversation at
// remote site via HoCo().SendData() or HoCo().SendStream()
//
// It is prohibited to `recv` before the posting conversation is closed, i.e. can only do
// within the `recv` phase. As the landing of back-scripts from triggered hosting conversation
// at remote site can be queue up behind other hosting conversations triggered by other remote
// posting conversations, those hosting conversations can not start landing before this posting
// conversation is closed, `recv` before this po co closed will deadlock in such cases.
func (co *PoCo) RecvStream(ds func() ([]byte, error)) error {
	co.hbic.muCo.Lock()
	sendDone := co.sendDone
	co.hbic.muCo.Unlock()
	if sendDone != nil {
		panic(errors.New("PoCo still open, must close it before doing recv"))
	}

	for {
		d, err := ds()
		if err != nil {
			return err
		}

		select {
		case <-co.hbic.Done():
			err := co.hbic.Err()
			if err == nil {
				err = errors.New("disconnected")
			}
			return err
		case co.rdq <- d:
			// normal case
		}

		if d == nil {
			// all data received
			break
		}

		select {
		case <-co.hbic.Done():
			err := co.hbic.Err()
			if err == nil {
				err = errors.New("disconnected")
			}
			return err
		case pd := <-co.rddq:
			// normal case
			if pd != &d[0] {
				panic("?!")
			}
		}
	}

	return nil
}

// HoCo is the passive, hosting conversation.
//
// A HoCo is triggered by a PoCo from peer's posting endpoint, it is automatically available to
// application, obtained by calling HostingEnd.Co()
type HoCo struct {
	hbic  *HBIC
	coSeq string

	sendDone chan struct{}
}

// CoSeq returns the sequence number of this conversation.
//
// The sequence of a hosting conversation is always the same as the peer's posting conversation
// that triggered it.
func (co *HoCo) CoSeq() string {
	return co.coSeq
}

// SendCode sends `code` as back-script to peer's hosting endpoint for landing by its hosting
// environment. Only side effects are expected from landing of `code` at peer site.
//
// Both `send` and `recv` are allowed with a hosting conversation as long as it is open, such
// operations will panic after the conversation is closed.
// A hosting conversation is implicitly closed after all peer-scripting-code (and data/stream
// if any) from the remote posting conversation have been landed and received.
//
// Note this can only be called from the landing thread of the hosting endpoint, i.e. from
// functions exposed to the hosting environment and called by the peer-scripting-code from the
// remote posting conversation which triggered this ho co.
func (co *HoCo) SendCode(code string) error {
	if co.sendDone == nil {
		panic("ho co closed")
	}
	if co != co.hbic.ho.co {
		panic("open ho co not inplace ?!")
	}

	return co.hbic.sendPacket(code, "")
}

// SendObj sends `code` to peer's hosting endpoint for landing by its hosting environment, and
// the landed value to be received by calling PoCo.RecvObj() with the remote posting conversation
// which triggered this ho co.
//
// Both `send` and `recv` are allowed with a hosting conversation as long as it is open, such
// operations will panic after the conversation is closed.
// A hosting conversation is implicitly closed after all peer-scripting-code (and data/stream
// if any) from the remote posting conversation have been landed and received.
//
// Note this can only be called from the landing thread of the hosting endpoint, i.e. from
// functions exposed to the hosting environment and called by the peer-scripting-code from the
// remote posting conversation which triggered this ho co.
func (co *HoCo) SendObj(code string) error {
	if co.sendDone == nil {
		panic("ho co closed")
	}
	if co != co.hbic.ho.co {
		panic("open ho co not inplace ?!")
	}

	return co.hbic.sendPacket(code, "co_recv")
}

// SendData sends a single chunk of binary data to peer site, to be received with the remote
// posting conversation which triggered this ho co, by calling PoCo.RecvData() or
// PoCo.RecvStream()
//
// Both `send` and `recv` are allowed with a hosting conversation as long as it is open, such
// operations will panic after the conversation is closed.
// A hosting conversation is implicitly closed after all peer-scripting-code (and data/stream
// if any) from the remote posting conversation have been landed and received.
//
// Note this can only be called from the landing thread of the hosting endpoint, i.e. from
// functions exposed to the hosting environment and called by the peer-scripting-code from the
// remote posting conversation which triggered this ho co.
func (co *HoCo) SendData(d []byte) error {
	if co.sendDone == nil {
		panic("ho co closed")
	}
	if co != co.hbic.ho.co {
		panic("open ho co not inplace ?!")
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
// Both `send` and `recv` are allowed with a hosting conversation as long as it is open, such
// operations will panic after the conversation is closed.
// A hosting conversation is implicitly closed after all peer-scripting-code (and data/stream
// if any) from the remote posting conversation have been landed and received.
//
// Note this can only be called from the landing thread of the hosting endpoint, i.e. from
// functions exposed to the hosting environment and called by the peer-scripting-code from the
// remote posting conversation which triggered this ho co.
func (co *HoCo) SendStream(ds func() ([]byte, error)) error {
	if co.sendDone == nil {
		panic("ho co closed")
	}
	if co != co.hbic.ho.co {
		panic("open ho co not inplace ?!")
	}

	if err := co.hbic.sendPacket(co.coSeq, "po_data"); err != nil {
		return err
	}
	return co.hbic.sendStream(ds)
}

// RecvObj returns the landed result of a piece of peer-scripting-code sent by calling
// PoCo.SendObj() with the remote posting conversation which triggered this ho co.
//
// Both `send` and `recv` are allowed with a hosting conversation as long as it is open, such
// operations will panic after the conversation is closed.
// A hosting conversation is implicitly closed after all peer-scripting-code (and data/stream
// if any) from the remote posting conversation have been landed and received.
//
// Note this can only be called from the landing thread of the hosting endpoint, i.e. from
// functions exposed to the hosting environment and called by the peer-scripting-code from the
// remote posting conversation which triggered this ho co.
func (co *HoCo) RecvObj() (interface{}, error) {
	if co.sendDone == nil {
		panic("ho co closed")
	}
	if co != co.hbic.ho.co {
		panic("open ho co not inplace ?!")
	}

	return co.hbic.recvOneObj()
}

// RecvData receives the binary data/stream sent by calling PoCo.SendData() or PoCo.SendStream()
// with the remote posting conversation which triggered this ho co.
//
// Both `send` and `recv` are allowed with a hosting conversation as long as it is open, such
// operations will panic after the conversation is closed.
// A hosting conversation is implicitly closed after all peer-scripting-code (and data/stream
// if any) from the remote posting conversation have been landed and received.
//
// Note this can only be called from the landing thread of the hosting endpoint, i.e. from
// functions exposed to the hosting environment and called by the peer-scripting-code from the
// remote posting conversation which triggered this ho co.
func (co *HoCo) RecvData(d []byte) error {
	if co.sendDone == nil {
		panic("ho co closed")
	}
	if co != co.hbic.ho.co {
		panic("open ho co not inplace ?!")
	}

	return co.hbic.recvData(d)
}

// RecvStream receives the binary data/stream sent by calling PoCo.SendData() or PoCo.SendStream()
// with the remote posting conversation which triggered this ho co.
//
// Both `send` and `recv` are allowed with a hosting conversation as long as it is open, such
// operations will panic after the conversation is closed.
// A hosting conversation is implicitly closed after all peer-scripting-code (and data/stream
// if any) from the remote posting conversation have been landed and received.
//
// Note this can only be called from the landing thread of the hosting endpoint, i.e. from
// functions exposed to the hosting environment and called by the peer-scripting-code from the
// remote posting conversation which triggered this ho co.
func (co *HoCo) RecvStream(ds func() ([]byte, error)) error {
	if co.sendDone == nil {
		panic("ho co closed")
	}
	if co != co.hbic.ho.co {
		panic("open ho co not inplace ?!")
	}

	return co.hbic.recvStream(ds)
}

// IsClosed tells whether this hosting conversation has been closed.
//
// Both `send` and `recv` are allowed with a hosting conversation as long as it is open, such
// operations will panic after the conversation is closed.
// A hosting conversation is implicitly closed after all peer-scripting-code (and data/stream
// if any) from the remote posting conversation have been landed and received.
//
// Note this can only be called from the landing thread of the hosting endpoint, i.e. from
// functions exposed to the hosting environment and called by the peer-scripting-code from the
// remote posting conversation which triggered this ho co.
func (co *HoCo) IsClosed() bool {
	if co.sendDone != nil && co != co.hbic.ho.co {
		panic("open ho co not inplace ?!")
	}

	return co.sendDone == nil
}
