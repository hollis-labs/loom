package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	paths "github.com/hollis-labs/libs/util/apppaths"
	"gopkg.in/yaml.v3"
)

const FileName = "config.yaml"

type Config struct {
	Paths   PathConfig   `yaml:"paths" json:"paths"`
	Sources SourceConfig `yaml:"sources" json:"sources"`
	Drafts  DraftConfig  `yaml:"drafts" json:"drafts"`
	Ingest  IngestConfig `yaml:"ingest" json:"ingest"`
	Nanite  NaniteConfig `yaml:"nanite" json:"nanite"`
	Filters FilterConfig `yaml:"filters" json:"filters"`
	LLM     LLMConfig    `yaml:"llm" json:"llm"`
}

type PathConfig struct {
	CorpusDir string `yaml:"corpus_dir" json:"corpus_dir"`
	ExportDir string `yaml:"export_dir" json:"export_dir"`
	MediaDir  string `yaml:"media_dir" json:"media_dir"`
}

type SourceConfig struct {
	Git             []GitSource `yaml:"git" json:"git"`
	ClaudeLogs      string      `yaml:"claude_logs" json:"claude_logs"`
	ChatGPTLocal    string      `yaml:"chatgpt_local" json:"chatgpt_local"`
	ChatGPTMarkdown string      `yaml:"chatgpt_md" json:"chatgpt_md"`
	ChatGPTExports  string      `yaml:"chatgpt_exports" json:"chatgpt_exports"`
	NaniteVaults    []string    `yaml:"nanite_vaults" json:"nanite_vaults"`
	External        []string    `yaml:"external" json:"external"`
}

type GitSource struct {
	Name        string   `yaml:"name" json:"name"`
	Path        string   `yaml:"path" json:"path"`
	ExcludePath []string `yaml:"exclude_paths" json:"exclude_paths"`
}

type DraftConfig struct {
	Provider string `yaml:"provider" json:"provider"`
	Model    string `yaml:"model" json:"model"`
	Command  string `yaml:"command" json:"command"`
	Window   string `yaml:"window" json:"window"`
}

type IngestConfig struct {
	Since string `yaml:"since" json:"since"`
}

type NaniteConfig struct {
	BaseURL string `yaml:"base_url" json:"base_url"`
	Vault   string `yaml:"vault" json:"vault"`
}

type FilterConfig struct {
	RedactPatterns []string `yaml:"redact_patterns" json:"redact_patterns"`
	GitExcludePath []string `yaml:"git_exclude_paths" json:"git_exclude_paths"`
}

// LLMConfig configures the LLM Provider (internal/llm) used for compile
// jobs requesting generation_mode=llm (see compiler.GenerationMode). It is
// only consulted when an LLM provider can actually be constructed (e.g. an
// ANTHROPIC_API_KEY is present); Provider/Model may be left blank to use
// the wiring layer's defaults.
type LLMConfig struct {
	// Provider selects the LLM backend, e.g. "anthropic". Currently
	// informational: cmd/loom wires an Anthropic provider whenever
	// ANTHROPIC_API_KEY is set, regardless of this value.
	Provider string `yaml:"provider" json:"provider"`
	// Model is the provider-specific model identifier (e.g.
	// "claude-sonnet-4-6"). Left blank, callers fall back to a built-in
	// default model.
	Model string `yaml:"model" json:"model"`
}

func Default(layout paths.Layout) Config {
	data := layout.DataDir()
	return Config{
		Paths: PathConfig{
			CorpusDir: filepath.Join(data, "corpus"),
			ExportDir: filepath.Join(data, "exports"),
			MediaDir:  filepath.Join(data, "media"),
		},
		Drafts: DraftConfig{
			Provider: "deterministic",
			Window:   "7d",
		},
		Ingest: IngestConfig{Since: "1970-01-01"},
		Nanite: NaniteConfig{
			BaseURL: "http://127.0.0.1:8080",
			Vault:   "default",
		},
		Filters: FilterConfig{
			GitExcludePath: []string{".git", "node_modules", "dist"},
		},
	}
}

func Load(layout paths.Layout, projectFile string) (Config, []string, error) {
	cfg := Default(layout)
	var loaded []string
	userFile := filepath.Join(layout.ConfigDir(), FileName)
	for _, file := range []string{userFile, projectFile} {
		if strings.TrimSpace(file) == "" {
			continue
		}
		next, ok, err := readFile(file)
		if err != nil {
			return Config{}, loaded, err
		}
		if !ok {
			continue
		}
		merge(&cfg, next)
		loaded = append(loaded, file)
	}
	expandConfig(&cfg)
	return cfg, loaded, nil
}

func LoadFile(file string) (Config, error) {
	cfg, ok, err := readFile(file)
	if err != nil {
		return Config{}, err
	}
	if !ok {
		return Config{}, os.ErrNotExist
	}
	expandConfig(&cfg)
	return cfg, nil
}

func readFile(file string) (Config, bool, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, false, nil
		}
		return Config{}, false, fmt.Errorf("read config %q: %w", file, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, false, fmt.Errorf("parse config %q: %w", file, err)
	}
	return cfg, true, nil
}

func merge(dst *Config, src Config) {
	mergeStruct(reflect.ValueOf(dst).Elem(), reflect.ValueOf(src))
}

func mergeStruct(dst, src reflect.Value) {
	for i := 0; i < dst.NumField(); i++ {
		df := dst.Field(i)
		sf := src.Field(i)
		switch df.Kind() {
		case reflect.Struct:
			mergeStruct(df, sf)
		case reflect.String:
			if sf.String() != "" {
				df.SetString(sf.String())
			}
		case reflect.Slice:
			if sf.Len() > 0 {
				df.Set(sf)
			}
		}
	}
}

func expandConfig(cfg *Config) {
	cfg.Paths.CorpusDir = Expand(cfg.Paths.CorpusDir)
	cfg.Paths.ExportDir = Expand(cfg.Paths.ExportDir)
	cfg.Paths.MediaDir = Expand(cfg.Paths.MediaDir)
	cfg.Sources.ClaudeLogs = Expand(cfg.Sources.ClaudeLogs)
	cfg.Sources.ChatGPTLocal = Expand(cfg.Sources.ChatGPTLocal)
	cfg.Sources.ChatGPTMarkdown = Expand(cfg.Sources.ChatGPTMarkdown)
	cfg.Sources.ChatGPTExports = Expand(cfg.Sources.ChatGPTExports)
	cfg.Sources.NaniteVaults = expandSlice(cfg.Sources.NaniteVaults)
	cfg.Sources.External = expandSlice(cfg.Sources.External)
	for i := range cfg.Sources.Git {
		cfg.Sources.Git[i].Path = Expand(cfg.Sources.Git[i].Path)
	}
}

func expandSlice(in []string) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = Expand(v)
	}
	return out
}

func Expand(value string) string {
	if value == "" {
		return ""
	}
	value = os.ExpandEnv(value)
	if value == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(value, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(value, "~/"))
	}
	return value
}
