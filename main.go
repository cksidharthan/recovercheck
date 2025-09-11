package main

import (
	"github.com/cksidharthan/recovercheck/analyzer"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(analyzer.New())
}
