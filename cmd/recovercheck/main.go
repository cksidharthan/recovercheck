package main

import (
	"flag"

	"github.com/cksidharthan/recovercheck"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	var skipTestFiles bool

	flag.BoolVar(&skipTestFiles, "skip-test-files", false, "skip analysis of *_test.go files")

	settings := &recovercheck.RecovercheckSettings{
		SkipTestFiles: skipTestFiles,
	}

	singlechecker.Main(recovercheck.New(settings))
}
