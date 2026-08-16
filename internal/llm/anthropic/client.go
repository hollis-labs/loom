// Package anthropic implements llm.Provider using Anthropic's Messages API
// (github.com/anthropics/anthropic-sdk-go).
package anthropic

import (
	"context"
	"fmt"
	"strings"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/hollis-labs/loom/internal/llm"
)

// defaultMaxTokens bounds a single wiki-page generation response. Wiki
// pages are short-to-medium markdown documents, not long-form output, so
// this is intentionally conservative.
const defaultMaxTokens = int64(4096)

// Client implements llm.Provider using Anthropic's Messages API.
type Client struct {
	sdk       anthropicsdk.Client
	model     string
	maxTokens int64
}

// New constructs a Client for the given model, authenticating with apiKey.
// model is required. apiKey may be left blank to fall back to
// anthropic-sdk-go's default credential chain (e.g. the ANTHROPIC_API_KEY
// environment variable); extra opts are forwarded to the underlying SDK
// client and exist primarily so tests can point the client at a local fake
// server via option.WithBaseURL.
func New(apiKey, model string, opts ...option.RequestOption) (*Client, error) {
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("llm/anthropic: model is required")
	}
	allOpts := make([]option.RequestOption, 0, len(opts)+1)
	if strings.TrimSpace(apiKey) != "" {
		allOpts = append(allOpts, option.WithAPIKey(apiKey))
	}
	allOpts = append(allOpts, opts...)
	return &Client{
		sdk:       anthropicsdk.NewClient(allOpts...),
		model:     model,
		maxTokens: defaultMaxTokens,
	}, nil
}

// Generate implements llm.Provider.
func (c *Client) Generate(ctx context.Context, prompt llm.Prompt) (string, error) {
	message, err := c.sdk.Messages.New(ctx, anthropicsdk.MessageNewParams{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System: []anthropicsdk.TextBlockParam{
			{Text: llm.SystemPrompt()},
		},
		Messages: []anthropicsdk.MessageParam{
			anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock(prompt.UserPrompt())),
		},
	})
	if err != nil {
		return "", fmt.Errorf("llm/anthropic: generate: %w", err)
	}
	var out strings.Builder
	for _, block := range message.Content {
		if block.Type == "text" {
			out.WriteString(block.Text)
		}
	}
	text := strings.TrimSpace(out.String())
	if text == "" {
		return "", fmt.Errorf("llm/anthropic: empty response content")
	}
	return text, nil
}
