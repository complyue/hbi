package repl

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/complyue/hbi"
	"github.com/complyue/hbi/pkg/errors"
	"github.com/complyue/liner"
	"github.com/golang/glog"
)

func ReplWith(he *hbi.HostingEnv, bannerScript string) {

	defer func() {
		if e := recover(); e != nil {
			glog.Errorf("Unexpected error: %+v", errors.RichError(e))
			os.Exit(3)
		}
		os.Exit(0)
	}()

	he.ExposeValue("ps1", "HBI:> ")

	he.ExposeFunction("dir", func() string {
		var out strings.Builder
		for _, name := range he.ExposedNames() {
			val := he.Get(name)
			out.WriteString(fmt.Sprintf("\n # %s // %T\n := %#v\n", name, val, val))
		}
		return out.String()
	})
	he.ExposeFunction("repr", func(v interface{}) string {
		return hbi.Repr(v)
	})
	he.ExposeFunction("print", func(args ...interface{}) string {
		fmt.Print(args...)
		return ""
	})
	he.ExposeFunction("printf", func(f string, args ...interface{}) string {
		fmt.Printf(f, args...)
		return ""
	})

	line := liner.NewLiner()
	defer line.Close()
	line.SetCtrlCAborts(true)

	// fancy count down to test/showcase async control of liner
	he.ExposeFunction("sd", func(cnt int) string {
		go func() {
			for n := cnt; n > 0; n-- {
				line.ChangePrompt(fmt.Sprintf("SelfDtor(%d): ", n))

				time.Sleep(1 * time.Second)

				line.HidePrompt()
				fmt.Printf(" %d seconds passed.\n", 1+cnt-n)
				line.ShowPrompt()
			}

			line.HidePrompt()
			fmt.Println("Boom!!!")
			line.ShowPrompt()

			line.ChangePrompt("!!<defunct> ")
		}()

		return fmt.Sprintf("Self Destruction in %d seconds ...", cnt)
	})

	var (
		code string
		err  error
	)

	if bannerScript != "" {
		_, err = runOne(he, bannerScript)
	}

	for {

		code, err = line.Prompt(fmt.Sprintf("%v", he.Get("ps1")))
		if err != nil {
			switch err {
			case io.EOF: // Ctrl^D to quit
			case liner.ErrPromptAborted: // Ctrl^C to giveup whatever input
				continue
			default:
				panic(errors.RichError(err))
			}
			break
		}
		if len(code) < 1 {
			continue
		}
		line.AppendHistory(code)

		_, err = runOne(he, code)
	}

	println("\nBye.")

}

func runOne(he *hbi.HostingEnv, code string) (result interface{}, err error) {

	result, err = he.RunInEnv(code, context.Background())
	if err != nil {
		fmt.Printf("[Error]: %+v\n", err)
	} else if text, ok := result.(string); ok {
		fmt.Println(text)
	} else {
		fmt.Printf(" // %T\n  = %s\n", result, hbi.Repr(result))
	}

	return
}
