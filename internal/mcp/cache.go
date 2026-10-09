package mcp

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/hollis-labs/libs/plugin-mcp/go-mcp/budget"
)

// DefaultSoftTruncBytes is the byte threshold above which a wiki_* list-tool
// result is cached in full and its Hint annotated with a wiki_result://<id>
// pointer, instead of being returned inline. Chosen to match Nanite's
// tool-result cache (apps/nanite/internal/tool/cache.go) - CW-20260816-0014
// models this cache directly on that implementation.
const DefaultSoftTruncBytes = 2 * 1024 // 2 KiB

// DefaultHardCapBytes is the maximum body size actually stored in the cache.
// Results exceeding this are recorded as metadata-only (body=NULL); Fetch
// and Search on such rows return an error rather than a partial body.
const DefaultHardCapBytes = 1024 * 1024 // 1 MiB

// DefaultCacheTTLSeconds is the default time-to-live for cached results.
const DefaultCacheTTLSeconds = 3600 // 1 hour

// ResultCache stores and retrieves large wiki_* MCP tool results using the
// wiki_result_cache table (internal/storage/migrations/009_wiki_result_cache.sql).
// It is the per-caller deep-dive-by-id mechanism for CW-20260816-0014: a
// caller who received a truncated/summarized loom_page_search (etc.) result
// can retrieve the full untruncated result later via loom_fetch_result /
// loom_search_result, scoped to the caller_id that produced it.
type ResultCache struct {
	db              *sql.DB
	softTruncBytes  int
	hardCapBytes    int
	cacheTTLSeconds int
}

// ResultCacheConfig configures the result cache thresholds. Zero values fall
// back to package defaults.
type ResultCacheConfig struct {
	SoftTruncBytes  int
	HardCapBytes    int
	CacheTTLSeconds int
}

// NewResultCache creates a ResultCache backed by the given DB connection.
func NewResultCache(db *sql.DB, cfg ResultCacheConfig) *ResultCache {
	soft := cfg.SoftTruncBytes
	if soft <= 0 {
		soft = DefaultSoftTruncBytes
	}
	hard := cfg.HardCapBytes
	if hard <= 0 {
		hard = DefaultHardCapBytes
	}
	ttl := cfg.CacheTTLSeconds
	if ttl <= 0 {
		ttl = DefaultCacheTTLSeconds
	}
	if soft > hard {
		soft = hard
	}
	return &ResultCache{db: db, softTruncBytes: soft, hardCapBytes: hard, cacheTTLSeconds: ttl}
}

// SoftTruncBytes returns the configured soft-truncation threshold, i.e. the
// byte size above which a caller should consider caching a result.
func (c *ResultCache) SoftTruncBytes() int {
	return c.softTruncBytes
}

