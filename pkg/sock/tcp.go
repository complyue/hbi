package sock

import (
	"fmt"
	"io"
	"net"
	"strings"

	details "github.com/complyue/hbi/pkg/_details"
	"github.com/complyue/hbi/pkg/errors"
	"github.com/complyue/hbi/pkg/proto"
	"github.com/golang/glog"
)

// ServeTCP listens on the specified local address (host:port), serves each incoming connection with the
// hosting environment created from the `heFactory` function.
//
// `cb` will be called with the created `*net.TCPListener`, it's handful to specify port as 0,
// and receive the actual port from the cb.
//
// This func won't return until the listener is closed.
func ServeTCP(addr string, heFactory func() *proto.HostingEnv, cb func(*net.TCPListener)) (err error) {
	var raddr *net.TCPAddr
	raddr, err = net.ResolveTCPAddr("tcp", addr)
	if nil != err {
		glog.Errorf("addr error: %+v", errors.RichError(err))
		return
	}
	var listener *net.TCPListener
	listener, err = net.ListenTCP("tcp", raddr)
	if err != nil {
		glog.Errorf("listen error: %+v", errors.RichError(err))
		return
	}
	if cb != nil {
		cb(listener)
	}

	for {
		var conn *net.TCPConn
		conn, err = listener.AcceptTCP()
		if nil != err {
			glog.Errorf("accept error: %+v", errors.RichError(err))
			return
		}

		// todo DoS react
		he := heFactory()

		wire := NewTCPWire(conn)
		netIdent := wire.NetIdent()
		glog.V(1).Infof("New HBI connection accepted: %s", netIdent)

		proto.NewConnection(wire, he)
	}
}

// DialTCP connects to specified remote address (host:port), react with specified hosting environment.
//
// The returned posting endpoint is used to create posting conversations to send code & data to remote
// site for active communication.
//
// The returned hosting endpoint is used to obtain the current hosting conversation triggered by a
// posting conversation from remote site for passive communication.
func DialTCP(addr string, he *proto.HostingEnv) (po *proto.PostingEnd, ho *proto.HostingEnd, err error) {
	raddr, err := net.ResolveTCPAddr("tcp", addr)
	if nil != err {
		glog.Errorf("addr error: %+v", errors.RichError(err))
		return
	}
	conn, err := net.DialTCP("tcp", nil, raddr)
	if nil != err {
		glog.Errorf("conn error: %+v", errors.RichError(err))
		return
	}

	wire := NewTCPWire(conn)
	netIdent := wire.NetIdent()
	glog.V(1).Infof("New HBI connection established: %s", netIdent)

	po, ho, err = proto.NewConnection(wire, he)

	return
}

// TCPWire is HBI wire protocol over a plain tcp socket.
type TCPWire struct {
	conn *net.TCPConn

	netIdent  string
	readahead []byte
}

func NewTCPWire(conn *net.TCPConn) *TCPWire {
	return &TCPWire{
		conn:     conn,
		netIdent: fmt.Sprintf("%s<->%s", conn.LocalAddr(), conn.RemoteAddr()),
	}
}

func (wire *TCPWire) NetIdent() string {
	return wire.netIdent
}

func (wire *TCPWire) LocalAddr() string {
	return fmt.Sprintf("%s", wire.conn.LocalAddr())
}

func (wire *TCPWire) RemoteAddr() string {
	return fmt.Sprintf("%s", wire.conn.RemoteAddr())
}

// SendPacket sends a packet to peer endpoint.
func (wire *TCPWire) SendPacket(payload, wireDir string) (n int64, err error) {
	if glog.V(3) {
		defer func() {
			if err != nil {
				glog.Error(errors.RichError(err))
			} else {
				glog.Infof("HBI wire %s sent pkt %d:\n%+v\n",
					wire.netIdent, n, proto.Packet{WireDir: wireDir, Payload: payload})
			}
		}()
	}
	header := fmt.Sprintf("[%v#%s]", len(payload), wireDir)
	bufs := net.Buffers{
		[]byte(header), []byte(payload),
	}
	n, err = bufs.WriteTo(wire.conn)
	return
}

