package cli

import (
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

var (
	version = ""
	commit  = ""
)

func SetVersion(versionValue, commitValue string) {
	version = strings.TrimSpace(versionValue)
	commit = strings.TrimSpace(commitValue)
}

func configureVersion(cmd *cobra.Command) {
	versionValue, commitValue := effectiveVersion()
	if commitValue == "" {
		cmd.Version = versionValue
	} else {
		cmd.Version = fmt.Sprintf("%s (%s)", versionValue, commitValue)
	}
	cmd.SetVersionTemplate("{{.Version}}\n")
}

func effectiveVersion() (string, string) {
	versionValue := normalizeVersion(version)
	commitValue := normalizeCommit(commit)

	if versionValue == "" {
		versionValue = buildInfoVersion()
	}
	if commitValue == "" {
		commitValue = buildInfoCommit()
	}

	versionValue = normalizeVersion(versionValue)
	if versionValue == "" {
		versionValue = "dev"
	}

	return versionValue, commitValue
}

func buildInfoVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}

	versionValue := strings.TrimSpace(info.Main.Version)
	if versionValue == "" || versionValue == "(devel)" {
		return "dev"
	}

	return versionValue
}

func buildInfoCommit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}

	return ""
}

// normalizeVersion keeps release output stable by restoring the canonical
// v-prefixed semver format when build metadata strips it.
func normalizeVersion(versionValue string) string {
	versionValue = strings.TrimSpace(versionValue)
	if versionValue == "" || versionValue == "dev" || strings.HasPrefix(versionValue, "v") {
		return versionValue
	}

	return "v" + versionValue
}

// normalizeCommit shortens long git SHAs so --version stays compact.
func normalizeCommit(commitValue string) string {
	commitValue = strings.TrimSpace(commitValue)
	if commitValue == "" || len(commitValue) <= 7 {
		return commitValue
	}

	return commitValue[:7]
}
