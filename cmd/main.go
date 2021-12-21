package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/fenollp/fmtd"
)

var dryrun bool

func init() {
	flag.BoolVar(&dryrun, "n", false, "dry run: no files will be written")
	flag.Parse()

	log.SetFlags(log.Lshortfile | log.Lmicroseconds | log.LUTC)
}

func main() {
	ctx := context.Background()

	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	if err := fmtd.Fmt(ctx, pwd, dryrun, os.Stderr, flag.Args()); err != nil {
		if err == fmtd.ErrDryRunFoundFiles {
			os.Exit(2)
		}
		log.Fatal(err)
	}
}
