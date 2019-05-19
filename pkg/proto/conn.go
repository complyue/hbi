package proto

import (
	"fmt"
	"io"
	"sync"

	details "github.com/complyue/hbi/pkg/_details"
	"github.com/complyue/hbi/pkg/errors"
	"github.com/golang/glog"
)

// HBIC is designed to interface with HBI wire protocol implementations,
// HBI applications should not use HBIC directly.
type HBIC struct {
	// embed a cancellable context
	CancellableContext

	po *PostingEnd
	ho *HostingEnd

	wire     HBIWire
	netIdent string

	// increamental record for non-repeating conversation ID sequence
	nextCoSeq int
	// conversation queue to serialize sending activities
	coq []*coState

	// to guard access to conversation related fields
	muCo sync.Mutex
}

func (hbic *HBIC) Po() *PostingEnd {
	return hbic.po
}

func (hbic *HBIC) Ho() *HostingEnd {
	return hbic.ho
}

func (hbic *HBIC) Wire() HBIWire {
	return hbic.wire
}

func (hbic *HBIC) NetIdent() string {
	return hbic.netIdent
}

func (hbic *HBIC) Disconnect(errReason string, trySendPeerError bool) {
	if hbic.CancellableContext.Cancelled() {
		// wire has been closed, send won't succeed
		if trySendPeerError {
			trySendPeerError = false
			if len(errReason) > 0 {
				glog.Warningf("Not sending peer error as wire has been closed: %s", errReason)
			}
		}
	} else {
		// can close wire only once
		defer hbic.wire.Disconnect()
	}

	if len(errReason) > 0 {
		// cancel or update err if already cancelled
		hbic.CancellableContext.Cancel(errors.New(errReason))
	} else if hbic.CancellableContext.Cancelled() {
		if err := hbic.CancellableContext.Err(); err != nil {
			errReason = fmt.Sprintf("cancelled due to: %+v", errors.RichError(err))
		}
	} else {
		hbic.CancellableContext.Cancel(nil)
	}

	if len(errReason) > 0 {
		glog.Errorf("HBI %s disconnecting due to error: %s", hbic.netIdent, errReason)
		if trySendPeerError {
			if _, e := hbic.wire.SendPacket(errReason, "err"); e != nil {
				glog.Warningf("Failed sending peer error %s - %+v", errReason, errors.RichError(e))
			}
		}
	}
}

func (hbic *HBIC) Cancel(err error) {
	if err == nil {
		hbic.Disconnect("", false)
	} else {
		hbic.Disconnect(fmt.Sprintf("%+v", err), true)
	}
}

func (hbic *HBIC) Close() {
	hbic.Disconnect("", false)
}

func (hbic *HBIC) sendPacket(payload, wireDir string) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = errors.RichError(e)
		} else if err != nil {
			err = errors.RichError(err)
			errReason := fmt.Sprintf("%+v", err)
			hbic.Disconnect(errReason, false)
		}
	}()
	_, err = hbic.wire.SendPacket(payload, wireDir)
	return
}

func (hbic *HBIC) sendData(d []byte) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = errors.RichError(e)
		} else if err != nil {
			err = errors.RichError(err)
			errReason := fmt.Sprintf("%+v", err)
			hbic.Disconnect(errReason, false)
		}
	}()
	_, err = hbic.wire.SendData(d)
	return
}

func (hbic *HBIC) sendStream(ds func() ([]byte, error)) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = errors.RichError(e)
		} else if err != nil {
			err = errors.RichError(err)
			errReason := fmt.Sprintf("%+v", err)
			hbic.Disconnect(errReason, false)
		}
	}()
	_, err = hbic.wire.SendStream(ds)
	return
}

func (hbic *HBIC) recvData(d []byte) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = errors.RichError(e)
		} else if err != nil {
			err = errors.RichError(err)
			errReason := fmt.Sprintf("%+v", err)
			hbic.Disconnect(errReason, false)
		}
	}()
	_, err = hbic.wire.RecvData(d)
	return
}

func (hbic *HBIC) recvStream(ds func() ([]byte, error)) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = errors.RichError(e)
		} else if err != nil {
			err = errors.RichError(err)
			errReason := fmt.Sprintf("%+v", err)
			hbic.Disconnect(errReason, false)
		}
	}()
	_, err = hbic.wire.RecvStream(ds)
	return
}

