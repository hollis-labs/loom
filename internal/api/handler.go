package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/hollis-labs/loom/internal/compiler"
	"github.com/hollis-labs/loom/internal/directivex"
	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/exporter"
	"github.com/hollis-labs/loom/internal/service"
	"github.com/hollis-labs/loom/internal/storage"
)

type Handler struct {
	repo     *storage.Repository
	compiler *service.Compiler
}

func New(repo *storage.Repository, compiler *service.Compiler) *Handler {
	return &Handler{repo: repo, compiler: compiler}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", h.health)
	mux.HandleFunc("/api/status", h.status)
	mux.HandleFunc("/api/bundles", h.bundles)
	mux.HandleFunc("/api/bundles/", h.bundle)
	mux.HandleFunc("/api/pages", h.pages)
	mux.HandleFunc("/api/pages/", h.page)
	mux.HandleFunc("/api/compile-jobs", h.jobs)
	mux.HandleFunc("/api/compile-jobs/", h.job)
	mux.HandleFunc("/api/ingest", h.ingest)
	mux.HandleFunc("/api/ingest/text", h.ingestText)
	mux.HandleFunc("/api/ingest/", h.ingestEntry)
	mux.HandleFunc("/api/directives", h.directiveLedger)
	mux.HandleFunc("/api/directives/", h.directiveLedgerEntry)
	mux.HandleFunc("/api/directives/parse", h.directives)
	mux.HandleFunc("/api/directives/compile", h.directivesCompile)
	mux.HandleFunc("/api/templates", h.templates)
	mux.HandleFunc("/api/templates/", h.template)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		method(w)
		return
	}
	stats, err := h.repo.Stats(r.Context())
	if err != nil {
		respond(w, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"stats":  stats,
	})
}

func (h *Handler) bundles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		bundles, err := h.repo.ListBundles(r.Context())
		respond(w, bundles, err)
	case http.MethodPost:
		var req struct {
			Slug        string `json:"slug"`
			Title       string `json:"title"`
			Description string `json:"description"`
		}
		if !decode(w, r, &req) {
			return
		}
		bundle, err := h.repo.UpsertBundle(r.Context(), domain.Bundle{Slug: req.Slug, Title: req.Title, Description: req.Description})
		respond(w, bundle, err)
	default:
		method(w)
	}
}

func (h *Handler) bundle(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/api/bundles/"))
	if len(parts) == 0 {
		notFound(w)
		return
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		b, err := h.repo.GetBundle(r.Context(), parts[0])
		respond(w, b, err)
	case len(parts) == 1 && r.Method == http.MethodPut:
		var req struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		}
		if !decode(w, r, &req) {
			return
		}
		bundle, err := h.repo.UpsertBundle(r.Context(), domain.Bundle{Slug: parts[0], Title: req.Title, Description: req.Description})
		respond(w, bundle, err)
	case len(parts) == 2 && parts[1] == "pages" && r.Method == http.MethodGet:
		b, err := h.repo.GetBundle(r.Context(), parts[0])
		if err != nil {
			respond(w, nil, err)
			return
		}
		pages, err := h.repo.ListPages(r.Context(), b.ID, r.URL.Query().Get("q"), queryLimit(r, 100))
		respond(w, pages, err)
	case len(parts) == 2 && parts[1] == "links" && r.Method == http.MethodGet:
		b, err := h.repo.GetBundle(r.Context(), parts[0])
		if err != nil {
			respond(w, nil, err)
			return
		}
		links, err := h.repo.ListBundleLinks(r.Context(), b.ID, queryLimit(r, 100))
		respond(w, links, err)
	case len(parts) == 2 && parts[1] == "verifications" && r.Method == http.MethodGet:
		b, err := h.repo.GetBundle(r.Context(), parts[0])
		if err != nil {
			respond(w, nil, err)
			return
		}
		verifications, err := h.repo.ListBundleVerifications(r.Context(), b.ID, queryLimit(r, 100))
		respond(w, verifications, err)
	case len(parts) == 2 && parts[1] == "compile-jobs" && r.Method == http.MethodPost:
		var req struct {
			Generator string `json:"generator"`
			Input     string `json:"input"`
		}
		if !decode(w, r, &req) {
			return
		}
		job, err := h.compiler.Request(r.Context(), parts[0], req.Generator, req.Input)
		respond(w, job, err)
	case len(parts) == 2 && parts[1] == "compile-jobs" && r.Method == http.MethodGet:
		b, err := h.repo.GetBundle(r.Context(), parts[0])
		if err != nil {
			respond(w, nil, err)
			return
		}
		jobs, err := h.repo.ListJobs(r.Context(), storage.JobFilter{BundleID: b.ID, Status: r.URL.Query().Get("status"), Limit: queryLimit(r, 50)})
		respond(w, jobs, err)
	case len(parts) == 2 && parts[1] == "export" && r.Method == http.MethodPost:
		var req struct {
			Dir string `json:"dir"`
		}
		if r.Body != nil && r.ContentLength != 0 {
			if !decode(w, r, &req) {
				return
			}
		}
		exp, err := exporter.ExportBundle(r.Context(), h.repo, parts[0], req.Dir)
		respond(w, exp, err)
	default:
		notFound(w)
	}
}

