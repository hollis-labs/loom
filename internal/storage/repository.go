package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/wiki"
)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid input")
)

type Repository struct {
	db *sql.DB
}

type JobFilter struct {
	BundleID int64
	Status   string
	Limit    int
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) DB() *sql.DB {
	return r.db
}

func (r *Repository) Stats(ctx context.Context) (domain.RepositoryStats, error) {
	var stats domain.RepositoryStats
	counts := []struct {
		dst   *int64
		query string
	}{
		{&stats.Bundles, `SELECT COUNT(*) FROM wiki_bundles`},
		{&stats.Pages, `SELECT COUNT(*) FROM wiki_pages`},
		{&stats.Links, `SELECT COUNT(*) FROM wiki_links`},
		{&stats.Verifications, `SELECT COUNT(*) FROM wiki_verifications`},
		{&stats.CompileJobs, `SELECT COUNT(*) FROM compile_jobs`},
		{&stats.CompileEvents, `SELECT COUNT(*) FROM compile_events`},
		{&stats.DirectiveLedger, `SELECT COUNT(*) FROM directive_ledger`},
		{&stats.IngestLedger, `SELECT COUNT(*) FROM ingest_ledger`},
		{&stats.Templates, `SELECT COUNT(*) FROM templates`},
		{&stats.CompletedJobs, `SELECT COUNT(*) FROM compile_jobs WHERE status = 'completed'`},
		{&stats.FailedJobs, `SELECT COUNT(*) FROM compile_jobs WHERE status = 'failed'`},
		{&stats.VerificationWarnings, `SELECT COUNT(*) FROM wiki_verifications WHERE status = 'warning'`},
	}
	for _, count := range counts {
		if err := r.db.QueryRowContext(ctx, count.query).Scan(count.dst); err != nil {
			return domain.RepositoryStats{}, err
		}
	}
	return stats, nil
}

const bundleColumns = `id, slug, title, description, scope, COALESCE(okf_export_path, ''), created_at, updated_at`

