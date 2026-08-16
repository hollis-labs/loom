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
