CREATE TABLE IF NOT EXISTS compile_jobs (
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

CREATE TABLE IF NOT EXISTS compile_events (
	id INTEGER PRIMARY KEY,
	job_id INTEGER NOT NULL REFERENCES compile_jobs(id) ON DELETE CASCADE,
	status TEXT NOT NULL,
	message TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_compile_jobs_bundle ON compile_jobs(bundle_id, created_at DESC);
