// internal/docs/docs.go
package docs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/johnknott/repocontext/internal/git"
)

type Metadata struct {
	CommitHash   string            `json:"commit_hash"`
	GeneratedAt  time.Time         `json:"generated_at"`
	ModelUsed    string            `json:"model_used"`
	FileVersions map[string]string `json:"file_versions"`
	Deduplicated bool              `json:"deduplicated"` // Add this field
}

type Generator struct {
	RepoPath  string
	DocsPath  string
	Files     map[string]string // filepath -> content
	LLMClient LLMClient
	Meta      *Metadata
}

type LLMClient interface {
	GenerateWithStream(ctx context.Context, prompt string) (string, error)
}

const (
	OverviewFileName       = "01_overview.md"
	GettingStartedFileName = "02_getting_started.md"
	UsageFileName          = "03_usage.md"
	FullDocFileName        = "full.md"
	MetadataFileName       = "metadata.json"
)

func New(repoPath string, commitHash string, tag string, llmClient LLMClient) (*Generator, error) {
	// repoPath is the src directory, go up one level to get the version directory
	versionDir := filepath.Dir(repoPath)
	docsPath := filepath.Join(versionDir, "docs")

	if err := os.MkdirAll(docsPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create docs directory: %w", err)
	}

	return &Generator{
		RepoPath:  repoPath,
		DocsPath:  docsPath,
		LLMClient: llmClient,
		Files:     make(map[string]string),
	}, nil
}

func (g *Generator) LoadOrGenerateDocs(files map[string]*git.RepoFile, meta *Metadata) error {
	if g.isCacheValid() {
		fmt.Println("Using cached documentation...")
		return g.loadFromCache()
	}

	g.Meta = meta
	if err := g.generateDocs(files); err != nil {
		return err
	}

	// Don't set Deduplicated here - that will be done in CleanupDuplicates

	// Save metadata
	return g.saveMetadata()
}

func (g *Generator) isCacheValid() bool {
	metaPath := filepath.Join(g.DocsPath, MetadataFileName)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return false
	}

	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return false
	}

	// TODO: Compare commit hash with current repo state
	// TODO: Compare file versions

	g.Meta = &meta
	return true
}

func (g *Generator) generateDocs(files map[string]*git.RepoFile) error {
	// Read file contents
	for path, _ := range files {
		content, err := os.ReadFile(filepath.Join(g.RepoPath, path))
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", path, err)
		}
		g.Files[path] = string(content)
	}

	// Generate each section
	sections := []string{OverviewFileName, GettingStartedFileName, UsageFileName}
	for _, section := range sections {
		content, err := g.generateSection(section)
		if err != nil {
			return fmt.Errorf("failed to generate section %s: %w", section, err)
		}

		if err := os.WriteFile(filepath.Join(g.DocsPath, section), []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write section %s: %w", section, err)
		}
	}

	return g.generateFullDoc()
}

func (g *Generator) generateSection(section string) (string, error) {
	var prompt string
	switch section {
	case OverviewFileName:
		prompt = g.buildOverviewPrompt()
	case GettingStartedFileName:
		prompt = g.buildGettingStartedPrompt()
	case UsageFileName:
		prompt = g.buildUsagePrompt()
	default:
		return "", fmt.Errorf("unknown section: %s", section)
	}

	fmt.Printf("\nGenerating %s...\n", section)
	return g.LLMClient.GenerateWithStream(context.Background(), prompt)
}

func (g *Generator) generateFullDoc() error {
	var fullDoc strings.Builder

	sections := []string{OverviewFileName, GettingStartedFileName, UsageFileName}
	for _, section := range sections {
		content, err := os.ReadFile(filepath.Join(g.DocsPath, section))
		if err != nil {
			return fmt.Errorf("failed to read section %s: %w", section, err)
		}
		fullDoc.Write(content)
		fullDoc.WriteString("\n\n")
	}

	return os.WriteFile(filepath.Join(g.DocsPath, FullDocFileName), []byte(fullDoc.String()), 0644)
}

func (g *Generator) buildOverviewPrompt() string {
	return fmt.Sprintf(`You are analyzing a software repository to create comprehensive documentation. 
Based on the repository files provided below, create a detailed overview document in markdown format that includes:

1. A clear description of what the project does
2. Key features and capabilities
3. High-level architecture/design
4. Technologies used and dependencies
5. Project status (based on what you can determine from the code)

Please ensure the output is well-formatted markdown with appropriate headers and sections.
Use code examples from the files where relevant.

Repository files:
%s

Contents:
%s`, g.formatFileList(), g.formatFileContents())
}

