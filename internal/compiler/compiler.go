package compiler

import (
	"encoding/json"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/storage"
)

// GeneratedBy identifies Loom's deterministic compiler as the "producer" actor
// for OKF's trust family (domain.Page.GeneratedBy).
const GeneratedBy = "loom-compiler"

type Request struct {
	Title string `json:"title"`
	Slug  string `json:"slug"`
	// Type is OKF's one REQUIRED field (open vocabulary). Defaults to "note"
	// in the repository layer when left blank.
	Type    string `json:"type"`
	Summary string `json:"summary"`
	// Description is OKF's frontmatter field for page description; defaults
	// to Summary in the repository layer when left blank.
	Description string `json:"description"`
	Body        string `json:"body"`
	Source      string `json:"source"`
	Template    string `json:"template"`
	// GenerationMode selects which compiler path authors Body: the
	// deterministic template compiler (default) or an LLM Provider
	// (internal/llm), see CompileWikiPageWithLLM. Defaults to
	// GenerationModeDeterministic when left blank.
	GenerationMode GenerationMode `json:"generation_mode,omitempty"`
}

type Result struct {
	Page domain.Page `json:"page"`
	// Document is a rendered preview of Page as a full OKF markdown
	// document: an OKF frontmatter block (see RenderFrontmatter) followed
	// by Page.Body. It exists purely as a view for callers that want to
	// show or export the page's OKF shape; it is never persisted, and
	// rendering it never mutates Page.Body or Page.ContentHash.
	Document string `json:"document"`
}

func CompileWikiPage(bundleID int64, raw string) (Result, error) {
	return CompileWikiPageWithTemplate(bundleID, raw, "")
}

func CompileWikiPageWithTemplate(bundleID int64, raw, templateBody string) (Result, error) {
	var req Request
	if strings.TrimSpace(raw) != "" && json.Valid([]byte(raw)) {
		_ = json.Unmarshal([]byte(raw), &req)
	}
	if req.Title == "" {
		req.Title = firstNonEmptyLine(raw)
	}
	if req.Title == "" {
		req.Title = "Untitled Page"
	}
	if req.Body == "" {
		req.Body = raw
	}
	if req.Slug == "" {
		req.Slug = storage.Slug(req.Title)
	}
	if req.Summary == "" {
		req.Summary = summarize(req.Body)
	}
	body := renderBody(req, templateBody)
	now := time.Now().UTC()
	page := domain.Page{
		BundleID:    bundleID,
		Slug:        req.Slug,
		Type:        req.Type,
		Title:       req.Title,
		Summary:     req.Summary,
		Description: req.Description,
		Body:        body,
		Source:      req.Source,
		GeneratedBy: GeneratedBy,
		GeneratedAt: &now,
	}
	page = applyOKFDefaults(page)
	document := RenderFrontmatter(page) + "\n" + page.Body + "\n"
	return Result{Page: page, Document: document}, nil
}

// applyOKFDefaults returns a copy of p with OKF's per-family defaults
// applied, mirroring storage.Repository.UpsertPage so that a page's
// compiled-but-not-yet-persisted frontmatter (RenderFrontmatter) shows the
// same shape it will have once actually persisted. It deliberately leaves
// ContentHash untouched: that value is only meaningful once a page has been
// persisted, and is never derived here.
func applyOKFDefaults(p domain.Page) domain.Page {
	if strings.TrimSpace(p.Type) == "" {
		p.Type = "note"
	}
	if strings.TrimSpace(p.Path) == "" {
		p.Path = p.Slug + ".md"
	}
	if strings.TrimSpace(p.Status) == "" {
		p.Status = "active"
	}
	if strings.TrimSpace(p.Description) == "" {
		p.Description = p.Summary
	}
	if len(p.Sources) == 0 && strings.TrimSpace(p.Source) != "" {
		p.Sources = []string{p.Source}
	}
	return p
}

