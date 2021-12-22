package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/fenollp/fmtd"
)

var dryrun bool

func init() {
	flag.BoolVar(&dryrun, "n", false, "dry run: no files will be written")
	flag.Parse()
}

func main() {
	ctx := context.Background()

	perr := func(err error) { fmt.Fprintf(os.Stderr, "fmtd: %v\n", err) }

	pwd, err := os.Getwd()
	if err != nil {
		perr(err)
		os.Exit(1)
	}

	switch err := fmtd.Fmt(ctx, pwd, dryrun, os.Stderr, flag.Args()); err {
	case nil:
	case fmtd.ErrDryRunFoundFiles:
		perr(err)
		os.Exit(2)
	default:
		perr(err)
		os.Exit(1)
	}
}
