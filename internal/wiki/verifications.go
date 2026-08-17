package wiki

import (
	"strconv"
	"strings"

	"github.com/hollis-labs/loom/internal/domain"
)

// StructuralCheckerActor identifies Loom's own deterministic structural/content
// checker as the "by" actor on the verification rows it writes. This is a
// distinct trust signal from an actor attestation (human or agent) that a page
// is actually correct - see domain.Verification.
const StructuralCheckerActor = "loom-structural-checker"

func VerifyPage(p domain.Page, links []domain.Link) []domain.Verification {
	out := []domain.Verification{
		{
			Kind:    "heading",
			Status:  status(hasTitleHeading(p.Body, p.Title)),
			Message: headingMessage(p.Body, p.Title),
			By:      StructuralCheckerActor,
		},
		{
			Kind:    "summary",
			Status:  status(strings.TrimSpace(p.Summary) != ""),
			Message: summaryMessage(p.Summary),
			By:      StructuralCheckerActor,
		},
		{
			Kind:    "source",
			Status:  status(strings.TrimSpace(p.Source) != ""),
			Message: sourceMessage(p.Source),
			By:      StructuralCheckerActor,
		},
		{
			Kind:    "links",
			Status:  "info",
			Message: linkMessage(len(links)),
			By:      StructuralCheckerActor,
		},
	}
	return out
}

func hasTitleHeading(body, title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") && strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(line, "# ")), title) {
			return true
		}
	}
	return false
}

func status(ok bool) string {
	if ok {
		return "passed"
	}
	return "warning"
}

func headingMessage(body, title string) string {
	if hasTitleHeading(body, title) {
		return "page contains an H1 matching the title"
	}
	return "page is missing an H1 matching the title"
}

func summaryMessage(summary string) string {
	if strings.TrimSpace(summary) == "" {
		return "page summary is empty"
	}
	return "page summary is present"
}

func sourceMessage(source string) string {
	if strings.TrimSpace(source) == "" {
		return "page source is empty"
	}
	return "page source is present"
}

func linkMessage(n int) string {
	if n == 1 {
		return "1 link extracted"
	}
	return strconv.Itoa(n) + " links extracted"
}
