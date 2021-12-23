package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/fenollp/fmtd"
)

var dryrun bool
var withstderr bool

func init() {
	flag.BoolVar(&dryrun, "n", false, "dry run: no files will be written")
	flag.BoolVar(&withstderr, "2", false, "show Docker progress")
	flag.Parse()
}

func main() {
	ctx := context.Background()

	stdout := os.Stdout

	perr := func(err error) { fmt.Fprintf(stdout, "fmtd: %v\n", err) }

	pwd, err := os.Getwd()
	if err != nil {
		perr(err)
		os.Exit(1)
	}

	stderr := ioutil.Discard
	if withstderr {
		stderr = os.Stderr
	}

	switch err := fmtd.Fmt(ctx, pwd, dryrun, stdout, stderr, flag.Args()); err {
	case nil:
	case fmtd.ErrDryRunFoundFiles:
		os.Exit(2)
	default:
		if err == fmtd.ErrDockerBuildFailure && !withstderr {
			err = fmt.Errorf("%w, maybe retry with flag -2", err)
		}
		perr(err)
		os.Exit(1)
	}
}