func (g *Generator) buildGettingStartedPrompt() string {
	return fmt.Sprintf(`Based on the repository files provided below, create a comprehensive "Getting Started" guide in markdown format that includes:

1. Prerequisites and system requirements
2. Installation instructions (step by step)
3. Basic setup and configuration
4. A simple "Hello World" or basic usage example
5. Common gotchas or important notes for new users

Format the output as clear, well-structured markdown with appropriate sections and code blocks.
Use actual examples from the codebase where possible.

Repository files:
%s

Contents:
%s`, g.formatFileList(), g.formatFileContents())
}

func (g *Generator) buildUsagePrompt() string {
	return fmt.Sprintf(`Based on the repository files provided below, create a detailed usage guide in markdown format that includes:

1. Common use cases and examples
2. API documentation (if applicable)
3. Configuration options and their effects
4. Best practices and recommendations
5. Advanced usage examples

Use actual code examples from the repository where possible.
Format the output as clear, well-structured markdown with appropriate sections and code blocks.

Repository files:
%s

Contents:
%s`, g.formatFileList(), g.formatFileContents())
}

func (g *Generator) formatFileList() string {
	var files []string
	for path := range g.Files {
		files = append(files, path)
	}
	sort.Strings(files)
	return strings.Join(files, "\n")
}

func (g *Generator) formatFileContents() string {
	var result strings.Builder
	files := make([]string, 0, len(g.Files))
	for path := range g.Files {
		files = append(files, path)
	}
	sort.Strings(files)

	for _, path := range files {
		result.WriteString(fmt.Sprintf("\n=== %s ===\n", path))
		result.WriteString(g.Files[path])
		result.WriteString("\n")
	}
	return result.String()
}

func (g *Generator) loadFromCache() error {
	sections := []string{OverviewFileName, GettingStartedFileName, UsageFileName, FullDocFileName}

	var fullDoc strings.Builder
	for _, section := range sections {
		content, err := os.ReadFile(filepath.Join(g.DocsPath, section))
		if err != nil {
			return fmt.Errorf("failed to read cached section %s: %w", section, err)
		}
		if section != FullDocFileName {
			fullDoc.Write(content)
			fullDoc.WriteString("\n\n")
		}
	}

	fmt.Println("Documentation loaded from cache.")
	fmt.Printf("\nGenerated with: %s\n", g.Meta.ModelUsed)
	fmt.Printf("Commit: %s\n", g.Meta.CommitHash)
	fmt.Printf("Generated at: %s\n", g.Meta.GeneratedAt.Format(time.RFC3339))

	return nil
}

func (g *Generator) CleanupDuplicates() error {
	// Check if already deduplicated
	if g.Meta.Deduplicated {
		fmt.Println("Documentation already deduplicated, skipping cleanup pass...")
		return nil
	}

	fullDocPath := filepath.Join(g.DocsPath, FullDocFileName)
	content, err := os.ReadFile(fullDocPath)
	if err != nil {
		return fmt.Errorf("failed to read full documentation: %w", err)
	}

	prompt := `You are cleaning up a combined markdown documentation file. 
The content is currently duplicated across Overview, Getting Started, and Usage sections.

Please:
1. Keep only ONE top-level title
2. Consolidate similar sections (e.g. combine all installation instructions into one section)
3. Remove duplicate explanations while keeping the most detailed version
4. Maintain a clear, logical flow from overview -> setup -> basic usage -> advanced usage
5. Preserve ALL unique examples, especially in the advanced usage section
6. Keep ALL technical information and details
7. Ensure section headers follow a logical hierarchy

Original sections to combine:
1. Overview & Features (#)
2. Getting Started (##)
3. Usage Guide (##)

Please output a single, well-structured markdown document with no duplicate information.
Keep the most comprehensive version of any duplicated content.

Content to clean up:
` + string(content)

	fmt.Println("\nPerforming final cleanup pass to remove duplicates...")
	cleaned, err := g.LLMClient.GenerateWithStream(context.Background(), prompt)
	if err != nil {
		return fmt.Errorf("failed to clean documentation: %w", err)
	}

	// Save the cleaned version
	if err := os.WriteFile(fullDocPath, []byte(cleaned), 0644); err != nil {
		return fmt.Errorf("failed to write cleaned documentation: %w", err)
	}

	// Update and save metadata
	g.Meta.Deduplicated = true
	return g.saveMetadata()
}

// Helper function to save metadata
func (g *Generator) saveMetadata() error {
	metaData, err := json.MarshalIndent(g.Meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(filepath.Join(g.DocsPath, MetadataFileName), metaData, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}
