package cli

import (
	"sort"
	"testing"

	"github.com/spf13/cobra"
)

func TestCommandTree(t *testing.T) {
	root := newRootCommand()

	names := commandNames(root.Commands())
	assertContainsAll(t, names, []string{"config", "server", "vm"})
	assertDoesNotContain(t, names, []string{"connect", "create", "destroy", "list"})

	vmCmd, _, err := root.Find([]string{"vm"})
	if err != nil {
		t.Fatalf("finding vm command: %v", err)
	}
	vmNames := commandNames(vmCmd.Commands())
	assertContainsAll(t, vmNames, []string{"connect", "create", "destroy", "list"})

	if vmCmd.PersistentFlags().Lookup("hypervisor") == nil {
		t.Fatal("vm command is missing persistent hypervisor flag")
	}
	if vmCmd.PersistentFlags().Lookup("identity-file") == nil {
		t.Fatal("vm command is missing persistent identity-file flag")
	}

	connectCmd, _, err := root.Find([]string{"vm", "connect"})
	if err != nil {
		t.Fatalf("finding vm connect command: %v", err)
	}
	if connectCmd.Flags().Lookup("user") == nil {
		t.Fatal("vm connect command is missing user flag")
	}

	configCmd, _, err := root.Find([]string{"config"})
	if err != nil {
		t.Fatalf("finding config command: %v", err)
	}
	configNames := commandNames(configCmd.Commands())
	assertContainsAll(t, configNames, []string{"get", "list", "set"})

	serverCmd, _, err := root.Find([]string{"server"})
	if err != nil {
		t.Fatalf("finding server command: %v", err)
	}
	serverNames := commandNames(serverCmd.Commands())
	assertContainsAll(t, serverNames, []string{"build-base", "install", "proxy", "run", "status"})
}

func commandNames(cmds []*cobra.Command) []string {
	names := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		names = append(names, cmd.Name())
	}
	sort.Strings(names)
	return names
}

func assertContainsAll(t *testing.T, got, want []string) {
	t.Helper()

	for _, item := range want {
		found := false
		for _, candidate := range got {
			if candidate == item {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %q in %v", item, got)
		}
	}
}

func assertDoesNotContain(t *testing.T, got, want []string) {
	t.Helper()

	for _, item := range want {
		for _, candidate := range got {
			if candidate == item {
				t.Fatalf("unexpected %q in %v", item, got)
			}
		}
	}
}
