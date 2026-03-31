package orchidcli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
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

	repoHost := repoHostFromURL(repoURL)
	cloneURL := repoSSHURL(repoURL)

	tmpDir, err := os.MkdirTemp("", "orchid-create-vm-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(tmpDir)

	userDataPath := filepath.Join(tmpDir, "user-data")
	metaDataPath := filepath.Join(tmpDir, "meta-data")
	networkConfigPath := filepath.Join(tmpDir, "network-config")

	if err := os.WriteFile(userDataPath, []byte(buildCreateVMUserData(vmName, repoName, repoHost, cloneURL, publicKey, string(privateKey))), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "writing user-data: %v\n", err)
		return 1
	}
	if err := os.WriteFile(metaDataPath, []byte(buildMetaData(vmName)), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "writing meta-data: %v\n", err)
		return 1
	}
	if err := os.WriteFile(networkConfigPath, []byte(defaultNetworkConfig()), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "writing network-config: %v\n", err)
		return 1
	}

	remoteTmpDir, err := runRemoteCommand(context.Background(), hypervisor, "mktemp", "-d", "/tmp/orchid-create-vm.XXXXXX")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer runRemoteCommand(context.Background(), hypervisor, "rm", "-rf", remoteTmpDir)

	if err := copyFileToRemote(hypervisor, userDataPath, filepath.Join(remoteTmpDir, "user-data")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := copyFileToRemote(hypervisor, metaDataPath, filepath.Join(remoteTmpDir, "meta-data")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := copyFileToRemote(hypervisor, networkConfigPath, filepath.Join(remoteTmpDir, "network-config")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	base, err := remoteReadlink(hypervisor, "/var/lib/libvirt/images/orchid-base.qcow2")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	virtType, err := runRemoteCommand(context.Background(), hypervisor, "sh", "-lc", "test -e /dev/kvm && echo kvm || echo qemu")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	vmDisk := "/var/lib/libvirt/images/" + vmName + ".qcow2"
	seedISO := "/var/lib/libvirt/images/" + vmName + "-seed.iso"

	fmt.Printf("Creating VM '%s' for %s...\n", vmName, repoURL)
	if _, err := runRemoteCommand(context.Background(), hypervisor, "sudo", "qemu-img", "create", "-f", "qcow2", "-b", base, "-F", "qcow2", vmDisk); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if _, err := runRemoteCommand(context.Background(), hypervisor, "sudo", "cloud-localds", "--network-config="+filepath.Join(remoteTmpDir, "network-config"), seedISO, filepath.Join(remoteTmpDir, "user-data"), filepath.Join(remoteTmpDir, "meta-data")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if _, err := runRemoteCommand(context.Background(), hypervisor, "sudo", "virt-install",
		"--connect", "qemu:///system",
		"--virt-type", virtType,
		"--name", vmName,
		"--memory", "2048",
		"--vcpus", "1",
		"--disk", "path="+vmDisk+",format=qcow2",
		"--disk", "path="+seedISO+",device=cdrom",
		"--security", "type=none",
		"--os-variant", "debian12",
		"--network", "network=default,model=virtio",
		"--graphics", "none",
		"--console", "pty,target_type=serial",
		"--noautoconsole",
		"--import",
	); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Println("Waiting for VM to get an IP...")
	ip, err := waitForVMIP(hypervisor, vmName, 20, 5)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Println("Waiting for SSH to become available...")
	if err := waitForGuestSSH(hypervisor, ip, defaultSSHUser, identityFile, 60, 2); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Println("Waiting for cloud-init to finish...")
	if err := waitForGuestCommand(hypervisor, ip, defaultSSHUser, identityFile, "sudo", "cloud-init", "status", "--wait"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("\nVM '%s' is ready!\n", vmName)
	fmt.Printf("  orchid connect %s\n\n", vmName)
	fmt.Println("cloud-init completed.")
	return 0
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
