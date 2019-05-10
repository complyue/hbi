package proto

import (
	"net"
)

// HBIWire is the interface an HBI trnasport should implement
type HBIWire interface {
	NetIdent() string
	LocalAddr() net.Addr
	RemoteAddr() net.Addr

	SendPacket(payload, wireDir string) (n int64, err error)
	SendData(buf []byte) (n int64, err error)
	SendStream(data <-chan []byte) (n int64, err error)

	RecvPacket() (packet *Packet, err error)
	RecvData(buf []byte) (n int64, err error)
	RecvStream(data <-chan []byte) (n int64, err error)

	Disconnect()
}
