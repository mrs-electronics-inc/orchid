package cli

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
)

func runList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	hypervisorFlag := fs.String("hypervisor", "", "SSH host for the libvirt hypervisor")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: orchid vm list [--hypervisor HOST]")
		return 2
	}

	hypervisor, err := resolveHypervisor(*hypervisorFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	vms, err := fetchDaemonVMs(hypervisor)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if len(vms) == 0 {
		fmt.Println("No VMs found.")
		return 0
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATE")
	for _, vm := range vms {
		fmt.Fprintf(w, "%s\t%s\n", vm.Name, vm.State)
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
