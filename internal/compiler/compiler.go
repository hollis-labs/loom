package compiler

import (
	"encoding/json"
	"strings"

	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/storage"
)

type Request struct {
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	Summary  string `json:"summary"`
	Body     string `json:"body"`
	Source   string `json:"source"`
	Template string `json:"template"`
}

type Result struct {
	Page domain.Page `json:"page"`
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
	return Result{Page: domain.Page{
		BundleID: bundleID,
		Slug:     req.Slug,
		Title:    req.Title,
		Summary:  req.Summary,
		Body:     body,
		Source:   req.Source,
	}}, nil
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
