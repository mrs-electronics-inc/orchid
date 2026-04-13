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
			if len(args) < 1 {
				_ = cmd.Help()
				return exitCode(2)
			}

			user, _ := cmd.Flags().GetString("user")
			hypervisor, _ := cmd.Flags().GetString("hypervisor")
			identityFile, _ := cmd.Flags().GetString("identity-file")
			return exitCode(vmConnect(user, hypervisor, identityFile, args[0], args[1:]))
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
			if len(args) != 1 {
				_ = cmd.Help()
				return exitCode(2)
			}

			name, _ := cmd.Flags().GetString("name")
			hypervisor, _ := cmd.Flags().GetString("hypervisor")
			identityFile, _ := cmd.Flags().GetString("identity-file")
			return exitCode(vmCreate(name, hypervisor, identityFile, args[0]))
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
			if len(args) != 1 {
				_ = cmd.Help()
				return exitCode(2)
			}

			hypervisor, _ := cmd.Flags().GetString("hypervisor")
			return exitCode(vmDestroy(hypervisor, args[0]))
		},
	}
	return cmd
}

func newVMListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List VMs on the hypervisor",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				_ = cmd.Help()
				return exitCode(2)
			}

			hypervisor, _ := cmd.Flags().GetString("hypervisor")
			return exitCode(vmList(hypervisor))
		},
	}
	return cmd
}

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage the local Orchid configuration",
		Long:  "The config command stores and queries the hypervisor host and SSH identity used by vm commands.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newConfigGetCommand())
	cmd.AddCommand(newConfigListCommand())
	cmd.AddCommand(newConfigSetCommand())
	return cmd
}

func newConfigGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <hypervisor|identity-file>",
		Short: "Show a local configuration value",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				printConfigUsage(cmd.ErrOrStderr())
				return exitCode(2)
			}

			cfg, path, err := loadCurrentConfig()
			if err != nil {
				return err
			}

			switch args[0] {
			case "hypervisor":
				if cfg.Hypervisor == "" {
					return fmt.Errorf("hypervisor is not set in %s", path)
				}
				fmt.Fprintln(cmd.OutOrStdout(), cfg.Hypervisor)
			case "identity-file", "identity_file":
				if cfg.IdentityFile == "" {
					return fmt.Errorf("identity file is not set in %s", path)
				}
				fmt.Fprintln(cmd.OutOrStdout(), cfg.IdentityFile)
			default:
				printConfigUsage(cmd.ErrOrStderr())
				return exitCode(2)
			}

			return nil
		},
	}
	return cmd
}

func newConfigListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show the current local configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				printConfigUsage(cmd.ErrOrStderr())
				return exitCode(2)
			}

			cfg, path, err := loadCurrentConfig()
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Config file: %s\n", path)
			fmt.Fprintf(out, "hypervisor = %s\n", configDisplayValue(cfg.Hypervisor))
			fmt.Fprintf(out, "identity_file = %s\n", configDisplayValue(cfg.IdentityFile))
			return nil
		},
	}
	return cmd
}

func newConfigSetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <hypervisor|identity-file> <value>",
		Short: "Set a local configuration value",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				printConfigUsage(cmd.ErrOrStderr())
				return exitCode(2)
			}

			var update func(*config) error
			switch args[0] {
			case "hypervisor":
				update = func(cfg *config) error {
					cfg.Hypervisor = args[1]
					return nil
				}
			case "identity-file", "identity_file":
				update = func(cfg *config) error {
					cfg.IdentityFile = args[1]
					return nil
				}
			default:
				printConfigUsage(cmd.ErrOrStderr())
				return exitCode(2)
			}

			if err := saveConfigUpdate(update); err != nil {
				return err
			}

			path, err := configPath()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", path)
			return nil
		},
	}
	return cmd
}

func printConfigUsage(w interface{ Write([]byte) (int, error) }) {
	fmt.Fprintln(w, "usage: orchid config get <hypervisor|identity-file>")
	fmt.Fprintln(w, "       orchid config list")
	fmt.Fprintln(w, "       orchid config set hypervisor <host>")
	fmt.Fprintln(w, "       orchid config set identity-file <path>")
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
	cmd.AddCommand(newServerLeafCommand("install", "Install the daemon on the hypervisor", serverInstall))
	cmd.AddCommand(newServerLeafCommand("build-base", "Refresh the shared Orchid base image", func() int {
		if err := buildOrchidBaseImage(); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), err)
			return 1
		}
		return 0
	}))
	cmd.AddCommand(newServerLeafCommand("proxy", "Proxy HTTP requests over SSH to the daemon", serverProxy))
	cmd.AddCommand(newServerLeafCommand("run", "Run the daemon in the foreground", serverRun))
	cmd.AddCommand(newServerLeafCommand("status", "Show daemon status", serverStatus))
	return cmd
}

func newServerLeafCommand(name, short string, run func() int) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				_ = cmd.Help()
				return exitCode(2)
			}
			return exitCode(run())
		},
	}
}
