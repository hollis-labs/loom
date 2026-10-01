package exporter

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hollis-labs/loom/internal/compiler"
	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/storage"
)

type BundleExport struct {
	Bundle domain.Bundle `json:"bundle"`
	Dir    string        `json:"dir"`
	Files  []string      `json:"files"`
}

type pageArtifact struct {
	Page          domain.Page
	Links         []domain.Link
	Verifications []domain.Verification
}

func ExportBundle(ctx context.Context, repo *storage.Repository, bundleSlug, dir, exportRoot string) (BundleExport, error) {
	b, err := repo.GetBundle(ctx, bundleSlug)
	if err != nil {
		return BundleExport{}, err
	}
	pages, err := repo.ListPages(ctx, b.ID, "", 1000)
	if err != nil {
		return BundleExport{}, err
	}
	root, dir, err := openExportDir(exportRoot, dir, b.Slug)
	if err != nil {
		return BundleExport{}, err
	}
	defer root.Close()
	artifacts := make([]pageArtifact, 0, len(pages))
	for _, p := range pages {
		links, err := repo.ListPageLinks(ctx, p.ID)
		if err != nil {
			return BundleExport{}, err
		}
		verifications, err := repo.ListPageVerifications(ctx, p.ID)
		if err != nil {
			return BundleExport{}, err
		}
		artifacts = append(artifacts, pageArtifact{Page: p, Links: links, Verifications: verifications})
	}
	var files []string
	for _, artifact := range artifacts {
		name := artifact.Page.Slug + ".md"
		if err := writeArtifact(root, name, []byte(pageMarkdown(artifact))); err != nil {
			return BundleExport{}, err
		}
		files = append(files, name)
	}
	sort.Strings(files)
	if err := writeArtifact(root, "index.md", []byte(indexMarkdown(b, pages))); err != nil {
		return BundleExport{}, err
	}
	if err := writeArtifact(root, "log.md", []byte(logMarkdown(b, artifacts))); err != nil {
		return BundleExport{}, err
	}
	files = append([]string{"index.md", "log.md"}, files...)
	return BundleExport{Bundle: b, Dir: dir, Files: files}, nil
}

func pageMarkdown(artifact pageArtifact) string {
	p := artifact.Page
	var b strings.Builder
	b.WriteString(compiler.RenderFrontmatter(p))
	b.WriteString("\n")
	b.WriteString(strings.TrimSpace(p.Body))
	b.WriteString("\n")
	if len(artifact.Links) > 0 {
		b.WriteString("\n## Links\n\n")
		for _, link := range artifact.Links {
			label := link.Label
			if label == "" {
				label = link.Target
			}
			fmt.Fprintf(&b, "- `%s` [%s](%s)\n", link.Kind, linkLabel(label), linkTarget(link.Target))
		}
	}
	if len(artifact.Verifications) > 0 {
		b.WriteString("\n## Verifications\n\n")
		b.WriteString("| Check | Status | Message |\n")
		b.WriteString("|---|---|---|\n")
		for _, verification := range artifact.Verifications {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", tableCell(verification.Kind), tableCell(verification.Status), tableCell(verification.Message))
		}
	}
	return b.String()
}

func linkLabel(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "[", `\[`)
	s = strings.ReplaceAll(s, "]", `\]`)
	return strings.ReplaceAll(s, "\n", " ")
}

func linkTarget(s string) string {
	s = strings.ReplaceAll(s, " ", "%20")
	s = strings.ReplaceAll(s, ")", "%29")
	return strings.ReplaceAll(s, "\n", "")
}

func tableCell(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\r\n", "<br>")
	s = strings.ReplaceAll(s, "\n", "<br>")
	return strings.TrimSpace(s)
}

func indexMarkdown(bundle domain.Bundle, pages []domain.Page) string {
	var b strings.Builder
	b.WriteString("# " + bundle.Title + "\n\n")
	if bundle.Description != "" {
		b.WriteString(bundle.Description + "\n\n")
	}
	for _, p := range pages {
		fmt.Fprintf(&b, "- [%s](%s.md)", p.Title, p.Slug)
		if p.Summary != "" {
			b.WriteString(" - " + p.Summary)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func logMarkdown(bundle domain.Bundle, artifacts []pageArtifact) string {
	var b strings.Builder
	b.WriteString("# " + bundle.Title + " Export Log\n\n")
	fmt.Fprintf(&b, "- bundle: `%s`\n", bundle.Slug)
	fmt.Fprintf(&b, "- pages: `%d`\n", len(artifacts))
	fmt.Fprintf(&b, "- links: `%d`\n", linkCount(artifacts))
	fmt.Fprintf(&b, "- verification_warnings: `%d`\n", verificationWarningCount(artifacts))
	for _, artifact := range artifacts {
		p := artifact.Page
		fmt.Fprintf(&b, "- `%s` updated `%s` links `%d` warnings `%d`\n", p.Slug, p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"), len(artifact.Links), pageWarningCount(artifact.Verifications))
	}
	return b.String()
}

func linkCount(artifacts []pageArtifact) int {
	total := 0
	for _, artifact := range artifacts {
		total += len(artifact.Links)
	}
	return total
}

func verificationWarningCount(artifacts []pageArtifact) int {
	total := 0
	for _, artifact := range artifacts {
		total += pageWarningCount(artifact.Verifications)
	}
	return total
}

func pageWarningCount(verifications []domain.Verification) int {
	total := 0
	for _, verification := range verifications {
		if verification.Status == "warning" {
			total++
		}
	}
	return total
}