// Store persists body under a newly generated cache id, scoped to callerID.
// If body exceeds the hard cap, only metadata is stored (body=NULL) and
// Fetch/Search will report the body as unavailable; wasTruncated is true in
// that case. Store does not itself decide whether a result is "big enough"
// to cache - callers (e.g. cacheListResult) make that call using
// SoftTruncBytes and call Store only when they've decided to.
func (c *ResultCache) Store(callerID, toolCallID, toolName, body string) (id string, expiresAt time.Time, err error) {
	id = uuid.NewString()
	now := time.Now().UTC()
	expiresAt = now.Add(time.Duration(c.cacheTTLSeconds) * time.Second)
	bodyLen := len(body)

	var storeBody sql.NullString
	wasTruncated := 0
	if bodyLen <= c.hardCapBytes {
		storeBody = sql.NullString{String: body, Valid: true}
	} else {
		wasTruncated = 1
	}

	_, err = c.db.Exec(
		`INSERT INTO wiki_result_cache (id, caller_id, tool_name, tool_call_id, created_at, expires_at, byte_size, was_truncated, body)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, callerID, toolName, toolCallID,
		now.Format(time.RFC3339), expiresAt.Format(time.RFC3339),
		bodyLen, wasTruncated, storeBody,
	)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("wiki result cache store: %w", err)
	}
	return id, expiresAt, nil
}

// Fetch retrieves a slice of the cached body for (callerID, id). callerID
// scopes the lookup so one caller cannot read another caller's cached
// results. offset/length page through the body; length<=0 (or an offset+
// length past the end) returns the remainder of the body from offset.
func (c *ResultCache) Fetch(callerID, id string, offset, length int) (slice string, totalSize int, err error) {
	var body sql.NullString
	var byteSize int
	var expiresAt string

	err = c.db.QueryRow(
		`SELECT body, byte_size, expires_at FROM wiki_result_cache WHERE id = ? AND caller_id = ?`, id, callerID,
	).Scan(&body, &byteSize, &expiresAt)
	if err == sql.ErrNoRows {
		return "", 0, fmt.Errorf("cached result %q not found", id)
	}
	if err != nil {
		return "", 0, fmt.Errorf("wiki result cache fetch: %w", err)
	}

	expires, parseErr := time.Parse(time.RFC3339, expiresAt)
	if parseErr != nil {
		return "", 0, fmt.Errorf("wiki result cache fetch: malformed expires_at for %q: %w", id, parseErr)
	}
	if time.Now().UTC().After(expires) {
		return "", 0, fmt.Errorf("cached result %q has expired", id)
	}

	if !body.Valid {
		return "", byteSize, fmt.Errorf("cached result %q exceeded hard cap (%d bytes); body not stored", id, byteSize)
	}

	content := body.String
	if offset < 0 {
		offset = 0
	}
	if offset >= len(content) {
		return "", byteSize, nil
	}
	end := offset + length
	if length <= 0 || end > len(content) {
		end = len(content)
	}
	return content[offset:end], byteSize, nil
}

// Match describes a single regex match in a cached result.
type Match struct {
	LineStart int    `json:"line_start"`
	MatchText string `json:"match"`
	Context   string `json:"context"`
}

// Search performs a regex match over the cached body for (callerID, id) and
// returns matches with two lines of context, like grep -C 2. This is the
// companion to Fetch for locating one specific item (e.g. a page by slug)
// inside an otherwise-untruncated cached result without paging through it
// byte-by-byte.
func (c *ResultCache) Search(callerID, id, pattern string, maxMatches int) ([]Match, error) {
	if maxMatches <= 0 {
		maxMatches = 20
	}

	var body sql.NullString
	var expiresAt string
	err := c.db.QueryRow(
		`SELECT body, expires_at FROM wiki_result_cache WHERE id = ? AND caller_id = ?`, id, callerID,
	).Scan(&body, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("cached result %q not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("wiki result cache search: %w", err)
	}

	expires, parseErr := time.Parse(time.RFC3339, expiresAt)
	if parseErr != nil {
		return nil, fmt.Errorf("wiki result cache search: malformed expires_at for %q: %w", id, parseErr)
	}
	if time.Now().UTC().After(expires) {
		return nil, fmt.Errorf("cached result %q has expired", id)
	}

	if !body.Valid {
		return nil, fmt.Errorf("cached result %q body not stored (exceeded hard cap)", id)
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid search pattern: %w", err)
	}

	lines := strings.Split(body.String, "\n")
	var matches []Match
	for i, line := range lines {
		if !re.MatchString(line) {
			continue
		}
		start := i - 2
		if start < 0 {
			start = 0
		}
		end := i + 3
		if end > len(lines) {
			end = len(lines)
		}
		matches = append(matches, Match{
			LineStart: i + 1,
			MatchText: re.FindString(line),
			Context:   strings.Join(lines[start:end], "\n"),
		})
		if len(matches) >= maxMatches {
			break
		}
	}
	return matches, nil
}

// Purge deletes expired cache entries and returns the count removed. Loom
// has no background scheduler today, so this is available for callers (CLI
// maintenance commands, tests) to invoke explicitly rather than being run on
// a timer.
func (c *ResultCache) Purge() (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := c.db.Exec(`DELETE FROM wiki_result_cache WHERE expires_at < ?`, now)
	if err != nil {
		return 0, fmt.Errorf("wiki result cache purge: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// cacheListResult wraps budget.Apply for a wiki_* search/list tool. It
// preserves the existing budget.Envelope shape exactly
// (items/count/total/truncated/hint) - the only change visible to callers
// is that, when the full (pre-budget-limit) item slice's JSON exceeds the
// cache's soft-truncation threshold, a wiki_result://<id> pointer is
// appended to Hint. That id resolves via loom_fetch_result /
// loom_search_result to the full, untruncated result for callerID - the
// deep-dive-by-id mechanism for CW-20260816-0014.
//
// The returned Envelope is a raw value, not a pre-marshaled JSON string:
// go-mcp's ToolHandler contract (v0.5.0+) JSON-marshals a returned value
// into CallToolResult.StructuredContent itself, so callers should return
// this value directly from their tool handler rather than re-marshaling it.
//
// cache may be nil, in which case this behaves exactly like a plain
// budget.Apply(...) call.
// defaultCallerID is the sentinel every wiki_* list/search tool falls back
// to when a caller omits caller_id (see tools.go's stringArg(args,
// "caller_id", "default") call sites). cacheListResult refuses to cache
// under this value: the wiki_result_cache table is a single shared SQLite
// store, not per-process memory, so anything cached under the shared
// default bucket would be fetchable/searchable by any other caller that
// also omits caller_id (and later learns the resulting id) - defeating
// the per-caller isolation this cache exists for. Callers that want the
// deep-dive-by-id feature must identify themselves with a real caller_id.
const defaultCallerID = "default"

func cacheListResult[T any](cache *ResultCache, callerID, toolName string, items []T, cfg budget.Config, hintTemplate string) budget.Envelope {
	env := budget.Apply(items, cfg, hintTemplate)
	if cache != nil && strings.TrimSpace(callerID) != "" && callerID != defaultCallerID && len(items) > 0 {
		if full, err := json.Marshal(items); err == nil && len(full) > cache.SoftTruncBytes() {
			id, expiresAt, cerr := cache.Store(callerID, uuid.NewString(), toolName, string(full))
			if cerr == nil {
				pointer := fmt.Sprintf(
					"Full result (%d items, %d bytes) cached as wiki_result://%s for caller_id=%q, expires_at=%s. Use loom_fetch_result or loom_search_result to retrieve it.",
					len(items), len(full), id, callerID, expiresAt.Format(time.RFC3339),
				)
				if env.Hint == "" {
					env.Hint = pointer
				} else {
					env.Hint = env.Hint + " " + pointer
				}
			}
		}
	}
	return env
}
