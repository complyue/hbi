package proto

import (
	"github.com/complyue/hbi/pkg/he"
)

// PostingEnd is the application programming interface of an HBI posting endpoint.
type HostingEnd struct {
	hbic *HBIC

	env *he.HostingEnv

	localAddr string

	co *HoCo
}

func (ho *HostingEnd) Env() *he.HostingEnv {
	return ho.env
}

func (ho *HostingEnd) LocalAddr() string {
	return ho.localAddr
}

func (ho *HostingEnd) NetIdent() string {
	return ho.hbic.netIdent
}

func (ho *HostingEnd) Co() *HoCo {
	// lock muCo in case it's called from some goroutines other than the landing thread
	ho.hbic.muCo.Lock()
	defer ho.hbic.muCo.Unlock()

	return ho.co
}

func (ho *HostingEnd) Disconnect(errReason string, trySendPeerError bool) {
	ho.hbic.Disconnect(errReason, trySendPeerError)
}
