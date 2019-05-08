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

	Co(ho HostingEnd) (co *PoCo, err error)

	Notif(code string) (err error)
	NotifData(code string, data <-chan []byte) (err error)

	Disconnect(errReason string, trySendPeerError bool)

	Close()
}

func NewPostingEnd(wire details.HBIWire) PostingEnd {
	po := &postingEnd{
		CancellableContext: NewCancellableContext(),

		wire:       wire,
		netIdent:   wire.NetIdent(),
		remoteAddr: wire.RemoteAddr(),
	}
	return po
}

type postingEnd struct {
	// embed a cancellable context
	CancellableContext

	wire details.HBIWire

	netIdent   string
	remoteAddr net.Addr

	// conversation queue to serialize sending activities
	coq []Conver
	// to guard access to `coq`
	muCoq sync.Mutex
}

func (po *postingEnd) coEnd(co Conver, popOff bool) {
	po.muCoq.Lock()
	defer po.muCoq.Unlock()

	ql := len(po.coq)
	if ql < 1 {
		panic(errors.New("coq empty"))
	}
	if co != po.coq[ql-1] {
		panic(errors.New("co mismatch coq tail"))
	}

	// close this conversation's ended channel, so next pending enqueued conversation
	// get the signal to procede get enqueued.
	close(co.Ended())

	if popOff {
		po.coq = po.coq[0 : ql-1]
	}
}

func (po *postingEnd) coEnqueue(co Conver) {
	po.muCoq.Lock()
	defer po.muCoq.Unlock()

	ql := len(po.coq)
	if ql > 0 {
		prevCo := po.coq[ql-1]
		select {
		case <-po.Done():
			err := po.Err()
			if err == nil {
				err = errors.New("po cancelled")
			}
			panic(err)
		case <-prevCo.Ended():
			// normal case
		}
	}

	po.coq = append(po.coq, co)
}

func (po *postingEnd) coAssertSender(co Conver) {
	// no sync, use thread local cache should be fairly okay
	ql := len(po.coq)
	if ql < 1 {
		panic(errors.New("coq empty"))
	}
	if co != po.coq[ql-1] {
		panic(errors.New("co mismatch coq tail"))
	}
}

func (po *postingEnd) coPeek() (co Conver) {
	po.muCoq.Lock()
	defer po.muCoq.Unlock()

	ql := len(po.coq)
	if ql < 1 {
		panic("coq empty")
	}

	co = po.coq[0]
	return
}

func (po *postingEnd) coAssertReceiver(co Conver) {
	// no sync, use thread local cache should be fairly okay
	ql := len(po.coq)
	if ql < 1 {
		panic(errors.New("coq empty"))
	}
	if co != po.coq[0] {
		panic(errors.New("co mismatch coq head"))
	}
}

func (po *postingEnd) coDequeue() (co Conver) {
	po.muCoq.Lock()
	defer po.muCoq.Unlock()

	ql := len(po.coq)
	if ql < 1 {
		panic("coq empty")
	}

	co = po.coq[0]
	po.coq = po.coq[1:]
	return
}

func (po *postingEnd) Co(ho HostingEnd) (co *PoCo, err error) {
	co = &PoCo{
		po:        po,
		ho:        ho.(*hostingEnd),
		ended:     make(chan struct{}),
		respBegin: make(chan struct{}),
	}
	po.coEnqueue(co)

	_, err = po.wire.SendPacket(co.CoID(), "co_begin")
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
	if co, err = po.Co(nil); err != nil {
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

func (po *postingEnd) NotifData(code string, data <-chan []byte) (err error) {
	var co *PoCo
	if co, err = po.Co(nil); err != nil {
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
	if _, err = po.wire.SendData(data); err != nil {
		errReason := fmt.Sprintf("%+v", errors.RichError(err))
		po.Disconnect(errReason, false)
		return
	}
	return
}

func (po *postingEnd) Cancel(err error) {
	if po.CancellableContext.Cancelled() {
		// already cancelled
		return
	}

	var errReason string
	if err != nil {
		errReason = fmt.Sprintf("%+v", errors.RichError(err))
	}

	po.Disconnect(errReason, true)
}

func (po *postingEnd) Disconnect(errReason string, trySendPeerError bool) {
	defer po.wire.Disconnect()

	alreadyCancelled := po.CancellableContext.Cancelled()
	alreadyErr := po.Err()
	po.CancellableContext.Cancel(errors.New(errReason))

	if errReason != "" {
		glog.Errorf("HBI %s disconnecting due to error: %s", po.netIdent, errReason)
		if trySendPeerError {
			if alreadyCancelled {
				glog.Warningf("Not sending peer error as po already cancelled because: %+v", alreadyErr)
			} else if _, e := po.wire.SendPacket(errReason, "err"); e != nil {
				glog.Warningf("Failed sending peer error %s - %+v", errReason, errors.RichError(e))
			}
		}
	}
}

func (po *postingEnd) Close() {
	po.Disconnect("", false)
}
