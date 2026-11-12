-- The Loom schema, squashed to one migration (clean break from the pre-Substrate
-- numbered files). Loom is SQLite only.
--
-- Not here, on purpose: accounts, passwords and sessions are the identity module's
-- identity_* tables.
--
-- Until the first release this is the ONLY migration: a schema change edits this file,
-- because there are no deployed databases to migrate. After the first release changes
-- are new numbered files.

-- +goose Up

CREATE TABLE wiki_bundles (
	id INTEGER PRIMARY KEY,
	slug TEXT NOT NULL UNIQUE,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	scope TEXT NOT NULL DEFAULT 'project',
	okf_export_path TEXT
);

CREATE TABLE wiki_pages (
	id INTEGER PRIMARY KEY,
	bundle_id INTEGER NOT NULL REFERENCES wiki_bundles(id) ON DELETE CASCADE,
	slug TEXT NOT NULL,
	title TEXT NOT NULL,
	summary TEXT NOT NULL DEFAULT '',
	body TEXT NOT NULL,
	source TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	type TEXT NOT NULL DEFAULT 'note',
	path TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	tags TEXT NOT NULL DEFAULT '[]',
	status TEXT NOT NULL DEFAULT 'active',
	stale_after TEXT,
	generated_by TEXT NOT NULL DEFAULT '',
	generated_at TEXT,
	sources TEXT NOT NULL DEFAULT '[]',
	content_hash TEXT NOT NULL DEFAULT '',
	source_fragment_ids TEXT NOT NULL DEFAULT '[]',
	UNIQUE(bundle_id, slug)
);

CREATE INDEX idx_wiki_pages_bundle_slug ON wiki_pages(bundle_id, slug);

CREATE TABLE wiki_links (
	id INTEGER PRIMARY KEY,
	from_page_id INTEGER NOT NULL REFERENCES wiki_pages(id) ON DELETE CASCADE,
	to_page_id INTEGER REFERENCES wiki_pages(id) ON DELETE SET NULL,
	target TEXT NOT NULL,
	kind TEXT NOT NULL DEFAULT 'wiki',
	label TEXT NOT NULL DEFAULT '',
	position INTEGER NOT NULL DEFAULT 0,
	relation TEXT NOT NULL DEFAULT 'references'
);

CREATE INDEX idx_wiki_links_from_page ON wiki_links(from_page_id);
CREATE INDEX idx_wiki_links_to_page ON wiki_links(to_page_id);

-- "by" is a reserved word in SQLite's grammar: always quote it in application SQL.
CREATE TABLE wiki_verifications (
	id INTEGER PRIMARY KEY,
	page_id INTEGER NOT NULL REFERENCES wiki_pages(id) ON DELETE CASCADE,
	kind TEXT NOT NULL,
	status TEXT NOT NULL,
	message TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	"by" TEXT NOT NULL DEFAULT ''
);

CREATE TABLE compile_jobs (
	id INTEGER PRIMARY KEY,
	bundle_id INTEGER NOT NULL REFERENCES wiki_bundles(id) ON DELETE CASCADE,
	generator TEXT NOT NULL,
	status TEXT NOT NULL,
	input TEXT NOT NULL,
	output TEXT NOT NULL DEFAULT '',
	error TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX idx_compile_jobs_bundle ON compile_jobs(bundle_id, created_at DESC);

CREATE TABLE compile_events (
	id INTEGER PRIMARY KEY,
	job_id INTEGER NOT NULL REFERENCES compile_jobs(id) ON DELETE CASCADE,
	status TEXT NOT NULL,
	message TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE directive_ledger (
	hash TEXT PRIMARY KEY,
	source TEXT NOT NULL DEFAULT '',
	command TEXT NOT NULL,
	prompt TEXT NOT NULL,
	line INTEGER NOT NULL,
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE templates (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	generator TEXT NOT NULL,
	body TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE ingest_ledger (
	hash TEXT PRIMARY KEY,
	source TEXT NOT NULL DEFAULT '',
	source_key TEXT NOT NULL DEFAULT '',
	content_hash TEXT NOT NULL,
	job_id INTEGER REFERENCES compile_jobs(id) ON DELETE SET NULL,
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX idx_ingest_ledger_source_key ON ingest_ledger(source, source_key);

-- Per-caller MCP tool result cache. Search/list results too large to return in full are
-- truncated for the caller while the full result is kept here for loom_fetch_result,
-- keyed by the caller's verified principal subject. Rows are TTL-bound (expires_at) and
-- swept lazily.
CREATE TABLE wiki_result_cache (
	id TEXT PRIMARY KEY,
	caller_id TEXT NOT NULL,
	tool_name TEXT NOT NULL,
	tool_call_id TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	expires_at TEXT NOT NULL,
	byte_size INTEGER NOT NULL,
	was_truncated INTEGER NOT NULL DEFAULT 0,
	body TEXT
);

CREATE INDEX idx_wiki_result_cache_caller ON wiki_result_cache (caller_id, tool_name);
CREATE INDEX idx_wiki_result_cache_expires ON wiki_result_cache (expires_at);

-- Seed: the bundle and the template every fresh database starts with.
INSERT INTO wiki_bundles (slug, title, description)
VALUES ('nanite', 'Nanite', 'Seed bundle for Nanite wiki output');

INSERT INTO templates (name, generator, body)
VALUES ('wiki_page.default', 'wiki_page', '# {{title}}

{{body}}');
