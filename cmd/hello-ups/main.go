package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/complyue/hbi"
	"github.com/complyue/hbi/mp"
	"github.com/golang/glog"
)

func init() {
	// change glog default destination to stderr
	if glog.V(0) { // should always be true, mention glog so it defines its flags before we change them
		if err := flag.CommandLine.Set("logtostderr", "true"); nil != err {
			log.Printf("Failed changing glog default desitination, err: %s", err)
		}
	}
}

func main() {
	flag.Parse()

	mp.UpstartTCP("localhost:3232", func() *hbi.HostingEnv {
		he := hbi.NewHostingEnv()

		he.ExposeFunction("__hbi_init__", // callback on wire connected
			func(po *hbi.PostingEnd, ho *hbi.HostingEnd) {
				po.Notif(fmt.Sprintf(`
print("Welcome to HBI world!")
print("This is worker pid=%v serving you.")
`, os.Getpid()))
			})

		he.ExposeFunction("hello", func() {
			co := he.Ho().Co()
			if err := co.StartSend(); err != nil {
				panic(err)
			}
			consumerName := he.Get("my_name")
			if err := co.SendObj(hbi.Repr(fmt.Sprintf(
				`Hello, %s from %s!`,
				consumerName, he.Po().RemoteAddr(),
			))); err != nil {
				panic(err)
			}
		})
		return he
	}, func(listener *net.TCPListener) error {
		fmt.Println("upstart hello server listening:", listener.Addr())
		return nil
	})

}
