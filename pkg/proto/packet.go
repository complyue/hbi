package proto

import (
	"fmt"
	"io"
)

// Packet is the atomic textual unit to be transfered over an HBI wire
type Packet struct {
	// wire directive
	WireDir string

	// textual payload
	Payload string
}

// Format make Packet implement fmt.Formatter
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

func (pkt Packet) String() string {
	return fmt.Sprintf("%v", pkt)
}
