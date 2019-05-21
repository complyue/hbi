package proto

// PostingEnd is the application programming interface of a posting endpoint.
type PostingEnd struct {
	hbic *HBIC

	remoteAddr string
}

// RemoteAddr returns the remote network address of the underlying wire.
func (po *PostingEnd) RemoteAddr() string {
	return po.remoteAddr
}

// NetIdent returns the network identification string of the underlying wire.
func (po *PostingEnd) NetIdent() string {
	return po.hbic.netIdent
}

// NewCo starts a new posting conversation.
func (po *PostingEnd) NewCo() (*PoCo, error) {
	return po.hbic.newPoCo()
}

// Notif is shorthand to (implicitly) create a posting conversation, which is closed
// immediately after called its SendCode(code)
func (po *PostingEnd) Notif(code string) (err error) {
	var co *PoCo
	if co, err = po.hbic.newPoCo(); err != nil {
		return
	}
	defer co.Close()

	if _, err = po, po.hbic.sendPacket(code, ""); err != nil {
		return
	}
	return
}

// NotifData is shorthand to (implicitly) create a posting conversation, which is closed
// immediately after called its SendCode(code) and SendData(d)
func (po *PostingEnd) NotifData(code string, d []byte) (err error) {
	var co *PoCo
	if co, err = po.hbic.newPoCo(); err != nil {
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

// Disconnect disconnects the underlying wire of the HBI connection, optionally with
// a error message sent to peer site for information purpose.
func (po *PostingEnd) Disconnect(errReason string, trySendPeerError bool) {
	po.hbic.Disconnect(errReason, trySendPeerError)
}

// Close disconnects the underlying wire of the HBI connection.
func (po *PostingEnd) Close() {
	po.hbic.Disconnect("", false)
}

// Done implements the ctx.context interface by returning a channel closed after the
// underlying HBI connection is disconnected.
func (po *PostingEnd) Done() <-chan struct{} {
	return po.hbic.Done()
}
