package ingest

import (
	"fmt"
	"strings"
)

type TextSource struct {
	Source    string
	SourceKey string
	Title     string
	Body      string
	Redact    []string
}

func (s TextSource) Item() (Item, error) {
	body := strings.TrimSpace(s.Body)
	if body == "" {
		return Item{}, fmt.Errorf("%w: text body is required", ErrInvalid)
	}
	redactors, err := compileRedactors(s.Redact)
	if err != nil {
		return Item{}, err
	}
	body = applyRedactions(body, redactors)
	source := strings.TrimSpace(s.Source)
	if source == "" {
		source = "text"
	}
	sourceKey := strings.TrimSpace(s.SourceKey)
	if sourceKey == "" {
		sourceKey = Hash(source + "\x00" + body)
	}
	title := strings.TrimSpace(s.Title)
	if title == "" {
		title = titleFor(sourceKey, body)
	}
	return Item{
		Source:      source,
		SourceKey:   sourceKey,
		Title:       title,
		Body:        body,
		ContentHash: Hash(body),
	}, nil
}
