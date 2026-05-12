package cli

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestBuildCreateVMUserDataExplainsGitCloneAuthFailures(t *testing.T) {
	userData := buildCreateVMUserData(
		"example-vm",
		"example-repo",
		"example.com",
		"git@example.com:org/example-repo.git",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample test@example",
		"-----BEGIN OPENSSH PRIVATE KEY-----\nexample\n-----END OPENSSH PRIVATE KEY-----",
	)

	wantSnippets := []string{
		"orchid: git clone failed.",
		"if this is a private repository, make sure the SSH private key configured with `orchid config set identity-file <path>` can access the repo, then add its public key to your account SSH keys and retry.",
		"write_files:\n",
		"runcmd:\n",
		"/usr/local/bin/orchid-vm-setup.sh",
	}

	for _, snippet := range wantSnippets {
		if !strings.Contains(userData, snippet) {
			t.Fatalf("cloud-init user-data missing %q", snippet)
		}
	}
	if strings.Contains(userData, "bootcmd:\n") {
		t.Fatal("cloud-init user-data should not inject an ssh bootcmd override")
	}
	if strings.Contains(userData, "ssh_authorized_keys:") {
		t.Fatal("cloud-init user-data should not rely on ssh_authorized_keys for vm login")
	}
	if strings.Contains(userData, "/home/dev/.ssh/authorized_keys") {
		t.Fatal("cloud-init user-data should not write the guest authorized_keys file")
	}
	if strings.Contains(userData, "  - su - dev -c \"git clone ") {
		t.Fatal("cloud-init user-data should not inline the git clone command in runcmd")
	}
}

func TestBuildCreateVMUserDataIncludesTimezoneAndGitIdentity(t *testing.T) {
	userData := buildCreateVMUserData(
		"example-vm",
		"example-repo",
		"example.com",
		"git@example.com:org/example-repo.git",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample test@example",
		"-----BEGIN OPENSSH PRIVATE KEY-----\nexample\n-----END OPENSSH PRIVATE KEY-----",
		createVMUserDataExtras{
			Timezone:     "America/New_York",
			GitUserName:  "Test User",
			GitUserEmail: "test@example.com",
		},
	)

	wantSnippets := []string{
		"timezone: \"America/New_York\"",
		"/home/dev/.gitconfig",
		"[user]",
		"name = Test User",
		"email = test@example.com",
	}

	for _, snippet := range wantSnippets {
		if !strings.Contains(userData, snippet) {
			t.Fatalf("cloud-init user-data missing %q", snippet)
		}
	}
}

func TestBuildCreateVMUserDataQuotesTimezone(t *testing.T) {
	userData := buildCreateVMUserData(
		"example-vm",
		"example-repo",
		"example.com",
		"git@example.com:org/example-repo.git",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample test@example",
		"-----BEGIN OPENSSH PRIVATE KEY-----\nexample\n-----END OPENSSH PRIVATE KEY-----",
		createVMUserDataExtras{
			Timezone: ":America/New_York",
		},
	)

	if !strings.Contains(userData, "timezone: \":America/New_York\"") {
		t.Fatalf("cloud-init user-data missing quoted timezone: %q", userData)
	}
}

