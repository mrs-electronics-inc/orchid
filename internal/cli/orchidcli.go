package cli

import (
	"context"
	"errors"
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
	cmd := newRootCommand()
	cmd.SetArgs(args)
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	if err := cmd.Execute(); err != nil {
		var exitErr exitCodeError
		if errors.As(err, &exitErr) {
			return exitErr.code
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

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
	cmd.Env = envWithOverride("TERM", "xterm-256color")
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

func envWithOverride(key, value string) []string {
	prefix := key + "="
	env := make([]string, 0, len(os.Environ())+1)
	replaced := false
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, prefix) {
			if !replaced {
				env = append(env, prefix+value)
				replaced = true
			}
			continue
		}
		env = append(env, entry)
	}
	if !replaced {
		env = append(env, prefix+value)
	}
	return env
}
