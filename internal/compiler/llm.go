package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/llm"
	"github.com/hollis-labs/loom/internal/storage"
)

// GeneratedByLLM identifies an LLM Provider (internal/llm) as the
// "producer" actor for OKF's trust family (domain.Page.GeneratedBy),
// distinguishing LLM-authored pages from GeneratedBy's deterministic-
// compiler pages.
const GeneratedByLLM = "loom-compiler-llm"

// CompileWikiPageWithLLM mirrors CompileWikiPageWithTemplate's request
// parsing, template rendering, and OKF frontmatter/document construction,
// but delegates body authoring to provider instead of using the raw
// request body verbatim. provider must be non-nil; callers (see
// internal/service.Compiler.run) are expected to check for a configured
// provider before selecting this path, but CompileWikiPageWithLLM returns a
// clear error here too rather than panicking. redactPatterns are the
// user-configured redaction patterns (config.Filters.RedactPatterns) to
// apply, in addition to llm's baseline defaultRedactors, before req.Body is
// sent to provider.Generate; see llm.RedactWithPatterns.
func CompileWikiPageWithLLM(ctx context.Context, provider llm.Provider, bundleID int64, raw, templateBody string, redactPatterns []string) (Result, error) {
	if provider == nil {
		return Result{}, fmt.Errorf("compiler: generation_mode=llm requires a configured LLM provider")
	}
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

	generated, err := provider.Generate(ctx, llm.Prompt{
		Title:       req.Title,
		Type:        req.Type,
		Summary:     req.Summary,
		Description: req.Description,
		Source:      req.Source,
		Material:    llm.RedactWithPatterns(req.Body, redactPatterns),
	})
	if err != nil {
		return Result{}, fmt.Errorf("compiler: llm generate: %w", err)
	}
	generated = strings.TrimSpace(generated)
	if generated == "" {
		return Result{}, fmt.Errorf("compiler: llm provider returned empty content")
	}

	// Route the generated markdown through the same template substitution
	// and title-heading dedup logic (renderBody/bodyForTemplate) that the
	// deterministic path uses, by swapping it in as req.Body.
	genReq := req
	genReq.Body = generated
	body := renderBody(genReq, templateBody)

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
		GeneratedBy: GeneratedByLLM,
		GeneratedAt: &now,
	}
	page = applyOKFDefaults(page)
	document := RenderFrontmatter(page) + "\n" + page.Body + "\n"
	return Result{Page: page, Document: document}, nil
}
