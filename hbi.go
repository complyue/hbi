package hbi

import (
	"github.com/complyue/hbi/pkg/conn"
	"github.com/complyue/hbi/pkg/he"
	"github.com/complyue/hbi/pkg/proto"
)

type (
	HostingEnv = he.HostingEnv
	HostingEnd = proto.HostingEnd
	PostingEnd = proto.PostingEnd
	Conver     = proto.Conver
)

var (
	NewHostingEnv = he.NewHostingEnv

	ServeTCP = conn.ServeTCP
	DialTCP  = conn.DialTCP
)
