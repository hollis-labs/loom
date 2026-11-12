// Package mcptest builds a Loom MCP server without the Substrate mcp module, for tests that call
// tools directly. It is imported by tests only.
package mcptest

import (
	gmcp "github.com/hollis-labs/libs/mcp/plugin-mcp/go-mcp/server"

	loommcp "github.com/hollis-labs/loom/internal/mcp"
	"github.com/hollis-labs/loom/internal/service"
	"github.com/hollis-labs/loom/internal/storage"
)

// New returns a server with every Loom tool registered.
func New(repo *storage.Repository, comp *service.Compiler) *gmcp.Server {
	return NewWithOptions(repo, comp, loommcp.Options{})
}

// NewWithOptions is New with tool options.
func NewWithOptions(repo *storage.Repository, comp *service.Compiler, opts loommcp.Options) *gmcp.Server {
	srv := gmcp.NewServer("loom", "test")
	loommcp.RegisterTools(srv, repo, comp, opts)
	return srv
}
