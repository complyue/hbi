package main

import (
	"flag"
	"log"

	"github.com/complyue/hbi"
	"github.com/complyue/hbi/interop"
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

	he := hbi.NewHostingEnv()

	interop.ExposeInterOpValues(he)

	bannerScript := `
ps1 = "HBI:> "
print("\n### Exposed artifacts of current hosting env:\n")
dir()
`
	// if debugged with delve, no interactive input can be sent in and as a workaround,
	// add calls to suspicious stuff from above script to invoke the buggy cases.

	repl.ReplWith(he, bannerScript)
}
