package orchidcli

import (
	"fmt"
	"strings"
	"time"
)

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

func buildCreateVMUserData(vmName, repoName, repoHost, repoURL, publicKey, privateKey string) string {
	var b strings.Builder
	b.WriteString("#cloud-config\n")
	b.WriteString("hostname: ")
	b.WriteString(vmName)
	b.WriteString("\n")
	b.WriteString("ssh_pwauth: false\n")
	b.WriteString("users:\n")
	b.WriteString("  - name: dev\n")
	b.WriteString("    sudo: ALL=(ALL) NOPASSWD:ALL\n")
	b.WriteString("    shell: /usr/bin/zsh\n")
	b.WriteString("    lock_passwd: true\n")
	b.WriteString("    ssh_authorized_keys:\n")
	b.WriteString("      - ")
	b.WriteString(publicKey)
	b.WriteString("\n")
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
	b.WriteString("  - path: /home/dev/.zprofile\n")
	b.WriteString("    permissions: '0644'\n")
	b.WriteString("    owner: dev:dev\n")
	b.WriteString("    content: |\n")
	b.WriteString("      cd /home/dev/")
	b.WriteString(repoName)
	b.WriteString("\n")
	b.WriteString("  - path: /home/dev/")
	b.WriteString(repoName)
	b.WriteString("/.envrc\n")
	b.WriteString("    permissions: '0644'\n")
	b.WriteString("    owner: dev:dev\n")
	b.WriteString("    content: |\n")
	b.WriteString("      if [ -f flake.nix ]; then\n")
	b.WriteString("        use flake\n")
	b.WriteString("      fi\n")
	b.WriteString("runcmd:\n")
	b.WriteString("  - mkdir -p /home/dev/.ssh\n")
	b.WriteString("  - chown -R dev:dev /home/dev/.ssh\n")
	b.WriteString("  - su - dev -c 'cd /home/dev/")
	b.WriteString(repoName)
	b.WriteString(" && direnv allow' || true\n")
	b.WriteString("  - systemctl restart sshd\n")
	return b.String()
}

func buildMetaData(vmName string) string {
	return "instance-id: " + vmName + "\nlocal-hostname: " + vmName + "\n"
}

func defaultNetworkConfig() string {
	return "version: 2\nethernets:\n  default:\n    match:\n      name: \"e*\"\n    dhcp4: true\n"
}
