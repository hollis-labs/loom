package wiki_test

import (
	"testing"

	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/wiki"
)

func TestVerifyPageReportsDeterministicChecks(t *testing.T) {
	verifications := wiki.VerifyPage(domain.Page{Title: "Runtime", Summary: "notes", Body: "# Runtime\n\nBody", Source: "runtime.md"}, []domain.Link{{Target: "other"}})
	if len(verifications) != 4 {
		t.Fatalf("verifications = %+v, want four", verifications)
	}
	for _, verification := range verifications {
		if verification.Status == "warning" {
			t.Fatalf("unexpected warning verification: %+v", verification)
		}
	}
	if verifications[3].Kind != "links" || verifications[3].Message != "1 link extracted" {
		t.Fatalf("link verification = %+v", verifications[3])
	}
}

func TestVerifyPageWarnsForMissingMetadata(t *testing.T) {
	verifications := wiki.VerifyPage(domain.Page{Title: "Runtime", Body: "Body"}, nil)
	warnings := 0
	for _, verification := range verifications {
		if verification.Status == "warning" {
			warnings++
		}
	}
	if warnings != 3 {
		t.Fatalf("verifications = %+v, want three warnings", verifications)
	}
}
