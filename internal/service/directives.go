package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hollis-labs/loom/internal/compiler"
	"github.com/hollis-labs/loom/internal/directivex"
	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/storage"
)

// directiveHandler executes one newly-ledgered directive (see
// directivex.ParseAndLedger / Directive.New) and returns the compile job it
// produced. text is the full source text that was parsed (a directive's
// prompt/config alone don't always carry enough content to compile a page
// from); source is the caller-supplied source identifier.
type directiveHandler func(ctx context.Context, c *Compiler, bundleSlug, text, source string, d directivex.ParsedDirective) (domain.CompileJob, error)

// directiveHandlers is the Command dispatch table for
// Compiler.RequestFromDirectives (CW-20260816-0017). Keys are canonical
// command names — alias resolution already happened inside go-directives
// (see directives.DefaultAliases) before a Directive reaches here.
//
// Commands with no dedicated handler (e.g. "note", "blog-draft",
// "document-feature", or anything else go-directives classifies as
// CategoryAction/CategoryMeta) run through handleGenericDirective, which
// preserves Loom's original config-driven "compile a wiki_page from the
// full source text" behavior. That fallback is a deliberate scope
// boundary: this task dispatches draft/log-adr/reminder/extract; auditing
// every other possible directive command is future work.
var directiveHandlers = map[string]directiveHandler{
	"draft":    handleDraftDirective,
	"log-adr":  handleLogADRDirective,
	"reminder": handleReminderDirective,
	"extract":  handleExtractDirective,
}

// dispatchDirective looks up d.Command in directiveHandlers and runs it,
// falling back to handleGenericDirective for unlisted commands.
func dispatchDirective(ctx context.Context, c *Compiler, bundleSlug, text, source string, d directivex.ParsedDirective) (domain.CompileJob, error) {
	handler := directiveHandlers[d.Command]
	if handler == nil {
		handler = handleGenericDirective
	}
	return handler(ctx, c, bundleSlug, text, source, d)
}

// compileDirectiveRequest marshals req and routes it through Compiler.Request,
// the same job/event/page pipeline every other compile path in this package
// (Compiler.Request callers, ingestItems) already uses. This is what makes
// "draft is essentially compile a page from this context" true in code, not
// just in the task description: every handler below funnels into this one
// call, so directive-triggered compiles get identical persistence, event
// logging, idempotency, and generation-mode branching (deterministic vs LLM,
// see Compiler.run) as every other compile job in Loom.
func compileDirectiveRequest(ctx context.Context, c *Compiler, bundleSlug, generator string, req compiler.Request) (domain.CompileJob, error) {
	input, err := json.Marshal(req)
	if err != nil {
		return domain.CompileJob{}, err
	}
	return c.Request(ctx, bundleSlug, generator, string(input))
}

// directiveGenerator returns d's explicit ::config generator=... override, or
// "wiki_page" (the only generator Compiler.Request currently accepts — an
// explicit override to anything else surfaces as a clear per-directive
// failure via Compiler.Request's own validation, see RequestFromDirectives).
func directiveGenerator(d directivex.ParsedDirective) string {
	if g := strings.TrimSpace(d.Config["generator"]); g != "" {
		return g
	}
	return "wiki_page"
}

// extractDirectiveContext returns the lines of text spanned by contextRange
// (1-based, inclusive) — the same line range go-directives resolved and
// hashed to produce Directive.Hash (see go-directives' buildAction /
// resolveContextRange). Used by handleExtractDirective to pull just the
// context attributed to a directive, rather than the whole source document.
func extractDirectiveContext(text string, contextRange [2]int) string {
	lines := strings.Split(text, "\n")
	start, end := contextRange[0], contextRange[1]
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return ""
	}
	return strings.Join(lines[start-1:end], "\n")
}

// handleGenericDirective is the fallback for any directive.Command without a
// dedicated entry in directiveHandlers. It reproduces
// RequestFromDirectives' pre-dispatch behavior verbatim: compile a
// wiki_page from the directive's prompt/config, using the full source text
// as the page body.
func handleGenericDirective(ctx context.Context, c *Compiler, bundleSlug, text, source string, d directivex.ParsedDirective) (domain.CompileJob, error) {
	req := compiler.Request{
		Title:          directiveTitle(d),
		Slug:           d.Config["slug"],
		Summary:        d.Config["summary"],
		Body:           text,
		Source:         source,
		Template:       d.Config["template"],
		GenerationMode: compiler.GenerationMode(d.Config["generation_mode"]),
	}
	return compileDirectiveRequest(ctx, c, bundleSlug, directiveGenerator(d), req)
}

