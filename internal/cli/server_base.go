package cli

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	serverImageDir              = "/var/lib/libvirt/images"
	serverDebianBaseImage       = serverImageDir + "/debian-12-base.qcow2"
	serverDebianBaseURL         = "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2"
	baseBuilderIPRetrySleep     = 5 * time.Second
	baseBuilderIPRetryAttempts  = 20
	baseBuilderSSHRetrySleep    = 5 * time.Second
	baseBuilderSSHRetryAttempts = 60
)

const orchidBaseBootstrapScript = `#!/usr/bin/env bash
set -euxo pipefail
exec > >(tee -a /var/log/orchid-bootstrap.log) 2>&1

systemctl restart sshd
update-locale LANG=C.UTF-8

export HOME=/root
curl -L https://nixos.org/nix/install | sh -s -- --daemon --yes

mkdir -p /etc/nix /etc/profile.d
if ! grep -q '^experimental-features = .*flakes' /etc/nix/nix.conf 2>/dev/null; then
  printf '\nexperimental-features = nix-command flakes\n' >> /etc/nix/nix.conf
fi

cat > /etc/profile.d/orchid-path.sh <<'ORCHID_PATH'
export PATH="${HOME}/.local/bin:/nix/var/nix/profiles/default/bin:/nix/var/nix/profiles/default/sbin:/usr/local/bin:${PATH}"
ORCHID_PATH
chmod 0644 /etc/profile.d/orchid-path.sh

export PATH="/nix/var/nix/profiles/default/bin:/nix/var/nix/profiles/default/sbin:${PATH}"
nix profile install nixpkgs#helix nixpkgs#zellij nixpkgs#nodejs nixpkgs#go

systemctl enable --now qemu-guest-agent

ln -sf /usr/bin/fdfind /usr/local/bin/fd

rm -rf /usr/local/share/oh-my-zsh
git clone --depth 1 https://github.com/ohmyzsh/ohmyzsh.git /usr/local/share/oh-my-zsh

mkdir -p /home/dev/.local
chown dev:dev /home/dev/.local

cat > /home/dev/.npmrc <<'ORCHID_NPMRC'
prefix=/home/dev/.local
ORCHID_NPMRC
chown dev:dev /home/dev/.npmrc

NPM_CONFIG_PREFIX=/home/dev/.local npm install -g @mariozechner/pi-coding-agent @openai/codex
HOME=/home/dev PATH="/home/dev/.local/bin:${PATH}" bash -c 'curl -fsSL https://raw.githubusercontent.com/NousResearch/hermes-agent/main/scripts/install.sh | bash -s -- --skip-setup'
chown -R dev:dev /home/dev/.local
chown -R dev:dev /home/dev/.hermes

usermod -s /usr/bin/zsh dev

cat > /home/dev/.zshenv <<'ORCHID_ZSHENV'
export PATH="${HOME}/.local/bin:/nix/var/nix/profiles/default/bin:/nix/var/nix/profiles/default/sbin:/usr/local/bin:${PATH}"
ORCHID_ZSHENV
chown dev:dev /home/dev/.zshenv

cat > /home/dev/.zshrc <<'ORCHID_ZSHRC'
export ZSH=/usr/local/share/oh-my-zsh
ZSH_THEME="robbyrussell"
plugins=(git)

source "${ZSH}/oh-my-zsh.sh"
eval "$(direnv hook zsh)"
ORCHID_ZSHRC
chown dev:dev /home/dev/.zshrc

mkdir -p /home/dev/.codex
cat > /home/dev/.codex/config.toml <<'ORCHID_CODEX'
approval_policy = "never"
sandbox_mode = "danger-full-access"
model = "gpt-5.4-mini"
model_reasoning_effort = "high"

[features]
guardian_approval = true

[plugins."github@openai-curated"]
enabled = true

[tui]
status_line = ["model-with-reasoning", "current-dir", "git-branch", "context-used", "five-hour-limit", "weekly-limit", "codex-version", "session-id"]
ORCHID_CODEX
chown -R dev:dev /home/dev/.codex
`

