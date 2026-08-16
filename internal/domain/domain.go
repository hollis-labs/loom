package domain

import "time"

type Bundle struct {
	ID          int64     `json:"id"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Page struct {
	ID        int64     `json:"id"`
	BundleID  int64     `json:"bundle_id"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	Body      string    `json:"body"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Link struct {
	ID       int64  `json:"id"`
	PageID   int64  `json:"page_id"`
	Target   string `json:"target"`
	Kind     string `json:"kind"`
	Label    string `json:"label"`
	Position int    `json:"position"`
}

type Verification struct {
	ID        int64     `json:"id"`
	PageID    int64     `json:"page_id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
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
