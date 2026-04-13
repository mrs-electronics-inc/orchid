package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestWaitForGuestCloudInitRetriesTransientSSHErrors(t *testing.T) {
	originalTry := tryGuestCommandDirectFunc
	originalSleep := sleepFunc
	defer func() {
		tryGuestCommandDirectFunc = originalTry
		sleepFunc = originalSleep
	}()

	var calls int
	tryGuestCommandDirectFunc = func(ip, identityFile string, remoteArgs ...string) error {
		calls++
		if calls == 1 {
			return fmt.Errorf("ssh to %s failed: ssh: connect to host %s port 22: Connection refused", ip, ip)
		}
		return nil
	}
	sleepFunc = func(time.Duration) {}

	if err := waitForGuestCloudInit("192.168.122.43", "/tmp/id"); err != nil {
		t.Fatalf("waitForGuestCloudInit returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("waitForGuestCloudInit calls = %d, want 2", calls)
	}
}

func TestWaitForGuestCloudInitFailsFastOnNonTransientErrors(t *testing.T) {
	originalTry := tryGuestCommandDirectFunc
	originalSleep := sleepFunc
	defer func() {
		tryGuestCommandDirectFunc = originalTry
		sleepFunc = originalSleep
	}()

	var calls int
	tryGuestCommandDirectFunc = func(ip, identityFile string, remoteArgs ...string) error {
		calls++
		return fmt.Errorf("ssh to %s failed: sudo cloud-init status --wait exited with status 1", ip)
	}
	sleepFunc = func(time.Duration) {}

	err := waitForGuestCloudInit("192.168.122.43", "/tmp/id")
	if err == nil {
		t.Fatal("waitForGuestCloudInit returned nil, want error")
	}
	if calls != 1 {
		t.Fatalf("waitForGuestCloudInit calls = %d, want 1", calls)
	}
	if !strings.Contains(err.Error(), "cloud-init status --wait") {
		t.Fatalf("waitForGuestCloudInit error = %q, want cloud-init failure", err)
	}
}

func TestIsTransientSSHError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "connection refused",
			err:  fmt.Errorf("ssh to vm failed: ssh: connect to host 192.168.122.43 port 22: Connection refused"),
			want: true,
		},
		{
			name: "cloud-init failure",
			err:  fmt.Errorf("ssh to vm failed: sudo cloud-init status --wait exited with status 1"),
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTransientSSHError(tc.err); got != tc.want {
				t.Fatalf("isTransientSSHError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