func buildOrchidBaseImage() error {
	if err := ensureDebianBaseImagePresent(); err != nil {
		return err
	}
	if err := os.MkdirAll(serverImageDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", serverImageDir, err)
	}
	if info, err := os.Lstat(serverBaseLink); err == nil && info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("refusing to overwrite non-symlink base image at %s", serverBaseLink)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("checking %s: %w", serverBaseLink, err)
	}

	now := time.Now().UTC()
	timestamp := now.Format("20060102150405")
	baseVersion := fmt.Sprintf("orchid-base-%s.qcow2", timestamp)
	buildVM := fmt.Sprintf("orchid-base-build-%s", timestamp)
	baseImage := filepath.Join(serverImageDir, baseVersion)
	buildDisk := filepath.Join(serverImageDir, buildVM+".qcow2")
	seedISO := filepath.Join(serverImageDir, buildVM+"-seed.iso")

	tmpDir, err := os.MkdirTemp("", buildVM+".XXXXXX")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	builderKeyPath, builderPublicKey, err := createBuilderSSHKeyPair(tmpDir, buildVM)
	if err != nil {
		return err
	}

	cleanup := func() {
		_ = exec.Command("virsh", "-c", "qemu:///system", "destroy", buildVM).Run()
		_ = exec.Command("virsh", "-c", "qemu:///system", "undefine", buildVM).Run()
		_ = os.Remove(seedISO)
		_ = os.Remove(buildDisk)
	}
	defer cleanup()

	if err := writeOrchidBaseSeedFiles(tmpDir, buildVM, builderPublicKey); err != nil {
		return err
	}

	fmt.Printf("Creating shared Orchid base image '%s'...\n", baseVersion)
	if _, err := runLocalCommand("qemu-img", "create", "-f", "qcow2", "-b", serverDebianBaseImage, "-F", "qcow2", buildDisk, "30G"); err != nil {
		return fmt.Errorf("creating base disk: %w", err)
	}

	if _, err := runLocalCommand("cloud-localds", "--network-config="+filepath.Join(tmpDir, "network-config"), seedISO, filepath.Join(tmpDir, "user-data"), filepath.Join(tmpDir, "meta-data")); err != nil {
		return fmt.Errorf("creating cloud-init seed: %w", err)
	}

	virtType := selectVirtType()
	if _, err := runLocalCommand("virt-install",
		"--connect", "qemu:///system",
		"--virt-type", virtType,
		"--name", buildVM,
		"--memory", "2048",
		"--vcpus", "1",
		"--disk", "path="+buildDisk+",format=qcow2",
		"--disk", "path="+seedISO+",device=cdrom",
		"--security", "type=none",
		"--os-variant", "debian12",
		"--network", "network=default,model=virtio",
		"--channel", "unix,target_type=virtio,name=org.qemu.guest_agent.0",
		"--graphics", "none",
		"--console", "pty,target_type=serial",
		"--noautoconsole",
		"--import",
	); err != nil {
		return fmt.Errorf("starting base build VM: %w", err)
	}
	// Tag the builder so daemon queries can keep it out of normal VM listings.
	if err := setOrchidDomainRole(buildVM, orchidMetadataRoleBase); err != nil {
		return fmt.Errorf("tagging base build VM: %w", err)
	}

	fmt.Println("Waiting for base builder VM to get an IP via guest agent or DHCP...")
	ip, err := waitForDaemonVMIP(buildVM, baseBuilderIPRetryAttempts, baseBuilderIPRetrySleep)
	if err != nil {
		return fmt.Errorf("waiting for base builder IP: %w", err)
	}

	fmt.Println("Waiting for SSH to become available...")
	if err := waitForSSHKey(ip, builderKeyPath, baseBuilderSSHRetryAttempts, baseBuilderSSHRetrySleep); err != nil {
		return fmt.Errorf("waiting for base builder SSH: %w", err)
	}

	fmt.Println("Waiting for cloud-init to finish...")
	if err := runSSHKeyCommand(ip, builderKeyPath, "sudo", "cloud-init", "status", "--wait"); err != nil {
		return fmt.Errorf("waiting for base builder cloud-init: %w", err)
	}

	fmt.Println("Cleaning the image for cloning...")
	if err := runSSHKeyShellCommand(ip, builderKeyPath, `
sudo cloud-init clean --logs --seed &&
sudo rm -f /etc/ssh/ssh_host_* &&
sudo truncate -s 0 /etc/machine-id &&
sudo rm -f /var/lib/dbus/machine-id &&
sudo sync &&
sudo shutdown -h now
`); err != nil {
		return fmt.Errorf("cleaning base image: %w", err)
	}

	fmt.Println("Waiting for builder VM to shut down...")
	if err := waitForDomainState(buildVM, "shut off", 60, 2); err != nil {
		return fmt.Errorf("waiting for base builder shutdown: %w", err)
	}

	if _, err := runLocalCommand("virsh", "-c", "qemu:///system", "undefine", buildVM); err != nil {
		return fmt.Errorf("undefining base builder VM: %w", err)
	}

	if err := os.Rename(buildDisk, baseImage); err != nil {
		return fmt.Errorf("finalizing base image: %w", err)
	}
	if err := refreshBaseSymlink(baseVersion); err != nil {
		return err
	}

	fmt.Println("")
	fmt.Println("Shared Orchid base image is ready.")
	fmt.Printf("  %s\n", baseImage)
	fmt.Println("")
	fmt.Println("Current base image link:")
	fmt.Printf("  %s -> %s\n", serverBaseLink, baseVersion)
	fmt.Println("")
	fmt.Println("Old versioned Orchid base images are kept in place so existing overlays keep working.")
	return nil
}