// SendData will have len() of buf sent to peer endpoint, regardless of its cap()
func (wire *TCPWire) SendData(d []byte) (n int64, err error) {
	if glog.V(3) {
		defer func() {
			glog.Infof("HBI wire %s sent binary data of %d bytes.",
				wire.netIdent, n)
		}()
	}
	if len(d) <= 0 {
		// zero buf, ignore it
		return
	}
	bufs := net.Buffers{d}
	n, err = bufs.WriteTo(wire.conn)
	if err != nil {
		return
	}
	return
}

// SendStream pulls all `[]byte` from the specified func and sends them as binary stream to peer endpoint.
// each `[]byte` will have its len() of data sent, regardless of its cap().
func (wire *TCPWire) SendStream(ds func() ([]byte, error)) (n int64, err error) {
	if glog.V(3) {
		defer func() {
			glog.Infof("HBI wire %s sent binary data of %d bytes.",
				wire.netIdent, n)
		}()
	}
	var bufs net.Buffers
	var nb int64
	var d []byte
	for {
		d, err = ds()
		if err != nil {
			return
		}
		if d == nil {
			// no more buf to send
			break
		}
		if len(d) <= 0 {
			// ignore zero sized buf
			continue
		}
		bufs = append(bufs, d)
		for len(bufs) > 0 {
			nb, err = bufs.WriteTo(wire.conn)
			if err != nil {
				return
			}
			n += nb
		}
	}
	return
}

// RecvPacket receives next packet from peer endpoint.
func (wire *TCPWire) RecvPacket() (packet *proto.Packet, err error) {
	if glog.V(3) {
		defer func() {
			if packet != nil {
				glog.Infof("HBI wire %s got pkt:\n%+v\n", wire.netIdent, packet)
			}
		}()
	}
	var (
		wireDir, payload string
		n, start, newLen int
		hdrBuf           = make([]byte, 0, details.MaxHeaderLen)
		payloadBuf       []byte
	)

	// read header
	for {
		start = len(hdrBuf)
		readInto := hdrBuf[start:cap(hdrBuf)]
		if wire.readahead != nil {
			// consume readahead first
			if len(wire.readahead) > len(readInto) {
				n = len(readInto)
				copy(readInto, wire.readahead[:n])
				wire.readahead = wire.readahead[n:]
			} else {
				n = len(wire.readahead)
				copy(readInto[:n], wire.readahead)
				wire.readahead = nil
			}
		} else {
			// no readahead, read wire
			n, err = wire.conn.Read(readInto)
			if err == io.EOF {
				if start+n <= 0 {
					// normal EOF after full packet, return nil + EOF
					return
				}
				// fall through to receive this last packet, it's possible we already got the full data in hdrBuf
			} else if err != nil {
				// other error occurred
				return
			}
		}
		newLen = start + n
		hdrBuf = hdrBuf[:newLen]
		for i, c := range hdrBuf[start:newLen] {
			if ']' == c {
				header := string(hdrBuf[0 : start+i+1])
				if '[' != header[0] {
					err = errors.New(fmt.Sprintf("Invalid header: %#v", header))
					return
				}
				lenEnd := strings.Index(header, "#")
				if -1 == lenEnd {
					err = errors.New("no # in header")
					return
				}
				wireDir = string(header[lenEnd+1 : start+i])
				var payloadLen int
				fmt.Sscan(header[1:lenEnd], &payloadLen)
				if payloadLen < 0 {
					err = errors.New("negative payload length")
					return
				}
				plBegin := start + i + 1
				extraLen := newLen - plBegin
				payloadBuf = make([]byte, 0, payloadLen)
				if extraLen > payloadLen {
					// got more data than this packet's payload
					payloadBuf = payloadBuf[:payloadLen]
					plEnd := plBegin + payloadLen
					copy(payloadBuf, hdrBuf[plBegin:plEnd])
					if wire.readahead == nil {
						wire.readahead = make([]byte, newLen-plEnd)
						copy(wire.readahead, hdrBuf[plEnd:newLen])
					} else {
						readahead := make([]byte, newLen-plEnd+len(wire.readahead))
						copy(readahead[:newLen-plEnd], hdrBuf[plEnd:newLen])
						readahead = append(readahead, wire.readahead...)
						wire.readahead = readahead
					}
				} else if extraLen > 0 {
					// got some data but no more than this packet's payload
					payloadBuf = payloadBuf[:extraLen]
					copy(payloadBuf, hdrBuf[plBegin:newLen])
				}
				break
			}
		}
		if payloadBuf != nil {
			break
		}
		if newLen >= details.MaxHeaderLen {
			err = errors.New(fmt.Sprintf("No header within first %v bytes!", details.MaxHeaderLen))
			return
		}
		if err == io.EOF {
			// reached EOF without full header
			err = errors.New("incomplete header at EOF")
			return
		}
	}

	// read payload
	for len(payloadBuf) < cap(payloadBuf) {
		if err == io.EOF {
			err = errors.New("premature packet at EOF")
			return
		}
		start = len(payloadBuf)
		n, err = wire.conn.Read(payloadBuf[start:cap(payloadBuf)])
		newLen = start + n
		payloadBuf = payloadBuf[:newLen]
		if newLen >= cap(payloadBuf) {
			break
		}
	}
	payload = string(payloadBuf)

	packet = &proto.Packet{WireDir: wireDir, Payload: payload}
	if err == io.EOF {
		// clear EOF if got a complete packet.
		// todo what if the underlying Reader not tolerating our next read passing EOF
		err = nil
	}
	return
}

