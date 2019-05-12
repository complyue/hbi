package proto

import (
	"fmt"
	"net"
	"sync"

	details "github.com/complyue/hbi/pkg/_details"
	"github.com/complyue/hbi/pkg/errors"
	"github.com/golang/glog"
)

type PostingEnd interface {
	// be a cancellable context
	CancellableContext

	NetIdent() string
	RemoteAddr() net.Addr

	Co() (co *PoCo, err error)

	Notif(code string) (err error)
	NotifData(code string, buf []byte) (err error)

	Disconnect(errReason string, trySendPeerError bool)

	Close()
}

type postingEnd struct {
	// embed a cancellable context
	CancellableContext

	ho   *hostingEnd
	wire HBIWire

	netIdent   string
	remoteAddr net.Addr

	// increamental record for non-repeating conversation ID sequence
	nextCoSeq int
	// conversation queue to serialize sending activities
	coq []coState
	// to guard access to `nextCoSeq/coq`
	muCoq sync.Mutex
}

func (po *postingEnd) coEnd(co *coState, popOff bool) {
	po.muCoq.Lock()
	defer po.muCoq.Unlock()

	ql := len(po.coq)
	if ql < 1 {
		panic(errors.New("coq empty"))
	}
	if co != &po.coq[ql-1] {
		panic(errors.New("co mismatch coq tail"))
	}

	// close this conversation's ended channel, so next pending enqueued conversation
	// get the signal to procede get enqueued.
	close(co.ended)

	if popOff {
		po.coq = po.coq[:ql-1]
	}
}

func (po *postingEnd) coEnqueue(coSeq string) (co *coState) {
	po.muCoq.Lock()
	defer po.muCoq.Unlock()

	ql := len(po.coq)
	if ql > 0 {
		// wait tail co ended (i.e. finished all sending work) before
		// a new co can be enqueued.
		prevCo := po.coq[ql-1]

		func() {
			// release muCoq during waiting for prevCo to end,
			// or `coEnd()` on prevCo will deadlock.
			po.muCoq.Unlock()
			defer po.muCoq.Lock()

			select {
			case <-po.Done():
				err := po.Err()
				if err == nil {
					err = errors.New("po cancelled")
				}
				panic(err)
			case <-prevCo.ended:
				// normal case
			}
		}()
	}

	// these'll be kept nil for ho co, only assigned for po co
	var (
		respBegan chan struct{}
		respObj   chan interface{}
	)

	if len(coSeq) > 0 {
		// creating a new ho co with coSeq received from peer
	} else {
		// creating a new po co, assign a new coSeq at this endpoint
		coSeq = fmt.Sprintf("%d", po.nextCoSeq)
		po.nextCoSeq++
		if po.nextCoSeq > details.MaxCoSeq {
			po.nextCoSeq = details.MinCoSeq
		}
		// these are only used for a po co
		respBegan = make(chan struct{})
		respObj = make(chan interface{})
	}

	po.coq = append(po.coq, coState{
		// common fields
		ho: po.ho, coSeq: coSeq, ended: make(chan struct{}),
		// po co only fields
		respBegan: respBegan, respObj: respObj,
	})
	co = &po.coq[len(po.coq)-1] // do NOT use `ql` here, that's outdated if last co is a ho co

	return
}

func (po *postingEnd) coAssertSender(co *coState) {
	// no sync, use thread local cache should be fairly okay
	ql := len(po.coq)
	if ql < 1 {
		panic(errors.New("coq empty"))
	}
	if co != &po.coq[ql-1] {
		panic(errors.New("co mismatch coq tail"))
	}
}

func (po *postingEnd) coPeek() (co *coState) {
	po.muCoq.Lock()
	defer po.muCoq.Unlock()

	ql := len(po.coq)
	if ql < 1 {
		panic("coq empty")
	}

	co = &po.coq[0]
	return
}

func (po *postingEnd) coAssertReceiver(co *coState) {
	// no sync, use thread local cache should be fairly okay
	ql := len(po.coq)
	if ql < 1 {
		panic(errors.New("coq empty"))
	}
	if co != &po.coq[0] {
		panic(errors.New("co mismatch coq head"))
	}
}

func (po *postingEnd) coDequeue() (co *coState) {
	po.muCoq.Lock()
	defer po.muCoq.Unlock()

	ql := len(po.coq)
	if ql < 1 {
		panic("coq empty")
	}

	co = &po.coq[0]
	po.coq = po.coq[1:]
	return
}

func (po *postingEnd) Co() (co *PoCo, err error) {
	co = (*PoCo)(po.coEnqueue(""))

	_, err = po.wire.SendPacket(co.CoSeq(), "co_begin")
	if err != nil {
		errReason := fmt.Sprintf("%+v", errors.RichError(err))
		po.Disconnect(errReason, false)
	}
	return
}
func (po *postingEnd) NetIdent() string {
	return po.netIdent
}

func (po *postingEnd) RemoteAddr() net.Addr {
	return po.remoteAddr
}

func (po *postingEnd) Notif(code string) (err error) {
	var co *PoCo
	if co, err = po.Co(); err != nil {
		errReason := fmt.Sprintf("%+v", errors.RichError(err))
		po.Disconnect(errReason, false)
		return
	}
	defer co.Close()

	if _, err = po.wire.SendPacket(code, ""); err != nil {
		errReason := fmt.Sprintf("%+v", errors.RichError(err))
		po.Disconnect(errReason, false)
		return
	}
	return
}

func (po *postingEnd) NotifData(code string, buf []byte) (err error) {
	var co *PoCo
	if co, err = po.Co(); err != nil {
		errReason := fmt.Sprintf("%+v", errors.RichError(err))
		po.Disconnect(errReason, false)
		return
	}
	defer co.Close()

	if _, err = po.wire.SendPacket(code, ""); err != nil {
		errReason := fmt.Sprintf("%+v", errors.RichError(err))
		po.Disconnect(errReason, false)
		return
	}
	if _, err = po.wire.SendData(buf); err != nil {
		errReason := fmt.Sprintf("%+v", errors.RichError(err))
		po.Disconnect(errReason, false)
		return
	}
	return
}

func (po *postingEnd) Disconnect(errReason string, trySendPeerError bool) {
	defer po.wire.Disconnect()

	if len(errReason) > 0 {
		if !po.CancellableContext.Cancelled() {
			po.CancellableContext.Cancel(errors.New(errReason))
		}
	} else if po.CancellableContext.Cancelled() {
		if err := po.CancellableContext.Err(); err != nil {
			errReason = fmt.Sprintf("Posting cancelled due to: %+v", errors.RichError(err))
		}
	} else {
		po.CancellableContext.Cancel(nil)
	}

	if len(errReason) > 0 {
		glog.Errorf("HBI %s disconnecting due to error: %s", po.netIdent, errReason)
		if trySendPeerError {
			if _, e := po.wire.SendPacket(errReason, "err"); e != nil {
				glog.Warningf("Failed sending peer error %s - %+v", errReason, errors.RichError(e))
			}
		}
	}
}

func (po *postingEnd) Close() {
	po.Disconnect("", false)
}
