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

	// increamental record for conversation sequence numbers
	nextCoSeq int
	// pending posting conversations
	ppc map[string]*PoCo

	// current sending conversation
	sender interface{}
	// to guard access to sender related fields
	sendMutex sync.Mutex

	// current receiving conversation
	recver interface{}
	// to guard access to recver related fields
	recvMutex sync.Mutex
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

func (hbic *HBIC) hoCoStartSend(co *HoCo) error {
	hbic.sendMutex.Lock()
	defer hbic.sendMutex.Unlock()

	// wait current sender done
	var sendDone chan struct{}
	sender := hbic.sender
	for sender != nil {
		switch senderCo := sender.(type) {
		case *PoCo:
			sendDone = senderCo.sendDone
		case *HoCo:
			sendDone = senderCo.sendDone
		default:
			panic("unexpected sender type")
		}

		if sendDone == nil {
			panic("inplace sender sendDone cleared ?!")
		}

		if err := func() error {
			// release sendMutex during waiting for send done, or it's deadlock
			hbic.sendMutex.Unlock()
			defer hbic.sendMutex.Lock()

			select {
			case <-hbic.Done():
				// disconnected already
				err := hbic.Err()
				return err
			case <-sendDone:
				// normal case
			}

			return nil
		}(); err != nil {
			return err
		}

		// locked sendMutex again, if current sender is cleared, this goroutine has won the race
		// to set a new sender
		sender = hbic.sender
	}

	if _, err := hbic.wire.SendPacket(co.coSeq, "co_ack_begin"); err != nil {
		return err
	}

	hbic.sender = co

	return nil
}

func (hbic *HBIC) recverHoCo() *HoCo {
	hbic.recvMutex.Lock()
	defer hbic.recvMutex.Unlock()

	if hoCo, ok := hbic.recver.(*HoCo); ok {
		return hoCo
	}

	return nil
}

func (hbic *HBIC) newPoCo() (co *PoCo, err error) {
	hbic.sendMutex.Lock()
	defer hbic.sendMutex.Unlock()

	// wait current sender done
	var sendDone chan struct{}
	sender := hbic.sender
	for sender != nil {
		switch senderCo := sender.(type) {
		case *PoCo:
			sendDone = senderCo.sendDone
		case *HoCo:
			sendDone = senderCo.sendDone
		default:
			panic("unexpected sender type")
		}

		if sendDone == nil {
			panic("inplace sender sendDone cleared ?!")
		}

		if err = func() error {
			// release sendMutex during waiting for send done, or it's deadlock
			hbic.sendMutex.Unlock()
			defer hbic.sendMutex.Lock()

			select {
			case <-hbic.Done():
				// disconnected already
				err := hbic.Err()
				if err == nil {
					err = errors.New("hbic disconnected")
				}
				return err
			case <-sendDone:
				// normal case
			}

			return nil
		}(); err != nil {
			return
		}

		// locked sendMutex again, if current sender is cleared, this goroutine has won the race
		// to set a new sender
		sender = hbic.sender
	}

	var coSeq string
	for { // find a co seq unique among current pending ones
		coSeq = fmt.Sprintf("%d", hbic.nextCoSeq)
		hbic.nextCoSeq++
		if hbic.nextCoSeq > details.MaxCoSeq {
			hbic.nextCoSeq = details.MinCoSeq
		}
		if _, ok := hbic.ppc[coSeq]; !ok {
			break // the new coSeq must not be occupied by a pending co
		}
	}

	co = &PoCo{
		hbic: hbic, coSeq: coSeq,
		sendDone:   make(chan struct{}),
		recvDone:   nil, // not sure to do receiving, will be set by StartRecv()
		beginAcked: make(chan struct{}),
		endAcked:   make(chan struct{}),
	}
	hbic.ppc[coSeq] = co
	hbic.sender = co

	err = hbic.sendPacket(co.CoSeq(), "co_begin")
	return
}

