package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/complyue/hbi"
	"github.com/complyue/hbi/pkg/repl"
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

	context := hbi.NewHostingCtx()

	context["ps1"] = "HBI:> "

	context["print"] = fmt.Print
	context["printf"] = fmt.Printf

	repl.ReplWith(context)

}
