CREATE TABLE IF NOT EXISTS wiki_bundles (
	id INTEGER PRIMARY KEY,
	slug TEXT NOT NULL UNIQUE,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS wiki_pages (
	id INTEGER PRIMARY KEY,
	bundle_id INTEGER NOT NULL REFERENCES wiki_bundles(id) ON DELETE CASCADE,
	slug TEXT NOT NULL,
	title TEXT NOT NULL,
	summary TEXT NOT NULL DEFAULT '',
	body TEXT NOT NULL,
	source TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	UNIQUE(bundle_id, slug)
);

CREATE TABLE IF NOT EXISTS wiki_links (
	id INTEGER PRIMARY KEY,
	page_id INTEGER NOT NULL REFERENCES wiki_pages(id) ON DELETE CASCADE,
	target TEXT NOT NULL,
	kind TEXT NOT NULL DEFAULT 'wiki',
	label TEXT NOT NULL DEFAULT '',
	position INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS wiki_verifications (
	id INTEGER PRIMARY KEY,
	page_id INTEGER NOT NULL REFERENCES wiki_pages(id) ON DELETE CASCADE,
	kind TEXT NOT NULL,
	status TEXT NOT NULL,
	message TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_wiki_pages_bundle_slug ON wiki_pages(bundle_id, slug);