func (r *Repository) ListBundles(ctx context.Context) ([]domain.Bundle, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+bundleColumns+` FROM wiki_bundles ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]domain.Bundle, 0)
	for rows.Next() {
		b, err := scanBundle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *Repository) GetBundle(ctx context.Context, slug string) (domain.Bundle, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+bundleColumns+` FROM wiki_bundles WHERE slug = ?`, slug)
	b, err := scanBundle(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Bundle{}, ErrNotFound
	}
	return b, err
}

func (r *Repository) UpsertBundle(ctx context.Context, b domain.Bundle) (domain.Bundle, error) {
	b.Slug = Slug(b.Slug)
	if b.Slug == "" {
		b.Slug = Slug(b.Title)
	}
	if b.Slug == "" || strings.TrimSpace(b.Title) == "" {
		return domain.Bundle{}, fmt.Errorf("%w: bundle slug and title are required", ErrInvalid)
	}
	scope := strings.TrimSpace(b.Scope)
	if scope == "" {
		scope = "project"
	}
	if scope != "project" && scope != "meta" && scope != "personal" {
		return domain.Bundle{}, fmt.Errorf("%w: bundle scope must be one of project|meta|personal, got %q", ErrInvalid, scope)
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO wiki_bundles (slug, title, description, scope, okf_export_path)
VALUES (?, ?, ?, ?, NULLIF(?, ''))
ON CONFLICT(slug) DO UPDATE SET
	title = excluded.title,
	description = excluded.description,
	scope = excluded.scope,
	okf_export_path = excluded.okf_export_path,
	updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		b.Slug, strings.TrimSpace(b.Title), strings.TrimSpace(b.Description), scope, strings.TrimSpace(b.OKFExportPath))
	if err != nil {
		return domain.Bundle{}, err
	}
	return r.GetBundle(ctx, b.Slug)
}

func (r *Repository) UpsertPage(ctx context.Context, p domain.Page) (domain.Page, error) {
	if strings.TrimSpace(p.Slug) == "" {
		p.Slug = Slug(p.Title)
	} else {
		p.Slug = Slug(p.Slug)
	}
	p.Title = strings.TrimSpace(p.Title)
	if p.Slug == "" || p.Title == "" {
		return domain.Page{}, fmt.Errorf("%w: page slug and title are required", ErrInvalid)
	}
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
	p.ContentHash = contentHash(p.Body)
	tagsJSON := marshalStrings(p.Tags)
	sourcesJSON := marshalStrings(p.Sources)
	fragmentIDsJSON := marshalInt64s(p.SourceFragmentIDs)
	var staleAfter, generatedAt any
	if p.StaleAfter != nil {
		staleAfter = p.StaleAfter.UTC().Format(time.RFC3339Nano)
	}
	if p.GeneratedAt != nil {
		generatedAt = p.GeneratedAt.UTC().Format(time.RFC3339Nano)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Page{}, err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
INSERT INTO wiki_pages (bundle_id, slug, path, type, title, summary, description, tags, status, stale_after, generated_by, generated_at, body, source, sources, content_hash, source_fragment_ids)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(bundle_id, slug) DO UPDATE SET
	path = excluded.path,
	type = excluded.type,
	title = excluded.title,
	summary = excluded.summary,
	description = excluded.description,
	tags = excluded.tags,
	status = excluded.status,
	stale_after = excluded.stale_after,
	generated_by = excluded.generated_by,
	generated_at = excluded.generated_at,
	body = excluded.body,
	source = excluded.source,
	sources = excluded.sources,
	content_hash = excluded.content_hash,
	source_fragment_ids = excluded.source_fragment_ids,
	updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		p.BundleID, p.Slug, p.Path, p.Type, p.Title, p.Summary, p.Description, tagsJSON, p.Status, staleAfter, p.GeneratedBy, generatedAt, p.Body, p.Source, sourcesJSON, p.ContentHash, fragmentIDsJSON)
	if err != nil {
		return domain.Page{}, err
	}
	var pageID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM wiki_pages WHERE bundle_id = ? AND slug = ?`, p.BundleID, p.Slug).Scan(&pageID); err != nil {
		return domain.Page{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM wiki_links WHERE from_page_id = ?`, pageID); err != nil {
		return domain.Page{}, err
	}
	links := wiki.ExtractLinks(p.Body)
	for _, link := range links {
		toPageID, err := resolveLinkTarget(ctx, tx, p.BundleID, link)
		if err != nil {
			return domain.Page{}, err
		}
		relation := strings.TrimSpace(link.Relation)
		if relation == "" {
			relation = "references"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO wiki_links (from_page_id, to_page_id, relation, target, kind, label, position) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			pageID, toPageID, relation, link.Target, link.Kind, link.Label, link.Position); err != nil {
			return domain.Page{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM wiki_verifications WHERE page_id = ?`, pageID); err != nil {
		return domain.Page{}, err
	}
	p.ID = pageID
	for _, verification := range wiki.VerifyPage(p, links) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO wiki_verifications (page_id, kind, status, message, "by") VALUES (?, ?, ?, ?, ?)`,
			pageID, verification.Kind, verification.Status, verification.Message, verification.By); err != nil {
			return domain.Page{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.Page{}, err
	}
	return r.GetPageByBundleSlug(ctx, p.BundleID, p.Slug)
}

// resolveLinkTarget resolves an internal ("wiki") link's target slug to a
// sibling page id in the same bundle. External URLs and anchors never
// resolve; internal links that don't match a known page also stay
// unresolved (nil), matching the migration's best-effort backfill.
func resolveLinkTarget(ctx context.Context, tx *sql.Tx, bundleID int64, link domain.Link) (sql.NullInt64, error) {
	if link.Kind != "wiki" {
		return sql.NullInt64{}, nil
	}
	var resolved int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM wiki_pages WHERE bundle_id = ? AND slug = ?`, bundleID, link.Target).Scan(&resolved)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullInt64{}, nil
	}
	if err != nil {
		return sql.NullInt64{}, err
	}
	return sql.NullInt64{Int64: resolved, Valid: true}, nil
}

