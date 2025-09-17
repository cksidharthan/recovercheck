package main

import (
	"github.com/cksidharthan/recovercheck"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(recovercheck.New())
}