// okfFrontmatterView is the YAML shape RenderFrontmatter emits. Field order
// and grouping follow domain.Page's documented OKF families: core/recommended
// (Path, Type, Title, Description, Tags), lifecycle (Status, StaleAfter),
// trust (GeneratedBy, GeneratedAt), and provenance (Sources, ContentHash,
// SourceFragmentIDs). Type is OKF's one required field, so it's the only
// member without "omitempty"; every other field is optional and dropped from
// the rendered block when zero-valued.
type okfFrontmatterView struct {
	Path        string   `yaml:"path,omitempty"`
	Type        string   `yaml:"type"`
	Title       string   `yaml:"title,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`

	Status     string `yaml:"status,omitempty"`
	StaleAfter string `yaml:"stale_after,omitempty"`

	GeneratedBy string `yaml:"generated_by,omitempty"`
	GeneratedAt string `yaml:"generated_at,omitempty"`

	Sources           []string `yaml:"sources,omitempty"`
	ContentHash       string   `yaml:"content_hash,omitempty"`
	SourceFragmentIDs []int64  `yaml:"source_fragment_ids,omitempty"`
}

// RenderFrontmatter renders p's OKF-tracked fields as a YAML frontmatter
// block delimited by "---" lines. It performs no defaulting of its own —
// callers that want a persisted-equivalent shape (e.g.
// CompileWikiPageWithTemplate) should run p through applyOKFDefaults first.
// StaleAfter is truncated to a date (YYYY-MM-DD); GeneratedAt keeps full
// timestamp precision, since only StaleAfter is an OKF date-only field.
func RenderFrontmatter(p domain.Page) string {
	view := okfFrontmatterView{
		Path:              p.Path,
		Type:              p.Type,
		Title:             p.Title,
		Description:       p.Description,
		Tags:              p.Tags,
		Status:            p.Status,
		GeneratedBy:       p.GeneratedBy,
		Sources:           p.Sources,
		ContentHash:       p.ContentHash,
		SourceFragmentIDs: p.SourceFragmentIDs,
	}
	if p.StaleAfter != nil {
		view.StaleAfter = p.StaleAfter.UTC().Format("2006-01-02")
	}
	if p.GeneratedAt != nil {
		view.GeneratedAt = p.GeneratedAt.UTC().Format(time.RFC3339Nano)
	}
	out, err := yaml.Marshal(view)
	if err != nil {
		// okfFrontmatterView is a plain struct of strings/slices; yaml.Marshal
		// has no realistic failure mode here.
		out = nil
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.Write(out)
	b.WriteString("---\n")
	return b.String()
}

func TemplateName(raw string) string {
	if strings.TrimSpace(raw) == "" || !json.Valid([]byte(raw)) {
		return ""
	}
	var req Request
	_ = json.Unmarshal([]byte(raw), &req)
	return strings.TrimSpace(req.Template)
}

func renderBody(req Request, templateBody string) string {
	if strings.TrimSpace(templateBody) == "" {
		body := strings.TrimSpace(req.Body)
		if !strings.HasPrefix(body, "# ") {
			body = "# " + req.Title + "\n\n" + body
		}
		return body
	}
	replacer := strings.NewReplacer(
		"{{title}}", req.Title,
		"{{slug}}", req.Slug,
		"{{summary}}", req.Summary,
		"{{body}}", bodyForTemplate(req),
		"{{source}}", req.Source,
	)
	return strings.TrimSpace(replacer.Replace(templateBody))
}

func bodyForTemplate(req Request) string {
	body := strings.TrimSpace(req.Body)
	lines := strings.Split(body, "\n")
	if len(lines) == 0 {
		return body
	}
	first := strings.TrimSpace(lines[0])
	if strings.HasPrefix(first, "# ") && strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(first, "# ")), req.Title) {
		return strings.TrimSpace(strings.Join(lines[1:], "\n"))
	}
	return body
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if line != "" {
			return line
		}
	}
	return ""
}

func summarize(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= 180 {
		return s
	}
	return strings.TrimSpace(s[:180]) + "..."
}
