package orchidcli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"
)

func runCreateVM(args []string) int {
	fs := flag.NewFlagSet("create-vm", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	name := fs.String("name", "", "Override the VM name")
	hypervisorFlag := fs.String("hypervisor", "", "SSH host for the libvirt hypervisor")
	identityFileFlag := fs.String("identity-file", "", "SSH private key used for VM login and git access")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: orchid create-vm [--identity-file <path>] [--hypervisor <host>] [--name VM] <repo-url>")
		return 2
	}

	repoURL := fs.Arg(0)
	repoName := repoNameFromURL(repoURL)
	vmName := *name
	if vmName == "" {
		vmOwner := currentUsername()
		vmName = vmOwner + "-" + repoName
	}

	hypervisor, err := resolveHypervisor(*hypervisorFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	identityFile, err := resolveIdentityFile(*identityFileFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	publicKey, err := readPublicKey(identityFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	privateKey, err := os.ReadFile(identityFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading identity file %s: %v\n", identityFile, err)
		return 1
	}

	if err := saveConfigUpdate(func(cfg *config) error {
		cfg.Hypervisor = hypervisor
		cfg.IdentityFile = identityFile
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	req := daemonCreateVMRequest{
		Name:       vmName,
		RepoURL:    repoURL,
		PublicKey:  publicKey,
		PrivateKey: string(privateKey),
	}

	fmt.Printf("Creating VM '%s' for %s...\n", vmName, repoURL)
	submit, err := submitDaemonCreateVM(hypervisor, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	status, err := waitForDaemonJob(hypervisor, submit.JobID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("\nVM '%s' is ready!\n", status.VMName)
	fmt.Printf("  orchid connect %s\n\n", status.VMName)
	return 0
}

func waitForDaemonJob(hypervisor, jobID string) (daemonJobStatus, error) {
	var last daemonJobStatus
	for {
		status, err := fetchDaemonJob(hypervisor, jobID)
		if err != nil {
			return daemonJobStatus{}, err
		}

		if status.Stage != "" && (status.Stage != last.Stage || status.Message != last.Message || status.State != last.State) {
			if status.Message != "" {
				fmt.Printf("%s: %s\n", status.Stage, status.Message)
			} else {
				fmt.Println(status.Stage)
			}
		}
		last = status

		switch status.State {
		case daemonJobStateSucceeded:
			return status, nil
		case daemonJobStateFailed:
			if status.Error != "" {
				return daemonJobStatus{}, fmt.Errorf("%s", status.Error)
			}
			return daemonJobStatus{}, fmt.Errorf("job %s failed", jobID)
		default:
			time.Sleep(2 * time.Second)
		}
	}
}

func remoteReadlink(hypervisor, path string) (string, error) {
	output, err := runRemoteCommand(context.Background(), hypervisor, "readlink", "-f", path)
	if err != nil {
		return "", err
	}
	return output, nil
}

func copyFileToRemote(hypervisor, localPath, remotePath string) error {
	cmd := exec.Command("ssh",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		hypervisor,
		"sh", "-c", "cat > "+shellQuote(remotePath),
	)

	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	cmd.Stdin = file

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("copying %s to %s failed: %s", localPath, remotePath, strings.TrimSpace(string(output)))
	}
	return nil
}

func waitForVMIP(hypervisor, vmName string, attempts, sleepSeconds int) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		ip, err := resolveIP(hypervisor, vmName)
		if err == nil && ip != "" {
			return ip, nil
		}
		lastErr = err
		if attempt < attempts {
			time.Sleep(time.Duration(sleepSeconds) * time.Second)
		}
	}
	if lastErr == nil {
		return "", fmt.Errorf("VM %s did not receive an IP address", vmName)
	}
	return "", lastErr
}

func waitForGuestSSH(hypervisor, ip, user, identityFile string, attempts, sleepSeconds int) error {
	return pollGuestCommand(hypervisor, ip, user, identityFile, attempts, sleepSeconds, "true")
}

func waitForGuestCommand(hypervisor, ip, user, identityFile string, remoteArgs ...string) error {
	return tryGuestCommand(hypervisor, ip, user, identityFile, remoteArgs...)
}

func pollGuestCommand(hypervisor, ip, user, identityFile string, attempts, sleepSeconds int, remoteArgs ...string) error {
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := tryGuestCommand(hypervisor, ip, user, identityFile, remoteArgs...); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt < attempts {
			time.Sleep(time.Duration(sleepSeconds) * time.Second)
		}
	}
	if lastErr == nil {
		return fmt.Errorf("ssh to %s is not ready", ip)
	}
	return lastErr
}

func tryGuestCommand(hypervisor, ip, user, identityFile string, remoteArgs ...string) error {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ProxyCommand=ssh -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -W %h:%p " + hypervisor,
	}
	if identityFile != "" {
		args = append(args, "-i", identityFile, "-o", "IdentitiesOnly=yes")
	}
	if len(remoteArgs) == 0 {
		args = append(args, "-tt")
	}
	args = append(args, fmt.Sprintf("%s@%s", user, ip))
	args = append(args, remoteArgs...)

	cmd := exec.Command("ssh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func buildCreateVMUserData(vmName, repoName, repoHost, repoURL, publicKey, privateKey string) string {
	var b strings.Builder
	b.WriteString("#cloud-config\n")
	b.WriteString("hostname: ")
	b.WriteString(vmName)
	b.WriteString("\n")
	b.WriteString("ssh_pwauth: false\n")
	b.WriteString("users:\n")
	b.WriteString("  - name: dev\n")
	b.WriteString("    sudo: ALL=(ALL) NOPASSWD:ALL\n")
	b.WriteString("    shell: /usr/bin/zsh\n")
	b.WriteString("    lock_passwd: true\n")
	b.WriteString("    ssh_authorized_keys:\n")
	b.WriteString("      - ")
	b.WriteString(publicKey)
	b.WriteString("\n")
	b.WriteString("write_files:\n")
	b.WriteString("  - path: /home/dev/.ssh/id_orchid\n")
	b.WriteString("    permissions: '0600'\n")
	b.WriteString("    owner: dev:dev\n")
	b.WriteString("    content: |\n")
	for _, line := range strings.Split(privateKey, "\n") {
		b.WriteString("      ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("  - path: /home/dev/.ssh/config\n")
	b.WriteString("    permissions: '0600'\n")
	b.WriteString("    owner: dev:dev\n")
	b.WriteString("    content: |\n")
	b.WriteString("      Host ")
	b.WriteString(repoHost)
	b.WriteString("\n")
	b.WriteString("        User git\n")
	b.WriteString("        IdentitiesOnly yes\n")
	b.WriteString("        IdentityFile ~/.ssh/id_orchid\n")
	b.WriteString("  - path: /home/dev/.zprofile\n")
	b.WriteString("    permissions: '0644'\n")
	b.WriteString("    owner: dev:dev\n")
	b.WriteString("    content: |\n")
	b.WriteString("      cd /home/dev/")
	b.WriteString(repoName)
	b.WriteString("\n")
	b.WriteString("  - path: /home/dev/")
	b.WriteString(repoName)
	b.WriteString("/.envrc\n")
	b.WriteString("    permissions: '0644'\n")
	b.WriteString("    owner: dev:dev\n")
	b.WriteString("    content: |\n")
	b.WriteString("      if [ -f flake.nix ]; then\n")
	b.WriteString("        use flake\n")
	b.WriteString("      fi\n")
	b.WriteString("runcmd:\n")
	b.WriteString("  - mkdir -p /home/dev/.ssh\n")
	b.WriteString("  - chown -R dev:dev /home/dev/.ssh\n")
	b.WriteString("  - su - dev -c 'cd /home/dev/")
	b.WriteString(repoName)
	b.WriteString(" && direnv allow' || true\n")
	b.WriteString("  - systemctl restart sshd\n")
	return b.String()
}

func buildMetaData(vmName string) string {
	return "instance-id: " + vmName + "\nlocal-hostname: " + vmName + "\n"
}

func defaultNetworkConfig() string {
	return "version: 2\nethernets:\n  default:\n    match:\n      name: \"e*\"\n    dhcp4: true\n"
}

func repoNameFromURL(repoURL string) string {
	trimmed := strings.TrimSuffix(repoURL, ".git")
	trimmed = strings.TrimRight(trimmed, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return trimmed
	}
	return parts[len(parts)-1]
}

func repoHostFromURL(repoURL string) string {
	if strings.HasPrefix(repoURL, "git@") {
		parts := strings.SplitN(strings.TrimPrefix(repoURL, "git@"), ":", 2)
		if len(parts) == 2 {
			return parts[0]
		}
	}
	if strings.HasPrefix(repoURL, "ssh://") {
		trimmed := strings.TrimPrefix(repoURL, "ssh://")
		trimmed = strings.TrimPrefix(trimmed, "git@")
		if idx := strings.Index(trimmed, "/"); idx >= 0 {
			return trimmed[:idx]
		}
	}
	if strings.HasPrefix(repoURL, "http://") || strings.HasPrefix(repoURL, "https://") {
		trimmed := strings.TrimPrefix(strings.TrimPrefix(repoURL, "https://"), "http://")
		if idx := strings.Index(trimmed, "/"); idx >= 0 {
			return trimmed[:idx]
		}
	}
	return "github.com"
}

func repoSSHURL(repoURL string) string {
	if strings.HasPrefix(repoURL, "git@") || strings.HasPrefix(repoURL, "ssh://") {
		return repoURL
	}
	if strings.HasPrefix(repoURL, "http://") || strings.HasPrefix(repoURL, "https://") {
		trimmed := strings.TrimPrefix(strings.TrimPrefix(repoURL, "https://"), "http://")
		trimmed = strings.TrimSuffix(trimmed, ".git")
		parts := strings.SplitN(trimmed, "/", 2)
		if len(parts) == 2 {
			return fmt.Sprintf("git@%s:%s.git", parts[0], parts[1])
		}
	}
	return repoURL
}

func readPublicKey(identityFile string) (string, error) {
	if data, err := os.ReadFile(identityFile + ".pub"); err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	cmd := exec.Command("ssh-keygen", "-y", "-f", identityFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("reading public key for %s failed: %s", identityFile, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func currentUsername() string {
	if current, err := user.Current(); err == nil && current.Username != "" {
		return current.Username
	}
	return "dev"
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
