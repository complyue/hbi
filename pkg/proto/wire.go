package proto

// HBIWire is the abstract interface an HBI wire should implement
type HBIWire interface {
	NetIdent() string
	LocalAddr() string
	RemoteAddr() string

	SendPacket(payload, wireDir string) (n int64, err error)
	SendData(d []byte) (n int64, err error)
	SendStream(ds func() ([]byte, error)) (n int64, err error)

	RecvPacket() (packet *Packet, err error)
	RecvData(d []byte) (n int64, err error)
	RecvStream(ds func() ([]byte, error)) (n int64, err error)

	Disconnect()
}
