package domain

import "time"

// Bundle is a wiki bundle: a named collection of wiki pages sharing one scope
// (project | meta | personal). See loom-architecture.md §5.
type Bundle struct {
	ID          int64  `json:"id"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// Scope is one of "project", "meta", or "personal". Defaults to "project".
	Scope string `json:"scope"`
	// OKFExportPath is the git repo path once OKF export is enabled for this
	// bundle; empty until then.
	OKFExportPath string    `json:"okf_export_path,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Page is a wiki page. Field names track OKF's frontmatter families directly
// (loom-architecture.md §5): core/recommended (Path, Type, Title, Description,
// Tags), lifecycle (Status, StaleAfter), trust (GeneratedBy, GeneratedAt), and
// provenance (Sources, ContentHash, SourceFragmentIDs).
type Page struct {
	ID       int64  `json:"id"`
	BundleID int64  `json:"bundle_id"`
	Slug     string `json:"slug"`
	// Path is the page's relative export path (e.g. "runtime-notes.md").
	Path string `json:"path"`
	// Type is OKF's one REQUIRED field. Open vocabulary, no registry. Defaults
	// to "note" until callers set it explicitly.
	Type  string `json:"type"`
	Title string `json:"title"`
	// Summary is Loom's original short-form field, still used throughout the
	// API/CLI/exporter. Description is OKF's frontmatter field for the same
	// concept; it defaults to Summary when not set explicitly. Both are kept
	// rather than renaming Summary, to avoid a wide, low-value rename churn.
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	// Status and StaleAfter are OKF's lifecycle family. Status defaults to
	// "active".
	Status     string     `json:"status"`
	StaleAfter *time.Time `json:"stale_after,omitempty"`
	// GeneratedBy and GeneratedAt are OKF's trust family: actor as
	// producer/version, and when it produced this page.
	GeneratedBy string     `json:"generated_by"`
	GeneratedAt *time.Time `json:"generated_at,omitempty"`
	Body        string     `json:"body"`
	// Source is Loom's original single-value provenance field (kept for
	// backward compatibility). Sources is OKF's json provenance family; when
	// not set explicitly it defaults to a one-element array containing Source.
	Source            string    `json:"source"`
	Sources           []string  `json:"sources"`
	ContentHash       string    `json:"content_hash"`
	SourceFragmentIDs []int64   `json:"source_fragment_ids"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Link is a page-to-page relation extracted from a page's body, mirroring FE's
// fragment_links pattern (loom-architecture.md §5). ToPageID is nil when Target
// is an external URL/anchor, or an internal wiki link that didn't resolve to a
// known page in the same bundle.
type Link struct {
	ID         int64  `json:"id"`
	FromPageID int64  `json:"from_page_id"`
	ToPageID   *int64 `json:"to_page_id,omitempty"`
	// Relation defaults to "references"; no richer taxonomy exists yet.
	Relation string `json:"relation"`
	Target   string `json:"target"`
	Kind     string `json:"kind"`
	Label    string `json:"label"`
	Position int    `json:"position"`
}

// Verification is a trust-signal record for a page. Kind/Status/Message are
// Loom's own deterministic structural/content checks (internal/wiki/verifications.go,
// run at write-time). By is OKF's "verified" family actor field - it
// distinguishes "Loom's own structural checker said X" from "someone/something
// actually attested this page is correct", which are different trust signals.
type Verification struct {
	ID        int64     `json:"id"`
	PageID    int64     `json:"page_id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	By        string    `json:"by"`
	CreatedAt time.Time `json:"created_at"`
}

type CompileJob struct {
	ID        int64     `json:"id"`
	BundleID  int64     `json:"bundle_id"`
	Generator string    `json:"generator"`
	Status    string    `json:"status"`
	Input     string    `json:"input"`
	Output    string    `json:"output"`
	Error     string    `json:"error"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DiffSummary describes how a compiled page's new body compares to the page
// that already existed at the same bundle+slug, if any (see
// CompileOutput.Diff). It is computed once, in service.Compiler.run, from
// the pre-write state of the existing page — i.e. before
// storage.Repository.UpsertPage overwrites it — since that's the only point
// in the compile flow where "what did this page look like right before this
// write" is still observable.
type DiffSummary struct {
	OldContentHash string `json:"old_content_hash"`
	NewContentHash string `json:"new_content_hash"`
	OldLineCount   int    `json:"old_line_count"`
	NewLineCount   int    `json:"new_line_count"`
	// LineDelta is NewLineCount - OldLineCount (positive = grew, negative =
	// shrank).
	LineDelta int `json:"line_delta"`
	// SimilarityRatio is 2*|LCS(oldLines,newLines)| / (OldLineCount +
	// NewLineCount), the same line-based ratio classic diff tools (e.g.
	// Python's difflib.SequenceMatcher) use to describe "how much of the
	// two sequences matches" — 1.0 means the bodies are line-identical
	// (or both empty), 0.0 means they share no lines at all. See
	// internal/service/confidence.go for the full heuristic this feeds.
	SimilarityRatio float64 `json:"similarity_ratio"`
	// UnifiedDiff is a single-hunk unified-diff-style rendering (---/+++/@@
	// header, then full-context " "/"-"/"+" prefixed lines) of old vs new
	// body. Left empty when OldContentHash == NewContentHash (nothing to
	// show) or when the bodies are too large to diff cheaply (see
	// maxDiffCells in internal/service/confidence.go) — callers should
	// treat an empty UnifiedDiff as "not computed", not "no changes",
	// unless SimilarityRatio is also 1.0.
	UnifiedDiff string `json:"unified_diff,omitempty"`
}

// CompileOutput is the JSON shape marshaled into CompileJob.Output for a
// successful wiki_page compile (see service.Compiler.run). Confidence and
// ConfidenceTier are a heuristic safety signal — not a guarantee — meant
// for a caller (e.g. Nanite's Curator, CW-20260816-0020) deciding whether
// to accept this compile's write directly or stage it for review; see
// internal/service/confidence.go for the exact formula and reasoning. Diff
// is nil when PageExisted is false: a new page can't conflict with
// anything, so there's nothing to compare against.
type CompileOutput struct {
	Page Page `json:"page"`
	// PageExisted reports whether a page already lived at this
	// bundle+slug before this compile's write.
	PageExisted bool `json:"page_existed"`
	// Confidence is 0.0-1.0: how safe this write is to accept without
	// human review. 1.0 for a brand-new page or a no-op (content_hash
	// unchanged) overwrite; otherwise equal to Diff.SimilarityRatio.
	Confidence float64 `json:"confidence"`
	// ConfidenceTier buckets Confidence into "high" (>=0.85), "medium"
	// (>=0.50), or "low" (<0.50) — see confidenceTierHigh/Medium/Low.
	ConfidenceTier string       `json:"confidence_tier"`
	Diff           *DiffSummary `json:"diff,omitempty"`
}

type CompileEvent struct {
	ID        int64     `json:"id"`
	JobID     int64     `json:"job_id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type LedgerEntry struct {
	Hash      string    `json:"hash"`
	Source    string    `json:"source"`
	Command   string    `json:"command"`
	Prompt    string    `json:"prompt"`
	Line      int       `json:"line"`
	CreatedAt time.Time `json:"created_at"`
}

type Template struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Generator string    `json:"generator"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type IngestLedgerEntry struct {
	Hash        string    `json:"hash"`
	Source      string    `json:"source"`
	SourceKey   string    `json:"source_key"`
	ContentHash string    `json:"content_hash"`
	JobID       int64     `json:"job_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type RepositoryStats struct {
	Bundles              int64 `json:"bundles"`
	Pages                int64 `json:"pages"`
	Links                int64 `json:"links"`
	Verifications        int64 `json:"verifications"`
	CompileJobs          int64 `json:"compile_jobs"`
	CompileEvents        int64 `json:"compile_events"`
	DirectiveLedger      int64 `json:"directive_ledger"`
	IngestLedger         int64 `json:"ingest_ledger"`
	Templates            int64 `json:"templates"`
	CompletedJobs        int64 `json:"completed_jobs"`
	FailedJobs           int64 `json:"failed_jobs"`
	VerificationWarnings int64 `json:"verification_warnings"`
}
