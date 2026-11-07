package util

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"
)

const (
	Red   = "\033[31m"
	Green = "\033[32m"
	Reset = "\033[0m"
)

var std = func() *log.Logger {
	spew.Config = spew.ConfigState{
		Indent:                  "\t",
		MaxDepth:                30,
		DisablePointerAddresses: true,
		DisableCapacities:       true,
		SortKeys:                true,
		SpewKeys:                true,
	}

	return log.New(os.Stderr, "", 0)
}()

func Logger(args ...any) {
	if len(args) == 0 {
		return
	}

	level := "INFO"
	color := Green

	var err error
	var msg string
	var dumpArgs []any

	for _, a := range args {
		switch v := a.(type) {
		case string:
			if msg == "" {
				msg = v
			} else {
				dumpArgs = append(dumpArgs, v)
			}
		case error:
			if v != nil && err == nil {
				err = v
				level = "ERROR"
				color = Red
			}
		default:
			dumpArgs = append(dumpArgs, v)
		}
	}

	if err != nil && msg != "" {
		msg = fmt.Sprintf("%s | %s", err.Error(), msg)
	} else if err != nil && msg == "" {
		msg = err.Error()
	}

	ts := time.Now().UTC().Format("02-01-2006 at 15:04")

	// color only the prefix
	prefix := fmt.Sprintf("%s%s [%s] %s", color, ts, level, Reset)
	std.SetPrefix(prefix)

	// Emit
	if msg == "" && len(dumpArgs) == 0 {
		return
	}

	if len(dumpArgs) == 0 {
		std.Print(msg)
		return
	}

	if msg == "" {
		std.Print(spew.Sdump(dumpArgs...))
		return
	}

	std.Printf("\n%s\n%s\n", msg, spew.Sdump(dumpArgs...))
}
