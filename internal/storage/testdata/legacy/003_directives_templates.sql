CREATE TABLE IF NOT EXISTS directive_ledger (
	hash TEXT PRIMARY KEY,
	source TEXT NOT NULL DEFAULT '',
	command TEXT NOT NULL,
	prompt TEXT NOT NULL,
	line INTEGER NOT NULL,
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS templates (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	generator TEXT NOT NULL,
	body TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
