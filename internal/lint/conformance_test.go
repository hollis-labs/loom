package lint_test

import (
	"strings"
	"testing"

	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/lint"
)

func TestCheckPagePassesWithType(t *testing.T) {
	report := lint.CheckPage(domain.Page{ID: 1, Slug: "runtime-notes", Path: "runtime-notes.md", Type: "reference", Title: "Runtime Notes"})
	if !report.Passed {
		t.Fatalf("report = %+v, want passed", report)
	}
	if len(report.Findings) != 0 {
		t.Fatalf("findings = %+v, want none", report.Findings)
	}
	if report.PageID != 1 || report.Slug != "runtime-notes" || report.Path != "runtime-notes.md" {
		t.Fatalf("report identity fields = %+v", report)
	}
}

func TestCheckPageFailsWithoutType(t *testing.T) {
	report := lint.CheckPage(domain.Page{ID: 2, Slug: "no-type", Title: "No Type"})
	if report.Passed {
		t.Fatalf("report = %+v, want failed", report)
	}
	var sawTypeRequired bool
	for _, f := range report.Findings {
		if f.Rule == lint.RuleTypeRequired {
			sawTypeRequired = true
		}
	}
	if !sawTypeRequired {
		t.Fatalf("findings = %+v, want a %s finding", report.Findings, lint.RuleTypeRequired)
	}
}

func TestCheckPageFailsWhenTypeIsWhitespaceOnly(t *testing.T) {
	// storage.Repository.UpsertPage only defaults Type when it's blank
	// after TrimSpace; a whitespace-only value passed straight into
	// CheckPage (bypassing that write-time path, as a legacy row or a
	// direct-SQL import might) should still fail structural conformance.
	report := lint.CheckPage(domain.Page{ID: 3, Slug: "space-type", Title: "Space Type", Type: "   "})
	if report.Passed {
		t.Fatalf("report = %+v, want failed", report)
	}
}

func TestCheckBundleReportsOneEntryPerPage(t *testing.T) {
	reports := lint.CheckBundle([]domain.Page{
		{ID: 1, Slug: "a", Type: "note"},
		{ID: 2, Slug: "b", Type: ""},
	})
	if len(reports) != 2 {
		t.Fatalf("reports = %+v, want two", reports)
	}
	if !reports[0].Passed {
		t.Fatalf("reports[0] = %+v, want passed", reports[0])
	}
	if reports[1].Passed {
		t.Fatalf("reports[1] = %+v, want failed", reports[1])
	}
}

func TestCheckPageFindingMessagesAreDescriptive(t *testing.T) {
	report := lint.CheckPage(domain.Page{ID: 4, Slug: "no-type", Title: "No Type"})
	if len(report.Findings) == 0 {
		t.Fatalf("expected findings for a typeless page")
	}
	for _, f := range report.Findings {
		if strings.TrimSpace(f.Message) == "" {
			t.Fatalf("finding %+v has an empty message", f)
		}
	}
}
