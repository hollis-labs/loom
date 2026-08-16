package ingest_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/hollis-labs/loom/internal/ingest"
)

func TestTextSourceBuildsItemWithRedaction(t *testing.T) {
	item, err := (ingest.TextSource{
		Source:    "chatgpt",
		SourceKey: "thread-1",
		Body:      "# Thread Notes\n\nsecret=abc123",
		Redact:    []string{`secret=\w+`},
	}).Item()
	if err != nil {
		t.Fatalf("Item: %v", err)
	}
	if item.Source != "chatgpt" || item.SourceKey != "thread-1" || item.Title != "Thread Notes" {
		t.Fatalf("item = %+v", item)
	}
	if strings.Contains(item.Body, "abc123") || !strings.Contains(item.Body, "[REDACTED]") {
		t.Fatalf("body was not redacted: %q", item.Body)
	}
}

func TestTextSourceInvalidInputsUseSentinel(t *testing.T) {
	if _, err := (ingest.TextSource{}).Item(); !errors.Is(err, ingest.ErrInvalid) {
		t.Fatalf("empty body err = %v, want ErrInvalid", err)
	}
	if _, err := (ingest.TextSource{Body: "body", Redact: []string{"["}}).Item(); !errors.Is(err, ingest.ErrInvalid) {
		t.Fatalf("bad redaction err = %v, want ErrInvalid", err)
	}
}
