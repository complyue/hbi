package repl

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/complyue/hbi"
	"github.com/complyue/hbi/pkg/errors"
	"github.com/golang/glog"
	"github.com/peterh/liner"
)

func ReplWith(he *hbi.HostingEnv) {

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
		return fmt.Sprintf("%#v", v)
	})
	he.ExposeFunction("print", func(args ...interface{}) string {
		return fmt.Sprint(args...)
	})
	he.ExposeFunction("printf", func(f string, args ...interface{}) string {
		return fmt.Sprintf(f, args...)
	})

	line := liner.NewLiner()
	defer line.Close()
	line.SetCtrlCAborts(true)

	var (
		code string
		err  error
	)

	// _, err = runOne(he, "dir()") // help some with delve debug

	for {

		code, err = line.Prompt(fmt.Sprintf("%v", he.Get("ps1")))
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
		fmt.Printf(" // %T\n := %#v\n", result, result)
	}

	return
}
