package cli

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/tabwriter"
)

func vmConnect(user, hypervisorFlag, identityFileFlag, vmName string, remoteArgs []string) int {
	hypervisor, err := resolveHypervisor(hypervisorFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	identity, err := resolveIdentityFile(identityFileFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	ip, err := fetchDaemonVMIP(hypervisor, vmName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("Connecting to %s (%s)\n", vmName, ip)
	return execSSH(hypervisor, ip, identity, user, remoteArgs)
}

func vmCreate(nameFlag, hypervisorFlag, identityFileFlag, repoURL string) int {
	repoName := repoNameFromURL(repoURL)
	vmName := nameFlag
	if vmName == "" {
		vmOwner := currentUsername()
		vmName = vmOwner + "-" + repoName
	}

	hypervisor, err := resolveHypervisor(hypervisorFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	identityFile, err := resolveIdentityFile(identityFileFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	publicKey, err := readPublicKey(identityFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	privateKey, err := os.ReadFile(identityFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading identity file %s: %v\n", identityFile, err)
		return 1
	}

	if err := saveConfigUpdate(func(cfg *config) error {
		cfg.Hypervisor = hypervisor
		cfg.IdentityFile = identityFile
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	req := daemonCreateVMRequest{
		Name:       vmName,
		RepoURL:    repoURL,
		PublicKey:  publicKey,
		PrivateKey: string(privateKey),
	}

	fmt.Printf("Creating VM '%s' for %s...\n", vmName, repoURL)
	submit, err := submitDaemonCreateVM(hypervisor, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	status, err := waitForDaemonJob(hypervisor, submit.JobID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("\nVM '%s' is ready!\n", status.VMName)
	fmt.Printf("  orchid vm connect %s\n\n", status.VMName)
	return 0
}

func vmDestroy(hypervisorFlag, vmName string) int {
	hypervisor, err := resolveHypervisor(hypervisorFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("Removing VM '%s'...\n", vmName)
	if err := submitDaemonDestroyVM(hypervisor, vmName); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("VM '%s' removed.\n", vmName)
	return 0
}

func vmList(hypervisorFlag string) int {
	hypervisor, err := resolveHypervisor(hypervisorFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	vms, err := fetchDaemonVMs(hypervisor)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if len(vms) == 0 {
		fmt.Println("No VMs found.")
		return 0
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATE")
	for _, vm := range vms {
		fmt.Fprintf(w, "%s\t%s\n", vm.Name, vm.State)
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func serverInstall() int {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "orchid server install must be run with sudo")
		return 1
	}

	if err := runCommandChecked("groupadd", "--system", "--force", serverSocketGroup); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if err := ensureBaseImagePresent(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	unit, err := serverUnitFS.ReadFile("systemd/orchid.service")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	unitPath := filepath.Join("/etc/systemd/system", serverUnitName)

	tmpUnit, err := os.CreateTemp("", "orchid-service-*.service")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	tmpUnitPath := tmpUnit.Name()
	if _, err := tmpUnit.Write(unit); err != nil {
		tmpUnit.Close()
		os.Remove(tmpUnitPath)
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := tmpUnit.Close(); err != nil {
		os.Remove(tmpUnitPath)
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.Remove(tmpUnitPath)

	if err := installFile(tmpUnitPath, unitPath, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if err := runCommandChecked("systemctl", "daemon-reload"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := runCommandChecked("systemctl", "enable", "--now", serverUnitName); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := runCommandChecked("systemctl", "restart", serverUnitName); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("Installed %s and refreshed it.\n", serverUnitName)
	fmt.Println("Run `orchid server status` to confirm the daemon is active.")
	return 0
}

func serverProxy() int {
	conn, err := net.Dial("unix", serverSocketPath)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			fmt.Fprintf(os.Stderr, "access denied connecting to %s; the SSH user on the hypervisor must be in the %s group\n", serverSocketPath, serverSocketGroup)
			return 1
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer conn.Close()

	unixConn, _ := conn.(*net.UnixConn)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(conn, os.Stdin)
		if unixConn != nil {
			_ = unixConn.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(os.Stdout, conn)
		if unixConn != nil {
			_ = unixConn.CloseRead()
		}
	}()

	wg.Wait()
	return 0
}

func serverRun() int {
	if err := serveOrchidDaemon(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func serverStatus() int {
	active := strings.TrimSpace(runCommandOutput("systemctl", "is-active", serverUnitName))
	enabled := strings.TrimSpace(runCommandOutput("systemctl", "is-enabled", serverUnitName))
	if active == "" {
		active = "unknown"
	}
	if enabled == "" {
		enabled = "unknown"
	}

	fmt.Printf("%s: enabled=%s active=%s\n", serverUnitName, enabled, active)
	return 0
}
