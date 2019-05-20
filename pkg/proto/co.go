package proto

import (
	"github.com/complyue/hbi/pkg/errors"
)

// Conver is the application programming interface of a conversation, common to PoCo and HoCo.
//
// There're 2 types of conversation:
//  * the active, posting conversation
//    which is created by application by calling PostingEnd.Po()
//  * the passive, hosting conversation
//    which is automatically available to application, and obtained by calling HostingEnv.Co()
type Conver interface {
	// CoSeq returns the sequence number of this conversation.
	//
	// The sequence number of a posting conversation is assigned by the posting endpoint created it,
	// the number value does not necessarily be unique within a long time period, but won't repeat
	// among millons of conversations per sent over a wire in line.
	//
	// The sequence of a hosting conversation is always the same as the posting conversation's that
	// triggered it.
	CoSeq() string

	SendCode(code string) error
	SendObj(code string) error
	SendData(d []byte) error
	SendStream(ds func() ([]byte, error)) error

	RecvObj() (obj interface{}, err error)
	RecvData(d []byte) error
	RecvStream(ds func() ([]byte, error)) error

	// Closed returns a channel that is closed when all out sending works through this
	// conversation has been done.
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
	rddq       chan *byte
}

// PoCo is the active, posting conversation.
//
// A PoCo is automatically available to application, and obtained by calling HostingEnv.Co()
type PoCo coState

// CoSeq returns the sequence number of this conversation.
func (co *PoCo) CoSeq() string {
	return co.coSeq
}

// SendCode sends `code` to peer's hosting endpoint for landing by its hosting environment.
//
// Only side effects are expected from landing of `code` at peer site.
func (co *PoCo) SendCode(code string) error {
	co.hbic.coAssertSender((*coState)(co))

	return co.hbic.sendPacket(code, "")
}

// SendObj sends `code` to peer's hosting endpoint for landing by its hosting environment.
//
// The respective hosting conversation at peer site is expected to receive the result value
// from landing of `code`, by calling Ho().RecvObj()
func (co *PoCo) SendObj(code string) error {
	co.hbic.coAssertSender((*coState)(co))

	return co.hbic.sendPacket(code, "co_recv")
}

// SendData sends a single chunk of binary data to peer site.
//
// The respective hosting conversation at peer site is expected to receive the data by
// calling Ho().RecvData() or Ho().RecvStream()
func (co *PoCo) SendData(d []byte) error {
	co.hbic.coAssertSender((*coState)(co))

	return co.hbic.sendData(d)
}

// SendStream polls callback function `ds()` until it returns a nil []byte or non-nil error,
// and send each chunk to peer site in line. `ds()` will be called another time after the
// chunk returned from the previous call has been sent out.
//
// The respective hosting conversation at peer site is expected to receive the data by
// calling Ho().RecvData() or Ho().RecvStream()
func (co *PoCo) SendStream(ds func() ([]byte, error)) error {
	co.hbic.coAssertSender((*coState)(co))

	return co.hbic.sendStream(ds)
}

// RecvObj of a posting conversation receives the landing result of a piece of `code` sent
// by its respective hosting conversation via HoCo().SendObj(code)
//
// You would at best not receive through a posting conversation, but if you do, ONLY call
// this after the posting conversation's `Close()` has been called, i.e. the posting
// conversation has entered into `after-posting stage`.
//
// If this is called before the posting conversation is closed, i.e. still in its
// `posting stage`, the underlying wire is pended to wait the RTT, thus HARM A LOT to overall
// throughput.
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

// RecvData of a posting conversation receives the binary data sent by its respective
// hosting conversation via HoCo().SendData() or HoCo().SendStream()
//
// You would at best not receive through a posting conversation, but if you do, ONLY call
// this after the posting conversation's `Close()` has been called, i.e. the posting
// conversation has entered into `after-posting stage`.
//
// If this is called before the posting conversation is closed, i.e. still in its
// `posting stage`, the underlying wire is pended to wait the RTT, thus HARM A LOT to overall
// throughput.
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

	select {
	case <-co.hbic.Done():
		return errors.New("disconnected")
	case pd := <-co.rddq:
		// normal case
		if pd != &d[0] {
			panic("?!")
		}
	}

	return nil
}

