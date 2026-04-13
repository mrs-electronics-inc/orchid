package cli

import "testing"

func TestRepoHelpers(t *testing.T) {
	cases := []struct {
		name     string
		url      string
		repoName string
		host     string
		sshURL   string
	}{
		{
			name:     "https",
			url:      "https://github.com/org/repo.git",
			repoName: "repo",
			host:     "github.com",
			sshURL:   "git@github.com:org/repo.git",
		},
		{
			name:     "ssh",
			url:      "git@github.com:org/repo.git",
			repoName: "repo",
			host:     "github.com",
			sshURL:   "git@github.com:org/repo.git",
		},
		{
			name:     "ssh-url",
			url:      "ssh://git@example.com/org/repo",
			repoName: "repo",
			host:     "example.com",
			sshURL:   "ssh://git@example.com/org/repo",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := repoNameFromURL(tc.url); got != tc.repoName {
				t.Fatalf("repoNameFromURL(%q) = %q, want %q", tc.url, got, tc.repoName)
			}
			if got := repoHostFromURL(tc.url); got != tc.host {
				t.Fatalf("repoHostFromURL(%q) = %q, want %q", tc.url, got, tc.host)
			}
			if got := repoSSHURL(tc.url); got != tc.sshURL {
				t.Fatalf("repoSSHURL(%q) = %q, want %q", tc.url, got, tc.sshURL)
			}
		})
	}
}

func TestParseHelpers(t *testing.T) {
	t.Run("domifaddr", func(t *testing.T) {
		output := "vnet0 ipv4 ignored 192.168.122.55/24\nvnet0 ipv6 ignored fe80::1/64\n"
		if got := parseDomifaddr(output); got != "192.168.122.55" {
			t.Fatalf("parseDomifaddr() = %q, want %q", got, "192.168.122.55")
		}
	})

	t.Run("mac", func(t *testing.T) {
		output := "vnet0 bridge virbr0 virtio 52:54:00:aa:bb:cc\n"
		if got := parseMAC(output); got != "52:54:00:aa:bb:cc" {
			t.Fatalf("parseMAC() = %q, want %q", got, "52:54:00:aa:bb:cc")
		}
	})

	t.Run("lease ip", func(t *testing.T) {
		output := "default 52:54:00:aa:bb:cc 192.168.122.55/24 2024-04-02 12:00:00\n"
		if got := parseLeaseIP(output, "52:54:00:aa:bb:cc"); got != "192.168.122.55" {
			t.Fatalf("parseLeaseIP() = %q, want %q", got, "192.168.122.55")
		}
	})

	t.Run("virsh list", func(t *testing.T) {
		output := `
Id   Name   State
----------------------
1    vm-b   running
-    vm-a   shut off
`
		vms := parseVirshList(output)
		if len(vms) != 2 {
			t.Fatalf("parseVirshList() returned %d items, want 2", len(vms))
		}
		if vms[0].Name != "vm-a" || vms[0].State != "shut off" {
			t.Fatalf("first VM = %#v, want vm-a shut off", vms[0])
		}
		if vms[1].Name != "vm-b" || vms[1].State != "running" {
			t.Fatalf("second VM = %#v, want vm-b running", vms[1])
		}
	})

	t.Run("orchid metadata role", func(t *testing.T) {
		xmlText := `<domain><metadata><role xmlns="https://mrs-electronics-inc/orchid">vm</role></metadata></domain>`
		if !domainHasOrchidRole(xmlText, orchidMetadataRoleVM) {
			t.Fatal("domainHasOrchidRole did not detect vm role")
		}
		if domainHasOrchidRole(xmlText, orchidMetadataRoleBase) {
			t.Fatal("domainHasOrchidRole incorrectly detected base role")
		}
	})

	t.Run("legacy orchid vm", func(t *testing.T) {
		xmlText := `<domain><name>vm</name><source file="/var/lib/libvirt/images/orchid-base-20240101.qcow2"/></domain>`
		if !isLegacyOrchidVM("vm", xmlText) {
			t.Fatal("isLegacyOrchidVM did not detect legacy base image reference")
		}
		if isLegacyOrchidVM("orchid-base-build-20240101", xmlText) {
			t.Fatal("isLegacyOrchidVM should ignore legacy base builder domains")
		}
	})
}
