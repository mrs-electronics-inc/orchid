package orchidcli

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	createVMRetrySleep     = 5 * time.Second
	createVMRetryAttempts  = 20
	createVMVerifyAttempts = 20
	createVMVerifySleep    = 5 * time.Second
)

func (s *daemonJobStore) startCreateVM(req daemonCreateVMRequest) (*daemonJob, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if strings.TrimSpace(req.RepoURL) == "" {
		return nil, fmt.Errorf("repo_url is required")
	}
	if strings.TrimSpace(req.PublicKey) == "" {
		return nil, fmt.Errorf("public_key is required")
	}
	if strings.TrimSpace(req.PrivateKey) == "" {
		return nil, fmt.Errorf("private_key is required")
	}

	job := s.create(daemonJobStatus{
		Stage:   daemonJobStageValidatingRequest,
		Message: "validating create-vm request",
		VMName:  req.Name,
	})

	go runCreateVMJob(job, req)
	return job, nil
}

func runCreateVMJob(job *daemonJob, req daemonCreateVMRequest) {
	vmName := req.Name
	repoName := repoNameFromURL(req.RepoURL)
	repoHost := repoHostFromURL(req.RepoURL)
	cloneURL := repoSSHURL(req.RepoURL)

	job.update(daemonJobStateRunning, daemonJobStageValidatingRequest, "validating create-vm request", vmName, "")

	tmpDir, err := os.MkdirTemp("", "orchid-create-vm-*")
	if err != nil {
		job.fail(daemonJobStageValidatingRequest, "creating temporary workspace", err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)

	privateKeyPath := filepath.Join(tmpDir, "id_orchid")
	if err := os.WriteFile(privateKeyPath, []byte(req.PrivateKey), 0o600); err != nil {
		job.fail(daemonJobStageValidatingRequest, "writing private key", err.Error())
		return
	}

	userDataPath := filepath.Join(tmpDir, "user-data")
	metaDataPath := filepath.Join(tmpDir, "meta-data")
	networkConfigPath := filepath.Join(tmpDir, "network-config")

	if err := os.WriteFile(userDataPath, []byte(buildCreateVMUserData(vmName, repoName, repoHost, cloneURL, strings.TrimSpace(req.PublicKey), req.PrivateKey)), 0o600); err != nil {
		job.fail(daemonJobStageValidatingRequest, "writing user-data", err.Error())
		return
	}
	if err := os.WriteFile(metaDataPath, []byte(buildMetaData(vmName)), 0o600); err != nil {
		job.fail(daemonJobStageValidatingRequest, "writing meta-data", err.Error())
		return
	}
	if err := os.WriteFile(networkConfigPath, []byte(defaultNetworkConfig()), 0o600); err != nil {
		job.fail(daemonJobStageValidatingRequest, "writing network-config", err.Error())
		return
	}

	base, err := resolveSharedBaseImage()
	if err != nil {
		job.fail(daemonJobStageValidatingRequest, "checking shared base image", err.Error())
		return
	}

	virtType := "qemu"
	if _, err := os.Stat("/dev/kvm"); err == nil {
		virtType = "kvm"
	}

	vmDisk := "/var/lib/libvirt/images/" + vmName + ".qcow2"
	seedISO := "/var/lib/libvirt/images/" + vmName + "-seed.iso"

	job.update(daemonJobStateRunning, daemonJobStageCreatingDisk, "creating VM disk", vmName, "")
	if _, err := runLocalCommand("qemu-img", "create", "-f", "qcow2", "-b", base, "-F", "qcow2", vmDisk); err != nil {
		job.fail(daemonJobStageCreatingDisk, "creating VM disk", err.Error())
		return
	}

	job.update(daemonJobStateRunning, daemonJobStageWritingSeed, "writing cloud-init seed", vmName, "")
	if _, err := runLocalCommand("cloud-localds", "--network-config="+networkConfigPath, seedISO, userDataPath, metaDataPath); err != nil {
		job.fail(daemonJobStageWritingSeed, "writing cloud-init seed", err.Error())
		return
	}

	job.update(daemonJobStateRunning, daemonJobStageStartingVM, "starting VM", vmName, "")
	if _, err := runLocalCommand("virt-install",
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
		"--channel", "unix,target_type=virtio,name=org.qemu.guest_agent.0",
		"--graphics", "none",
		"--console", "pty,target_type=serial",
		"--noautoconsole",
		"--import",
	); err != nil {
		job.fail(daemonJobStageStartingVM, "starting VM", err.Error())
		return
	}
	// Mark the domain immediately so the daemon only indexes Orchid-managed VMs.
	if err := setOrchidDomainRole(vmName, orchidMetadataRoleVM); err != nil {
		if cleanupErr := destroyVM(vmName); cleanupErr != nil {
			job.fail(daemonJobStageStartingVM, "tagging VM", fmt.Sprintf("%v (cleanup failed: %v)", err, cleanupErr))
			return
		}
		job.fail(daemonJobStageStartingVM, "tagging VM", err.Error())
		return
	}

	job.update(daemonJobStateRunning, daemonJobStageWaitingForIP, "waiting for IP address", vmName, "")
	ip, err := waitForDaemonVMIP(vmName, createVMRetryAttempts, createVMRetrySleep)
	if err != nil {
		job.fail(daemonJobStageWaitingForIP, "waiting for IP address", err.Error())
		return
	}
	job.update(daemonJobStateRunning, daemonJobStageWaitingForIP, "VM has an IP address", vmName, ip)

	job.update(daemonJobStateRunning, daemonJobStageWaitingForSSH, "waiting for SSH", vmName, ip)
	if err := waitForGuestSSHDirect(ip, privateKeyPath, createVMRetryAttempts, createVMRetrySleep); err != nil {
		job.fail(daemonJobStageWaitingForSSH, "waiting for SSH", err.Error())
		return
	}

	job.update(daemonJobStateRunning, daemonJobStageWaitingForCloudInit, "waiting for cloud-init", vmName, ip)
	if err := waitForGuestCloudInit(ip, privateKeyPath); err != nil {
		job.fail(daemonJobStageWaitingForCloudInit, "waiting for cloud-init", err.Error())
		return
	}

	job.update(daemonJobStateRunning, daemonJobStageWaitingForCloudInit, "verifying repo checkout", vmName, ip)
	if err := waitForGuestRepoCheckout(ip, privateKeyPath, repoName, createVMVerifyAttempts, createVMVerifySleep); err != nil {
		_ = destroyVM(vmName)
		job.fail(daemonJobStageWaitingForCloudInit, "verifying repo checkout", err.Error())
		return
	}

	job.update(daemonJobStateRunning, daemonJobStageWarmingDevShell, "warming dev shell", vmName, ip)
	if err := warmGuestRepoDevShell(ip, privateKeyPath, repoName); err != nil {
		_ = destroyVM(vmName)
		job.fail(daemonJobStageWarmingDevShell, "warming dev shell", err.Error())
		return
	}

	job.update(daemonJobStateSucceeded, daemonJobStageReady, "VM is ready", vmName, ip)
}

func runLocalCommand(args ...string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		log.Printf("command %q failed: %s", strings.Join(args, " "), trimmed)
		if trimmed == "" {
			return "", fmt.Errorf("%s failed: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("%s failed: %s", strings.Join(args, " "), trimmed)
	}
	return strings.TrimSpace(string(output)), nil
}

func resolveSharedBaseImage() (string, error) {
	info, err := os.Lstat(serverBaseLink)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("shared base image %s is missing; run `sudo orchid server build-base` on the hypervisor", serverBaseLink)
		}
		return "", fmt.Errorf("checking %s: %w", serverBaseLink, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return "", fmt.Errorf("refusing to use non-symlink base image at %s; run `sudo orchid server build-base` on the hypervisor", serverBaseLink)
	}
	if _, err := os.Stat(serverBaseLink); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("shared base image %s points to a missing target; run `sudo orchid server build-base` on the hypervisor", serverBaseLink)
		}
		return "", fmt.Errorf("checking %s target: %w", serverBaseLink, err)
	}

	base, err := filepath.EvalSymlinks(serverBaseLink)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", serverBaseLink, err)
	}
	return base, nil
}

