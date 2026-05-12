package cli

import (
	"os"
	"os/exec"
	"strings"
)

func readLocalTimezone() string {
	if timezone, err := runQuietLocalCommand("timedatectl", "show", "-p", "Timezone", "--value"); err == nil {
		if trimmed := strings.TrimSpace(timezone); trimmed != "" {
			return trimmed
		}
	}

	if timezone := strings.TrimSpace(os.Getenv("TZ")); timezone != "" {
		return timezone
	}

	return "UTC"
}

func readLocalGitIdentity() (string, string) {
	return readGitConfigValue("user.name"), readGitConfigValue("user.email")
}

func readGitConfigValue(key string) string {
	value, err := runQuietLocalCommand("git", "config", "--global", "--get", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func runQuietLocalCommand(args ...string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}
