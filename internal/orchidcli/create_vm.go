package orchidcli

import (
	"flag"
	"fmt"
	"os"
)

func runCreateVM(args []string) int {
	fs := flag.NewFlagSet("create-vm", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	name := fs.String("name", "", "Override the VM name")
	hypervisorFlag := fs.String("hypervisor", "", "SSH host for the libvirt hypervisor")
	identityFileFlag := fs.String("identity-file", "", "SSH private key used for VM login and git access")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: orchid create-vm [--identity-file <path>] [--hypervisor <host>] [--name VM] <repo-url>")
		return 2
	}

	repoURL := fs.Arg(0)
	repoName := repoNameFromURL(repoURL)
	vmName := *name
	if vmName == "" {
		vmOwner := currentUsername()
		vmName = vmOwner + "-" + repoName
	}

	hypervisor, err := resolveHypervisor(*hypervisorFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	identityFile, err := resolveIdentityFile(*identityFileFlag)
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
	fmt.Printf("  orchid connect %s\n\n", status.VMName)
	return 0
}
