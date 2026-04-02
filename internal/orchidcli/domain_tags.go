package orchidcli

import (
	"encoding/xml"
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
	output, err := domainXML(domain)
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

func domainHasOrchidRole(xmlText, role string) bool {
	decoder := xml.NewDecoder(strings.NewReader(xmlText))
	for {
		tok, err := decoder.Token()
		if err != nil {
			return false
		}

		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != orchidMetadataRoleKey || !hasOrchidMetadataNamespace(start) {
			continue
		}

		var content strings.Builder
		for {
			tok, err := decoder.Token()
			if err != nil {
				return false
			}

			switch t := tok.(type) {
			case xml.CharData:
				content.WriteString(string(t))
			case xml.EndElement:
				if t.Name.Local == orchidMetadataRoleKey {
					return strings.TrimSpace(content.String()) == role
				}
			}
		}
	}
}

func domainXML(domain string) (string, error) {
	output, err := runLocalCommand("virsh", "-c", "qemu:///system", "dumpxml", "--inactive", domain)
	if err == nil {
		return output, nil
	}

	altOutput, altErr := runLocalCommand("virsh", "-c", "qemu:///system", "dumpxml", domain)
	if altErr == nil {
		return altOutput, nil
	}

	return "", fmt.Errorf("dumpxml for %s failed: inactive view: %v; active view: %v", domain, err, altErr)
}

func hasOrchidMetadataNamespace(start xml.StartElement) bool {
	if start.Name.Space == orchidMetadataURI {
		return true
	}
	for _, attr := range start.Attr {
		if attr.Name.Space == "xmlns" && attr.Value == orchidMetadataURI {
			return true
		}
		if attr.Name.Space == "" && attr.Name.Local == "xmlns" && attr.Value == orchidMetadataURI {
			return true
		}
	}
	return false
}

func isLegacyOrchidVM(domain, xml string) bool {
	if strings.HasPrefix(domain, orchidLegacyBaseBuilder) {
		return false
	}
	return strings.Contains(xml, orchidLegacyBasePrefix)
}
