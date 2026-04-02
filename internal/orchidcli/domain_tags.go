package orchidcli

import (
	"fmt"
	"strings"
)

const (
	orchidMetadataURI       = "https://mrs-electronics-inc/orchid"
	orchidMetadataRoleKey   = "role"
	orchidMetadataRoleVM    = "vm"
	orchidMetadataRoleBase  = "base"
	orchidLegacyBasePrefix  = "/var/lib/libvirt/images/orchid-base-"
	orchidLegacyBaseBuilder = "orchid-base-build-"
)

func setOrchidDomainRole(domain, role string) error {
	if strings.TrimSpace(domain) == "" {
		return fmt.Errorf("domain is required")
	}
	if strings.TrimSpace(role) == "" {
		return fmt.Errorf("role is required")
	}

	metadata := fmt.Sprintf("<%s xmlns=\"%s\">%s</%s>", orchidMetadataRoleKey, orchidMetadataURI, role, orchidMetadataRoleKey)
	_, err := runLocalCommand(
		"virsh",
		"-c", "qemu:///system",
		"metadata", domain, orchidMetadataURI,
		"--config",
		"--key", orchidMetadataRoleKey,
		"--set", metadata,
	)
	if err != nil {
		return fmt.Errorf("tagging %s as %s: %w", domain, role, err)
	}
	return nil
}

func domainIsOrchidVM(domain string) (bool, error) {
	output, err := runLocalCommand("virsh", "-c", "qemu:///system", "dumpxml", domain)
	if err != nil {
		return false, err
	}

	if domainHasOrchidRole(output, orchidMetadataRoleVM) {
		return true, nil
	}
	if domainHasOrchidRole(output, orchidMetadataRoleBase) {
		return false, nil
	}
	if isLegacyOrchidVM(domain, output) {
		return true, nil
	}
	return false, nil
}

func domainHasOrchidRole(xml, role string) bool {
	return strings.Contains(xml, "<"+orchidMetadataRoleKey+">"+role+"</"+orchidMetadataRoleKey+">") ||
		strings.Contains(xml, "<"+orchidMetadataRoleKey+" xmlns=\""+orchidMetadataURI+"\">"+role+"</"+orchidMetadataRoleKey+">")
}

func isLegacyOrchidVM(domain, xml string) bool {
	if strings.HasPrefix(domain, orchidLegacyBaseBuilder) {
		return false
	}
	return strings.Contains(xml, orchidLegacyBasePrefix)
}
