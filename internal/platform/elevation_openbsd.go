//go:build openbsd

package platform

import "os"

// CheckPrivileges returns the current privilege level of the process.
// It prints a hint to stderr if not running as root but does not exit.
func CheckPrivileges() PrivLevel {
	if os.Geteuid() == 0 {
		return PrivRoot
	}
	return PrivUser
}

// EnsureRoot exits the program if not running as root.
// Deprecated: use CheckPrivileges for adaptive behavior.
func EnsureRoot() {
	if os.Geteuid() != 0 {
		os.Exit(1)
	}
}
