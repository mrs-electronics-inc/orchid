package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type createVMUserDataExtras struct {
	Timezone     string
	GitUserName  string
	GitUserEmail string
}

func waitForDaemonJob(hypervisor, jobID string) (daemonJobStatus, error) {
	var last daemonJobStatus
	for {
		status, err := fetchDaemonJob(hypervisor, jobID)
		if err != nil {
			return daemonJobStatus{}, err
		}

		if status.Stage != "" && (status.Stage != last.Stage || status.Message != last.Message || status.State != last.State) {
			if status.Message != "" {
				fmt.Printf("%s: %s\n", status.Stage, status.Message)
			} else {
				fmt.Println(status.Stage)
			}
		}
		last = status

		switch status.State {
		case daemonJobStateSucceeded:
			return status, nil
		case daemonJobStateFailed:
			if status.Error != "" {
				return daemonJobStatus{}, fmt.Errorf("%s", status.Error)
			}
			return daemonJobStatus{}, fmt.Errorf("job %s failed", jobID)
		default:
			time.Sleep(2 * time.Second)
		}
	}
}

func buildCreateVMUserData(vmName, repoName, repoHost, repoURL, publicKey, privateKey string, extras ...createVMUserDataExtras) string {
	var settings createVMUserDataExtras
	if len(extras) > 0 {
		settings = extras[0]
	}

	var b strings.Builder
	b.WriteString("#cloud-config\n")
	b.WriteString("hostname: ")
	b.WriteString(vmName)
	b.WriteString("\n")
	if settings.Timezone != "" {
		b.WriteString("timezone: ")
		b.WriteString(strconv.Quote(settings.Timezone))
		b.WriteString("\n")
	}
	b.WriteString("ssh_pwauth: false\n")
	b.WriteString("users:\n")
	b.WriteString("  - name: dev\n")
	b.WriteString("    sudo: ALL=(ALL) NOPASSWD:ALL\n")
	b.WriteString("    shell: /usr/bin/zsh\n")
	b.WriteString("    lock_passwd: true\n")
	b.WriteString("write_files:\n")
	b.WriteString("  - path: /home/dev/.ssh/id_orchid\n")
	b.WriteString("    permissions: '0600'\n")
	b.WriteString("    owner: dev:dev\n")
	b.WriteString("    content: |\n")
	for _, line := range strings.Split(privateKey, "\n") {
		b.WriteString("      ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("  - path: /home/dev/.ssh/config\n")
	b.WriteString("    permissions: '0600'\n")
	b.WriteString("    owner: dev:dev\n")
	b.WriteString("    content: |\n")
	b.WriteString("      Host ")
	b.WriteString(repoHost)
	b.WriteString("\n")
	b.WriteString("        User git\n")
	b.WriteString("        IdentitiesOnly yes\n")
	b.WriteString("        IdentityFile ~/.ssh/id_orchid\n")
	b.WriteString("        StrictHostKeyChecking accept-new\n")
	b.WriteString("        UserKnownHostsFile ~/.ssh/known_hosts\n")
	b.WriteString("  - path: /home/dev/.zprofile\n")
	b.WriteString("    permissions: '0644'\n")
	b.WriteString("    owner: dev:dev\n")
	b.WriteString("    content: |\n")
	b.WriteString("      cd ")
	b.WriteString(shellQuote("/home/dev/" + repoName))
	b.WriteString("\n")
	if settings.GitUserName != "" || settings.GitUserEmail != "" {
		b.WriteString("  - path: /home/dev/.gitconfig\n")
		b.WriteString("    permissions: '0644'\n")
		b.WriteString("    owner: dev:dev\n")
		b.WriteString("    content: |\n")
		b.WriteString("      [user]\n")
		if settings.GitUserName != "" {
			b.WriteString("        name = ")
			b.WriteString(settings.GitUserName)
			b.WriteString("\n")
		}
		if settings.GitUserEmail != "" {
			b.WriteString("        email = ")
			b.WriteString(settings.GitUserEmail)
			b.WriteString("\n")
		}
	}
	b.WriteString("  - path: /home/dev/.orchid-envrc\n")
	b.WriteString("    permissions: '0644'\n")
	b.WriteString("    owner: dev:dev\n")
	b.WriteString("    content: |\n")
	b.WriteString("      if [ -f flake.nix ]; then\n")
	b.WriteString("        use flake\n")
	b.WriteString("      fi\n")
	// Keep the repo setup logic in a script file so cloud-init never has to parse
	// colon-heavy shell error messages in runcmd.
	b.WriteString("  - path: /usr/local/bin/orchid-vm-setup.sh\n")
	b.WriteString("    permissions: '0755'\n")
	b.WriteString("    owner: root:root\n")
	b.WriteString("    content: |\n")
	b.WriteString("      #!/usr/bin/env bash\n")
	b.WriteString("      set -euo pipefail\n")
	b.WriteString("      su - dev -c \"git clone ")
	b.WriteString(shellQuote(repoURL))
	b.WriteString(" ")
	b.WriteString(shellQuote("/home/dev/" + repoName))
	b.WriteString("\" || { echo 'orchid: git clone failed.' >&2; echo 'orchid: if this is a private repository, make sure the SSH private key configured with `orchid config set identity-file <path>` can access the repo, then add its public key to your account SSH keys and retry.' >&2; exit 1; }\n")
	b.WriteString("      su - dev -c \"printf '.direnv/\\\\n' >> ")
	b.WriteString(shellQuote("/home/dev/" + repoName + "/.git/info/exclude"))
	b.WriteString("\" || true\n")
	b.WriteString("      mv /home/dev/.orchid-envrc ")
	b.WriteString(shellQuote("/home/dev/" + repoName + "/.envrc"))
	b.WriteString("\n")
	b.WriteString("      su - dev -c \"cd ")
	b.WriteString(shellQuote("/home/dev/" + repoName))
	b.WriteString(" && direnv allow\" || true\n")
	b.WriteString("      systemctl restart sshd\n")
	b.WriteString("runcmd:\n")
	b.WriteString("  - /usr/local/bin/orchid-vm-setup.sh\n")
	return b.String()
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func buildMetaData(vmName string) string {
	return "instance-id: " + vmName + "\nlocal-hostname: " + vmName + "\n"
}

func defaultNetworkConfig() string {
	return "version: 2\nethernets:\n  default:\n    match:\n      name: \"e*\"\n    dhcp4: true\n"
}