func verifyGuestRepoCheckout(ip, identityFile, repoName string) error {
	return runSSHKeyShellCommand(ip, identityFile, fmt.Sprintf("test -d %s && test -f %s", shellQuote("/home/dev/"+repoName), shellQuote("/home/dev/"+repoName+"/.envrc")))
}

func guestRepoCheckoutState(ip, identityFile, repoName string) (string, error) {
	shellCommand := fmt.Sprintf(`
repo=%s
repo_dir=missing
envrc=missing
git_dir=missing
flake=missing
[ -d "$repo" ] && repo_dir=present
[ -f "$repo/.envrc" ] && envrc=present
[ -d "$repo/.git" ] && git_dir=present
[ -f "$repo/flake.nix" ] && flake=present
printf 'repo_dir=%%s envrc=%%s git=%%s flake=%%s\n' "$repo_dir" "$envrc" "$git_dir" "$flake"
`, shellQuote("/home/dev/"+repoName))
	return runSSHKeyCommandOutput(ip, identityFile, "sh", "-lc", shellQuote(shellCommand))
}

func warmGuestRepoDevShell(ip, identityFile, repoName string) error {
	return runSSHKeyShellCommand(ip, identityFile, fmt.Sprintf("cd %s && if [ -f flake.nix ]; then nix develop --command true; fi", shellQuote("/home/dev/"+repoName)))
}

