package llm

import (
	"regexp"
	"strings"
)

// defaultRedactors match common secret-shaped substrings (API keys, bearer
// tokens, AWS access key IDs, GitHub tokens, JWTs, PEM private key blocks)
// so they do not reach a third-party LLM API. This is a baseline safety net
// specific to sending content off-machine to an LLM Provider; it runs in
// addition to, not instead of, any user-configured redaction patterns
// (config.Filters.RedactPatterns) passed to RedactWithPatterns, and to any
// user-configured ingest redaction (internal/ingest), which is opt-in per
// source.
var defaultRedactors = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{10,}`),                                                     // OpenAI/Anthropic-style secret keys
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),                                                          // AWS access key IDs
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._-]{10,}`),                                          // Bearer tokens
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),                                                // GitHub tokens (ghp_, gho_, ghu_, ghs_, ghr_)
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`),             // JWTs
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`), // PEM private key blocks
}

// Redact replaces secret-shaped substrings in body with "[REDACTED]" before
// it is sent to an LLM Provider, using only the baseline defaultRedactors.
// It is equivalent to RedactWithPatterns(body, nil). Most callers that have
// access to the caller/user-configured redaction patterns
// (config.Filters.RedactPatterns) should prefer RedactWithPatterns so those
// patterns are also applied.
func Redact(body string) string {
	return RedactWithPatterns(body, nil)
}

// RedactWithPatterns replaces secret-shaped substrings in body with
// "[REDACTED]" before it is sent to an LLM Provider. It applies the
// baseline defaultRedactors AND the caller-supplied regex patterns
// (typically config.Filters.RedactPatterns, the same user-configurable
// mechanism internal/ingest applies via compileRedactors/applyRedactions in
// internal/ingest/files.go), so LLM-bound content benefits from both the
// generic secret-shaped baseline and any repo/company-specific patterns a
// user has configured.
//
// Unlike internal/ingest's compileRedactors, an invalid pattern here is
// silently skipped rather than failing the whole call: this sits on the
// pre-LLM safety path (best-effort defense in depth applied automatically
// on every LLM generation), not user-facing config validation, so one
// malformed custom pattern must never prevent the baseline redactors (or
// the other configured patterns) from running before content reaches the
// LLM Provider.
func RedactWithPatterns(body string, patterns []string) string {
	for _, re := range defaultRedactors {
		body = re.ReplaceAllString(body, "[REDACTED]")
	}
	for _, re := range compileRedactPatterns(patterns) {
		body = re.ReplaceAllString(body, "[REDACTED]")
	}
	return body
}

func compileRedactPatterns(patterns []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		out = append(out, re)
	}
	return out
}
