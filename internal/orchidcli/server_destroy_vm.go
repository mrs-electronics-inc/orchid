package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func destroyVM(vmName string) error {
	if _, err := runVirshCommand("dominfo", vmName); err == nil {
		state, err := runVirshCommand("domstate", vmName)
		if err != nil {
			return fmt.Errorf("checking state for %s: %w", vmName, err)
		}
		if state == "running" || state == "in shutdown" || state == "paused" {
			if _, err := runVirshCommand("destroy", vmName); err != nil {
				return fmt.Errorf("destroying %s: %w", vmName, err)
			}
		}
		if _, err := runVirshCommand("undefine", vmName); err != nil {
			return fmt.Errorf("undefining %s: %w", vmName, err)
		}
	}

	for _, path := range []string{
		filepath.Join(serverImageDir, vmName+".qcow2"),
		filepath.Join(serverImageDir, vmName+"-seed.iso"),
	} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("removing %s: %w", path, err)
		}
	}

	return nil
}
