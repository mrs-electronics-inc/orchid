package orchidcli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

func runList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	hypervisorFlag := fs.String("hypervisor", "", "SSH host for the libvirt hypervisor")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: orchid list [--hypervisor HOST]")
		return 2
	}

	hypervisor, err := resolveHypervisor(*hypervisorFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	output, err := runHypervisorCommand(context.Background(), hypervisor, "virsh", "-c", "qemu:///system", "list", "--all")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	vms := parseVirshList(output)
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

type virshVM struct {
	Name  string
	State string
}

func runHypervisorCommand(ctx context.Context, hypervisor string, remoteArgs ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		hypervisor,
	}
	args = append(args, remoteArgs...)

	cmd := exec.CommandContext(ctx, "ssh", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("remote command via %s timed out after %s", hypervisor, 10*time.Second)
		}
		return "", fmt.Errorf("remote command via %s failed: %s", hypervisor, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func parseVirshList(output string) []virshVM {
	var vms []virshVM
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if fields[0] == "Id" || strings.HasPrefix(fields[0], "---") {
			continue
		}
		vms = append(vms, virshVM{
			Name:  fields[1],
			State: strings.Join(fields[2:], " "),
		})
	}

	sort.Slice(vms, func(i, j int) bool {
		if vms[i].Name == vms[j].Name {
			return vms[i].State < vms[j].State
		}
		return vms[i].Name < vms[j].Name
	})
	return vms
}