func (r *Repository) ListPages(ctx context.Context, bundleID int64, query string, limit int) ([]domain.Page, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	args := []any{bundleID}
	stmt := `SELECT ` + pageColumns + ` FROM wiki_pages WHERE bundle_id = ?`
	if strings.TrimSpace(query) != "" {
		stmt += ` AND (slug LIKE ? OR title LIKE ? OR summary LIKE ? OR body LIKE ?)`
		q := "%" + query + "%"
		args = append(args, q, q, q, q)
	}
	stmt += ` ORDER BY title LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]domain.Page, 0)
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) GetPageBySlug(ctx context.Context, bundleSlug, pageSlug string) (domain.Page, error) {
	b, err := r.GetBundle(ctx, bundleSlug)
	if err != nil {
		return domain.Page{}, err
	}
	return r.GetPageByBundleSlug(ctx, b.ID, pageSlug)
}

func (r *Repository) GetPageByBundleSlug(ctx context.Context, bundleID int64, pageSlug string) (domain.Page, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+pageColumns+` FROM wiki_pages WHERE bundle_id = ? AND slug = ?`, bundleID, pageSlug)
	p, err := scanPage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Page{}, ErrNotFound
	}
	return p, err
}

func (r *Repository) ListPageLinks(ctx context.Context, pageID int64) ([]domain.Link, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, from_page_id, to_page_id, relation, target, kind, label, position FROM wiki_links WHERE from_page_id = ? ORDER BY position, id`, pageID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]domain.Link, 0)
	for rows.Next() {
		link, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

func (r *Repository) ListBundleLinks(ctx context.Context, bundleID int64, limit int) ([]domain.Link, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT l.id, l.from_page_id, l.to_page_id, l.relation, l.target, l.kind, l.label, l.position
FROM wiki_links l
JOIN wiki_pages p ON p.id = l.from_page_id
WHERE p.bundle_id = ?
ORDER BY p.slug, l.position, l.id
LIMIT ?`, bundleID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]domain.Link, 0)
	for rows.Next() {
		link, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

func (r *Repository) ListPageVerifications(ctx context.Context, pageID int64) ([]domain.Verification, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, page_id, kind, status, message, "by", created_at FROM wiki_verifications WHERE page_id = ? ORDER BY id`, pageID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]domain.Verification, 0)
	for rows.Next() {
		verification, err := scanVerification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, verification)
	}
	return out, rows.Err()
}

func (r *Repository) ListBundleVerifications(ctx context.Context, bundleID int64, limit int) ([]domain.Verification, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT v.id, v.page_id, v.kind, v.status, v.message, v."by", v.created_at
FROM wiki_verifications v
JOIN wiki_pages p ON p.id = v.page_id
WHERE p.bundle_id = ?
ORDER BY p.slug, v.id
LIMIT ?`, bundleID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]domain.Verification, 0)
	for rows.Next() {
		verification, err := scanVerification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, verification)
	}
	return out, rows.Err()
}

func (r *Repository) CreateJob(ctx context.Context, bundleID int64, generator, input string) (domain.CompileJob, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO compile_jobs (bundle_id, generator, status, input) VALUES (?, ?, 'queued', ?)`, bundleID, generator, input)
	if err != nil {
		return domain.CompileJob{}, err
	}
	id, _ := res.LastInsertId()
	if err := r.AddEvent(ctx, id, "queued", "compile job created"); err != nil {
		return domain.CompileJob{}, err
	}
	return r.GetJob(ctx, id)
}

func (r *Repository) UpdateJob(ctx context.Context, id int64, status, output, errText string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE compile_jobs SET status = ?, output = ?, error = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?`, status, output, errText, id)
	return err
}

func (r *Repository) AddEvent(ctx context.Context, jobID int64, status, message string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO compile_events (job_id, status, message) VALUES (?, ?, ?)`, jobID, status, message)
	return err
}

func (r *Repository) ListEvents(ctx context.Context, jobID int64) ([]domain.CompileEvent, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, job_id, status, message, created_at FROM compile_events WHERE job_id = ? ORDER BY id`, jobID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]domain.CompileEvent, 0)
	for rows.Next() {
		var e domain.CompileEvent
		var created string
		if err := rows.Scan(&e.ID, &e.JobID, &e.Status, &e.Message, &created); err != nil {
			return nil, err
		}
		e.CreatedAt = parseDBTime(created)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) ListJobs(ctx context.Context, filter JobFilter) ([]domain.CompileJob, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	stmt := `SELECT id, bundle_id, generator, status, input, output, error, created_at, updated_at FROM compile_jobs WHERE 1=1`
	args := make([]any, 0, 3)
	if filter.BundleID > 0 {
		stmt += ` AND bundle_id = ?`
		args = append(args, filter.BundleID)
	}
	if strings.TrimSpace(filter.Status) != "" {
		stmt += ` AND status = ?`
		args = append(args, strings.TrimSpace(filter.Status))
	}
	stmt += ` ORDER BY id DESC LIMIT ?`
	args = append(args, filter.Limit)
	rows, err := r.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]domain.CompileJob, 0)
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (r *Repository) GetJob(ctx context.Context, id int64) (domain.CompileJob, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, bundle_id, generator, status, input, output, error, created_at, updated_at FROM compile_jobs WHERE id = ?`, id)
	j, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CompileJob{}, ErrNotFound
	}
	return j, err
}

