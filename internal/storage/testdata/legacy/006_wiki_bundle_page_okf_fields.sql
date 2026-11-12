-- Adds OKF-aligned fields to wiki_bundles and wiki_pages (loom-architecture.md §5).
-- Additive only: existing rows get sensible defaults, nothing is dropped or renamed here.

ALTER TABLE wiki_bundles ADD COLUMN scope TEXT NOT NULL DEFAULT 'project';
ALTER TABLE wiki_bundles ADD COLUMN okf_export_path TEXT;

-- OKF core + recommended fields.
ALTER TABLE wiki_pages ADD COLUMN type TEXT NOT NULL DEFAULT 'note';
ALTER TABLE wiki_pages ADD COLUMN path TEXT NOT NULL DEFAULT '';
ALTER TABLE wiki_pages ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE wiki_pages ADD COLUMN tags TEXT NOT NULL DEFAULT '[]';

-- OKF lifecycle family.
ALTER TABLE wiki_pages ADD COLUMN status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE wiki_pages ADD COLUMN stale_after TEXT;

-- OKF trust family.
ALTER TABLE wiki_pages ADD COLUMN generated_by TEXT NOT NULL DEFAULT '';
ALTER TABLE wiki_pages ADD COLUMN generated_at TEXT;

-- OKF provenance family + FE fragment dedup convention.
ALTER TABLE wiki_pages ADD COLUMN sources TEXT NOT NULL DEFAULT '[]';
ALTER TABLE wiki_pages ADD COLUMN content_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE wiki_pages ADD COLUMN source_fragment_ids TEXT NOT NULL DEFAULT '[]';

-- Backfill sensible defaults for rows that predate these columns.
UPDATE wiki_pages SET path = slug || '.md' WHERE path = '';
UPDATE wiki_pages SET description = summary WHERE description = '' AND summary != '';
UPDATE wiki_pages SET sources = json_array(source) WHERE sources = '[]' AND source != '';
