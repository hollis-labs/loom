package service

import (
	"fmt"
	"strings"

	"github.com/hollis-labs/loom/internal/domain"
)

// Confidence tiers for domain.CompileOutput.ConfidenceTier. See
// buildCompileOutput's doc comment for the full heuristic and the
// reasoning behind these specific thresholds.
const (
	confidenceTierHigh   = "high"
	confidenceTierMedium = "medium"
	confidenceTierLow    = "low"

	similarityHighThreshold   = 0.85
	similarityMediumThreshold = 0.50
)

// maxDiffCells caps the O(len(oldLines)*len(newLines)) dynamic-programming
// table buildCompileOutput's line-similarity computation (lcsWithOps) uses.
// Beyond this, computing an exact, order-aware similarity/diff gets
// expensive in both time and memory for what is meant to be a lightweight,
// per-compile-job heuristic — so bodies whose line-count product exceeds
// this fall back to approxCommonLines (a cheap, order-insensitive bag
// intersection) for SimilarityRatio, and skip UnifiedDiff entirely rather
// than compute or return something very large. 1,000 lines-by-1,000 lines
// comfortably covers ordinary wiki pages; only unusually large bodies hit
// the fallback.
const maxDiffCells = 1_000_000

// buildCompileOutput scores how safe it is to accept a compile's write
// directly vs. stage it for review, and — when an existing page is being
// overwritten — computes a diff summary against its pre-write state.
// existing is the page that lived at newPage's bundle+slug immediately
// before this compile's UpsertPage call, or nil if none did (see
// Compiler.existingPage).
//
// This is the actual decision surface CW-20260816-0013 exists for: Nanite's
// Curator (CW-20260816-0020) needs to tell "safe to write directly" from
// "needs review" from a compile-job response, without re-deriving that
// itself. The heuristic:
//
//   - No page exists yet at this bundle+slug: Confidence 1.0, tier "high".
//     There is nothing to conflict with — writing is always safe. Diff is
//     nil (nothing to diff against).
//   - A page exists and its content_hash is unchanged: Confidence 1.0,
//     tier "high". The write is a no-op relative to what's already stored
//     — trivially safe. Diff is non-nil but trivial (SimilarityRatio 1.0,
//     LineDelta 0, empty UnifiedDiff).
//   - A page exists and its body differs: Confidence equals a line-based
//     similarity ratio between the old and new body (see
//     domain.DiffSummary.SimilarityRatio) — computed by aligning lines
//     with a longest-common-subsequence and scoring
//     2*|LCS| / (len(oldLines)+len(newLines)), the same style of ratio
//     classic diff tools (e.g. Python's difflib.SequenceMatcher) use to
//     describe how much of two sequences matches. A small, additive edit
//     (e.g. one paragraph appended, a typo fixed) keeps most lines intact
//     and scores close to 1.0; a substantial rewrite or wholesale
//     replacement shares few lines and scores close to 0.0.
//
// Tiers (ConfidenceTier): >=0.85 "high", >=0.50 "medium", else "low".
// These exist so a caller can act on ConfidenceTier alone without picking
// its own threshold: "high" means "safe to accept unattended" (either no
// conflict is possible, or the edit is minor/additive and very unlikely to
// clobber meaningfully different content); "low" means "the new body
// shares little with what's stored — stage for review, this may be an
// unrelated or conflicting page"; "medium" is the deliberately cautious
// middle ground (a real but partial rewrite) that Curator should still
// stage rather than auto-accept, just without the same urgency as "low".
//
// This is a line-similarity heuristic, not semantic understanding — it
// cannot tell "the same facts reworded" from "different facts of similar
// length" apart. That's an explicit, documented limitation: the task this
// implements frames LLM self-reported confidence as a future improvement
// for exactly that reason (see compiler.CompileWikiPageWithLLM), and this
// heuristic is judged actionable specifically because Curator's real
// decision — "is there existing content here I might be about to clobber,
// and how much of it does the new write keep" — is answerable from line
// overlap even without semantic understanding.
func buildCompileOutput(existing *domain.Page, newPage domain.Page) domain.CompileOutput {
	out := domain.CompileOutput{Page: newPage, PageExisted: existing != nil}
	if existing == nil {
		out.Confidence = 1.0
		out.ConfidenceTier = confidenceTierHigh
		return out
	}
	if existing.ContentHash == newPage.ContentHash {
		oldLines, newLines := countLines(existing.Body), countLines(newPage.Body)
		out.Confidence = 1.0
		out.ConfidenceTier = confidenceTierHigh
		out.Diff = &domain.DiffSummary{
			OldContentHash:  existing.ContentHash,
			NewContentHash:  newPage.ContentHash,
			OldLineCount:    oldLines,
			NewLineCount:    newLines,
			LineDelta:       newLines - oldLines,
			SimilarityRatio: 1.0,
		}
		return out
	}
	diff := lineDiff(existing.Body, newPage.Body)
	diff.OldContentHash = existing.ContentHash
	diff.NewContentHash = newPage.ContentHash
	out.Diff = &diff
	out.Confidence = diff.SimilarityRatio
	out.ConfidenceTier = tierFor(diff.SimilarityRatio)
	return out
}

