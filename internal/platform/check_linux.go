//go:build linux

package platform

import (
	"fmt"
	"os"
)

func CheckRequirements() error {
	if _, err := os.Stat("/proc/mounts"); err != nil {
		return fmt.Errorf("/proc/mounts not accessible")
	}
	if _, err := os.Stat("/sys/block"); err != nil {
		return fmt.Errorf("/sys/block not accessible - device detection may fail")
	}
	return nil
}
