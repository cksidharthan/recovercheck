package main

import (
	"github.com/cksidharthan/recovercheck"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	settings := &recovercheck.RecovercheckSettings{}

	singlechecker.Main(recovercheck.New(settings))
}
