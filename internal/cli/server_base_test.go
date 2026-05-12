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
		"nixpkgs#just",
		"NPM_CONFIG_PREFIX=/home/dev/.local npm install -g @mariozechner/pi-coding-agent @openai/codex",
		"HOME=/home/dev PATH=\"/home/dev/.local/bin:${PATH}\" bash -c 'curl -fsSL https://raw.githubusercontent.com/NousResearch/hermes-agent/main/scripts/install.sh | bash -s -- --skip-setup'",
		"chown -R dev:dev /home/dev",
		"model = \"gpt-5.4-mini\"",
		"model_reasoning_effort = \"high\"",
		"[plugins.\"github@openai-curated\"]",
		"enabled = true",
		"[tui]",
		"status_line = [\"model-with-reasoning\", \"current-dir\", \"git-branch\", \"context-used\", \"five-hour-limit\", \"weekly-limit\", \"codex-version\", \"session-id\"]",
	}

	for _, snippet := range wantSnippets {
		if !strings.Contains(userData, snippet) {
			t.Fatalf("cloud-init user-data missing %q", snippet)
		}
	}

	if strings.Contains(userData, "NPM_CONFIG_PREFIX=/usr/local") {
		t.Fatal("cloud-init user-data still installs codex into /usr/local")
	}
	if strings.Contains(userData, "  - just\n") {
		t.Fatal("cloud-init user-data should not install just via apt; nix profile already covers it")
	}
	if strings.Contains(userData, "[projects.") {
		t.Fatal("cloud-init user-data should not embed per-project Codex settings")
	}
}

func TestOrchidBaseFinalizeScriptRemovesGuestSshState(t *testing.T) {
	script := orchidBaseFinalizeScript()
	wantSnippets := []string{
		"sudo cloud-init clean --logs --seed",
		"sudo rm -rf /home/dev/.ssh",
		"sudo rm -f /etc/ssh/ssh_host_*",
		"sudo shutdown -h now",
	}
	for _, snippet := range wantSnippets {
		if !strings.Contains(script, snippet) {
			t.Fatalf("finalize script missing %q", snippet)
		}
	}
}
