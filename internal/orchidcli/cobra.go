package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type exitCodeError struct {
	code int
}

func (e exitCodeError) Error() string {
	return fmt.Sprintf("exit code %d", e.code)
}

func exitCode(code int) error {
	if code == 0 {
		return nil
	}
	return exitCodeError{code: code}
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orchid",
		Short: "Orchid manages disposable Debian 12 VMs for coding agents.",
		Long: `Orchid manages disposable Debian 12 VMs for coding agents.

It keeps per-VM disks small by building a shared Orchid base image with the
common toolchain already installed, then creating thin qcow2 overlays for each
repo-specific VM.

Command groups:
  orchid config  Set local hypervisor and SSH identity settings
  orchid server  Install and manage the daemon on the hypervisor
  orchid vm      Create, connect to, list, and destroy VMs

Typical flow:
  orchid config set hypervisor <hypervisor-host>
  orchid config set identity-file <path-to-identity>
  orchid vm create <repo-url>
  orchid vm connect <vm-name>
`,
		Example: `# Configure the local client once
orchid config set hypervisor hypervisor.example
orchid config set identity-file ~/.ssh/id_ed25519

# Create and use a VM
orchid vm create https://github.com/org/repo.git
orchid vm connect dev-repo

# Inspect or clean up
orchid vm list
orchid vm destroy dev-repo

# Hypervisor-side maintenance
orchid server status
orchid server build-base`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newConfigCommand())
	cmd.AddCommand(newServerCommand())
	cmd.AddCommand(newVMCommand())
	return cmd
}

func newVMCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vm",
		Short: "Manage VM lifecycle commands",
		Long:  "VM commands run from your laptop and talk to the hypervisor daemon over SSH.",
		Example: `orchid vm create https://github.com/org/repo.git
orchid vm connect dev-repo
orchid vm list
orchid vm destroy dev-repo`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.PersistentFlags().String("hypervisor", "", "SSH host for the libvirt hypervisor")
	cmd.PersistentFlags().String("identity-file", "", "SSH private key used for VM login and git access")

	cmd.AddCommand(newVMConnectCommand())
	cmd.AddCommand(newVMCreateCommand())
	cmd.AddCommand(newVMDestroyCommand())
	cmd.AddCommand(newVMListCommand())
	return cmd
}

func newVMConnectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect <vm-name> [-- <ssh-args...>]",
		Short: "Connect to a VM over SSH",
		RunE: func(cmd *cobra.Command, args []string) error {
			user, _ := cmd.Flags().GetString("user")
			hypervisor, _ := cmd.Flags().GetString("hypervisor")
			identityFile, _ := cmd.Flags().GetString("identity-file")

			runArgs := []string{
				"--user", user,
				"--hypervisor", hypervisor,
				"--identity-file", identityFile,
			}
			runArgs = append(runArgs, args...)
			return exitCode(runConnect(runArgs))
		},
	}
	cmd.Flags().String("user", defaultSSHUser, "SSH user")
	return cmd
}

func newVMCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <repo-url>",
		Short: "Create a new VM for a repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("name")
			hypervisor, _ := cmd.Flags().GetString("hypervisor")
			identityFile, _ := cmd.Flags().GetString("identity-file")

			runArgs := []string{
				"--name", name,
				"--hypervisor", hypervisor,
				"--identity-file", identityFile,
			}
			runArgs = append(runArgs, args...)
			return exitCode(runCreateVM(runArgs))
		},
	}
	cmd.Flags().String("name", "", "Override the VM name")
	return cmd
}

func newVMDestroyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "destroy <vm-name>",
		Short: "Remove a VM and its disk artifacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			hypervisor, _ := cmd.Flags().GetString("hypervisor")

			runArgs := []string{"--hypervisor", hypervisor}
			runArgs = append(runArgs, args...)
			return exitCode(runDestroyVM(runArgs))
		},
	}
	return cmd
}

func newVMListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List VMs on the hypervisor",
		RunE: func(cmd *cobra.Command, args []string) error {
			hypervisor, _ := cmd.Flags().GetString("hypervisor")

			runArgs := []string{"--hypervisor", hypervisor}
			runArgs = append(runArgs, args...)
			return exitCode(runList(runArgs))
		},
	}
	return cmd
}

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Set the local Orchid configuration",
		Long:  "The config command stores the hypervisor host and SSH identity used by vm commands.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newConfigSetCommand())
	return cmd
}

func newConfigSetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <hypervisor|identity-file> <value>",
		Short: "Set a local configuration value",
		RunE: func(cmd *cobra.Command, args []string) error {
			runArgs := append([]string{"set"}, args...)
			return exitCode(runConfig(runArgs))
		},
	}
	return cmd
}

func newServerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Manage the Orchid daemon on the hypervisor",
		Long:  "Server commands are for the hypervisor host: install the daemon, build the shared base image, or inspect service status.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newServerLeafCommand("install", "Install the daemon on the hypervisor"))
	cmd.AddCommand(newServerLeafCommand("build-base", "Refresh the shared Orchid base image"))
	cmd.AddCommand(newServerLeafCommand("proxy", "Proxy HTTP requests over SSH to the daemon"))
	cmd.AddCommand(newServerLeafCommand("run", "Run the daemon in the foreground"))
	cmd.AddCommand(newServerLeafCommand("status", "Show daemon status"))
	return cmd
}

func newServerLeafCommand(name, short string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return exitCode(runServer([]string{name}))
		},
	}
}
