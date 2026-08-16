-- Per-caller MCP tool result cache (CW-20260816-0014).
--
-- Modeled on Nanite's tool_result_cache (apps/nanite/internal/tool/cache.go):
-- search/list-shaped wiki_* MCP tool results that are too large to return in
-- full get truncated for the immediate LLM-visible response, while the full
-- result is stored here, keyed by caller/session + tool + call, and fetchable
-- later by id via the loom_fetch_result deep-dive tool. Rows are TTL-bound
-- (expires_at) and lazily purged - no cron job, just filtered out of Fetch
-- reads and periodically swept by ResultCache.Purge.
CREATE TABLE IF NOT EXISTS wiki_result_cache (
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

CREATE INDEX IF NOT EXISTS idx_wiki_result_cache_caller ON wiki_result_cache (caller_id, tool_name);
CREATE INDEX IF NOT EXISTS idx_wiki_result_cache_expires ON wiki_result_cache (expires_at);
