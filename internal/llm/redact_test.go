package llm_test

import (
	"strings"
	"testing"

	"github.com/hollis-labs/loom/internal/llm"
)

func TestRedactMasksSecretShapedSubstrings(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"anthropic-style key", "export ANTHROPIC_API_KEY=sk-ant-api03-abcdefghijklmnop"},
		{"aws access key", "aws_access_key_id = AKIAABCDEFGHIJKLMNOP"},
		{"bearer token", "Authorization: Bearer abcdef0123456789.token"},
		{"github token", "token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"},
		{"jwt", "id_token=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dQw4w9WgXcQ_examplesig"},
		{"pem private key", "-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAK\n-----END RSA PRIVATE KEY-----"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := llm.Redact(tc.body)
			if !strings.Contains(got, "[REDACTED]") {
				t.Fatalf("Redact(%q) = %q, want a [REDACTED] substring", tc.body, got)
			}
		})
	}
}

func TestRedactLeavesOrdinaryTextAlone(t *testing.T) {
	body := "# Runtime Notes\n\nThe scheduler polls every 30s and logs to stdout."
	if got := llm.Redact(body); got != body {
		t.Fatalf("Redact modified ordinary text: got %q, want %q", got, body)
	}
}

func TestRedactWithPatternsAppliesConfiguredPatternsNotCaughtByBaseline(t *testing.T) {
	// INTERNAL-TOKEN-<digits> is a stand-in for a company-specific secret
	// format (e.g. an internal credential or hostname convention) that a
	// user would configure via config.Filters.RedactPatterns. None of the
	// baseline defaultRedactors match this shape.
	body := "our internal token is INTERNAL-TOKEN-48213, keep it secret"
	baseline := llm.Redact(body)
	if strings.Contains(baseline, "[REDACTED]") {
		t.Fatalf("Redact (baseline only) unexpectedly redacted %q; test assumption invalid", body)
	}

	got := llm.RedactWithPatterns(body, []string{`INTERNAL-TOKEN-\d+`})
	if strings.Contains(got, "INTERNAL-TOKEN-48213") {
		t.Fatalf("RedactWithPatterns(%q) = %q, want the configured pattern redacted", body, got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("RedactWithPatterns(%q) = %q, want a [REDACTED] substring", body, got)
	}
}

func TestRedactWithPatternsStillAppliesBaseline(t *testing.T) {
	body := "export ANTHROPIC_API_KEY=sk-ant-api03-abcdefghijklmnop"
	got := llm.RedactWithPatterns(body, []string{`INTERNAL-TOKEN-\d+`})
	if strings.Contains(got, "sk-ant-api03") {
		t.Fatalf("RedactWithPatterns(%q) = %q, want baseline defaultRedactors to still apply", body, got)
	}
}

func TestRedactWithPatternsSkipsEmptyAndInvalidPatternsWithoutErroring(t *testing.T) {
	body := "keep INTERNAL-TOKEN-999 secret, and also sk-ant-api03-abcdefghijklmnop"
	// "[" is an invalid/unterminated regex; "" is empty. Neither should
	// panic, error, or prevent the valid pattern (or the baseline) from
	// being applied - this path is best-effort pre-LLM safety, not
	// user-facing config validation.
	got := llm.RedactWithPatterns(body, []string{"", "[", `INTERNAL-TOKEN-\d+`})
	if strings.Contains(got, "INTERNAL-TOKEN-999") {
		t.Fatalf("RedactWithPatterns(%q) = %q, want the valid configured pattern still redacted despite invalid siblings", body, got)
	}
	if strings.Contains(got, "sk-ant-api03") {
		t.Fatalf("RedactWithPatterns(%q) = %q, want baseline defaultRedactors to still apply despite invalid patterns", body, got)
	}
}

func TestRedactWithPatternsNilPatternsMatchesRedact(t *testing.T) {
	body := "export ANTHROPIC_API_KEY=sk-ant-api03-abcdefghijklmnop"
	if got, want := llm.RedactWithPatterns(body, nil), llm.Redact(body); got != want {
		t.Fatalf("RedactWithPatterns(body, nil) = %q, want it to match Redact(body) = %q", got, want)
	}
}