func tierFor(similarity float64) string {
	switch {
	case similarity >= similarityHighThreshold:
		return confidenceTierHigh
	case similarity >= similarityMediumThreshold:
		return confidenceTierMedium
	default:
		return confidenceTierLow
	}
}

func countLines(body string) int {
	return len(splitLines(body))
}

// splitLines splits body on "\n" the way lineDiff/countLines want: an
// empty body is zero lines, not strings.Split's one-element []string{""}.
func splitLines(body string) []string {
	if body == "" {
		return nil
	}
	return strings.Split(body, "\n")
}

// lineDiff computes a domain.DiffSummary (minus the content-hash fields,
// which buildCompileOutput fills in from context lineDiff doesn't have)
// comparing oldBody to newBody line-by-line.
func lineDiff(oldBody, newBody string) domain.DiffSummary {
	oldLines := splitLines(oldBody)
	newLines := splitLines(newBody)
	summary := domain.DiffSummary{
		OldLineCount: len(oldLines),
		NewLineCount: len(newLines),
		LineDelta:    len(newLines) - len(oldLines),
	}

	var (
		lcsLen int
		ops    []diffOp
	)
	if len(oldLines)*len(newLines) > maxDiffCells {
		// Too large to diff exactly and cheaply — fall back to an
		// order-insensitive similarity and skip the unified diff (see
		// maxDiffCells).
		lcsLen = approxCommonLines(oldLines, newLines)
	} else {
		lcsLen, ops = lcsWithOps(oldLines, newLines)
	}

	total := len(oldLines) + len(newLines)
	if total == 0 {
		summary.SimilarityRatio = 1.0
	} else {
		summary.SimilarityRatio = 2 * float64(lcsLen) / float64(total)
	}
	summary.UnifiedDiff = renderUnifiedDiff(ops)
	return summary
}

// approxCommonLines counts lines common to both a and b as a multiset
// (bag) intersection — order-insensitive, unlike lcsWithOps's LCS, but
// O(len(a)+len(b)) instead of O(len(a)*len(b)). Used only when the exact
// LCS table would exceed maxDiffCells.
func approxCommonLines(a, b []string) int {
	counts := make(map[string]int, len(a))
	for _, line := range a {
		counts[line]++
	}
	common := 0
	for _, line := range b {
		if counts[line] > 0 {
			counts[line]--
			common++
		}
	}
	return common
}

type diffOpKind int

const (
	diffEqual diffOpKind = iota
	diffDelete
	diffInsert
)

type diffOp struct {
	kind diffOpKind
	line string
}

// lcsWithOps returns the length of the longest common subsequence of lines
// between a and b, and the sequence of equal/delete/insert operations that
// aligns a onto b along that LCS (a standard O(len(a)*len(b))
// dynamic-programming line diff, the same technique tools like `diff` use).
// Callers must keep len(a)*len(b) within maxDiffCells — this is the
// expensive path lineDiff guards with that cap.
func lcsWithOps(a, b []string) (int, []diffOp) {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				dp[i][j] = dp[i+1][j+1] + 1
			case dp[i+1][j] >= dp[i][j+1]:
				dp[i][j] = dp[i+1][j]
			default:
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	ops := make([]diffOp, 0, n+m)
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{diffEqual, a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, diffOp{diffDelete, a[i]})
			i++
		default:
			ops = append(ops, diffOp{diffInsert, b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{diffDelete, a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{diffInsert, b[j]})
	}
	return dp[0][0], ops
}

// renderUnifiedDiff renders ops as a single-hunk unified-diff-style string:
// a "--- old" / "+++ new" header, one "@@ -1,<oldCount> +1,<newCount> @@"
// hunk header, then every line prefixed " " (equal), "-" (deleted from
// old), or "+" (inserted in new). Unlike a typical `diff -u`, this doesn't
// window hunks down to a few lines of context around each change — it's
// one hunk covering the whole body. That's a deliberate simplification for
// wiki-page-sized bodies (see maxDiffCells for the size this is skipped
// entirely beyond); it keeps the implementation straightforward while
// still being a real, line-accurate unified diff, not a placeholder.
// Returns "" when ops is empty (lcsWithOps was skipped — see lineDiff) or
// contains no delete/insert (shouldn't normally happen here, since
// buildCompileOutput only reaches lineDiff after content_hash differs, but
// handled defensively).
func renderUnifiedDiff(ops []diffOp) string {
	if len(ops) == 0 {
		return ""
	}
	oldCount, newCount, changed := 0, 0, false
	for _, op := range ops {
		switch op.kind {
		case diffEqual:
			oldCount++
			newCount++
		case diffDelete:
			oldCount++
			changed = true
		case diffInsert:
			newCount++
			changed = true
		}
	}
	if !changed {
		return ""
	}
	var b strings.Builder
	b.WriteString("--- old\n+++ new\n")
	fmt.Fprintf(&b, "@@ -1,%d +1,%d @@\n", oldCount, newCount)
	for _, op := range ops {
		switch op.kind {
		case diffEqual:
			b.WriteString(" " + op.line + "\n")
		case diffDelete:
			b.WriteString("-" + op.line + "\n")
		case diffInsert:
			b.WriteString("+" + op.line + "\n")
		}
	}
	return b.String()
}
