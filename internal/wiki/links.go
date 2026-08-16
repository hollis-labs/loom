package wiki

import (
	"regexp"
	"strings"

	"github.com/hollis-labs/loom/internal/domain"
)

var (
	markdownLinkRE = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	wikiLinkRE     = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
)

func ExtractLinks(body string) []domain.Link {
	links := make([]domain.Link, 0)
	seen := map[string]bool{}
	add := func(target, kind, label string, pos int) {
		target = strings.TrimSpace(target)
		label = strings.TrimSpace(label)
		if target == "" {
			return
		}
		key := kind + "\x00" + target + "\x00" + label
		if seen[key] {
			return
		}
		seen[key] = true
		links = append(links, domain.Link{Target: target, Kind: kind, Label: label, Position: pos})
	}
	for _, match := range markdownLinkRE.FindAllStringSubmatchIndex(body, -1) {
		label := body[match[2]:match[3]]
		target := body[match[4]:match[5]]
		add(target, linkKind(target), label, match[0])
	}
	for _, match := range wikiLinkRE.FindAllStringSubmatchIndex(body, -1) {
		target := body[match[2]:match[3]]
		label := target
		if match[4] >= 0 {
			label = body[match[4]:match[5]]
		}
		add(slug(target), "wiki", label, match[0])
	}
	return links
}

func linkKind(target string) string {
	target = strings.ToLower(strings.TrimSpace(target))
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "mailto:") {
		return "external"
	}
	if strings.HasPrefix(target, "#") {
		return "anchor"
	}
	return "wiki"
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "page"
	}
	return out
}
