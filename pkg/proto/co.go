package proto

import (
	"github.com/complyue/hbi/pkg/errors"
)

// Conver is the conversation interface.
//  there're basically 2 types of conversations:
//   * the active, posting conversation
//   * the passive, hosting conversation
type Conver interface {
	CoSeq() string

	SendCode(code string) error
	SendObj(code string) error
	SendData(buf []byte) error
	SendStream(data <-chan []byte) error

	RecvObj() (obj interface{}, err error)
	RecvData(buf []byte) error
	RecvStream(data <-chan []byte) error

	// Closed returns a channel that get closed when all its out sending activities have finished.
	//
	// for a posting conversation, such a channel is closed when its`Close()` is called;
	// for a hosting conversation, such a channel is closed when all packets from the peer
	// posting conversation have been landed, and all binary data streams have been received,
	// this is technically marked by a `co_end` wire directive received.
	Closed() chan struct{}
}

// the data structure for both conversation types
type coState struct {
	// common fields of ho/po co
	hbic  *HBIC
	coSeq string

	// closed
	sendDone chan struct{}

	// po co only fields
	beginAcked chan struct{}
	roq        chan interface{}
	rdq        chan []byte
}

// PoCo is the active, posting conversation
type PoCo coState

func (co *PoCo) CoSeq() string {
	return co.coSeq
}

func (co *PoCo) SendCode(code string) error {
	co.hbic.coAssertSender((*coState)(co))

	return co.hbic.sendPacket(code, "")
}

func (co *PoCo) SendObj(code string) error {
	co.hbic.coAssertSender((*coState)(co))

	return co.hbic.sendPacket(code, "co_recv")
}

func (co *PoCo) SendData(d []byte) error {
	co.hbic.coAssertSender((*coState)(co))

	return co.hbic.sendData(d)
}

func (co *PoCo) SendStream(ds <-chan []byte) error {
	co.hbic.coAssertSender((*coState)(co))

	return co.hbic.sendStream(ds)
}

func (co *PoCo) RecvObj() (obj interface{}, err error) {
	// wait co_ack_begin from peer
	select {
	case <-co.hbic.Done():
		err = errors.New("disconnected")
		return
	case <-co.beginAcked:
		// normal case
	}
	// must be the receiving conversation now
	co.hbic.coAssertReceiver((*coState)(co))

	// wait co_recv from peer, the payload will be landed and sent via roq
	select {
	case <-co.hbic.Done():
		err = errors.New("disconnected")
		return
	case result, ok := <-co.roq:
		if !ok {
			err = errors.New("no obj sent from peer hosting conversation")
			return
		}
		obj = result
	}

	return
}

func (co *PoCo) RecvData(d []byte) error {
	// wait co_ack_begin from peer
	select {
	case <-co.hbic.Done():
		return errors.New("disconnected")
	case <-co.beginAcked:
		// normal case
	}
	// must be the receiving conversation now
	co.hbic.coAssertReceiver((*coState)(co))

	select {
	case <-co.hbic.Done():
		return errors.New("disconnected")
	case co.rdq <- d:
		// normal case
	}

	return nil
}

func (co *PoCo) RecvStream(ds <-chan []byte) error {
	// wait co_ack_begin from peer
	select {
	case <-co.hbic.Done():
		return errors.New("disconnected")
	case <-co.beginAcked:
		// normal case
	}
	// must be the receiving conversation now
	co.hbic.coAssertReceiver((*coState)(co))

	for d := range ds {
		select {
		case <-co.hbic.Done():
			return errors.New("disconnected")
		case co.rdq <- d:
			// normal case
		}
	}

	return nil
}

func (co *PoCo) Close() error {
	if co.hbic.Cancelled() {
		return co.hbic.Err()
	}

	co.hbic.coAssertSender((*coState)(co))

	if err := co.hbic.sendPacket(co.coSeq, "co_end"); err != nil {
		return err
	}

	co.hbic.coEnd((*coState)(co), false)

	return nil
}

func (co *PoCo) Closed() chan struct{} {
	return co.sendDone
}

// HoCo is the passive, hosting conversation
type HoCo coState

func (co *HoCo) CoSeq() string {
	return co.coSeq
}

func (co *HoCo) SendCode(code string) error {
	co.hbic.coAssertSender((*coState)(co))

	return co.hbic.sendPacket(code, "")
}

func (co *HoCo) SendObj(code string) error {
	co.hbic.coAssertSender((*coState)(co))

	return co.hbic.sendPacket(code, "co_recv")
}

func (co *HoCo) SendData(d []byte) error {
	co.hbic.coAssertSender((*coState)(co))

	return co.hbic.sendData(d)
}

func (co *HoCo) SendStream(ds <-chan []byte) error {
	co.hbic.coAssertSender((*coState)(co))

	return co.hbic.sendStream(ds)
}

func (co *HoCo) RecvObj() (interface{}, error) {
	co.hbic.coAssertReceiver((*coState)(co))

	return co.hbic.recvOneObj()
}

func (co *HoCo) RecvData(d []byte) error {
	co.hbic.coAssertReceiver((*coState)(co))

	return co.hbic.recvData(d)
}

func (co *HoCo) RecvStream(ds <-chan []byte) error {
	co.hbic.coAssertReceiver((*coState)(co))

	return co.hbic.recvStream(ds)
}

func (co *HoCo) Closed() chan struct{} {
	return co.sendDone
}
