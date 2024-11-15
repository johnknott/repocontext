// main.go
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/johnknott/repocontext/internal/config"
	"github.com/johnknott/repocontext/internal/docs"
	"github.com/johnknott/repocontext/internal/git"
	"github.com/johnknott/repocontext/internal/llm"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: repocontext user/repo[@tag]")
		os.Exit(1)
	}

	cfg := config.New()
	if cfg.AnthropicKey == "" {
		log.Fatal("ANTHROPIC_API_KEY environment variable must be set")
	}

	// Initialize LLM client
	fmt.Println("Initializing Claude client...")
	client, err := llm.NewClient(cfg.AnthropicKey)
	if err != nil {
		log.Fatal(err)
	}

	// Parse and clone repository
	repoPath := os.Args[1]
	fmt.Printf("Parsing repository path: %s\n", repoPath)
	repo, err := git.ParseRepoPath(repoPath)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Cloning/updating repository %s/%s...\n", repo.User, repo.Repo)
	repoPath, err = repo.Clone()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Repository available at: %s\n", repoPath)

	// Get commit hash
	commitHash, err := repo.GetCurrentCommitHash()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Current commit: %s\n", commitHash)

	// Get file listing
	fmt.Println("\nScanning repository files...")
	files, err := repo.GetFiles()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Found %d files\n", len(files))

	// Select files to analyze
	fmt.Printf("\nSelecting files to include (max size: %d bytes)...\n", cfg.MaxContextSize)
	selectedFiles, totalSize, err := client.SelectFiles(files, cfg.MaxContextSize)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nSelected %d files for analysis (total size: %d bytes)\n", len(selectedFiles), totalSize)

	// Create filtered map of selected files
	selectedFilesMap := make(map[string]*git.RepoFile)
	for _, path := range selectedFiles {
		selectedFilesMap[path] = files[path]
	}

	// Initialize documentation generator with versioned path
	docGen, err := docs.New(repo.Path, commitHash, repo.Tag, client)
	if err != nil {
		log.Fatal(err)
	}

	// Generate or load documentation
	meta := &docs.Metadata{
		CommitHash:  commitHash,
		ModelUsed:   client.ModelName(),
		GeneratedAt: time.Now(),
	}

	fmt.Println("\nGenerating documentation...")
	if err := docGen.LoadOrGenerateDocs(selectedFilesMap, meta); err != nil {
		log.Fatal(err)
	}

	// Perform cleanup pass to remove duplicates
	if err := docGen.CleanupDuplicates(); err != nil {
		log.Fatal(err)
	}

	// Output the full documentation to stdout
	fullDocPath := filepath.Join(docGen.DocsPath, docs.FullDocFileName)
	fullDoc, err := os.ReadFile(fullDocPath)
	if err != nil {
		log.Fatal(err)
	}

	versionPath := filepath.Join(repo.User, repo.Repo, "versions", commitHash)
	fmt.Printf("\nDocumentation generated and saved to: %s\n", docGen.DocsPath)
	fmt.Printf("Version: %s\n", versionPath)
	fmt.Printf("Generated with: %s\n", meta.ModelUsed)
	fmt.Printf("Generated at: %s\n", meta.GeneratedAt.Format(time.RFC3339))
	fmt.Println("\n=== Generated Documentation ===\n")
	fmt.Println(string(fullDoc))
}
