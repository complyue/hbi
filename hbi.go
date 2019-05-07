package hbi

import (
	"github.com/complyue/hbigo/pkg/conn"
	"github.com/complyue/hbigo/pkg/proto"
)

type (
	HoContext = proto.HoContext
	TCPConn   = conn.TCPConn

	Hosting = proto.Hosting
	Posting = proto.Posting
	Conver  = proto.Conver
)

var (
	NewHoContext = proto.NewHoContext

	ServeTCP = conn.ServeTCP
	DialTCP  = conn.DialTCP
)
