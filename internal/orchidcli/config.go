package orchidcli

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
	Hypervisor string `toml:"hypervisor"`
}

func resolveHypervisor() (string, error) {
	if value := os.Getenv("ORCHID_HYPERVISOR"); value != "" {
		return value, nil
	}

	cfg, path, err := loadConfig()
	if err != nil {
		return "", err
	}
	if cfg.Hypervisor != "" {
		return cfg.Hypervisor, nil
	}

	return "", fmt.Errorf("ORCHID_HYPERVISOR is required or set hypervisor = \"<host>\" in %s, or run `orchid config set hypervisor <host>`", path)
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
	b.WriteString("hypervisor = ")
	b.WriteString(strconv.Quote(cfg.Hypervisor))
	b.WriteString("\n")

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