func (hbic *HBIC) NewPoCo() (co *PoCo, err error) {
	co = (*PoCo)(&coState{
		hbic: hbic, sendDone: make(chan struct{}),

		beginAcked: make(chan struct{}),
		roq:        make(chan interface{}),
		rdq:        make(chan []byte),
		rddq:       make(chan *byte),
	})

	hbic.coEnqueue((*coState)(co))

	err = hbic.sendPacket(co.CoSeq(), "co_begin")
	return
}

func (hbic *HBIC) coEnd(co *coState, popOff bool) {
	hbic.muCo.Lock()
	defer hbic.muCo.Unlock()

	ql := len(hbic.coq)
	if ql < 1 {
		panic(errors.New("coq empty"))
	}
	if co != hbic.coq[ql-1] {
		panic(errors.New("co mismatch coq tail"))
	}

	// close this conversation's ended channel, so next pending enqueued conversation
	// get the signal to procede get enqueued.
	close(co.sendDone)

	if popOff {
		hbic.coq = hbic.coq[:ql-1]
	}
}

func (hbic *HBIC) coEnqueue(co *coState) {
	hbic.muCo.Lock()
	defer hbic.muCo.Unlock()

	ql := len(hbic.coq)
	if ql > 0 {
		// wait tail co closed before a new co can be enqueued
		prevCo := hbic.coq[ql-1]

		func() {
			// release muCo during waiting for prevCo to be closed,
			// or `coEnd()` on prevCo will deadlock.
			hbic.muCo.Unlock()
			defer hbic.muCo.Lock()

			select {
			case <-hbic.Done():
				err := hbic.Err()
				if err == nil {
					err = errors.New("hbic disconnected")
				}
				panic(err)
			case <-prevCo.sendDone:
				// normal case
			}
		}()
	}

	isHoCo := len(co.coSeq) > 0
	if isHoCo {
		// creating a new ho co with coSeq received from peer
		if hbic.ho.co != nil {
			panic("unclean co_begin")
		}
	} else {
		// creating a new po co, assign a new coSeq at this endpoint
		coSeq := fmt.Sprintf("%d", hbic.nextCoSeq)
		hbic.nextCoSeq++
		if hbic.nextCoSeq > details.MaxCoSeq {
			hbic.nextCoSeq = details.MinCoSeq
		}
		co.coSeq = coSeq
	}

	hbic.coq = append(hbic.coq, co) // do enqueue

	if isHoCo { // assign here with muCo locked
		hbic.ho.co = (*HoCo)(co)
	}
}

func (hbic *HBIC) coAssertSender(co *coState) {
	hbic.muCo.Lock()
	defer hbic.muCo.Unlock()

	ql := len(hbic.coq)
	if ql < 1 {
		panic(errors.New("coq empty"))
	}
	if co != hbic.coq[ql-1] {
		panic(errors.New("co mismatch coq tail"))
	}
}

func (hbic *HBIC) coPeek(errIfNone string) (co *coState) {
	hbic.muCo.Lock()
	defer hbic.muCo.Unlock()

	ql := len(hbic.coq)
	if ql < 1 {
		panic(errIfNone)
	}

	co = hbic.coq[0]
	return
}

func (hbic *HBIC) coAssertReceiver(co *coState) {
	hbic.muCo.Lock()
	defer hbic.muCo.Unlock()

	ql := len(hbic.coq)
	if ql < 1 {
		panic(errors.New("coq empty"))
	}
	if co != hbic.coq[0] {
		panic(errors.New("co mismatch coq head"))
	}
}

func (hbic *HBIC) coDequeue() (co *coState) {
	hbic.muCo.Lock()
	defer hbic.muCo.Unlock()

	ql := len(hbic.coq)
	if ql < 1 {
		panic("coq empty")
	}

	co = hbic.coq[0]
	hbic.coq = hbic.coq[1:]
	return
}

// NewConnection creates the posting & hosting endpoints from a transport wire with a hosting environment
func NewConnection(wire HBIWire, env *HostingEnv) (*PostingEnd, *HostingEnd, error) {
	hbic := &HBIC{
		CancellableContext: NewCancellableContext(),

		wire:     wire,
		netIdent: wire.NetIdent(),

		nextCoSeq: details.MinCoSeq,
	}
	po := &PostingEnd{
		hbic: hbic,

		remoteAddr: wire.RemoteAddr(),
	}
	ho := &HostingEnd{
		hbic: hbic,
		env:  env,

		localAddr: wire.LocalAddr(),
	}
	hbic.po, hbic.ho = po, ho
	env.po, env.ho = po, ho

	initDone := make(chan error)
	// run the landing thread in a dedicated goroutine
	go hbic.landingThread(initDone)

	if err, ok := <-initDone; ok && err != nil {
		return nil, nil, err
	}

	return po, ho, nil
}

