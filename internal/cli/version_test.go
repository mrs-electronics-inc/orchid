package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCommand_VersionFlag(t *testing.T) {
	t.Cleanup(func() {
		SetVersion("", "")
	})

	SetVersion("v0.3.0", "abc1234")

	out := &bytes.Buffer{}
	cmd := newRootCommand()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected version flag to succeed, got: %v", err)
	}

	if got := strings.TrimSpace(out.String()); got != "v0.3.0 (abc1234)" {
		t.Fatalf("expected version output %q, got %q", "v0.3.0 (abc1234)", got)
	}
}

func TestRootCommand_VersionFlagWithoutCommit(t *testing.T) {
	t.Cleanup(func() {
		SetVersion("", "")
	})

	SetVersion("v0.3.0", "")

	out := &bytes.Buffer{}
	cmd := newRootCommand()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected version flag to succeed, got: %v", err)
	}

	if got := strings.TrimSpace(out.String()); got != "v0.3.0" {
		t.Fatalf("expected version output %q, got %q", "v0.3.0", got)
	}
}

func TestRootCommand_VersionFlagNormalizesVersionPrefix(t *testing.T) {
	t.Cleanup(func() {
		SetVersion("", "")
	})

	SetVersion("0.3.0", "abc1234")

	out := &bytes.Buffer{}
	cmd := newRootCommand()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected version flag to succeed, got: %v", err)
	}

	if got := strings.TrimSpace(out.String()); got != "v0.3.0 (abc1234)" {
		t.Fatalf("expected version output %q, got %q", "v0.3.0 (abc1234)", got)
	}
}

func TestRootCommand_VersionFlagTruncatesCommitHash(t *testing.T) {
	t.Cleanup(func() {
		SetVersion("", "")
	})

	SetVersion("v0.3.0", "c872008af8a9efb0a424d076df1798e5ff68637f")

	out := &bytes.Buffer{}
	cmd := newRootCommand()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected version flag to succeed, got: %v", err)
	}

	if got := strings.TrimSpace(out.String()); got != "v0.3.0 (c872008)" {
		t.Fatalf("expected version output %q, got %q", "v0.3.0 (c872008)", got)
	}
}
