package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Item struct {
	Source      string `json:"source"`
	SourceKey   string `json:"source_key"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	ContentHash string `json:"content_hash"`
}

var ErrInvalid = errors.New("invalid ingest input")

type FileSource struct {
	Paths         []string
	Source        string
	Exclude       []string
	Redact        []string
	AllowedSuffix []string
}

func (s FileSource) Items(ctx context.Context) ([]Item, error) {
	if len(s.Paths) == 0 {
		return nil, fmt.Errorf("%w: at least one path is required", ErrInvalid)
	}
	redactors, err := compileRedactors(s.Redact)
	if err != nil {
		return nil, err
	}
	suffixes := s.AllowedSuffix
	if len(suffixes) == 0 {
		suffixes = []string{".md", ".markdown", ".txt"}
	}
	var files []string
	for _, p := range s.Paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if err := collectPath(ctx, p, s.Exclude, suffixes, &files); err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	source := s.Source
	if strings.TrimSpace(source) == "" {
		source = "file"
	}
	items := make([]Item, 0, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
		body := applyRedactions(string(data), redactors)
		item := Item{
			Source:      source,
			SourceKey:   filepath.Clean(file),
			Title:       titleFor(file, body),
			Body:        body,
			ContentHash: Hash(body),
		}
		items = append(items, item)
	}
	return items, nil
}

func IngestHash(source, sourceKey, contentHash string) string {
	return Hash(strings.Join([]string{source, sourceKey, contentHash}, "\x00"))
}

func Hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func collectPath(ctx context.Context, root string, excludes, suffixes []string, out *[]string) error {
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("%w: stat %s: %v", ErrInvalid, root, err)
	}
	if !info.IsDir() {
		if includeFile(root, excludes, suffixes) {
			*out = append(*out, root)
		}
		return nil
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if path != root && excluded(path, excludes) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if includeSuffix(path, suffixes) {
			*out = append(*out, path)
		}
		return nil
	})
}

func includeFile(path string, excludes, suffixes []string) bool {
	return !excluded(path, excludes) && includeSuffix(path, suffixes)
}

func excluded(path string, excludes []string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	for _, exclude := range excludes {
		exclude = strings.TrimSpace(filepath.ToSlash(exclude))
		if exclude == "" {
			continue
		}
		if strings.Contains(clean, exclude) {
			return true
		}
	}
	return false
}

func includeSuffix(path string, suffixes []string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, suffix := range suffixes {
		if ext == strings.ToLower(suffix) {
			return true
		}
	}
	return false
}

func compileRedactors(patterns []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("%w: compile redaction pattern %q: %v", ErrInvalid, pattern, err)
		}
		out = append(out, re)
	}
	return out, nil
}

func applyRedactions(body string, redactors []*regexp.Regexp) string {
	for _, re := range redactors {
		body = re.ReplaceAllString(body, "[REDACTED]")
	}
	return body
}

func titleFor(path, body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.ReplaceAll(base, "_", " ")
	return strings.TrimSpace(base)
}
