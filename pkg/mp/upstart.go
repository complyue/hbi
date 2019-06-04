package mp

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"

	"github.com/complyue/hbi/pkg/errors"
	"github.com/golang/glog"
)

// Upstarter defines the interface for upstart to be implemented in different mechanisms,
// e.g. TCP, QUIC etc.
type Upstarter interface {
	// Listen will be called in the listener (or leading, or master) process,
	// it should listen for incoming upstart consumer connections.
	Listen() error

	// ServeFD will be called in a worker subprocess,
	// it should establish an HBI connection to the consumer to be served,
	// through the specified file descripter.
	ServeFD(fd int) (context.Context, error)
}

var (
	parallelismPerConsumer int
	servFD                 int
	consumerIdent          string
)

func init() {

	flag.IntVar(&parallelismPerConsumer, "ppc", 2,
		"limit GOMAXPROCS=<`parallelism`> for each dedicated worker subprocess")
	flag.IntVar(&servFD, "mpfd", 0,
		`(for multiprocessing worker subprocess only, not for human)
serve a consumer through the specified `+"`fd`"+` as a dedicated subprocess`)
	flag.StringVar(&consumerIdent, "mpc", "?!?",
		`(for multiprocessing worker subprocess only, not for human)
`+"`mpc`"+` specifies the consumer identification served by this worker subprocess`)

}

// ParallelismPerConsumer returns the specified per-consumer parallelism from command line.
func ParallelismPerConsumer() int {
	return parallelismPerConsumer
}

// Upstart runs `upstarter` in either listener mode or worker mode according to command line args.
func Upstart(upstarter Upstarter) error {
	if servFD == 0 {
		// run in listener/leading/master mode

		// limit listener to parallelism of 2
		runtime.GOMAXPROCS(2)

		return upstarter.Listen()
	}

	// run in worker mode

	if !(servFD > 2) {
		// should NOT be serving through stdin/stdout/stderr
		glog.Fatalf("Serving FD=%d ?!", servFD)
	}

	if parallelismPerConsumer >= 1 {
		runtime.GOMAXPROCS(parallelismPerConsumer)
	}

	ctx, err := upstarter.ServeFD(servFD)
	if err != nil {
		glog.Errorf("Error serving fd=%d: %+v", servFD, err)
		os.Exit(5)
	}

	glog.V(1).Infof("Serving HBI consumer %s (fd=%d) by worker subprocess %v with ppc=%d ...",
		consumerIdent, servFD, os.Getpid(), parallelismPerConsumer)

	<-ctx.Done()

	glog.V(1).Infof("Done serving HBI consumer %s (fd=%d) by worker subprocess %v", consumerIdent,
		servFD, os.Getpid())

	return nil
}

type fileBased interface {
	File() (*os.File, error)
}

func upstartWorker(conn net.Conn, consumerIdent string) {
	defer conn.Close()

	f, err := conn.(fileBased).File()
	if err != nil {
		glog.Errorf("socket to file error: %+v", errors.RichError(err))
		return
	}
	defer f.Close()

	cmd := exec.Command(mpExecutable, append([]string{
		"-mpfd", "3",
		"-mpc", consumerIdent,
	}, mpCmdlArgs...)...)

	// logs of worker subprocess going into stderr should go into same stderr of listener process
	cmd.Stderr = os.Stderr

	// use fd=3 to pass the accepted socket to worker subprocess
	cmd.ExtraFiles = []*os.File{f}

	if parallelismPerConsumer > 0 {
		// tho -ppc should have been passed along (in mpCmdlArgs), still set the env var here
		cmd.Env = append(os.Environ(), fmt.Sprintf("GOMAXPROCS=%d", parallelismPerConsumer))
	}

	if err = cmd.Start(); err != nil {
		glog.Errorf("error spawning upstart worker subprocess: %+v", errors.RichError(err))
		return
	}

	glog.V(1).Infof("Upstart worker subprocess %v started to serve %s", cmd.Process.Pid, consumerIdent)
}