func ensureDebianBaseImagePresent() error {
	if _, err := os.Stat(serverDebianBaseImage); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking %s: %w", serverDebianBaseImage, err)
	}

	fmt.Printf("Downloading Debian base image to %s...\n", serverDebianBaseImage)
	if err := downloadFile(serverDebianBaseImage, serverDebianBaseURL); err != nil {
		return err
	}
	return nil
}

func createBuilderSSHKeyPair(tmpDir, buildVM string) (string, string, error) {
	privateKeyPath := filepath.Join(tmpDir, buildVM)
	if _, err := runLocalCommand("ssh-keygen", "-t", "ed25519", "-N", "", "-f", privateKeyPath, "-C", buildVM); err != nil {
		return "", "", fmt.Errorf("creating builder SSH key pair: %w", err)
	}

	publicKeyPath := privateKeyPath + ".pub"
	publicKeyData, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return "", "", fmt.Errorf("reading builder public key: %w", err)
	}
	return privateKeyPath, strings.TrimSpace(string(publicKeyData)), nil
}

func writeOrchidBaseSeedFiles(tmpDir, buildVM, publicKey string) error {
	userData := buildOrchidBaseUserData(publicKey)
	metaData := "instance-id: " + buildVM + "\nlocal-hostname: orchid-base\n"
	networkConfig := "version: 2\nethernets:\n  default:\n    match:\n      name: \"e*\"\n    dhcp4: true\n"

	if err := os.WriteFile(filepath.Join(tmpDir, "user-data"), []byte(userData), 0o600); err != nil {
		return fmt.Errorf("writing user-data: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "meta-data"), []byte(metaData), 0o600); err != nil {
		return fmt.Errorf("writing meta-data: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "network-config"), []byte(networkConfig), 0o600); err != nil {
		return fmt.Errorf("writing network-config: %w", err)
	}
	return nil
}

func buildOrchidBaseUserData(publicKey string) string {
	var b strings.Builder
	b.WriteString("#cloud-config\n")
	b.WriteString("hostname: orchid-base\n")
	b.WriteString("ssh_pwauth: false\n")
	b.WriteString("locale: false\n")
	b.WriteString("users:\n")
	b.WriteString("  - name: dev\n")
	b.WriteString("    sudo: ALL=(ALL) NOPASSWD:ALL\n")
	b.WriteString("    shell: /bin/bash\n")
	b.WriteString("    lock_passwd: true\n")
	b.WriteString("    ssh_authorized_keys:\n")
	b.WriteString("      - ")
	b.WriteString(publicKey)
	b.WriteString("\n")
	b.WriteString("packages:\n")
	for _, pkg := range []string{"git", "curl", "locales", "xz-utils", "ripgrep", "fd-find", "zsh", "direnv", "qemu-guest-agent"} {
		b.WriteString("  - ")
		b.WriteString(pkg)
		b.WriteString("\n")
	}
	b.WriteString("package_update: true\n")
	b.WriteString("write_files:\n")
	b.WriteString("  - path: /etc/ssh/sshd_config.d/orchid.conf\n")
	b.WriteString("    content: |\n")
	b.WriteString("      PasswordAuthentication no\n")
	b.WriteString("  - path: /usr/local/bin/orchid-bootstrap.sh\n")
	b.WriteString("    permissions: '0755'\n")
	b.WriteString("    content: |\n")
	for _, line := range strings.Split(orchidBaseBootstrapScript, "\n") {
		b.WriteString("      ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("runcmd:\n")
	b.WriteString("  - /usr/local/bin/orchid-bootstrap.sh\n")
	return b.String()
}

func waitForSSHKey(ip, identityFile string, attempts int, sleep time.Duration) error {
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := runSSHKeyCommand(ip, identityFile, "true"); err == nil {
			if attempt > 1 {
				fmt.Printf("  SSH to %s became available after %d attempt(s)\n", ip, attempt)
			}
			return nil
		} else {
			lastErr = err
			fmt.Printf("  SSH to %s not ready yet (%d/%d): %v\n", ip, attempt, attempts, err)
		}
		if attempt < attempts {
			time.Sleep(sleep)
		}
	}
	if lastErr == nil {
		return fmt.Errorf("ssh to %s is not ready", ip)
	}
	return lastErr
}

func runSSHKeyCommand(ip, identityFile string, remoteArgs ...string) error {
	_, err := runSSHKeyCommandOutput(ip, identityFile, remoteArgs...)
	return err
}

func runSSHKeyCommandOutput(ip, identityFile string, remoteArgs ...string) (string, error) {
	args := sshKeyArgs(ip, identityFile, remoteArgs...)
	cmd := exec.Command("ssh", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		command := strings.Join(remoteArgs, " ")
		log.Printf("ssh to %s running %q failed: %s", ip, command, trimmed)
		if trimmed == "" {
			return "", fmt.Errorf("ssh to %s running %q failed: %w", ip, command, err)
		}
		return "", fmt.Errorf("ssh to %s running %q failed: %s", ip, command, trimmed)
	}
	return strings.TrimSpace(string(output)), nil
}

func runSSHKeyShellCommand(ip, identityFile, shellCommand string) error {
	return runSSHKeyCommand(ip, identityFile, "sh", "-lc", shellQuote(shellCommand))
}

func sshKeyArgs(ip, identityFile string, remoteArgs ...string) []string {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		"-o", "LogLevel=ERROR",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
	}
	if identityFile != "" {
		args = append(args, "-i", identityFile, "-o", "IdentitiesOnly=yes")
	}
	if len(remoteArgs) == 0 {
		args = append(args, "-tt")
	}
	args = append(args, "dev@"+ip)
	args = append(args, remoteArgs...)
	return args
}

func downloadFile(dstPath, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("downloading %s failed: %s: %s", url, resp.Status, strings.TrimSpace(string(body)))
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(dstPath), filepath.Base(dstPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file for %s: %w", dstPath, err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		_ = os.Remove(tmpPath)
	}()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return fmt.Errorf("writing %s: %w", dstPath, err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		return fmt.Errorf("moving %s into place: %w", dstPath, err)
	}
	return nil
}

func selectVirtType() string {
	if _, err := os.Stat("/dev/kvm"); err == nil {
		return "kvm"
	}
	return "qemu"
}

func waitForDomainState(vmName, desiredState string, attempts int, sleep time.Duration) error {
	var lastState string
	for attempt := 1; attempt <= attempts; attempt++ {
		output, err := runLocalCommand("virsh", "-c", "qemu:///system", "domstate", vmName)
		if err == nil {
			state := strings.TrimSpace(output)
			lastState = state
			if state == desiredState {
				return nil
			}
		}
		if attempt < attempts {
			time.Sleep(sleep)
		}
	}
	if lastState == "" {
		return fmt.Errorf("domain %s did not reach state %s", vmName, desiredState)
	}
	return fmt.Errorf("domain %s reached state %s instead of %s", vmName, lastState, desiredState)
}

func refreshBaseSymlink(baseVersion string) error {
	if err := os.Remove(serverBaseLink); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing %s: %w", serverBaseLink, err)
	}
	if err := os.Symlink(baseVersion, serverBaseLink); err != nil {
		return fmt.Errorf("updating %s: %w", serverBaseLink, err)
	}
	return nil
}
