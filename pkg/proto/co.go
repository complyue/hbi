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
	respBegan chan struct{}
	respObj   chan interface{}
}

// PoCo is the active, posting conversation
type PoCo coState

func (co *PoCo) CoSeq() string {
	return co.coSeq
}

func (co *PoCo) SendCode(code string) (err error) {
	co.ho.po.coAssertSender((*coState)(co))
	_, err = co.ho.wire.SendPacket(code, "")
	return
}

func (co *PoCo) SendObj(code string) (err error) {
	co.ho.po.coAssertSender((*coState)(co))
	_, err = co.ho.wire.SendPacket(code, "co_recv")
	return
}

func (co *PoCo) SendStream(data <-chan []byte) (err error) {
	co.ho.po.coAssertSender((*coState)(co))
	_, err = co.ho.wire.SendStream(data)
	return
}

func (co *PoCo) SendData(buf []byte) (err error) {
	co.ho.po.coAssertSender((*coState)(co))
	_, err = co.ho.wire.SendData(buf)
	return
}

func (co *PoCo) GetObj(code string) (obj interface{}, err error) {
	co.ho.po.coAssertSender((*coState)(co))
	_, err = co.ho.wire.SendPacket(code, "co_send")
	obj, err = co.RecvObj()
	return
}

func (co *PoCo) RecvObj() (obj interface{}, err error) {
	if co.ho == nil {
		panic(errors.New("receiving by a send-only conversation"))
	}

	// wait co_ack_begin from peer
	select {
	case <-co.ho.Done():
		err = errors.New("hosting endpoint closed")
		return
	case <-co.respBegan:
		// normal case
	}
	// must be the receiving conversation now
	co.ho.po.coAssertReceiver((*coState)(co))

	// wait co_recv from peer, its payload will be landed and sent via respObj
	select {
	case <-co.ho.Done():
		err = errors.New("hosting endpoint closed")
		return
	case result, ok := <-co.respObj:
		if !ok {
			err = errors.New("peer ho co sent no obj as expected")
			return
		}
		// must still be the receiving conversation now
		co.ho.po.coAssertReceiver((*coState)(co))
		obj = result
	}

	return
}

func (co *PoCo) RecvStream(data <-chan []byte) (err error) {
	if co.ho == nil {
		panic(errors.New("receiving by a send-only conversation"))
	}

	// wait co_ack_begin from peer
	select {
	case <-co.ho.Done():
		err = errors.New("hosting endpoint closed")
		return
	case <-co.respBegan:
		// normal case
	}
	// must be the receiving conversation now
	co.ho.po.coAssertReceiver((*coState)(co))

	_, err = co.ho.wire.RecvStream(data)
	return
}

func (co *PoCo) RecvData(buf []byte) (err error) {
	if co.ho == nil {
		panic(errors.New("receiving by a send-only conversation"))
	}

	// wait co_ack_begin from peer
	select {
	case <-co.ho.Done():
		err = errors.New("hosting endpoint closed")
		return
	case <-co.respBegan:
		// normal case
	}
	// must be the receiving conversation now
	co.ho.po.coAssertReceiver((*coState)(co))

	_, err = co.ho.wire.RecvData(buf)
	return
}

func (co *PoCo) Close() {
	if co.ho.po.Cancelled() {
		return
	}

	co.ho.po.coAssertSender((*coState)(co))

	if _, err := co.ho.wire.SendPacket(co.coSeq, "co_end"); err != nil {
		errReason := fmt.Sprintf("%+v", errors.RichError(err))
		co.ho.po.Disconnect(errReason, false)
		panic(err)
	}

	co.ho.po.coEnd((*coState)(co), false)
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
