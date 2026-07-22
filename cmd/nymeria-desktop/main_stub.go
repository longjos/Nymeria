//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "nymeria-desktop currently supports Windows only; use the nymeria binary on this platform")
	os.Exit(1)
}
