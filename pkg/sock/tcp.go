package sock

import (
	"fmt"
	"io"
	"net"
	"strings"

	details "github.com/complyue/hbi/pkg/_details"
	"github.com/complyue/hbi/pkg/errors"
	"github.com/complyue/hbi/pkg/he"
	"github.com/complyue/hbi/pkg/proto"
	"github.com/golang/glog"
)

// ServeTCP listens on the specified local address (host:port), serves each incoming connection with the
// hosting env created from the `heFactory` function.
//
// `cb` will be called with the created `*net.TCPListener`, it's handful to specify port as 0,
// and receive the actual port from the cb.
//
// This func won't return until the listener is closed.
func ServeTCP(heFactory func() *he.HostingEnv, addr string, cb func(*net.TCPListener)) (err error) {
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

		proto.NewConnection(he, wire)
	}
}

// DialTCP connects to specified remote address (host:port), react with specified hosting env.
//
// The returned `po` is used to send code & data to remote peer for hosted landing.
func DialTCP(he *he.HostingEnv, addr string) (po proto.PostingEnd, ho proto.HostingEnd, err error) {
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

	po, ho = proto.NewConnection(he, wire)

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

func (wire *TCPWire) LocalAddr() net.Addr {
	return wire.conn.LocalAddr()
}

func (wire *TCPWire) RemoteAddr() net.Addr {
	return wire.conn.RemoteAddr()
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

// SendStream pulls all `[]byte` from the specified channel and sends them as binary stream to peer endpoint.
// each `[]byte` will have its len() of data sent, regardless of its cap().
func (wire *TCPWire) SendStream(data <-chan []byte) (n int64, err error) {
	if glog.V(3) {
		defer func() {
			glog.Infof("HBI wire %s sent binary data of %d bytes.",
				wire.netIdent, n)
		}()
	}
	var bufs net.Buffers
	var nb int64
	for {
		select {
		case buf, ok := <-data:
			if !ok {
				// no more buf to send
				return
			}
			if len(buf) <= 0 {
				// zero buf, ignore it
				break
			}
			bufs = append(bufs, buf)
		}
		if len(bufs) <= 0 {
			// all data sent
			return
		}
		nb, err = bufs.WriteTo(wire.conn)
		if err != nil {
			return
		}
		n += nb
	}
}

// SendData will have len() of buf sent to peer endpoint, regardless of its cap()
func (wire *TCPWire) SendData(buf []byte) (n int64, err error) {
	if glog.V(3) {
		defer func() {
			glog.Infof("HBI wire %s sent binary data of %d bytes.",
				wire.netIdent, n)
		}()
	}
	if len(buf) <= 0 {
		// zero buf, ignore it
		return
	}
	bufs := net.Buffers{buf}
	n, err = bufs.WriteTo(wire.conn)
	if err != nil {
		return
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

// RecvStream receives binary data stream into all `[]byte` from the specified channel.
// each []byte will be filled up to its len(), regardless of its cap().
func (wire *TCPWire) RecvStream(data <-chan []byte) (n int64, err error) {
	if glog.V(3) {
		defer func() {
			glog.Infof("HBI wire %s received binary data of %d bytes.",
				wire.netIdent, n)
		}()
	}
	var nb int
	for {
		select {
		case buf, ok := <-data:
			if !ok {
				// no more buf to send
				return
			}
			if len(buf) <= 0 {
				// zero buf, ignore it
				break
			}
			for {
				if wire.readahead != nil {
					if len(buf) <= len(wire.readahead) {
						nb = len(buf)
						copy(buf, wire.readahead[:nb])
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
						copy(buf[:nb], wire.readahead)
						// this buf only partial filled by readahead data,
						// read rest from wire
						buf = buf[nb:]
						wire.readahead = nil
						n += int64(nb)
					}
				}
				nb, err = wire.conn.Read(buf)
				if err != nil {
					return
				}
				n += int64(nb)
				if nb >= len(buf) {
					// this buf fully filled
					break
				}
				// read into rest space
				buf = buf[nb:]
			}
		}
	}
}

// RecvData receives binary data into len() of `buf` from the specified channel, regardless of its cap().
func (wire *TCPWire) RecvData(buf []byte) (n int64, err error) {
	if glog.V(3) {
		defer func() {
			glog.Infof("HBI wire %s received binary data of %d bytes.",
				wire.netIdent, n)
		}()
	}
	if len(buf) <= 0 {
		// zero buf, ignore it
		return
	}
	var nb int
	for {
		if wire.readahead != nil {
			if len(buf) <= len(wire.readahead) {
				nb = len(buf)
				copy(buf, wire.readahead[:nb])
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
				copy(buf[:nb], wire.readahead)
				// this buf only partial filled by readahead data,
				// read rest from wire
				buf = buf[nb:]
				wire.readahead = nil
				n += int64(nb)
			}
		}
		nb, err = wire.conn.Read(buf)
		if err != nil {
			return
		}
		n += int64(nb)
		if nb >= len(buf) {
			// this buf fully filled
			break
		}
		// read into rest space
		buf = buf[nb:]
	}
	return
}

func (wire *TCPWire) Disconnect() {
	wire.conn.Close()
}