func (hbic *HBIC) landingThread(initDone chan<- error) {
	var (
		err error
		ok  bool

		wire   = hbic.wire
		po, ho = hbic.po, hbic.ho
		env    = ho.env

		pkt *Packet

		discReason       string
		trySendPeerError = true

		initFunc    InitMagicFunction
		cleanupFunc CleanupMagicFunction
	)

	defer func() {
		if e := recover(); e != nil {
			err = errors.RichError(e)
		} else if err != nil {
			err = errors.RichError(err)
		}

		if len(discReason) > 0 {
			if err == nil {
				err = errors.New(discReason)
			}
		} else if err != nil {
			discReason = fmt.Sprintf("landing error: %+v", err)
		}

		// finally disconnect wire on landing thread terminated
		hbic.Disconnect(discReason, trySendPeerError)

		if len(discReason) > 0 {
			glog.Errorf("Last HBI packet (possibly responsible for failure): %+v", pkt)
		}

		if cleanupFunc != nil { // call cleanup callback after disconnected
			func() {
				defer func() {
					if e := recover(); e != nil {
						glog.Warningf("HBIC %s cleanup callback failure ignored: %+v",
							hbic.netIdent, errors.RichError(e))
					}
				}()

				cleanupFunc(discReason)
			}()
		}
	}()

	func() {
		defer func() {
			if e := recover(); e != nil {
				err = errors.RichError(e)
			} else if err != nil {
				err = errors.RichError(err)
			}

			if len(discReason) > 0 {
				if err == nil {
					err = errors.New(discReason)
				}
			} else if err != nil {
				discReason = fmt.Sprintf("%+v", err)
			}

			if err != nil {
				// send the error to `NewConnection()`
				initDone <- err
			}
			// close `initDone` in all cases after init,
			// for `NewConnection()` to return as expected.
			close(initDone)
		}()

		if initMagic := env.Get("__hbi_init__"); initMagic != nil {
			if initFunc, ok = initMagic.(InitMagicFunction); !ok {
				discReason = fmt.Sprintf("Bad __hbi_init__() type: %T", initMagic)
				return
			}
		}

		if cleanupMagic := env.Get("__hbi_cleanup__"); cleanupMagic != nil {
			if cleanupFunc, ok = cleanupMagic.(CleanupMagicFunction); !ok {
				discReason = fmt.Sprintf("Bad __hbi_cleanup__() type: %T", cleanupMagic)
				return
			}
		}

		if initFunc == nil {
			return
		}

		func() {
			defer func() {
				if e := recover(); e != nil {
					err = errors.RichError(e)
					discReason = fmt.Sprintf("init callback failed: %+v", err)
				}
			}()

			initFunc(po, ho)
		}()
	}()

	// terminate if init failed
	if err != nil || len(discReason) > 0 {
		return
	}

	for !hbic.Cancelled() {
		if err != nil || len(discReason) > 0 {
			panic("?!")
		}

		pkt, err = wire.RecvPacket()
		if err != nil {
			if err == io.EOF {
				// wire disconnected by peer, break landing loop
				err = nil // not considered an error
			} else if hbic.CancellableContext.Cancelled() {
				// active disconnection caused wire reading
				err = nil // not considered an error
			}
			return
		}
		var result interface{}

		switch pkt.WireDir {
		case "co_begin":

			co := &coState{
				hbic: hbic, sendDone: make(chan struct{}),
				coSeq: pkt.Payload,
			}
			hbic.coEnqueue((*coState)(co)) // ho.co is assigned inside with muCo locked

			if _, err = hbic.wire.SendPacket(pkt.Payload, "co_ack_begin"); err != nil {
				trySendPeerError = false
				return
			}

		case "":

			// peer is pushing the textual code for side-effect of its landing

			if _, err = env.RunInEnv(pkt.Payload, hbic); err != nil {
				return
			}

		case "co_send":

			// peer is requesting this end to push landed result (in textual code) back

			if result, err = env.RunInEnv(pkt.Payload, hbic); err != nil {
				return
			}
			if _, err = wire.SendPacket(fmt.Sprintf("%#v", result), "co_recv"); err != nil {
				trySendPeerError = false
				return
			}

		case "co_recv":

			// `ho.co` is always assigned from this landing thread, no sync necessary to read it
			if ho.co != nil { // pushing obj to a ho co
				discReason = "co_recv without priori receiving code in landing"
				return
			}

			// no ho co means the sent object meant to be received by a po co

			// nor a po co to recv the pushed obj if coq empty
			recvCo := hbic.coPeek("no conversation to receive object")

			// pushing obj to a po co
			if result, err = env.RunInEnv(pkt.Payload, hbic); err != nil {
				return
			}
			recvCo.roq <- result

		case "co_end":

			hbic.muCo.Lock()
			co := ho.co
			ho.co = nil
			hbic.muCo.Unlock()

			if co == nil || co.coSeq != pkt.Payload {
				panic("ho co mismatch")
			}
			hbic.coAssertSender((*coState)(co))

			hbic.coEnd((*coState)(co), true)

			if _, err = hbic.wire.SendPacket(pkt.Payload, "co_ack_end"); err != nil {
				trySendPeerError = false
				return
			}

		case "co_ack_begin":

			recvCo := hbic.coPeek("po co lost")
			if recvCo.coSeq != pkt.Payload {
				panic("mismatch co_seq")
			}

			close(recvCo.beginAcked)

		case "po_data":

			// `ho.co` is always assigned from this landing thread, no sync necessary to read it
			if ho.co != nil { // pushing data/stream to a ho co
				discReason = "po_data to a ho co"
				return
			}

			// no ho co means the sent data/stream meant to be received by a po co

			// nor a po co to recv the pushed data if coq empty
			recvCo := hbic.coPeek("no po co to receive data")

			// pump bufs from a po co, receive the data/stream into each of them
			var pd *byte
			hbic.recvStream(func() ([]byte, error) {
				if pd != nil {
					select {
					case <-hbic.Done():
						if err == nil {
							err = errors.New("disconnected while receiving data")
						}
						return nil, err
					case recvCo.rddq <- pd:
						// normal case
					}
					pd = nil
				}
				select {
				case <-hbic.Done():
					if err == nil {
						err = errors.New("disconnected while receiving data")
					}
					return nil, err
				case d, ok := <-recvCo.rdq:
					if ok {
						if d != nil {
							pd = &d[0]
						}
						return d, nil
					}
				}
				return nil, nil
			})

		case "co_ack_end":

			recvCo := hbic.coDequeue()
			if recvCo.coSeq != pkt.Payload {
				panic("mismatch co_seq")
			}

			close(recvCo.roq)
			close(recvCo.rdq)

		case "err":

			discReason = fmt.Sprintf("peer error: %s", pkt.Payload)
			trySendPeerError = false
			return

		default:

			discReason = fmt.Sprintf("HBIC unexpected packet: %+v", pkt)
			return

		}
	}
}

