package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigRoundTripAndResolve(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	expectedPath := filepath.Join(configHome, "orchid", "config.toml")

	if err := saveConfigUpdate(func(cfg *config) error {
		cfg.Hypervisor = "hypervisor.example"
		cfg.IdentityFile = "/tmp/id_orchid"
		return nil
	}); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("config file missing at %s: %v", expectedPath, err)
	}

	cfg, path, err := loadConfig()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if path != expectedPath {
		t.Fatalf("config path = %s, want %s", path, expectedPath)
	}
	if cfg.Hypervisor != "hypervisor.example" {
		t.Fatalf("hypervisor = %q, want %q", cfg.Hypervisor, "hypervisor.example")
	}
	if cfg.IdentityFile != "/tmp/id_orchid" {
		t.Fatalf("identity file = %q, want %q", cfg.IdentityFile, "/tmp/id_orchid")
	}

	hypervisor, err := resolveHypervisor("")
	if err != nil {
		t.Fatalf("resolving hypervisor: %v", err)
	}
	if hypervisor != "hypervisor.example" {
		t.Fatalf("resolved hypervisor = %q, want %q", hypervisor, "hypervisor.example")
	}

	identityFile, err := resolveIdentityFile("")
	if err != nil {
		t.Fatalf("resolving identity file: %v", err)
	}
	if identityFile != "/tmp/id_orchid" {
		t.Fatalf("resolved identity file = %q, want %q", identityFile, "/tmp/id_orchid")
	}
}

func TestConfigCommands(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	expectedPath := filepath.Join(configHome, "orchid", "config.toml")

	stdout, stderr, err := executeCommand(t, "config", "set", "hypervisor", "hypervisor.example")
	if err != nil {
		t.Fatalf("setting hypervisor: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, expectedPath) {
		t.Fatalf("set output = %q, want path %q", stdout, expectedPath)
	}

	stdout, stderr, err = executeCommand(t, "config", "set", "identity-file", "/tmp/id_orchid")
	if err != nil {
		t.Fatalf("setting identity file: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, expectedPath) {
		t.Fatalf("set output = %q, want path %q", stdout, expectedPath)
	}

	stdout, stderr, err = executeCommand(t, "config", "get", "hypervisor")
	if err != nil {
		t.Fatalf("getting hypervisor: %v\nstderr: %s", err, stderr)
	}
	if got, want := stdout, "hypervisor.example\n"; got != want {
		t.Fatalf("get hypervisor output = %q, want %q", got, want)
	}

	stdout, stderr, err = executeCommand(t, "config", "get", "identity-file")
	if err != nil {
		t.Fatalf("getting identity file: %v\nstderr: %s", err, stderr)
	}
	if got, want := stdout, "/tmp/id_orchid\n"; got != want {
		t.Fatalf("get identity-file output = %q, want %q", got, want)
	}

	stdout, stderr, err = executeCommand(t, "config", "list")
	if err != nil {
		t.Fatalf("listing config: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Config file: "+expectedPath) {
		t.Fatalf("list output = %q, want config path %q", stdout, expectedPath)
	}
	if !strings.Contains(stdout, "hypervisor = \"hypervisor.example\"") {
		t.Fatalf("list output = %q, want hypervisor entry", stdout)
	}
	if !strings.Contains(stdout, "identity_file = \"/tmp/id_orchid\"") {
		t.Fatalf("list output = %q, want identity_file entry", stdout)
	}
}

func TestVMCreateDoesNotWriteConfig(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	identityDir := t.TempDir()
	identityFile := filepath.Join(identityDir, "id_orchid")
	if err := os.WriteFile(identityFile, []byte("PRIVATE KEY\n"), 0o600); err != nil {
		t.Fatalf("writing identity file: %v", err)
	}
	if err := os.WriteFile(identityFile+".pub", []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample test@example\n"), 0o600); err != nil {
		t.Fatalf("writing public key: %v", err)
	}

	originalSubmit := submitDaemonCreateVMFunc
	originalWait := waitForDaemonJobFunc
	submitDaemonCreateVMFunc = func(hypervisor string, req daemonCreateVMRequest) (daemonCreateVMResponse, error) {
		if hypervisor != "hypervisor.example" {
			t.Fatalf("hypervisor = %q, want %q", hypervisor, "hypervisor.example")
		}
		if req.Name != "demo-vm" {
			t.Fatalf("vm name = %q, want %q", req.Name, "demo-vm")
		}
		return daemonCreateVMResponse{JobID: "job-123"}, nil
	}
	waitForDaemonJobFunc = func(hypervisor, jobID string) (daemonJobStatus, error) {
		if hypervisor != "hypervisor.example" {
			t.Fatalf("wait hypervisor = %q, want %q", hypervisor, "hypervisor.example")
		}
		if jobID != "job-123" {
			t.Fatalf("job id = %q, want %q", jobID, "job-123")
		}
		return daemonJobStatus{State: daemonJobStateSucceeded, VMName: "demo-vm"}, nil
	}
	defer func() {
		submitDaemonCreateVMFunc = originalSubmit
		waitForDaemonJobFunc = originalWait
	}()

	if code := vmCreate("demo-vm", "hypervisor.example", identityFile, "https://github.com/org/repo.git"); code != 0 {
		t.Fatalf("vmCreate returned %d", code)
	}

	configPath := filepath.Join(configHome, "orchid", "config.toml")
	if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("config file exists at %s; vm create should not write config", configPath)
	}
}

func executeCommand(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	root := newRootCommand()
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(args)
	root.SilenceErrors = true
	root.SilenceUsage = true

	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}