// handleDraftDirective handles "::draft ..." — the directive-driven form of
// "compile a page/output from this context". It is intentionally almost
// identical to handleGenericDirective: a draft directive's job IS a normal
// wiki_page compile job, just reached through explicit Command dispatch
// instead of an implicit default. ::config type=... lets a draft directive
// pick its own OKF Type (default "note", applied downstream by
// applyOKFDefaults); ::config generation_mode=llm routes the page through
// CompileWikiPageWithLLM instead of the deterministic template compiler
// (requires Compiler.SetLLMProvider to have been configured — see
// compiler.CompileWikiPageWithLLM's own nil-provider error otherwise).
func handleDraftDirective(ctx context.Context, c *Compiler, bundleSlug, text, source string, d directivex.ParsedDirective) (domain.CompileJob, error) {
	req := compiler.Request{
		Title:          directiveTitle(d),
		Slug:           d.Config["slug"],
		Type:           d.Config["type"],
		Summary:        d.Config["summary"],
		Body:           text,
		Source:         source,
		Template:       d.Config["template"],
		GenerationMode: compiler.GenerationMode(d.Config["generation_mode"]),
	}
	return compileDirectiveRequest(ctx, c, bundleSlug, directiveGenerator(d), req)
}

// handleLogADRDirective handles "::log-adr ..." by compiling a wiki page of
// OKF type "adr" (Architecture Decision Record).
//
// Judgment call: Loom has no dedicated ADR store/schema, so "logging an
// ADR" is implemented as "produce a normal wiki page through the same
// compile pipeline, tagged type=adr, with an 'ADR: ' title prefix and an
// 'adr-' slug prefix" so ADR pages are easy to find/filter in
// ListPages/ListBundleVerifications without inventing new storage. The
// body is the directive's prompt (the ADR's own text, as authored inline
// after ::log-adr); it falls back to the full source text only when the
// prompt is empty, so a bare "::log-adr" still produces something rather
// than an empty page.
func handleLogADRDirective(ctx context.Context, c *Compiler, bundleSlug, text, source string, d directivex.ParsedDirective) (domain.CompileJob, error) {
	title := directiveTitle(d)
	if !strings.HasPrefix(strings.ToUpper(title), "ADR") {
		title = "ADR: " + title
	}
	slug := strings.TrimSpace(d.Config["slug"])
	if slug == "" && strings.TrimSpace(d.Prompt) != "" {
		slug = "adr-" + storage.Slug(d.Prompt)
	}
	body := strings.TrimSpace(d.Prompt)
	if body == "" {
		body = text
	}
	req := compiler.Request{
		Title:          title,
		Slug:           slug,
		Type:           "adr",
		Summary:        d.Config["summary"],
		Body:           body,
		Source:         source,
		Template:       d.Config["template"],
		GenerationMode: compiler.GenerationMode(d.Config["generation_mode"]),
	}
	return compileDirectiveRequest(ctx, c, bundleSlug, directiveGenerator(d), req)
}

// handleReminderDirective handles "::reminder ..." by compiling a small
// wiki page of OKF type "reminder".
//
// Judgment call: Loom has no reminders/tasks table, so "a lightweight
// record" is implemented as the smallest defensible page — no LLM pass, no
// template body, no source-text fallback — rather than a new storage
// concept. The body is just the directive's prompt itself (not the
// surrounding context, unlike draft/extract): a reminder is meant to be a
// short standalone note, not a compiled document. An empty prompt still
// produces a valid ("Reminder", body "Reminder") page rather than dumping
// the raw compile-request JSON as the body.
func handleReminderDirective(ctx context.Context, c *Compiler, bundleSlug, text, source string, d directivex.ParsedDirective) (domain.CompileJob, error) {
	title := strings.TrimSpace(d.Prompt)
	if title == "" {
		title = "Reminder"
	} else {
		title = "Reminder: " + title
	}
	body := strings.TrimSpace(d.Prompt)
	if body == "" {
		body = title
	}
	req := compiler.Request{
		Title:   title,
		Slug:    d.Config["slug"],
		Type:    "reminder",
		Summary: d.Config["summary"],
		Body:    body,
		Source:  source,
	}
	return compileDirectiveRequest(ctx, c, bundleSlug, directiveGenerator(d), req)
}

// handleExtractDirective handles "::extract ..." by compiling a wiki page of
// OKF type "extract" whose body is the source lines go-directives
// attributed to this directive (Directive.ContextRange — the same range it
// hashed to compute d.Hash; see extractDirectiveContext), not the whole
// source document.
//
// Judgment call: "pull structured content into a new page" is implemented
// literally as "extract the referenced context lines verbatim into their
// own page" rather than attempting real structured-data parsing
// (tables/lists/frontmatter/etc. detection) — that's a genuinely separate,
// much larger feature, not something to guess the shape of here. Falls
// back to the full source text when the resolved range is empty (e.g. an
// extract directive with no preceding context/zoom frame in a
// single-directive document).
func handleExtractDirective(ctx context.Context, c *Compiler, bundleSlug, text, source string, d directivex.ParsedDirective) (domain.CompileJob, error) {
	body := extractDirectiveContext(text, d.ContextRange)
	if strings.TrimSpace(body) == "" {
		body = text
	}
	title := directiveTitle(d)
	if !strings.HasPrefix(strings.ToLower(title), "extract") {
		title = "Extract: " + title
	}
	req := compiler.Request{
		Title:          title,
		Slug:           d.Config["slug"],
		Type:           "extract",
		Summary:        d.Config["summary"],
		Body:           body,
		Source:         source,
		Template:       d.Config["template"],
		GenerationMode: compiler.GenerationMode(d.Config["generation_mode"]),
	}
	return compileDirectiveRequest(ctx, c, bundleSlug, directiveGenerator(d), req)
}
