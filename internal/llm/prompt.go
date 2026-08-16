package llm

import "strings"

// Prompt is the input to a Provider's wiki-page generation call: everything
// Loom knows about the requested page, plus the source material the model
// should turn into a page body. Callers are responsible for redacting
// secrets out of Material (see Redact) before it reaches a Provider.
type Prompt struct {
	Title       string
	Type        string
	Summary     string
	Description string
	Source      string
	Material    string
}

// SystemPrompt is the fixed system instruction sent with every generation
// request. It defines the model's role (Loom wiki page author) and the
// output contract: markdown body only, no frontmatter, no commentary - so
// the result can be dropped straight into the same template pipeline the
// deterministic compiler uses.
func SystemPrompt() string {
	return strings.TrimSpace(`
You are Loom's wiki page compiler. Given source material, write a clear,
well-organized wiki page body in Markdown.

Rules:
- Output ONLY the markdown body. No frontmatter, no YAML, no code fence
  wrapping the whole response, no preamble or sign-off.
- Start with a single "# Title" heading that matches the page title.
- Use headings, lists, and short paragraphs where they help; do not pad
  content or invent facts that are not present in the source material.
- Preserve concrete details (names, commands, paths, numbers) from the
  source material verbatim.
- If the source material is thin, write a concise page rather than
  fabricating content to fill space.
`)
}

// UserPrompt renders the page request and source material into the user
// turn sent to the model.
func (p Prompt) UserPrompt() string {
	var b strings.Builder
	b.WriteString("Page title: ")
	b.WriteString(p.Title)
	b.WriteString("\n")
	if strings.TrimSpace(p.Type) != "" {
		b.WriteString("Page type: ")
		b.WriteString(p.Type)
		b.WriteString("\n")
	}
	if strings.TrimSpace(p.Summary) != "" {
		b.WriteString("Summary: ")
		b.WriteString(p.Summary)
		b.WriteString("\n")
	}
	if strings.TrimSpace(p.Description) != "" {
		b.WriteString("Description: ")
		b.WriteString(p.Description)
		b.WriteString("\n")
	}
	if strings.TrimSpace(p.Source) != "" {
		b.WriteString("Source: ")
		b.WriteString(p.Source)
		b.WriteString("\n")
	}
	b.WriteString("\nSource material:\n\n")
	b.WriteString(p.Material)
	return b.String()
}