// RecvStream of a posting conversation receives the binary data sent by its respective
// hosting conversation via HoCo().SendData() or HoCo().SendStream()
//
// You would at best not receive through a posting conversation, but if you do, ONLY call
// this after the posting conversation's `Close()` has been called, i.e. the posting
// conversation has entered into `after-posting stage`.
//
// If this is called before the posting conversation is closed, i.e. still in its
// `posting stage`, the underlying wire is pended to wait the RTT, thus HARM A LOT to overall
// throughput.
func (co *PoCo) RecvStream(ds func() ([]byte, error)) error {
	// wait co_ack_begin from peer
	select {
	case <-co.hbic.Done():
		return errors.New("disconnected")
	case <-co.beginAcked:
		// normal case
	}
	// must be the receiving conversation now
	co.hbic.coAssertReceiver((*coState)(co))

	for {
		d, err := ds()
		if err != nil {
			return err
		}

		select {
		case <-co.hbic.Done():
			return errors.New("disconnected")
		case co.rdq <- d:
			// normal case
		}

		if d == nil {
			// all data received
			break
		}

		select {
		case <-co.hbic.Done():
			return errors.New("disconnected")
		case pd := <-co.rddq:
			// normal case
			if pd != &d[0] {
				panic("?!")
			}
		}
	}

	return nil
}

// Close closes this posting conversation to transit it from `posting stage` to
// `after-posting stage`, and no more out sending can happen with this conversation.
//
// Once closed, the underlying wire is released for other posting conversation to start off,
// to maintain the wire in pipelined efficiency.
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

// Closed returns the channel that is closed when `Close()` of this posting conversation
// is called.
func (co *PoCo) Closed() chan struct{} {
	return co.sendDone
}

// HoCo is the passive, hosting conversation
type HoCo coState

// CoSeq returns the sequence number of this conversation.
func (co *HoCo) CoSeq() string {
	return co.coSeq
}

// SendCode sends `code` to peer's hosting endpoint for landing by its hosting environment.
//
// Only side effects are expected from landing of `code` at peer site.
func (co *HoCo) SendCode(code string) error {
	if co != co.hbic.ho.co {
		panic("send with ended ho co")
	}
	co.hbic.coAssertSender((*coState)(co))

	return co.hbic.sendPacket(code, "")
}

// SendObj sends `code` to peer's hosting endpoint for landing by its hosting environment.
//
// The originating posting conversation at peer site is expected to receive the result value
// from landing of `code`, by calling Po().RecvObj()
func (co *HoCo) SendObj(code string) error {
	if co != co.hbic.ho.co {
		panic("send with ended ho co")
	}
	co.hbic.coAssertSender((*coState)(co))

	return co.hbic.sendPacket(code, "co_recv")
}

// SendData sends a single chunk of binary data to peer site.
//
// The originating posting conversation at peer site is expected to receive the data by
// calling Po().RecvData() or Po().RecvStream()
func (co *HoCo) SendData(d []byte) error {
	if co != co.hbic.ho.co {
		panic("send with ended ho co")
	}
	co.hbic.coAssertSender((*coState)(co))

	if err := co.hbic.sendPacket(co.coSeq, "po_data"); err != nil {
		return err
	}
	return co.hbic.sendData(d)
}

// SendStream polls callback function `ds()` until it returns a nil []byte or non-nil error,
// and send each chunk to peer site in line. `ds()` will be called another time after the
// chunk returned from the previous call has been sent out.
//
// The originating posting conversation at peer site is expected to receive the data by
// calling Po().RecvData() or Po().RecvStream()
func (co *HoCo) SendStream(ds func() ([]byte, error)) error {
	if co != co.hbic.ho.co {
		panic("send with ended ho co")
	}
	co.hbic.coAssertSender((*coState)(co))

	if err := co.hbic.sendPacket(co.coSeq, "po_data"); err != nil {
		return err
	}
	return co.hbic.sendStream(ds)
}

// RecvObj of a hosting conversation receives the landing result of a piece of `code` sent
// by its originating posting conversation via PoCo().SendObj(code)
func (co *HoCo) RecvObj() (interface{}, error) {
	if co != co.hbic.ho.co {
		panic("recv with ended ho co")
	}

	return co.hbic.recvOneObj()
}

// RecvData of a hosting conversation receives the binary data sent by its originating
// posting conversation via PoCo().SendData() or PoCo().SendStream()
func (co *HoCo) RecvData(d []byte) error {
	if co != co.hbic.ho.co {
		panic("recv with ended ho co")
	}

	return co.hbic.recvData(d)
}

// RecvStream of a hosting conversation receives the binary data sent by its originating
// posting conversation via PoCo().SendData() or PoCo().SendStream()
func (co *HoCo) RecvStream(ds func() ([]byte, error)) error {
	if co != co.hbic.ho.co {
		panic("recv with ended ho co")
	}

	return co.hbic.recvStream(ds)
}

// Closed of a hosting conversation returns the channel that is closed when all packets from
// its originating posting conversation have been landed, and all binary data streams have
// been received.
//
// This is technically marked by a packet with `co_end` wire directive received at the
func (co *HoCo) Closed() chan struct{} {
	return co.sendDone
}
