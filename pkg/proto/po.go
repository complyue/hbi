package proto

// PostingEnd is the application programming interface of an HBI posting endpoint.
type PostingEnd struct {
	hbic *HBIC

	remoteAddr string
}

func (po *PostingEnd) RemoteAddr() string {
	return po.remoteAddr
}

func (po *PostingEnd) NetIdent() string {
	return po.hbic.netIdent
}

func (po *PostingEnd) NewCo() (*PoCo, error) {
	return po.hbic.NewPoCo()
}

func (po *PostingEnd) Notif(code string) (err error) {
	var co *PoCo
	if co, err = po.hbic.NewPoCo(); err != nil {
		return
	}
	defer co.Close()

	if _, err = po, po.hbic.sendPacket(code, ""); err != nil {
		return
	}
	return
}

func (po *PostingEnd) NotifData(code string, d []byte) (err error) {
	var co *PoCo
	if co, err = po.hbic.NewPoCo(); err != nil {
		return
	}
	defer co.Close()

	if err = po.hbic.sendPacket(code, ""); err != nil {
		return
	}
	if err = po.hbic.sendData(d); err != nil {
		return
	}
	return
}

func (po *PostingEnd) Disconnect(errReason string, trySendPeerError bool) {
	po.hbic.Disconnect(errReason, trySendPeerError)
}

func (po *PostingEnd) Close() {
	po.hbic.Disconnect("", false)
}

func (po *PostingEnd) Done() <-chan struct{} {
	return po.hbic.Done()
}
