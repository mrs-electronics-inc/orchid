package orchidcli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultSSHUser     = "dev"
	defaultIPAttempts  = 20
	defaultIPSleep     = 5 * time.Second
	defaultSSHAttempts = 20
	defaultSSHSleep    = 5 * time.Second
)

func Run(args []string) int {
	if len(args) < 1 {
		usage()
	}

	switch args[0] {
	case "connect":
		return runConnect(args[1:])
	case "ip":
		return runIP(args[1:])
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
	user := fs.String("user", envOr("ORCHID_VM_USER", defaultSSHUser), "SSH user")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: orchid connect [--user USER] <vm-name> [-- <ssh-args...>]")
		return 2
	}

	vmName := fs.Arg(0)
	remoteArgs := fs.Args()[1:]

	hypervisor, err := requireEnv("ORCHID_HYPERVISOR")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ip, err := waitForIP(hypervisor, vmName, defaultIPAttempts, defaultIPSleep)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if err := waitForSSH(ip, *user, defaultSSHAttempts, defaultSSHSleep); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("Connecting to %s (%s)\n", vmName, ip)
	return execSSH(ip, *user, remoteArgs)
}

func runIP(args []string) int {
	fs := flag.NewFlagSet("ip", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: orchid ip <vm-name>")
		return 2
	}

	hypervisor, err := requireEnv("ORCHID_HYPERVISOR")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ip, err := waitForIP(hypervisor, fs.Arg(0), defaultIPAttempts, defaultIPSleep)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Println(ip)
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: orchid <connect|ip> [options]")
	os.Exit(2)
}

func waitForIP(hypervisor, vmName string, attempts int, sleep time.Duration) (string, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		ip, err := resolveIP(hypervisor, vmName)
		if err == nil && ip != "" {
			return ip, nil
		}
		lastErr = err
		if i < attempts-1 {
			time.Sleep(sleep)
		}
	}
	if lastErr != nil {
		return "", fmt.Errorf("failed to resolve IP for %s: %w", vmName, lastErr)
	}
	return "", fmt.Errorf("failed to resolve IP for %s", vmName)
}

func waitForSSH(ip, user string, attempts int, sleep time.Duration) error {
	var lastErr error
	for i := 0; i < attempts; i++ {
		cmd := exec.Command("ssh",
			"-o", "BatchMode=yes",
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"-o", "ConnectTimeout=5",
			fmt.Sprintf("%s@%s", user, ip),
			"true",
		)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i < attempts-1 {
			time.Sleep(sleep)
		}
	}
	return fmt.Errorf("ssh to %s never became ready: %v", ip, lastErr)
}

func resolveIP(hypervisor, vmName string) (string, error) {
	script := fmt.Sprintf(`
set -e
ip="$(virsh -c qemu:///system domifaddr %s 2>/dev/null | awk '/ipv4/ && ip == "" {split($4,a,"/"); ip=a[1]} END {print ip}')"
if [ -z "$ip" ]; then
  mac="$(virsh -c qemu:///system domiflist %s 2>/dev/null | awk 'NR > 2 && $5 != "-" && mac == "" {mac=$5} END {print mac}' | tr '[:upper:]' '[:lower:]')"
  if [ -n "$mac" ]; then
    ip="$(virsh -c qemu:///system net-dhcp-leases default 2>/dev/null | awk -v mac="$mac" 'tolower($0) ~ mac && /ipv4/ && ip == "" {split($5,a,"/"); ip=a[1]} END {print ip}')"
  fi
fi
if [ -z "$ip" ]; then
  exit 1
fi
printf '%%s\n' "$ip"
`, shellQuote(vmName), shellQuote(vmName))

	cmd := exec.Command("ssh", hypervisor, "sh", "-lc", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("querying IP for %s via %s failed: %s", vmName, hypervisor, strings.TrimSpace(string(output)))
	}

	return strings.TrimSpace(string(output)), nil
}

func execSSH(ip, user string, remoteArgs []string) int {
	sshArgs := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
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

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func requireEnv(name string) (string, error) {
	if value := os.Getenv(name); value != "" {
		return value, nil
	}
	return "", fmt.Errorf("%s is required", name)
}
