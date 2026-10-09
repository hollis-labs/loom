package directivex

import (
	"context"

	directives "github.com/hollis-labs/libs/ui-go/directives"
	"github.com/hollis-labs/loom/internal/storage"
)

type ParsedDirective struct {
	Command      string            `json:"command"`
	Prompt       string            `json:"prompt"`
	Config       map[string]string `json:"config"`
	ContextRange [2]int            `json:"context_range"`
	Hash         string            `json:"hash"`
	Line         int               `json:"line"`
	Source       string            `json:"source"`
	Category     string            `json:"category"`
	New          bool              `json:"new"`
}

type ParseResponse struct {
	Directives []ParsedDirective    `json:"directives"`
	Warnings   []directives.Warning `json:"warnings"`
}

func Parse(text, source string) ParseResponse {
	result := directives.Parse(text, directives.ParserConfig{Source: source})
	out := ParseResponse{Warnings: result.Warnings}
	for _, d := range result.Directives {
		out.Directives = append(out.Directives, fromDirective(d))
	}
	return out
}

func ParseAndLedger(ctx context.Context, repo *storage.Repository, text, source string) (ParseResponse, error) {
	resp := Parse(text, source)
	for i := range resp.Directives {
		d := &resp.Directives[i]
		inserted, err := repo.InsertDirective(ctx, d.Hash, d.Source, d.Command, d.Prompt, d.Line)
		if err != nil {
			return ParseResponse{}, err
		}
		d.New = inserted
	}
	return resp, nil
}

func fromDirective(d directives.Directive) ParsedDirective {
	return ParsedDirective{
		Command:      d.Command,
		Prompt:       d.Prompt,
		Config:       d.Config,
		ContextRange: d.ContextRange,
		Hash:         d.Hash,
		Line:         d.Line,
		Source:       d.Source,
		Category:     d.Category.String(),
	}
}
