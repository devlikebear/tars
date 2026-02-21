package main

import (
	"os"

	"github.com/devlikebear/tarsncase/internal/tarsdapp"
)

func main() {
	os.Exit(tarsdapp.Run(os.Args[1:], os.Stdout, os.Stderr))
}
