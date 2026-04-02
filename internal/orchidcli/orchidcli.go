package orchidcli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	defaultSSHUser   = "dev"
	resolveIPTimeout = 10 * time.Second
)

func Run(args []string) int {
	if len(args) < 1 {
		usage()
	}

	switch args[0] {
	case "connect":
		return runConnect(args[1:])
	case "create-vm":
		return runCreateVM(args[1:])
	case "list":
		return runList(args[1:])
	case "server":
		return runServer(args[1:])
	case "config":
		return runConfig(args[1:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		usage()
	}

	return 0
}

func runConnect(args []string) int {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	user := fs.String("user", defaultSSHUser, "SSH user")
	hypervisorFlag := fs.String("hypervisor", "", "SSH host for the libvirt hypervisor")
	identityFileFlag := fs.String("identity-file", "", "SSH private key used for guest login")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: orchid connect [--hypervisor HOST] [--identity-file PATH] [--user USER] <vm-name> [-- <ssh-args...>]")
		return 2
	}

	vmName := fs.Arg(0)
	remoteArgs := fs.Args()[1:]

	hypervisor, err := resolveHypervisor(*hypervisorFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	identity, err := resolveIdentityFile(*identityFileFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ip, err := resolveIP(hypervisor, vmName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("Connecting to %s (%s)\n", vmName, ip)
	return execSSH(hypervisor, ip, identity, *user, remoteArgs)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: orchid <command> [args]")
	fmt.Fprintln(os.Stderr, "commands: connect, create-vm, list, server, config")
	os.Exit(2)
}

func runConfig(args []string) int {
	if len(args) < 1 {
		usageConfig()
	}

	switch args[0] {
	case "set":
		return runConfigSet(args[1:])
	case "-h", "--help", "help":
		usageConfig()
	default:
		fmt.Fprintf(os.Stderr, "unknown config command: %s\n\n", args[0])
		usageConfig()
	}

	return 0
}

func usageConfig() {
	fmt.Fprintln(os.Stderr, "usage: orchid config set hypervisor <host>")
	fmt.Fprintln(os.Stderr, "       orchid config set identity-file <path>")
	os.Exit(2)
}

func runConfigSet(args []string) int {
	if len(args) != 2 {
		usageConfig()
	}

	var update func(*config) error
	switch args[0] {
	case "hypervisor":
		update = func(cfg *config) error {
			cfg.Hypervisor = args[1]
			return nil
		}
	case "identity-file":
		update = func(cfg *config) error {
			cfg.IdentityFile = args[1]
			return nil
		}
	default:
		usageConfig()
	}

	if err := saveConfigUpdate(update); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	path, err := configPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("Wrote %s\n", path)
	return 0
}

func resolveIP(hypervisor, vmName string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), resolveIPTimeout)
	defer cancel()

	domifaddr, err := runRemoteCommand(ctx, hypervisor, "virsh", "-c", "qemu:///system", "domifaddr", vmName)
	if err != nil {
		return "", err
	}
	if ip := parseDomifaddr(domifaddr); ip != "" {
		return ip, nil
	}

	domiflist, err := runRemoteCommand(ctx, hypervisor, "virsh", "-c", "qemu:///system", "domiflist", vmName)
	if err != nil {
		return "", err
	}
	mac := parseMAC(domiflist)
	if mac == "" {
		return "", fmt.Errorf("querying IP for %s via %s failed: no MAC address found", vmName, hypervisor)
	}

	leases, err := runRemoteCommand(ctx, hypervisor, "virsh", "-c", "qemu:///system", "net-dhcp-leases", "default")
	if err != nil {
		return "", err
	}
	if ip := parseLeaseIP(leases, mac); ip != "" {
		return ip, nil
	}

	return "", fmt.Errorf("querying IP for %s via %s failed: lease not found for MAC %s", vmName, hypervisor, mac)
}

func runRemoteCommand(ctx context.Context, hypervisor string, remoteArgs ...string) (string, error) {
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
			return "", fmt.Errorf("querying IP via %s timed out after %s", hypervisor, resolveIPTimeout)
		}
		return "", fmt.Errorf("querying IP via %s failed: %s", hypervisor, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func parseDomifaddr(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "ipv4") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		if ip := strings.SplitN(fields[3], "/", 2)[0]; ip != "" {
			return ip
		}
	}
	return ""
}

func parseMAC(output string) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		mac := strings.ToLower(strings.TrimSpace(fields[4]))
		if mac != "-" && mac != "mac" {
			return mac
		}
	}
	return ""
}

var leaseIPRe = regexp.MustCompile(`(?i)\b([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)/\d+\b`)

func parseLeaseIP(output, mac string) string {
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(strings.ToLower(line), mac) {
			continue
		}
		if m := leaseIPRe.FindStringSubmatch(line); len(m) == 2 {
			return m[1]
		}
	}
	return ""
}

func execSSH(hypervisor, ip, identityFile, user string, remoteArgs []string) int {
	sshArgs := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=10",
		"-o", fmt.Sprintf("ProxyCommand=ssh -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -W %%h:%%p %s", hypervisor),
	}
	if identityFile != "" {
		sshArgs = append(sshArgs, "-i", identityFile, "-o", "IdentitiesOnly=yes")
	}

	if len(remoteArgs) == 0 {
		sshArgs = append(sshArgs, "-tt")
	}

	sshArgs = append(sshArgs, fmt.Sprintf("%s@%s", user, ip))
	sshArgs = append(sshArgs, remoteArgs...)

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	return 0
}