func (r *Repository) InsertDirective(ctx context.Context, hash, source, command, prompt string, line int) (bool, error) {
	res, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO directive_ledger (hash, source, command, prompt, line) VALUES (?, ?, ?, ?, ?)`, hash, source, command, prompt, line)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *Repository) GetDirective(ctx context.Context, hash string) (domain.LedgerEntry, error) {
	row := r.db.QueryRowContext(ctx, `SELECT hash, source, command, prompt, line, created_at FROM directive_ledger WHERE hash = ?`, hash)
	entry, err := scanDirective(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.LedgerEntry{}, ErrNotFound
	}
	return entry, err
}

func (r *Repository) ListDirectives(ctx context.Context, limit int) ([]domain.LedgerEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `SELECT hash, source, command, prompt, line, created_at FROM directive_ledger ORDER BY created_at DESC, hash LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]domain.LedgerEntry, 0)
	for rows.Next() {
		entry, err := scanDirective(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (r *Repository) InsertIngest(ctx context.Context, hash, source, sourceKey, contentHash string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
INSERT OR IGNORE INTO ingest_ledger (hash, source, source_key, content_hash)
VALUES (?, ?, ?, ?)`, hash, source, sourceKey, contentHash)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *Repository) AttachIngestJob(ctx context.Context, hash string, jobID int64) error {
	res, err := r.db.ExecContext(ctx, `
UPDATE ingest_ledger
SET job_id = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE hash = ?`, jobID, hash)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) GetIngest(ctx context.Context, hash string) (domain.IngestLedgerEntry, error) {
	row := r.db.QueryRowContext(ctx, `SELECT hash, source, source_key, content_hash, COALESCE(job_id, 0), created_at, updated_at FROM ingest_ledger WHERE hash = ?`, hash)
	entry, err := scanIngest(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.IngestLedgerEntry{}, ErrNotFound
	}
	return entry, err
}

func (r *Repository) ListIngest(ctx context.Context, limit int) ([]domain.IngestLedgerEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `SELECT hash, source, source_key, content_hash, COALESCE(job_id, 0), created_at, updated_at FROM ingest_ledger ORDER BY created_at DESC, hash LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]domain.IngestLedgerEntry, 0)
	for rows.Next() {
		entry, err := scanIngest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (r *Repository) ListTemplates(ctx context.Context) ([]domain.Template, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, generator, body, created_at, updated_at FROM templates ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]domain.Template, 0)
	for rows.Next() {
		tpl, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, tpl)
	}
	return out, rows.Err()
}

