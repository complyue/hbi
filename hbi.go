package hbi

import (
	"github.com/complyue/hbi/pkg/conn"
	"github.com/complyue/hbi/pkg/ctx"
	"github.com/complyue/hbi/pkg/proto"
)

type (
	HostingCtx = ctx.HostingCtx

	HostingEnd = proto.HostingEnd
	PostingEnd = proto.PostingEnd
	Conver     = proto.Conver
)

var (
	NewHostingCtx = ctx.NewHostingCtx

	ServeTCP = conn.ServeTCP
	DialTCP  = conn.DialTCP
)
