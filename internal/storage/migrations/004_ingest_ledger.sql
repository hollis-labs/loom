CREATE TABLE IF NOT EXISTS ingest_ledger (
	hash TEXT PRIMARY KEY,
	source TEXT NOT NULL DEFAULT '',
	source_key TEXT NOT NULL DEFAULT '',
	content_hash TEXT NOT NULL,
	job_id INTEGER REFERENCES compile_jobs(id) ON DELETE SET NULL,
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_ingest_ledger_source_key ON ingest_ledger(source, source_key);