func (hbic *HBIC) poCoFinishSend(co *PoCo, closeAsWell bool) error {
	hbic.sendMutex.Lock()
	defer hbic.sendMutex.Unlock()

	if closeAsWell {
		if co != hbic.sender {
			panic(errors.New("po co not sender"))
		}
	}

	if err := hbic.sendPacket(co.coSeq, "co_end"); err != nil {
		return err
	}

	close(co.sendDone)
	co.sendDone = nil

	hbic.sender = nil

	if !closeAsWell {
		co.recvDone = make(chan struct{})
	}

	return nil
}

// NewConnection creates the posting & hosting endpoints from a transport wire with a hosting environment
func NewConnection(wire HBIWire, env *HostingEnv) (*PostingEnd, *HostingEnd, error) {
	hbic := &HBIC{
		CancellableContext: NewCancellableContext(),

		wire:     wire,
		netIdent: wire.NetIdent(),

		nextCoSeq: details.MinCoSeq,
		ppc:       make(map[string]*PoCo),
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

	// run the conversation keeper in a dedicated goroutine
	go hbic.coKeeper(initDone)

	if err, ok := <-initDone; ok && err != nil {
		return nil, nil, err
	}

	return po, ho, nil
}

func (hbic *HBIC) coKeeper(initDone chan<- error) {
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
			} else {
				glog.Errorf("Detail error for disconnection: %+v", err)
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
				} else {
					glog.Errorf("Detail error for disconnection: %+v", err)
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

	hbic.recvMutex.Lock()
	defer hbic.recvMutex.Unlock()

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

		switch pkt.WireDir {
		case "co_begin":

			// start a new hosting conversation to accommodate peer's posting conversation

			recvDone := make(chan struct{})
			hoCo := &HoCo{
				hbic: hbic, coSeq: pkt.Payload,
				recvDone: recvDone,
				sendDone: make(chan struct{}),
			}

			hbic.recver = hoCo

			// start the hosting thread
			go hoCo.hostingThread()

			func() {
				// wait ho co done receiving with recvMutex unlocked
				hbic.recvMutex.Unlock()
				defer hbic.recvMutex.Lock()

				select {
				case <-hbic.Done():
					err = hbic.Err()
					return
				case <-recvDone:
					// ho co done receiving
				}
			}()

			// locked recvMutex again, clear recver
			hbic.recver = nil

		case "co_ack_begin":

			// direct response on wire to the respective local posting conversation

			poCo, ok := hbic.ppc[pkt.Payload]
			if !ok {
				panic("lost po co to ack ?!")
			}

			delete(hbic.ppc, poCo.coSeq)

			hbic.recver = poCo

			close(poCo.beginAcked)

			recvDone := poCo.recvDone
			if recvDone != nil {
				// po co intends to recv
				func() {
					// wait po co done receiving with recvMutex unlocked
					hbic.recvMutex.Unlock()
					defer hbic.recvMutex.Lock()

					select {
					case <-hbic.Done():
						err = hbic.Err()
						return
					case <-recvDone:
						// po co done receiving
					}
				}()
			} else {
				// po co does not intend to recv
				hbic.recver = nil
			}

		case "co_ack_end":

			// end of response to local po co

			if hbic.recver == nil {
				// co_ack_end without co_ack_begin

				poCo, ok := hbic.ppc[pkt.Payload]
				if !ok {
					panic("lost po co to ack ?!")
				}

				delete(hbic.ppc, poCo.coSeq)

				close(poCo.endAcked)

			} else {

				// co_ack_end after co_ack_begin

				poCo, ok := hbic.recver.(*PoCo)
				if !ok {
					panic("?!")
				}
				if poCo.coSeq != pkt.Payload {
					panic("co seq mismatch on co_ack_end ?!")
				}

				close(poCo.endAcked)

				hbic.recver = nil

			}

			continue

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

		if hbic.Cancelled() {
			err = hbic.Err()
			return
		}

		pkt, err = wire.RecvPacket()
		if err != nil {
			return
		}

		switch pkt.WireDir {
		case "co_recv":

			// the very expected packet

			obj, err = env.RunInEnv(hbic, pkt.Payload)
			return

		case "":

			// some code to execute preceding code for obj to be received.
			// todo this harmful and be explicitly disallowed ?

			if _, err = env.RunInEnv(hbic, pkt.Payload); err != nil {
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
