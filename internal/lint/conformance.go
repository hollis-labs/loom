// Package lint implements Loom's OKF §11 structural conformance checker.
//
// This is deliberately distinct from internal/wiki/verifications.go's
// wiki.VerifyPage, which runs Loom's own deterministic content-quality
// checks (has an H1 heading, has a summary, has a source, extracted link
// count) at write time and persists them to wiki_verifications. Package lint
// checks a narrower, spec-mandated rule instead: OKF §11's own structural
// conformance rule, "every page has parseable frontmatter and a `type`
// field" (see loom-architecture.md §6 and §11) - a shape check that applies
// even to narrative pages otherwise exempt from richer per-type validation.
// Results here are computed on demand, not persisted; nothing in this
// package writes to storage.
package lint

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hollis-labs/loom/internal/compiler"
	"github.com/hollis-labs/loom/internal/domain"
)

// CheckerActor identifies Loom's OKF §11 structural conformance checker as a
// distinct trust-signal source from wiki.StructuralCheckerActor (Loom's
// content-quality checker) and from an actor attestation.
const CheckerActor = "loom-conformance-checker"

const (
	// RuleTypeRequired fires when Page.Type is blank. In normal operation
	// this should never fire: storage.Repository.UpsertPage, and
	// compiler's applyOKFDefaults for compiled-but-not-yet-persisted
	// previews, both default Type to "note" whenever it's left blank, so
	// any page written through Loom's normal paths always has a
	// non-empty Type already. The rule stays in as a defensive backstop
	// for pages that reach wiki_pages some other way (direct SQL, a
	// future import path, a legacy row predating the OKF migration)
	// where that defaulting never ran.
	RuleTypeRequired = "type_required"
	// RuleFrontmatterParseable fires when the page's OKF frontmatter
	// block - compiler.RenderFrontmatter's actual output, the same
	// renderer the export path uses - doesn't round-trip through a YAML
	// parser, or parses but comes back with a blank type. This is the
	// literal "parseable frontmatter" half of OKF §11's rule: it
	// exercises the real rendering path rather than re-checking the Go
	// struct field a second time, so it also catches a broken
	// frontmatter renderer rather than only a blank Type.
	RuleFrontmatterParseable = "frontmatter_parseable"
)

// Finding is one OKF §11 structural conformance violation.
type Finding struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Report is one page's OKF §11 structural conformance result.
type Report struct {
	PageID   int64     `json:"page_id"`
	Slug     string    `json:"slug"`
	Path     string    `json:"path"`
	Passed   bool      `json:"passed"`
	Findings []Finding `json:"findings"`
}

// frontmatterShape mirrors just enough of compiler's rendered YAML to check
// that `type` round-trips - OKF §11 only requires that one field, so this
// deliberately doesn't decode the rest of the frontmatter block.
type frontmatterShape struct {
	Type string `yaml:"type"`
}

// CheckPage runs OKF §11 structural conformance against one page: it has a
// non-empty `type`, and that type actually survives a render-then-parse
// round trip through compiler.RenderFrontmatter.
func CheckPage(p domain.Page) Report {
	report := Report{PageID: p.ID, Slug: p.Slug, Path: p.Path, Passed: true}

	if strings.TrimSpace(p.Type) == "" {
		report.Passed = false
		report.Findings = append(report.Findings, Finding{
			Rule:    RuleTypeRequired,
			Message: "page has no type; OKF §11 requires every page to declare a type",
		})
	}

	block := frontmatterBody(compiler.RenderFrontmatter(p))
	var parsed frontmatterShape
	if err := yaml.Unmarshal([]byte(block), &parsed); err != nil {
		report.Passed = false
		report.Findings = append(report.Findings, Finding{
			Rule:    RuleFrontmatterParseable,
			Message: "page frontmatter did not parse as YAML: " + err.Error(),
		})
	} else if strings.TrimSpace(parsed.Type) == "" {
		report.Passed = false
		report.Findings = append(report.Findings, Finding{
			Rule:    RuleFrontmatterParseable,
			Message: "page frontmatter parsed but its type field is blank",
		})
	}

	return report
}

// CheckBundle runs CheckPage across every page in pages, e.g. every page
// returned for one wiki_bundle.
func CheckBundle(pages []domain.Page) []Report {
	out := make([]Report, 0, len(pages))
	for _, p := range pages {
		out = append(out, CheckPage(p))
	}
	return out
}

// frontmatterBody strips RenderFrontmatter's leading/trailing "---\n"
// delimiter lines, leaving the plain YAML mapping in between for
// yaml.Unmarshal. RenderFrontmatter always wraps its output in exactly this
// shape, so a plain prefix/suffix trim is enough - no need to duplicate its
// YAML-shaping logic here.
func frontmatterBody(block string) string {
	block = strings.TrimPrefix(block, "---\n")
	block = strings.TrimSuffix(block, "---\n")
	return block
}
