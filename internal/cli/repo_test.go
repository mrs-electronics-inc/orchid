package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadPublicKeyIgnoresStalePubFile(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not available")
	}

	dir := t.TempDir()
	privateKey := filepath.Join(dir, "id_orchid")
	originalPub := filepath.Join(dir, "id_orchid.pub")

	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-f", privateKey, "-C", "orchid-test")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generating key pair failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}

	if err := os.WriteFile(originalPub, []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIstale orchid-test\n"), 0o600); err != nil {
		t.Fatalf("overwriting public key failed: %v", err)
	}

	got, err := readPublicKey(privateKey)
	if err != nil {
		t.Fatalf("readPublicKey returned error: %v", err)
	}
	if strings.Contains(got, "Istale") {
		t.Fatalf("readPublicKey returned stale .pub content: %q", got)
	}

	wantCmd := exec.Command("ssh-keygen", "-y", "-f", privateKey)
	wantOut, err := wantCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deriving public key failed: %v\n%s", err, strings.TrimSpace(string(wantOut)))
	}
	want := strings.TrimSpace(string(wantOut))
	if got != want {
		t.Fatalf("readPublicKey() = %q, want %q", got, want)
	}
}
