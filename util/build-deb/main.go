package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/xor-gate/debpkg"
)

var (
	output   = flag.String("o", "unlockr.deb", "path to output file")
	spec     = flag.String("c", "spec.yaml", "path to spec.yaml")
	postinst = flag.String("postinst", "./postinst", "path to postinst script")
	version  = flag.String("version", "1.0~test", "package version number")
)

var pkg = debpkg.New()

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage:	build-deb [-i input-dir] [-o unlockr.deb] [-c spec.yaml] [-postinst script] src:dst...")
	}
	flag.Parse()

	if flag.NArg() <= 0 {
		flag.Usage()
		os.Exit(1)
	}

	if err := pkg.Config(*spec); err != nil {
		fmt.Fprintf(os.Stderr, "Loading config spec.yaml: %v\n", err)
		os.Exit(1)
	}

	if *version != "" {
		pkg.SetVersion(*version)
	}

	for _, filePair := range flag.Args() {
		src, dst, ok := strings.Cut(filePair, ":")
		if !ok {
			fmt.Fprintf(os.Stderr, "File must be src:dst, got: %s\n", filePair)
			os.Exit(1)
		}
		if err := pkg.AddFile(src, dst); err != nil {
			fmt.Fprintf(os.Stderr, "Adding file %s: %v\n", src, err)
			os.Exit(1)
		}
	}

	if err := pkg.AddControlExtra("postinst", *postinst); err != nil {
		fmt.Fprintf(os.Stderr, "Adding postinst script %s: %v\n", *postinst, err)
		os.Exit(1)
	}

	if err := pkg.Write(*output); err != nil {
		fmt.Fprintf(os.Stderr, "Writing package: %v\n", err)
		os.Exit(1)
	}
}
