package proto

import (
	"fmt"
	"unsafe"

	"github.com/complyue/hbi/pkg/errors"
)

// Conver is the conversation interface, there're basically 2 types of conversations:
//   * Posting/Active conversation
//   * Hosting/Passive conversation
type Conver interface {
	CoID() string

	SendCode(code string) (err error)

	SendObj(code string) (err error)

	SendData(data <-chan []byte) (err error)

	RecvObj() (obj interface{}, err error)

	RecvData(data <-chan []byte) (err error)

	Ended() chan struct{}
}

type PoCo struct {
	po *postingEnd
	ho *hostingEnd

	ended     chan struct{}
	respBegin chan struct{}
}

func (co *PoCo) CoID() string {
	return fmt.Sprintf("%d", uintptr(unsafe.Pointer(co)))
}

func (co *PoCo) SendCode(code string) (err error) {
	co.po.coAssertSender(co)
	_, err = co.po.wire.SendPacket(code, "")
	return
}

func (co *PoCo) SendObj(code string) (err error) {
	co.po.coAssertSender(co)
	_, err = co.po.wire.SendPacket(code, "co_recv")
	return
}

func (co *PoCo) SendData(data <-chan []byte) (err error) {
	co.po.coAssertSender(co)
	_, err = co.po.wire.SendData(data)
	return
}

func (co *PoCo) GetObj(code string) (obj interface{}, err error) {
	co.po.coAssertSender(co)
	_, err = co.po.wire.SendPacket(code, "co_send")
	obj, err = co.RecvObj()
	return
}

func (co *PoCo) RecvObj() (obj interface{}, err error) {
	if co.ho == nil {
		panic(errors.New("Receiving by a send-only conversation ?!"))
	}

	// must still be the sending conversation upon recv starting
	co.po.coAssertSender(co)

	// wait co_ack_begin from peer
	select {
	case <-co.po.Done():
		err = errors.New("posting endpoint closed")
		return
	case <-co.respBegin:
		// normal case
	}
	// must be the receiving conversation now
	co.po.coAssertReceiver(co)

	obj, err = co.ho.recvObj()
	return
}

func (co *PoCo) RecvData(data <-chan []byte) (err error) {
	if co.ho == nil {
		panic(errors.New("Receiving by a send-only conversation ?!"))
	}

	// must still be the sending conversation upon recv starting
	co.po.coAssertSender(co)

	// wait co_ack_begin from peer
	select {
	case <-co.po.Done():
		err = errors.New("posting endpoint closed")
		return
	case <-co.respBegin:
		// normal case
	}
	// must be the receiving conversation now
	co.po.coAssertReceiver(co)

	_, err = co.ho.wire.RecvData(data)
	return
}

func (co *PoCo) Close() {
	if co.po.Cancelled() {
		return
	}

	co.po.coAssertSender(co)

	if _, err := co.po.wire.SendPacket(co.CoID(), "co_end"); err != nil {
		errReason := fmt.Sprintf("%+v", errors.RichError(err))
		co.po.Disconnect(errReason, false)
		panic(err)
	}

	co.po.coEnd(co, false)
}

func (co *PoCo) Ended() chan struct{} {
	return co.ended
}

type HoCo struct {
	ho *hostingEnd

	coID string

	ended chan struct{}
}

func (co *HoCo) CoID() string {
	return co.coID
}

func (co *HoCo) SendCode(code string) (err error) {
	co.ho.po.coAssertSender(co)

	_, err = co.ho.wire.SendPacket(code, "")
	return
}

func (co *HoCo) SendObj(code string) (err error) {
	co.ho.po.coAssertSender(co)

	_, err = co.ho.wire.SendPacket(code, "co_recv")
	return
}

func (co *HoCo) SendData(data <-chan []byte) (err error) {
	co.ho.po.coAssertSender(co)

	_, err = co.ho.po.wire.SendData(data)
	return
}

func (co *HoCo) RecvObj() (result interface{}, err error) {
	co.ho.po.coAssertReceiver(co)

	result, err = co.ho.recvObj()
	return
}

func (co *HoCo) RecvData(data <-chan []byte) (err error) {
	co.ho.po.coAssertReceiver(co)

	_, err = co.ho.wire.RecvData(data)
	return
}

func (co *HoCo) Ended() chan struct{} {
	return co.ended
}
