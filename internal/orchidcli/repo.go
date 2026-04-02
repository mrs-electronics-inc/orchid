package orchidcli

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

func repoNameFromURL(repoURL string) string {
	trimmed := strings.TrimSuffix(repoURL, ".git")
	trimmed = strings.TrimRight(trimmed, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return trimmed
	}
	return parts[len(parts)-1]
}

func repoHostFromURL(repoURL string) string {
	if strings.HasPrefix(repoURL, "git@") {
		parts := strings.SplitN(strings.TrimPrefix(repoURL, "git@"), ":", 2)
		if len(parts) == 2 {
			return parts[0]
		}
	}
	if strings.HasPrefix(repoURL, "ssh://") {
		trimmed := strings.TrimPrefix(repoURL, "ssh://")
		trimmed = strings.TrimPrefix(trimmed, "git@")
		if idx := strings.Index(trimmed, "/"); idx >= 0 {
			return trimmed[:idx]
		}
	}
	if strings.HasPrefix(repoURL, "http://") || strings.HasPrefix(repoURL, "https://") {
		trimmed := strings.TrimPrefix(strings.TrimPrefix(repoURL, "https://"), "http://")
		if idx := strings.Index(trimmed, "/"); idx >= 0 {
			return trimmed[:idx]
		}
	}
	return "github.com"
}

func repoSSHURL(repoURL string) string {
	if strings.HasPrefix(repoURL, "git@") || strings.HasPrefix(repoURL, "ssh://") {
		return repoURL
	}
	if strings.HasPrefix(repoURL, "http://") || strings.HasPrefix(repoURL, "https://") {
		trimmed := strings.TrimPrefix(strings.TrimPrefix(repoURL, "https://"), "http://")
		trimmed = strings.TrimSuffix(trimmed, ".git")
		parts := strings.SplitN(trimmed, "/", 2)
		if len(parts) == 2 {
			return fmt.Sprintf("git@%s:%s.git", parts[0], parts[1])
		}
	}
	return repoURL
}

func readPublicKey(identityFile string) (string, error) {
	if data, err := os.ReadFile(identityFile + ".pub"); err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	cmd := exec.Command("ssh-keygen", "-y", "-f", identityFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("reading public key for %s failed: %s", identityFile, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func currentUsername() string {
	if current, err := user.Current(); err == nil && current.Username != "" {
		return current.Username
	}
	return "dev"
}
