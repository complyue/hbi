package proto

import (
	"fmt"

	"github.com/complyue/hbi/pkg/errors"
)

// Conver is the conversation interface.
//  there're basically 2 types of conversations:
//   * the active, posting conversation
//   * the passive, hosting conversation
type Conver interface {
	CoSeq() string

	SendCode(code string) (err error)
	SendObj(code string) (err error)
	SendData(buf []byte) (err error)
	SendStream(data <-chan []byte) (err error)

	RecvObj() (obj interface{}, err error)
	RecvData(buf []byte) (err error)
	RecvStream(data <-chan []byte) (err error)

	Ended() chan struct{}
}

// the data structure for both conversation types
type coState struct {
	// common fields of ho/po co
	ho    *hostingEnd
	coSeq string
	ended chan struct{}

	// po co only fields
	po        *postingEnd
	respBegan chan struct{}
}

// PoCo is the active, posting conversation
type PoCo coState

func (co *PoCo) CoSeq() string {
	return co.coSeq
}

func (co *PoCo) SendCode(code string) (err error) {
	co.po.coAssertSender((*coState)(co))
	_, err = co.po.wire.SendPacket(code, "")
	return
}

func (co *PoCo) SendObj(code string) (err error) {
	co.po.coAssertSender((*coState)(co))
	_, err = co.po.wire.SendPacket(code, "co_recv")
	return
}

func (co *PoCo) SendStream(data <-chan []byte) (err error) {
	co.po.coAssertSender((*coState)(co))
	_, err = co.po.wire.SendStream(data)
	return
}

func (co *PoCo) SendData(buf []byte) (err error) {
	co.po.coAssertSender((*coState)(co))
	_, err = co.po.wire.SendData(buf)
	return
}

func (co *PoCo) GetObj(code string) (obj interface{}, err error) {
	co.po.coAssertSender((*coState)(co))
	_, err = co.po.wire.SendPacket(code, "co_send")
	obj, err = co.RecvObj()
	return
}

func (co *PoCo) RecvObj() (obj interface{}, err error) {
	if co.ho == nil {
		panic(errors.New("receiving by a send-only conversation"))
	}

	// must still be the sending conversation upon recv starting
	co.po.coAssertSender((*coState)(co))

	// wait co_ack_begin from peer
	select {
	case <-co.po.Done():
		err = errors.New("posting endpoint closed")
		return
	case <-co.respBegan:
		// normal case
	}
	// must be the receiving conversation now
	co.po.coAssertReceiver((*coState)(co))

	obj, err = co.ho.recvObj()
	return
}

func (co *PoCo) RecvStream(data <-chan []byte) (err error) {
	if co.ho == nil {
		panic(errors.New("receiving by a send-only conversation"))
	}

	// must still be the sending conversation upon recv starting
	co.po.coAssertSender((*coState)(co))

	// wait co_ack_begin from peer
	select {
	case <-co.po.Done():
		err = errors.New("posting endpoint closed")
		return
	case <-co.respBegan:
		// normal case
	}
	// must be the receiving conversation now
	co.po.coAssertReceiver((*coState)(co))

	_, err = co.ho.wire.RecvStream(data)
	return
}

func (co *PoCo) RecvData(buf []byte) (err error) {
	if co.ho == nil {
		panic(errors.New("receiving by a send-only conversation"))
	}

	// must still be the sending conversation upon recv starting
	co.po.coAssertSender((*coState)(co))

	// wait co_ack_begin from peer
	select {
	case <-co.po.Done():
		err = errors.New("posting endpoint closed")
		return
	case <-co.respBegan:
		// normal case
	}
	// must be the receiving conversation now
	co.po.coAssertReceiver((*coState)(co))

	_, err = co.ho.wire.RecvData(buf)
	return
}

func (co *PoCo) Close() {
	if co.po.Cancelled() {
		return
	}

	co.po.coAssertSender((*coState)(co))

	if _, err := co.po.wire.SendPacket(co.coSeq, "co_end"); err != nil {
		errReason := fmt.Sprintf("%+v", errors.RichError(err))
		co.po.Disconnect(errReason, false)
		panic(err)
	}

	co.po.coEnd((*coState)(co), false)
}

func (co *PoCo) Ended() chan struct{} {
	return co.ended
}

// HoCo is the passive, hosting conversation
type HoCo coState

func (co *HoCo) CoSeq() string {
	return co.coSeq
}

func (co *HoCo) SendCode(code string) (err error) {
	co.ho.po.coAssertSender((*coState)(co))

	_, err = co.ho.wire.SendPacket(code, "")
	return
}

func (co *HoCo) SendObj(code string) (err error) {
	co.ho.po.coAssertSender((*coState)(co))

	_, err = co.ho.wire.SendPacket(code, "co_recv")
	return
}

func (co *HoCo) SendStream(data <-chan []byte) (err error) {
	co.ho.po.coAssertSender((*coState)(co))

	_, err = co.ho.po.wire.SendStream(data)
	return
}

func (co *HoCo) SendData(buf []byte) (err error) {
	co.ho.po.coAssertSender((*coState)(co))

	_, err = co.ho.po.wire.SendData(buf)
	return
}

func (co *HoCo) RecvObj() (result interface{}, err error) {
	co.ho.po.coAssertReceiver((*coState)(co))

	result, err = co.ho.recvObj()
	return
}

func (co *HoCo) RecvStream(data <-chan []byte) (err error) {
	co.ho.po.coAssertReceiver((*coState)(co))

	_, err = co.ho.wire.RecvStream(data)
	return
}

func (co *HoCo) RecvData(buf []byte) (err error) {
	co.ho.po.coAssertReceiver((*coState)(co))

	_, err = co.ho.wire.RecvData(buf)
	return
}

func (co *HoCo) Ended() chan struct{} {
	return co.ended
}