func TestWaitForGuestCloudInitRetriesTransientSSHErrors(t *testing.T) {
	originalTry := tryGuestCommandDirectFunc
	originalSleep := sleepFunc
	defer func() {
		tryGuestCommandDirectFunc = originalTry
		sleepFunc = originalSleep
	}()

	var calls int
	tryGuestCommandDirectFunc = func(ip, identityFile string, remoteArgs ...string) error {
		calls++
		if calls == 1 {
			return fmt.Errorf("ssh to %s failed: ssh: connect to host %s port 22: Connection refused", ip, ip)
		}
		return nil
	}
	sleepFunc = func(time.Duration) {}

	if err := waitForGuestCloudInit("192.168.122.43", "/tmp/id"); err != nil {
		t.Fatalf("waitForGuestCloudInit returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("waitForGuestCloudInit calls = %d, want 2", calls)
	}
}

func TestWaitForGuestAuthorizedKeyRetriesUntilKeyIsVisible(t *testing.T) {
	originalVirsh := runVirshCommandFunc
	originalSleep := sleepFunc
	defer func() {
		runVirshCommandFunc = originalVirsh
		sleepFunc = originalSleep
	}()

	var calls int
	expectedKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample test@example"
	runVirshCommandFunc = func(args ...string) (string, error) {
		calls++
		if calls == 1 {
			return "", fmt.Errorf("guest agent not ready")
		}
		return expectedKey + "\n", nil
	}
	sleepFunc = func(time.Duration) {}

	if err := waitForGuestAuthorizedKey("demo-vm", "dev", expectedKey, 3, time.Second); err != nil {
		t.Fatalf("waitForGuestAuthorizedKey returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("waitForGuestAuthorizedKey calls = %d, want 2", calls)
	}
}

func TestEnsureGuestAuthorizedKeyWritesKeyViaGuestAgent(t *testing.T) {
	originalVirsh := runVirshCommandFunc
	originalSleep := sleepFunc
	defer func() {
		runVirshCommandFunc = originalVirsh
		sleepFunc = originalSleep
	}()

	expectedKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample test@example"
	var calls int
	var guestExecRequest string
	runVirshCommandFunc = func(args ...string) (string, error) {
		calls++
		switch {
		case len(args) >= 1 && args[0] == "qemu-agent-command" && strings.Contains(args[2], `"execute":"guest-exec"`):
			guestExecRequest = args[2]
			return `{"return":{"pid":7}}`, nil
		case len(args) >= 1 && args[0] == "qemu-agent-command" && strings.Contains(args[2], `"execute":"guest-exec-status"`):
			return `{"return":{"exited":true,"exitcode":0,"out-data":"","err-data":""}}`, nil
		case len(args) >= 1 && args[0] == "get-user-sshkeys":
			return expectedKey + "\n", nil
		default:
			return "", fmt.Errorf("unexpected virsh call: %v", args)
		}
	}
	sleepFunc = func(time.Duration) {}

	if err := ensureGuestAuthorizedKey("demo-vm", "dev", expectedKey, 3, time.Second); err != nil {
		t.Fatalf("ensureGuestAuthorizedKey returned error: %v", err)
	}
	if !strings.Contains(guestExecRequest, "authorized_keys") {
		t.Fatalf("guest exec request = %q, want authorized_keys write", guestExecRequest)
	}
	if !strings.Contains(guestExecRequest, expectedKey) {
		t.Fatalf("guest exec request = %q, want embedded key", guestExecRequest)
	}
	if calls != 3 {
		t.Fatalf("ensureGuestAuthorizedKey calls = %d, want 3", calls)
	}
}

func TestWaitForGuestSSHDirectRetriesAuthErrors(t *testing.T) {
	originalTry := tryGuestCommandDirectFunc
	originalVirsh := runVirshCommandFunc
	originalSleep := sleepFunc
	defer func() {
		tryGuestCommandDirectFunc = originalTry
		runVirshCommandFunc = originalVirsh
		sleepFunc = originalSleep
	}()

	var calls int
	tryGuestCommandDirectFunc = func(ip, identityFile string, remoteArgs ...string) error {
		calls++
		if calls == 1 {
			return fmt.Errorf("ssh to %s failed: dev@%s: Permission denied (publickey).", ip, ip)
		}
		return nil
	}
	runVirshCommandFunc = func(args ...string) (string, error) {
		return "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample test@example\n", nil
	}
	sleepFunc = func(time.Duration) {}

	if err := waitForGuestSSHDirect("demo-vm", "192.168.122.43", "/tmp/id", "fingerprint", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample test@example", 3, time.Second); err != nil {
		t.Fatalf("waitForGuestSSHDirect returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("waitForGuestSSHDirect calls = %d, want 2", calls)
	}
}

func TestGuestAuthorizedKeysDiagnosticsIncludesGuestState(t *testing.T) {
	originalVirsh := runVirshCommandFunc
	originalSleep := sleepFunc
	defer func() {
		runVirshCommandFunc = originalVirsh
		sleepFunc = originalSleep
	}()

	var calls int
	stdout := "== id dev ==\nuid=1000(dev) gid=1000(dev) groups=1000(dev)\n"
	runVirshCommandFunc = func(args ...string) (string, error) {
		calls++
		switch {
		case len(args) >= 1 && args[0] == "get-user-sshkeys":
			return "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample test@example\n", nil
		case len(args) >= 1 && args[0] == "qemu-agent-command" && calls == 2:
			return `{"return":{"pid":7}}`, nil
		case len(args) >= 1 && args[0] == "qemu-agent-command":
			return `{"return":{"exited":true,"exitcode":0,"out-data":"` + base64.StdEncoding.EncodeToString([]byte(stdout)) + `","err-data":""}}`, nil
		default:
			return "", fmt.Errorf("unexpected virsh call: %v", args)
		}
	}
	sleepFunc = func(time.Duration) {}

	got := guestAuthorizedKeysDiagnostics("demo-vm", "dev", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample test@example")
	if !strings.Contains(got, "expected key present: true") {
		t.Fatalf("guestAuthorizedKeysDiagnostics = %q, want key presence", got)
	}
	if !strings.Contains(got, "guest state:") {
		t.Fatalf("guestAuthorizedKeysDiagnostics = %q, want guest state", got)
	}
	if !strings.Contains(got, "uid=1000(dev)") {
		t.Fatalf("guestAuthorizedKeysDiagnostics = %q, want guest exec output", got)
	}
	if calls != 3 {
		t.Fatalf("guestAuthorizedKeysDiagnostics calls = %d, want 3", calls)
	}
}

func TestGuestRepoCheckoutDiagnosticsIncludesCloudInitOutput(t *testing.T) {
	originalVirsh := runVirshCommandFunc
	originalSleep := sleepFunc
	defer func() {
		runVirshCommandFunc = originalVirsh
		sleepFunc = originalSleep
	}()

	var calls int
	output := "== cloud-init output ==\norchid: git clone failed.\nfatal: could not read from remote repository.\n"
	runVirshCommandFunc = func(args ...string) (string, error) {
		calls++
		switch {
		case len(args) >= 1 && args[0] == "qemu-agent-command" && strings.Contains(args[2], `"execute":"guest-exec"`):
			return `{"return":{"pid":7}}`, nil
		case len(args) >= 1 && args[0] == "qemu-agent-command" && strings.Contains(args[2], `"execute":"guest-exec-status"`):
			return `{"return":{"exited":true,"exitcode":0,"out-data":"` + base64.StdEncoding.EncodeToString([]byte(output)) + `","err-data":""}}`, nil
		default:
			return "", fmt.Errorf("unexpected virsh call: %v", args)
		}
	}
	sleepFunc = func(time.Duration) {}

	got := guestRepoCheckoutDiagnostics("demo-vm", "demo-repo")
	if !strings.Contains(got, "cloud-init output") {
		t.Fatalf("guestRepoCheckoutDiagnostics = %q, want cloud-init output", got)
	}
	if !strings.Contains(got, "git clone failed") {
		t.Fatalf("guestRepoCheckoutDiagnostics = %q, want clone failure", got)
	}
	if calls != 2 {
		t.Fatalf("guestRepoCheckoutDiagnostics calls = %d, want 2", calls)
	}
}

func TestWaitForGuestCloudInitFailsFastOnNonTransientErrors(t *testing.T) {
	originalTry := tryGuestCommandDirectFunc
	originalSleep := sleepFunc
	defer func() {
		tryGuestCommandDirectFunc = originalTry
		sleepFunc = originalSleep
	}()

	var calls int
	tryGuestCommandDirectFunc = func(ip, identityFile string, remoteArgs ...string) error {
		calls++
		return fmt.Errorf("ssh to %s failed: sudo cloud-init status --wait exited with status 1", ip)
	}
	sleepFunc = func(time.Duration) {}

	err := waitForGuestCloudInit("192.168.122.43", "/tmp/id")
	if err == nil {
		t.Fatal("waitForGuestCloudInit returned nil, want error")
	}
	if calls != 1 {
		t.Fatalf("waitForGuestCloudInit calls = %d, want 1", calls)
	}
	if !strings.Contains(err.Error(), "cloud-init status --wait") {
		t.Fatalf("waitForGuestCloudInit error = %q, want cloud-init failure", err)
	}
}

func TestWaitForGuestCloudInitRetriesAuthErrors(t *testing.T) {
	originalTry := tryGuestCommandDirectFunc
	originalSleep := sleepFunc
	defer func() {
		tryGuestCommandDirectFunc = originalTry
		sleepFunc = originalSleep
	}()

	var calls int
	tryGuestCommandDirectFunc = func(ip, identityFile string, remoteArgs ...string) error {
		calls++
		if calls == 1 {
			return fmt.Errorf("ssh to %s failed: dev@%s: Permission denied (publickey).", ip, ip)
		}
		return nil
	}
	sleepFunc = func(time.Duration) {}

	if err := waitForGuestCloudInit("192.168.122.43", "/tmp/id"); err != nil {
		t.Fatalf("waitForGuestCloudInit returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("waitForGuestCloudInit calls = %d, want 2", calls)
	}
}

func TestIsTransientSSHError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "connection refused",
			err:  fmt.Errorf("ssh to vm failed: ssh: connect to host 192.168.122.43 port 22: Connection refused"),
			want: true,
		},
		{
			name: "cloud-init failure",
			err:  fmt.Errorf("ssh to vm failed: sudo cloud-init status --wait exited with status 1"),
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTransientSSHError(tc.err); got != tc.want {
				t.Fatalf("isTransientSSHError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
