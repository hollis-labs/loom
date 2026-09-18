package mcp

// toolHints is the explicit, hand-reviewed MCP tool-annotation set for every
// tool this server registers (adopting the portfolio's go-mcp v0.5.0
// standard, adr_go-mcp-official-sdk-consolidation). go-mcp requires
// ReadOnlyHint/DestructiveHint/IdempotentHint/OpenWorldHint to be always
// explicit, never inferred from a tool's name -- this table is the human
// decision the framework refuses to make for us, kept as one reviewable,
// editable fact per tool rather than derived at runtime. Mirrors Torque's
// toolHints convention (internal/mcpadapter/tool_annotations.go).
//
// OpenWorldHint is false for every Loom tool: every tool operates on Loom's
// own closed-domain wiki/bundle/compile-job store, never an open-ended
// external system -- even loom_compile_request's optional Anthropic call
// targets one fixed external system, not an open-ended set of entities.
//
// Idempotent reflects verified service-layer behavior, not a guess:
// loom_compile_request always inserts a new compile_job row per call
// (non-idempotent, matches Torque's *_create convention), while
// loom_compile_from_directives/loom_ingest_files/loom_ingest_text/
// loom_directives_parse hash-ledger their input and skip already-seen
// entries on repeat calls -- see TestRequestFromDirectivesIsIdempotent,
// TestIngestFilesIsIdempotent, TestIngestTextIsIdempotent in
// internal/service/service_test.go. registerTool panics if a registered
// tool name has no entry here -- add one deliberately rather than letting a
// new tool ship with silently-defaulted hints.
type toolHints struct {
	ReadOnly    bool
	Destructive bool
	Idempotent  bool
}

var toolAnnotations = map[string]toolHints{
	"loom_status":                  {true, false, true},
	"loom_bundle_list":             {true, false, true},
	"loom_bundle_get":              {true, false, true},
	"loom_bundle_put":              {false, false, true},
	"loom_page_search":             {true, false, true},
	"loom_page_get":                {true, false, true},
	"loom_page_links":              {true, false, true},
	"loom_bundle_links":            {true, false, true},
	"loom_page_verifications":      {true, false, true},
	"loom_bundle_verifications":    {true, false, true},
	"loom_page_conformance":        {true, false, true},
	"loom_bundle_conformance":      {true, false, true},
	"loom_compile_request":         {false, false, false},
	"loom_compile_from_directives": {false, false, true},
	"loom_compile_job_get":         {true, false, true},
	"loom_compile_job_list":        {true, false, true},
	"loom_ingest_files":            {false, false, true},
	"loom_ingest_text":             {false, false, true},
	"loom_ingest_list":             {true, false, true},
	"loom_ingest_get":              {true, false, true},
	"loom_export_bundle":           {false, false, true},
	"loom_directives_parse":        {false, false, true},
	"loom_directive_list":          {true, false, true},
	"loom_directive_get":           {true, false, true},
	"loom_template_list":           {true, false, true},
	"loom_template_get":            {true, false, true},
	"loom_template_put":            {false, false, true},
	"loom_template_render":         {true, false, true},
	"loom_fetch_result":            {true, false, true},
	"loom_search_result":           {true, false, true},
}
