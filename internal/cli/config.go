package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

type config struct {
	Hypervisor   string `toml:"hypervisor,omitempty"`
	IdentityFile string `toml:"identity_file,omitempty"`
}

func resolveHypervisor(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	cfg, path, err := loadConfig()
	if err != nil {
		return "", err
	}
	if cfg.Hypervisor != "" {
		return cfg.Hypervisor, nil
	}

	return "", fmt.Errorf("hypervisor is required: set hypervisor = \"<host>\" in %s, run `orchid config set hypervisor <host>`, or pass --hypervisor", path)
}

func resolveIdentityFile(override string) (string, error) {
	if override != "" {
		return override, nil
	}

	cfg, path, err := loadConfig()
	if err != nil {
		return "", err
	}
	if cfg.IdentityFile != "" {
		return cfg.IdentityFile, nil
	}

	return "", fmt.Errorf("identity file is required: set identity_file = \"<path>\" in %s, run `orchid config set identity-file <path>`, or pass --identity-file", path)
}

func loadConfig() (config, string, error) {
	path, err := configPath()
	if err != nil {
		return config{}, "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config{}, path, nil
		}
		return config{}, "", fmt.Errorf("reading %s: %w", path, err)
	}

	var cfg config
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return config{}, "", fmt.Errorf("parsing %s: %w", path, err)
	}

	return cfg, path, nil
}

func writeConfig(path string, cfg config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}

	var b strings.Builder
	b.WriteString("# Orchid CLI configuration.\n")
	if cfg.Hypervisor != "" {
		b.WriteString("hypervisor = ")
		b.WriteString(strconv.Quote(cfg.Hypervisor))
		b.WriteString("\n")
	}
	if cfg.IdentityFile != "" {
		b.WriteString("identity_file = ")
		b.WriteString(strconv.Quote(cfg.IdentityFile))
		b.WriteString("\n")
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "orchid", "config.toml"), nil
}

func loadCurrentConfig() (config, string, error) {
	path, err := configPath()
	if err != nil {
		return config{}, "", err
	}

	cfg, _, err := loadConfig()
	if err != nil {
		return config{}, path, err
	}
	return cfg, path, nil
}

func configDisplayValue(value string) string {
	if value == "" {
		return "<unset>"
	}
	return strconv.Quote(value)
}

func saveConfigUpdate(update func(*config) error) error {
	cfg, path, err := loadConfig()
	if err != nil {
		return err
	}
	if err := update(&cfg); err != nil {
		return err
	}
	return writeConfig(path, cfg)
}