func (hbic *HBIC) recvOneObj() (obj interface{}, err error) {
	var (
		wire = hbic.wire
		env  = hbic.ho.env

		pkt *Packet

		discReason       string
		trySendPeerError = true
	)

	defer func() {
		if e := recover(); e != nil {
			err = errors.RichError(e)
		} else if err != nil {
			err = errors.RichError(err)
		}

		if len(discReason) > 0 {
			if err == nil {
				err = errors.New(discReason)
			}
		} else if err != nil {
			discReason = fmt.Sprintf("recv landing error: %+v", err)
		}

		if len(discReason) > 0 {
			hbic.Disconnect(discReason, trySendPeerError)
		}
	}()

	for {
		if err != nil || len(discReason) > 0 {
			panic("?!")
		}

		pkt, err = wire.RecvPacket()
		if err != nil {
			return
		}

		switch pkt.WireDir {
		case "co_recv":

			// the very expected packet

			obj, err = env.RunInEnv(pkt.Payload, hbic)
			return

		case "":

			// some code to execute preceding code for obj to be received.
			// todo this harmful and be explicitly disallowed ?

			if _, err = env.RunInEnv(pkt.Payload, hbic); err != nil {
				return
			}

		case "err":

			discReason = fmt.Sprintf("peer error: %s", pkt.Payload)
			trySendPeerError = false
			return

		case "co_send":

			discReason = "issued co_send before sending an object expected by prior receiving-code"
			return

		default:

			discReason = fmt.Sprintf("HBIC unexpected packet: %+v", pkt)
			return

		}
	}
}
