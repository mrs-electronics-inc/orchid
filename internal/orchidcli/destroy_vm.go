package orchidcli

import (
	"flag"
	"fmt"
	"os"
)

func runDestroyVM(args []string) int {
	fs := flag.NewFlagSet("destroy-vm", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	hypervisorFlag := fs.String("hypervisor", "", "SSH host for the libvirt hypervisor")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: orchid destroy-vm [--hypervisor HOST] <vm-name>")
		return 2
	}

	vmName := fs.Arg(0)
	hypervisor, err := resolveHypervisor(*hypervisorFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("Removing VM '%s'...\n", vmName)
	if err := submitDaemonDestroyVM(hypervisor, vmName); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("VM '%s' removed.\n", vmName)
	return 0
}