// RecvData receives binary data into len() of `buf` from the specified channel, regardless of its cap().
func (wire *TCPWire) RecvData(d []byte) (n int64, err error) {
	if glog.V(3) {
		defer func() {
			glog.Infof("HBI wire %s received binary data of %d bytes.",
				wire.netIdent, n)
		}()
	}
	if len(d) <= 0 {
		// zero buf, ignore it
		return
	}
	var nb int
	for {
		if wire.readahead != nil {
			if len(d) <= len(wire.readahead) {
				nb = len(d)
				copy(d, wire.readahead[:nb])
				if nb == len(wire.readahead) {
					wire.readahead = nil
				} else {
					wire.readahead = wire.readahead[nb:]
				}
				n += int64(nb)
				// this buf fully filed by readahead data
				break
			} else {
				nb = len(wire.readahead)
				copy(d[:nb], wire.readahead)
				// this buf only partial filled by readahead data,
				// read rest from wire
				d = d[nb:]
				wire.readahead = nil
				n += int64(nb)
			}
		}
		nb, err = wire.conn.Read(d)
		if err != nil {
			return
		}
		n += int64(nb)
		if nb >= len(d) {
			// this buf fully filled
			break
		}
		// read into rest space
		d = d[nb:]
	}
	return
}

// RecvStream receives binary data stream into all `[]byte` from the specified func.
// each []byte will be filled up to its len(), regardless of its cap().
func (wire *TCPWire) RecvStream(ds func() ([]byte, error)) (n int64, err error) {
	if glog.V(3) {
		defer func() {
			glog.Infof("HBI wire %s received binary data of %d bytes.",
				wire.netIdent, n)
		}()
	}
	var nb int
	var d []byte
	for {
		d, err = ds()
		if err != nil {
			return
		}
		if d == nil {
			// no more buf to recv
			return
		}
		if len(d) <= 0 {
			// ignore zero sized buf
			continue
		}
		for {
			if wire.readahead != nil {
				if len(d) <= len(wire.readahead) {
					nb = len(d)
					copy(d, wire.readahead[:nb])
					if nb == len(wire.readahead) {
						wire.readahead = nil
					} else {
						wire.readahead = wire.readahead[nb:]
					}
					n += int64(nb)
					// this buf fully filed by readahead data
					break
				} else {
					nb = len(wire.readahead)
					copy(d[:nb], wire.readahead)
					// this buf only partial filled by readahead data,
					// read rest from wire
					d = d[nb:]
					wire.readahead = nil
					n += int64(nb)
				}
			}
			nb, err = wire.conn.Read(d)
			if err != nil {
				return
			}
			n += int64(nb)
			if nb >= len(d) {
				// this buf fully filled
				break
			}
			// read into rest space
			d = d[nb:]
		}
	}
}

func (wire *TCPWire) Disconnect() {
	wire.conn.Close()
}
