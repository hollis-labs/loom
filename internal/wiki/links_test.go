package wiki_test

import (
	"testing"

	"github.com/hollis-labs/loom/internal/wiki"
)

func TestExtractLinksFindsMarkdownAndWikiLinks(t *testing.T) {
	links := wiki.ExtractLinks("See [Nanite](nanite-runtime), [site](https://example.com), [[Compile Jobs|jobs]], and [[Compile Jobs|jobs]].")
	if len(links) != 3 {
		t.Fatalf("links = %+v, want three unique links", links)
	}
	if links[0].Kind != "wiki" || links[0].Target != "nanite-runtime" || links[0].Label != "Nanite" {
		t.Fatalf("markdown wiki link = %+v", links[0])
	}
	if links[1].Kind != "external" || links[1].Target != "https://example.com" {
		t.Fatalf("external link = %+v", links[1])
	}
	if links[2].Kind != "wiki" || links[2].Target != "compile-jobs" || links[2].Label != "jobs" {
		t.Fatalf("wiki link = %+v", links[2])
	}
}
