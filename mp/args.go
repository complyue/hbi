package mp

import (
	"os"

	"github.com/complyue/hbi/pkg/errors"
	"github.com/golang/glog"
)

var (
	mpExecutable string
	mpCmdlArgs   []string
)

func init() {
	var err error

	mpExecutable, err = os.Executable()
	if err != nil {
		glog.Fatalf("error identifying self: %+v", errors.RichError(err))
	}

	mpCmdlArgs = os.Args[1:]

}
