-- Redesigns wiki_links from a raw extracted-markdown-link record into a real
-- page-to-page relation, mirroring FE's fragment_links pattern (loom-architecture.md §5).
--
-- page_id -> from_page_id (rename/clarify: it was always "the page this link was found on").
-- to_page_id is new and nullable: internal wiki links resolve to another wiki_pages.id;
-- external URLs (kind='external') and anchors never get a to_page_id.
-- relation is new, defaulted to 'references' - no richer taxonomy exists yet.
--
-- target/kind/label/position are kept as-is: they're still the source of truth for the
-- raw markdown link text and are useful independent of whether resolution succeeded.

ALTER TABLE wiki_links RENAME COLUMN page_id TO from_page_id;
ALTER TABLE wiki_links ADD COLUMN to_page_id INTEGER REFERENCES wiki_pages(id) ON DELETE SET NULL;
ALTER TABLE wiki_links ADD COLUMN relation TEXT NOT NULL DEFAULT 'references';

-- Best-effort backfill: resolve existing internal ('wiki') links to a sibling page in the
-- same bundle sharing the link's target slug. This is MVP-stage data (few/no real rows);
-- links that don't resolve simply keep to_page_id NULL, same as external targets.
UPDATE wiki_links
SET to_page_id = (
	SELECT p2.id
	FROM wiki_pages p1
	JOIN wiki_pages p2 ON p2.bundle_id = p1.bundle_id AND p2.slug = wiki_links.target
	WHERE p1.id = wiki_links.from_page_id
)
WHERE kind = 'wiki';

CREATE INDEX IF NOT EXISTS idx_wiki_links_from_page ON wiki_links(from_page_id);
CREATE INDEX IF NOT EXISTS idx_wiki_links_to_page ON wiki_links(to_page_id);
