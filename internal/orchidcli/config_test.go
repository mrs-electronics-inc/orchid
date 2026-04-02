package cli

import (
	"os"
	"path/filepath"
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