func (h *Handler) pages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		method(w)
		return
	}
	bundle := r.URL.Query().Get("bundle")
	if bundle == "" {
		bundle = "nanite"
	}
	b, err := h.repo.GetBundle(r.Context(), bundle)
	if err != nil {
		respond(w, nil, err)
		return
	}
	pages, err := h.repo.ListPages(r.Context(), b.ID, r.URL.Query().Get("q"), queryLimit(r, 100))
	respond(w, pages, err)
}

func (h *Handler) page(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		method(w)
		return
	}
	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/api/pages/"))
	if len(parts) != 2 && (len(parts) != 3 || (parts[2] != "links" && parts[2] != "verifications")) {
		notFound(w)
		return
	}
	p, err := h.repo.GetPageBySlug(r.Context(), parts[0], parts[1])
	if err != nil {
		respond(w, nil, err)
		return
	}
	if len(parts) == 3 {
		switch parts[2] {
		case "links":
			links, err := h.repo.ListPageLinks(r.Context(), p.ID)
			respond(w, links, err)
		case "verifications":
			verifications, err := h.repo.ListPageVerifications(r.Context(), p.ID)
			respond(w, verifications, err)
		default:
			notFound(w)
		}
		return
	}
	respond(w, p, err)
}

func (h *Handler) jobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var bundleID int64
		if bundle := r.URL.Query().Get("bundle"); bundle != "" {
			b, err := h.repo.GetBundle(r.Context(), bundle)
			if err != nil {
				respond(w, nil, err)
				return
			}
			bundleID = b.ID
		}
		jobs, err := h.repo.ListJobs(r.Context(), storage.JobFilter{BundleID: bundleID, Status: r.URL.Query().Get("status"), Limit: queryLimit(r, 50)})
		respond(w, jobs, err)
	case http.MethodPost:
		var req struct {
			Bundle    string `json:"bundle"`
			Generator string `json:"generator"`
			Input     string `json:"input"`
		}
		if !decode(w, r, &req) {
			return
		}
		if req.Bundle == "" {
			req.Bundle = "nanite"
		}
		job, err := h.compiler.Request(r.Context(), req.Bundle, req.Generator, req.Input)
		respond(w, job, err)
	default:
		method(w)
	}
}

func (h *Handler) job(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		method(w)
		return
	}
	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/api/compile-jobs/"))
	if len(parts) == 0 {
		notFound(w)
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "invalid job id", http.StatusBadRequest)
		return
	}
	if len(parts) == 2 && parts[1] == "events" {
		if _, err := h.repo.GetJob(r.Context(), id); err != nil {
			respond(w, nil, err)
			return
		}
		events, err := h.repo.ListEvents(r.Context(), id)
		respond(w, events, err)
		return
	}
	if len(parts) != 1 {
		notFound(w)
		return
	}
	job, err := h.repo.GetJob(r.Context(), id)
	respond(w, job, err)
}

func (h *Handler) ingest(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := queryLimit(r, 100)
		entries, err := h.repo.ListIngest(r.Context(), limit)
		respond(w, entries, err)
	case http.MethodPost:
		var req struct {
			Bundle         string   `json:"bundle"`
			Source         string   `json:"source"`
			Paths          []string `json:"paths"`
			ExcludePaths   []string `json:"exclude_paths"`
			RedactPatterns []string `json:"redact_patterns"`
		}
		if !decode(w, r, &req) {
			return
		}
		if req.Bundle == "" {
			req.Bundle = "nanite"
		}
		result, err := h.compiler.IngestFiles(r.Context(), req.Bundle, req.Paths, req.Source, req.ExcludePaths, req.RedactPatterns)
		respond(w, result, err)
	default:
		method(w)
	}
}

func (h *Handler) ingestEntry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		method(w)
		return
	}
	hash := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/ingest/"), "/")
	if hash == "" {
		notFound(w)
		return
	}
	entry, err := h.repo.GetIngest(r.Context(), hash)
	respond(w, entry, err)
}

