package hbi

import (
	"github.com/complyue/hbi/pkg/he"
	"github.com/complyue/hbi/pkg/proto"
	"github.com/complyue/hbi/pkg/sock"
)

type (
	HostingEnv = he.HostingEnv
	HostingEnd = proto.HostingEnd
	PostingEnd = proto.PostingEnd
	Conver     = proto.Conver
)

var (
	NewHostingEnv = he.NewHostingEnv

	ServeTCP = sock.ServeTCP
	DialTCP  = sock.DialTCP
)
