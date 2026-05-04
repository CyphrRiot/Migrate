//go:build linux

package platform

import (
	"fmt"
	"os"
)

func EnsureRoot() {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "Root required. Run: sudo migrate")
		os.Exit(1)
	}
}