func (r *Repository) GetTemplate(ctx context.Context, name string) (domain.Template, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, name, generator, body, created_at, updated_at FROM templates WHERE name = ?`, name)
	tpl, err := scanTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Template{}, ErrNotFound
	}
	return tpl, err
}

func (r *Repository) UpsertTemplate(ctx context.Context, tpl domain.Template) (domain.Template, error) {
	if strings.TrimSpace(tpl.Name) == "" || strings.TrimSpace(tpl.Generator) == "" {
		return domain.Template{}, fmt.Errorf("%w: template name and generator are required", ErrInvalid)
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO templates (name, generator, body)
VALUES (?, ?, ?)
ON CONFLICT(name) DO UPDATE SET
	generator = excluded.generator,
	body = excluded.body,
	updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		tpl.Name, tpl.Generator, tpl.Body)
	if err != nil {
		return domain.Template{}, err
	}
	return r.GetTemplate(ctx, tpl.Name)
}

type scanner interface {
	Scan(dest ...any) error
}

const pageColumns = `id, bundle_id, slug, path, type, title, summary, description, tags, status, stale_after, generated_by, generated_at, body, source, sources, content_hash, source_fragment_ids, created_at, updated_at`

func scanBundle(s scanner) (domain.Bundle, error) {
	var b domain.Bundle
	var created, updated string
	err := s.Scan(&b.ID, &b.Slug, &b.Title, &b.Description, &b.Scope, &b.OKFExportPath, &created, &updated)
	if err != nil {
		return domain.Bundle{}, err
	}
	b.CreatedAt = parseDBTime(created)
	b.UpdatedAt = parseDBTime(updated)
	return b, nil
}

func scanPage(s scanner) (domain.Page, error) {
	var p domain.Page
	var created, updated string
	var staleAfter, generatedAt sql.NullString
	var tagsJSON, sourcesJSON, fragmentIDsJSON string
	err := s.Scan(&p.ID, &p.BundleID, &p.Slug, &p.Path, &p.Type, &p.Title, &p.Summary, &p.Description, &tagsJSON,
		&p.Status, &staleAfter, &p.GeneratedBy, &generatedAt, &p.Body, &p.Source, &sourcesJSON, &p.ContentHash,
		&fragmentIDsJSON, &created, &updated)
	if err != nil {
		return domain.Page{}, err
	}
	p.CreatedAt = parseDBTime(created)
	p.UpdatedAt = parseDBTime(updated)
	if staleAfter.Valid && staleAfter.String != "" {
		t := parseDBTime(staleAfter.String)
		p.StaleAfter = &t
	}
	if generatedAt.Valid && generatedAt.String != "" {
		t := parseDBTime(generatedAt.String)
		p.GeneratedAt = &t
	}
	p.Tags = unmarshalStrings(tagsJSON)
	p.Sources = unmarshalStrings(sourcesJSON)
	p.SourceFragmentIDs = unmarshalInt64s(fragmentIDsJSON)
	return p, nil
}

func scanLink(s scanner) (domain.Link, error) {
	var link domain.Link
	var toPageID sql.NullInt64
	err := s.Scan(&link.ID, &link.FromPageID, &toPageID, &link.Relation, &link.Target, &link.Kind, &link.Label, &link.Position)
	if err != nil {
		return domain.Link{}, err
	}
	if toPageID.Valid {
		v := toPageID.Int64
		link.ToPageID = &v
	}
	return link, nil
}

func scanVerification(s scanner) (domain.Verification, error) {
	var verification domain.Verification
	var created string
	err := s.Scan(&verification.ID, &verification.PageID, &verification.Kind, &verification.Status, &verification.Message, &verification.By, &created)
	verification.CreatedAt = parseDBTime(created)
	return verification, err
}

// contentHash returns the hex-encoded sha256 of a page body, mirroring FE's
// existing fragment dedup convention.
func contentHash(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func marshalStrings(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func unmarshalStrings(raw string) []string {
	out := make([]string, 0)
	if strings.TrimSpace(raw) == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return make([]string, 0)
	}
	return out
}

func marshalInt64s(items []int64) string {
	if len(items) == 0 {
		return "[]"
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func unmarshalInt64s(raw string) []int64 {
	out := make([]int64, 0)
	if strings.TrimSpace(raw) == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return make([]int64, 0)
	}
	return out
}

func scanDirective(s scanner) (domain.LedgerEntry, error) {
	var entry domain.LedgerEntry
	var created string
	err := s.Scan(&entry.Hash, &entry.Source, &entry.Command, &entry.Prompt, &entry.Line, &created)
	entry.CreatedAt = parseDBTime(created)
	return entry, err
}

func scanJob(s scanner) (domain.CompileJob, error) {
	var j domain.CompileJob
	var created, updated string
	err := s.Scan(&j.ID, &j.BundleID, &j.Generator, &j.Status, &j.Input, &j.Output, &j.Error, &created, &updated)
	j.CreatedAt = parseDBTime(created)
	j.UpdatedAt = parseDBTime(updated)
	return j, err
}

func scanTemplate(s scanner) (domain.Template, error) {
	var tpl domain.Template
	var created, updated string
	err := s.Scan(&tpl.ID, &tpl.Name, &tpl.Generator, &tpl.Body, &created, &updated)
	tpl.CreatedAt = parseDBTime(created)
	tpl.UpdatedAt = parseDBTime(updated)
	return tpl, err
}

func scanIngest(s scanner) (domain.IngestLedgerEntry, error) {
	var entry domain.IngestLedgerEntry
	var created, updated string
	err := s.Scan(&entry.Hash, &entry.Source, &entry.SourceKey, &entry.ContentHash, &entry.JobID, &created, &updated)
	entry.CreatedAt = parseDBTime(created)
	entry.UpdatedAt = parseDBTime(updated)
	return entry, err
}

func parseDBTime(v string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999Z", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t
		}
	}
	return time.Time{}
}

func Slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "page-" + strconv.Itoa(len(s))
	}
	return out
}
