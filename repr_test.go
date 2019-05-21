package hbi

import (
	"fmt"
	"io"
	"time"
)

type Msg struct {
	From    string
	Content string
	Time    time.Time
}

func NewMsg(from, content string, unixTimestamp int64) *Msg {
	return &Msg{
		From: from, Content: content,
		Time: time.Unix(unixTimestamp, 0),
	}
}

func (msg *Msg) Format(s fmt.State, verb rune) {
	switch verb {
	case 's': // string form
		io.WriteString(s, "Msg<@")
		io.WriteString(s, msg.From)
	case 'v':
		if s.Flag('#') { // repr form
			io.WriteString(s, "Msg(")
			io.WriteString(s, fmt.Sprintf("%#v", msg.From))
			io.WriteString(s, ",")
			io.WriteString(s, fmt.Sprintf("%#v", msg.Content))
			io.WriteString(s, ",")
			io.WriteString(s, fmt.Sprintf("%d", msg.Time.Unix()))
			io.WriteString(s, ")")
		} else { // value form
			if s.Flag('+') {
				io.WriteString(s, "[")
				io.WriteString(s, msg.Time.Format("Jan 02 15:04:05Z07"))
				io.WriteString(s, "] ")
			}
			io.WriteString(s, "@")
			io.WriteString(s, msg.From)
			io.WriteString(s, ": ")
			io.WriteString(s, msg.Content)
		}
	}
}

func ExampleRepr() {

	saidZone, _ := time.LoadLocation("Asia/Chongqing")
	saidTime, _ := time.ParseInLocation(
		"2006-01-02 15:04:05",
		"2019-05-16 17:28:39",
		saidZone,
	)
	msg := NewMsg("Compl", "Hello, HBI world!", saidTime.Unix())

	fmt.Printf("(%6s) %s\n", "Repr", Repr(msg))
	fmt.Printf("(%6s) %#v\n", "Repr", msg)
	fmt.Printf("(%6s) %+v\n", "Long", msg)
	fmt.Printf("(%6s) %v\n", "Short", msg)
	fmt.Printf("(%6s) %s\n", "String", msg)

	// Output:
	// (  Repr) Msg("Compl","Hello, HBI world!",1557998919)
	// (  Repr) Msg("Compl","Hello, HBI world!",1557998919)
	// (  Long) [May 16 17:28:39+08] @Compl: Hello, HBI world!
	// ( Short) @Compl: Hello, HBI world!
	// (String) Msg<@Compl

}
