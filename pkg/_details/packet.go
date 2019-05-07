package details

import (
	"fmt"
	"io"
)

type Packet struct {
	WireDir string
	Payload string
}

func (pkt Packet) String() string {
	return fmt.Sprintf("%v", pkt)
}

func (pkt Packet) Format(s fmt.State, verb rune) {
	switch verb {
	case 's':
		fallthrough
	case 'v':
		io.WriteString(s, fmt.Sprintf("[%d#%s]", len(pkt.Payload), pkt.WireDir))
		if s.Flag('+') {
			io.WriteString(s, pkt.Payload)
		}
	}
}