func (h *Handler) ingestText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		method(w)
		return
	}
	var req struct {
		Bundle         string   `json:"bundle"`
		Source         string   `json:"source"`
		SourceKey      string   `json:"source_key"`
		Title          string   `json:"title"`
		Body           string   `json:"body"`
		RedactPatterns []string `json:"redact_patterns"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Bundle == "" {
		req.Bundle = "nanite"
	}
	result, err := h.compiler.IngestText(r.Context(), req.Bundle, req.Source, req.SourceKey, req.Title, req.Body, req.RedactPatterns)
	respond(w, result, err)
}

func (h *Handler) directiveLedger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		method(w)
		return
	}
	entries, err := h.repo.ListDirectives(r.Context(), queryLimit(r, 50))
	respond(w, entries, err)
}

func (h *Handler) directiveLedgerEntry(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/directives/parse" || r.URL.Path == "/api/directives/compile" {
		notFound(w)
		return
	}
	if r.Method != http.MethodGet {
		method(w)
		return
	}
	hash := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/directives/"), "/")
	if hash == "" {
		notFound(w)
		return
	}
	entry, err := h.repo.GetDirective(r.Context(), hash)
	respond(w, entry, err)
}

func (h *Handler) directives(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		method(w)
		return
	}
	var req struct {
		Text   string `json:"text"`
		Source string `json:"source"`
		Save   bool   `json:"save"`
	}
	if !decode(w, r, &req) {
		return
	}
	var (
		resp directivex.ParseResponse
		err  error
	)
	if req.Save {
		resp, err = directivex.ParseAndLedger(r.Context(), h.repo, req.Text, req.Source)
	} else {
		resp = directivex.Parse(req.Text, req.Source)
	}
	respond(w, resp, err)
}

func (h *Handler) directivesCompile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		method(w)
		return
	}
	var req struct {
		Bundle string `json:"bundle"`
		Text   string `json:"text"`
		Source string `json:"source"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Bundle == "" {
		req.Bundle = "nanite"
	}
	resp, err := h.compiler.RequestFromDirectives(r.Context(), req.Bundle, req.Text, req.Source)
	respond(w, resp, err)
}

func (h *Handler) templates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		templates, err := h.repo.ListTemplates(r.Context())
		respond(w, templates, err)
	case http.MethodPost:
		var req struct {
			Name      string `json:"name"`
			Generator string `json:"generator"`
			Body      string `json:"body"`
		}
		if !decode(w, r, &req) {
			return
		}
		tpl, err := h.repo.UpsertTemplate(r.Context(), domain.Template{Name: req.Name, Generator: req.Generator, Body: req.Body})
		respond(w, tpl, err)
	default:
		method(w)
	}
}

func (h *Handler) template(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/api/templates/"))
	if len(parts) == 0 {
		notFound(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if len(parts) != 1 {
			notFound(w)
			return
		}
		tpl, err := h.repo.GetTemplate(r.Context(), parts[0])
		respond(w, tpl, err)
	case http.MethodPut:
		if len(parts) != 1 {
			notFound(w)
			return
		}
		var req struct {
			Generator string `json:"generator"`
			Body      string `json:"body"`
		}
		if !decode(w, r, &req) {
			return
		}
		tpl, err := h.repo.UpsertTemplate(r.Context(), domain.Template{Name: parts[0], Generator: req.Generator, Body: req.Body})
		respond(w, tpl, err)
	case http.MethodPost:
		if len(parts) != 2 || parts[1] != "render" {
			notFound(w)
			return
		}
		var req struct {
			Title   string `json:"title"`
			Summary string `json:"summary"`
			Body    string `json:"body"`
			Source  string `json:"source"`
		}
		if !decode(w, r, &req) {
			return
		}
		tpl, err := h.repo.GetTemplate(r.Context(), parts[0])
		if err != nil {
			respond(w, nil, err)
			return
		}
		input, err := json.Marshal(compiler.Request{Title: req.Title, Summary: req.Summary, Body: req.Body, Source: req.Source})
		if err != nil {
			respond(w, nil, err)
			return
		}
		result, err := compiler.CompileWikiPageWithTemplate(0, string(input), tpl.Body)
		respond(w, result, err)
	default:
		method(w)
	}
}

func respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, storage.ErrInvalid) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	defer func() { _ = r.Body.Close() }()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func splitPath(s string) []string {
	var out []string
	for _, p := range strings.Split(strings.Trim(s, "/"), "/") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func queryLimit(r *http.Request, fallback int) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 {
		return fallback
	}
	return limit
}

func method(w http.ResponseWriter) {
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func notFound(w http.ResponseWriter) {
	http.Error(w, "not found", http.StatusNotFound)
}
