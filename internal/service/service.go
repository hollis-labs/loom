package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hollis-labs/loom/internal/compiler"
	"github.com/hollis-labs/loom/internal/directivex"
	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/ingest"
	"github.com/hollis-labs/loom/internal/storage"
)

type Compiler struct {
	repo *storage.Repository
}

func NewCompiler(repo *storage.Repository) *Compiler {
	return &Compiler{repo: repo}
}

type DirectiveCompileResult struct {
	Parse   directivex.ParseResponse `json:"parse"`
	Jobs    []domain.CompileJob      `json:"jobs"`
	Skipped []string                 `json:"skipped"`
}

type IngestResult struct {
	Items   int                 `json:"items"`
	Jobs    []domain.CompileJob `json:"jobs"`
	Skipped []string            `json:"skipped"`
}

func (c *Compiler) Request(ctx context.Context, bundleSlug, generator, input string) (domain.CompileJob, error) {
	if generator == "" {
		generator = "wiki_page"
	}
	if generator != "wiki_page" {
		return domain.CompileJob{}, fmt.Errorf("%w: only generator=wiki_page is supported", storage.ErrInvalid)
	}
	b, err := c.repo.GetBundle(ctx, bundleSlug)
	if err != nil {
		return domain.CompileJob{}, err
	}
	job, err := c.repo.CreateJob(ctx, b.ID, generator, input)
	if err != nil {
		return domain.CompileJob{}, err
	}
	if err := c.run(ctx, job); err != nil {
		_ = c.repo.UpdateJob(ctx, job.ID, "failed", "", err.Error())
		_ = c.repo.AddEvent(ctx, job.ID, "failed", err.Error())
		return c.repo.GetJob(ctx, job.ID)
	}
	return c.repo.GetJob(ctx, job.ID)
}

func (c *Compiler) RequestFromDirectives(ctx context.Context, bundleSlug, text, source string) (DirectiveCompileResult, error) {
	parsed, err := directivex.ParseAndLedger(ctx, c.repo, text, source)
	if err != nil {
		return DirectiveCompileResult{}, err
	}
	out := DirectiveCompileResult{Parse: parsed}
	for _, d := range parsed.Directives {
		if !d.New {
			out.Skipped = append(out.Skipped, d.Hash)
			continue
		}
		generator := d.Config["generator"]
		if generator == "" {
			generator = "wiki_page"
		}
		if generator != "wiki_page" {
			out.Skipped = append(out.Skipped, d.Hash)
			continue
		}
		input, err := json.Marshal(compiler.Request{
			Title:    directiveTitle(d),
			Slug:     d.Config["slug"],
			Summary:  d.Config["summary"],
			Body:     text,
			Source:   source,
			Template: d.Config["template"],
		})
		if err != nil {
			return DirectiveCompileResult{}, err
		}
		job, err := c.Request(ctx, bundleSlug, "wiki_page", string(input))
		if err != nil {
			return DirectiveCompileResult{}, fmt.Errorf("compile directive line %d: %w", d.Line, err)
		}
		out.Jobs = append(out.Jobs, job)
	}
	return out, nil
}

func (c *Compiler) IngestFiles(ctx context.Context, bundleSlug string, paths []string, source string, exclude, redact []string) (IngestResult, error) {
	items, err := ingest.FileSource{Paths: paths, Source: source, Exclude: exclude, Redact: redact}.Items(ctx)
	if err != nil {
		return IngestResult{}, classifyIngestError(err)
	}
	return c.ingestItems(ctx, bundleSlug, items)
}

func (c *Compiler) IngestText(ctx context.Context, bundleSlug, source, sourceKey, title, body string, redact []string) (IngestResult, error) {
	item, err := ingest.TextSource{Source: source, SourceKey: sourceKey, Title: title, Body: body, Redact: redact}.Item()
	if err != nil {
		return IngestResult{}, classifyIngestError(err)
	}
	return c.ingestItems(ctx, bundleSlug, []ingest.Item{item})
}

func (c *Compiler) ingestItems(ctx context.Context, bundleSlug string, items []ingest.Item) (IngestResult, error) {
	out := IngestResult{Items: len(items)}
	for _, item := range items {
		hash := ingest.IngestHash(item.Source, item.SourceKey, item.ContentHash)
		inserted, err := c.repo.InsertIngest(ctx, hash, item.Source, item.SourceKey, item.ContentHash)
		if err != nil {
			return IngestResult{}, err
		}
		if !inserted {
			out.Skipped = append(out.Skipped, hash)
			continue
		}
		input, err := json.Marshal(compiler.Request{
			Title:  item.Title,
			Slug:   storage.Slug(item.Title),
			Body:   item.Body,
			Source: item.SourceKey,
		})
		if err != nil {
			return IngestResult{}, err
		}
		job, err := c.Request(ctx, bundleSlug, "wiki_page", string(input))
		if err != nil {
			return IngestResult{}, fmt.Errorf("compile ingest item %s: %w", item.SourceKey, err)
		}
		if err := c.repo.AttachIngestJob(ctx, hash, job.ID); err != nil {
			return IngestResult{}, err
		}
		out.Jobs = append(out.Jobs, job)
	}
	return out, nil
}

func directiveTitle(d directivex.ParsedDirective) string {
	if d.Prompt != "" {
		return d.Prompt
	}
	return d.Command
}

func classifyIngestError(err error) error {
	if errors.Is(err, ingest.ErrInvalid) {
		return fmt.Errorf("%w: %v", storage.ErrInvalid, err)
	}
	return err
}

func (c *Compiler) run(ctx context.Context, job domain.CompileJob) error {
	if err := c.repo.UpdateJob(ctx, job.ID, "running", "", ""); err != nil {
		return err
	}
	_ = c.repo.AddEvent(ctx, job.ID, "running", "deterministic compiler started")
	templateBody, err := c.templateBody(ctx, job.Input)
	if err != nil {
		return err
	}
	result, err := compiler.CompileWikiPageWithTemplate(job.BundleID, job.Input, templateBody)
	if err != nil {
		return err
	}
	page, err := c.repo.UpsertPage(ctx, result.Page)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]any{"page": page})
	if err := c.repo.UpdateJob(ctx, job.ID, "completed", string(data), ""); err != nil {
		return err
	}
	return c.repo.AddEvent(ctx, job.ID, "completed", "wiki page stored")
}

func (c *Compiler) templateBody(ctx context.Context, raw string) (string, error) {
	name := compiler.TemplateName(raw)
	if name == "" {
		name = "wiki_page.default"
	}
	tpl, err := c.repo.GetTemplate(ctx, name)
	if err != nil {
		return "", fmt.Errorf("load template %s: %w", name, err)
	}
	return tpl.Body, nil
}