func waitForGuestRepoCheckout(ip, identityFile, repoName string, attempts int, sleep time.Duration) error {
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := verifyGuestRepoCheckout(ip, identityFile, repoName); err == nil {
			return nil
		} else {
			lastErr = err
			if status, statusErr := guestRepoCheckoutState(ip, identityFile, repoName); statusErr == nil {
				fmt.Printf("  repo checkout not ready yet (%d/%d): %s\n", attempt, attempts, status)
				lastErr = fmt.Errorf("repo checkout not ready: %s", status)
			} else {
				fmt.Printf("  repo checkout not ready yet (%d/%d): %v\n", attempt, attempts, err)
			}
		}
		if attempt < attempts {
			time.Sleep(sleep)
		}
	}
	if lastErr == nil {
		return fmt.Errorf("repo checkout did not become ready")
	}
	return lastErr
}

func waitForDaemonVMIP(vmName string, attempts int, sleep time.Duration) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		ip, err := resolveVMIP(vmName)
		if err == nil && ip != "" {
			if attempt > 1 {
				fmt.Printf("  %s IP became available after %d attempt(s): %s\n", vmName, attempt, ip)
			}
			return ip, nil
		}
		lastErr = err
		if err != nil {
			fmt.Printf("  %s IP not ready yet (%d/%d): %v\n", vmName, attempt, attempts, err)
		} else {
			fmt.Printf("  %s IP not ready yet (%d/%d)\n", vmName, attempt, attempts)
		}
		if attempt < attempts {
			time.Sleep(sleep)
		}
	}
	if lastErr == nil {
		return "", fmt.Errorf("VM %s did not receive an IP address", vmName)
	}
	return "", lastErr
}

func waitForGuestSSHDirect(ip, identityFile string, attempts int, sleep time.Duration) error {
	return pollGuestCommandDirect(ip, identityFile, attempts, sleep, "true")
}

func waitForGuestCloudInit(ip, identityFile string) error {
	return tryGuestCommandDirect(ip, identityFile, "sudo", "cloud-init", "status", "--wait")
}

func pollGuestCommandDirect(ip, identityFile string, attempts int, sleep time.Duration, remoteArgs ...string) error {
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := tryGuestCommandDirect(ip, identityFile, remoteArgs...); err == nil {
			return nil
		} else {
			lastErr = err
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

func tryGuestCommandDirect(ip, identityFile string, remoteArgs ...string) error {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
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

	cmd := exec.Command("ssh", args...)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		trimmed := strings.TrimSpace(stderr.String())
		if trimmed == "" {
			return err
		}
		return fmt.Errorf("ssh to %s failed: %s", ip, trimmed)
	}
	return nil
}
