package proto

// HostingEnd is the application programming interface of a hosting endpoint.
type HostingEnd struct {
	hbic *HBIC

	env *HostingEnv

	localAddr string

	co *HoCo
}

// Env returns the hosting environment that this hosting endpoint is attached to.
func (ho *HostingEnd) Env() *HostingEnv {
	return ho.env
}

// LocalAddr returns the local network address of the underlying wire.
func (ho *HostingEnd) LocalAddr() string {
	return ho.localAddr
}

// NetIdent returns the network identification string of the underlying wire.
func (ho *HostingEnd) NetIdent() string {
	return ho.hbic.netIdent
}

// Co of a hosting endpoint returns the current hosting conversation triggered by an
// originating posting conversation from its peer site.
func (ho *HostingEnd) Co() *HoCo {
	// lock muCo in case it's called from some goroutines other than the landing thread
	ho.hbic.muCo.Lock()
	defer ho.hbic.muCo.Unlock()

	return ho.co
}

// Disconnect disconnects the underlying wire of the HBI connection, optionally with
// a error message sent to peer site for information purpose.
func (ho *HostingEnd) Disconnect(errReason string, trySendPeerError bool) {
	ho.hbic.Disconnect(errReason, trySendPeerError)
}

// Close disconnects the underlying wire of the HBI connection.
func (ho *HostingEnd) Close() {
	ho.hbic.Disconnect("", false)
}

// Done implements the ctx.context interface by returning a channel closed after the
// underlying HBI connection is disconnected.
func (ho *HostingEnd) Done() <-chan struct{} {
	return ho.hbic.Done()
}
