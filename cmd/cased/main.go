package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(_ []string, stdout, _ io.Writer) int {
	fmt.Fprintln(stdout, "cased sentinel starting")
	return 0
}
