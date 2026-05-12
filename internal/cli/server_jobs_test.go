package cli

import (
	"strings"
	"testing"
)

func TestSummarizeJobErrorKeepsOnlyFirstLine(t *testing.T) {
	got := summarizeJobError("repo checkout not ready: repo_dir=missing\n\nguest repo diagnostics:\nline 1\nline 2")
	want := "repo checkout not ready: repo_dir=missing"
	if got != want {
		t.Fatalf("summarizeJobError() = %q, want %q", got, want)
	}
}

func TestSummarizeJobErrorTrimsWhitespaceAndLimitsLength(t *testing.T) {
	long := "  " + strings.Repeat("a", 300) + "  "
	got := summarizeJobError(long)
	if len(got) > 240 {
		t.Fatalf("summarizeJobError() length = %d, want <= 240", len(got))
	}
	if got != strings.Repeat("a", 237)+"..." {
		t.Fatalf("summarizeJobError() = %q, want truncated summary", got)
	}
}
