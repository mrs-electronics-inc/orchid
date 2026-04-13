package cli

import (
	"strings"
	"testing"
)

func TestBuildOrchidBaseUserDataConfiguresUserWritableNpmPrefix(t *testing.T) {
	userData := buildOrchidBaseUserData("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample test@example")

	wantSnippets := []string{
		"export PATH=\"${HOME}/.local/bin:/nix/var/nix/profiles/default/bin:/nix/var/nix/profiles/default/sbin:/usr/local/bin:${PATH}\"",
		"mkdir -p /home/dev/.local",
		"prefix=/home/dev/.local",
		"NPM_CONFIG_PREFIX=/home/dev/.local npm install -g @mariozechner/pi-coding-agent @openai/codex",
		"chown -R dev:dev /home/dev/.local",
	}

	for _, snippet := range wantSnippets {
		if !strings.Contains(userData, snippet) {
			t.Fatalf("cloud-init user-data missing %q", snippet)
		}
	}

	if strings.Contains(userData, "NPM_CONFIG_PREFIX=/usr/local") {
		t.Fatal("cloud-init user-data still installs codex into /usr/local")
	}
}
