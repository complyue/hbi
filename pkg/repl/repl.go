package repl

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/complyue/hbi"
	"github.com/complyue/hbi/pkg/errors"
	"github.com/golang/glog"
	"github.com/peterh/liner"
)

func ReplWith(context hbi.HostingCtx) {

	defer func() {
		if e := recover(); e != nil {
			glog.Errorf("Unexpected error: %+v", errors.RichError(e))
			os.Exit(3)
		}
		os.Exit(0)
	}()

	if _, ok := context["ps1"]; !ok {
		context["ps1"] = "HBI:> "
	}

	if _, ok := context["dir"]; !ok {
		context["dir"] = func() {
			names := make([]string, 0, len(context))
			for name := range context {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				val := context[name]
				fmt.Printf("\n * %s - %T\n:= %#v\n", name, val, val)
			}
		}
	}

	line := liner.NewLiner()
	defer line.Close()
	line.SetCtrlCAborts(true)

	var (
		code   string
		result interface{}
		err    error
	)

	for {

		code, err = line.Prompt(fmt.Sprintf("%s", context["ps1"]))
		if err != nil {
			switch err {
			case io.EOF: // Ctrl^D
			case liner.ErrPromptAborted: // Ctrl^C
			default:
				panic(errors.RichError(err))
			}
			break
		}
		if len(code) < 1 {
			continue
		}
		line.AppendHistory(code)

		result, err = context.Exec(code, "HBI-CODE")
		fmt.Println()
		if err != nil {
			fmt.Printf("[Error]: %+v\n", err)
		} else {
			fmt.Printf("[Result]: %#v\n", result)
		}

	}

	println("\nBye.")

}
