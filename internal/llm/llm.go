// llm.go
package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/johnknott/repocontext/internal/git"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
)

type Client struct {
	llm *anthropic.LLM
}

// internal/llm/llm.go
// internal/llm/llm.go
func (c *Client) GenerateWithStream(ctx context.Context, prompt string) (string, error) {
	fmt.Println("Generating response...")

	options := []llms.CallOption{
		llms.WithTemperature(0.7),
		llms.WithMaxTokens(4096),
	}

	completion, err := c.llm.Call(ctx, prompt, options...)
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	return completion, nil
}

// Add function to get model name
func (c *Client) ModelName() string {
	return "claude-3-5-sonnet-20240620"
}

func NewClient(apiKey string) (*Client, error) {
	llm, err := anthropic.New(
		anthropic.WithModel("claude-3-5-sonnet-20241022"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Anthropic client: %w", err)
	}

	return &Client{
		llm: llm,
	}, nil
}

func getTotalSize(files map[string]*git.RepoFile) int64 {
	var total int64
	for _, file := range files {
		total += file.Size
	}
	return total
}

func formatFilesForPrompt(files map[string]*git.RepoFile) string {
	var fileList []string
	totalSize := getTotalSize(files)

	for path, file := range files {
		fileList = append(fileList, fmt.Sprintf("%s (%d bytes)", path, file.Size))
	}

	return fmt.Sprintf("Total size: %d bytes\n\nFiles:\n%s", totalSize, strings.Join(fileList, "\n"))
}

func (c *Client) SelectFiles(files map[string]*git.RepoFile, maxSize int) ([]string, int64, error) {
	totalSize := getTotalSize(files)

	// If total size is already under maxSize, return all files
	if totalSize <= int64(maxSize) {
		fmt.Printf("Total size (%d bytes) is under limit (%d bytes), including all files\n", totalSize, maxSize)
		allFiles := make([]string, 0, len(files))
		for path := range files {
			allFiles = append(allFiles, path)
		}
		return allFiles, totalSize, nil
	}

	fmt.Printf("Total size (%d bytes) exceeds limit (%d bytes), asking Claude to select files...\n", totalSize, maxSize)

	fileInfo := formatFilesForPrompt(files)

	prompt := fmt.Sprintf(`You are selecting the most important files to understand a software project, within %d bytes limit.

Repository structure:
%s

Select files that help understand:
1. What the project does and its core functionality (especially README.md and any other english language documentation)
2. How to use/integrate the project - Especially tutorials and guides
3. Key configuration needed to make it work
4. Main implementation details, focusing on:
   - Entry points
   - Core logic
   - Public APIs/interfaces
   - Configuration options

Avoid files that are:
1. Duplicates (translations, versions)
2. Supporting files (tests, examples, licenses)
3. Build artifacts and dependencies
4. Auxiliary documentation (contribution guides, changelogs)

Format: One filepath per line
Stay under %d bytes total size
Reply ONLY with filepaths.`, maxSize, fileInfo, maxSize)

	ctx := context.Background()

	fmt.Println("\nWaiting for Claude's response...")
	completion, err := llms.GenerateFromSinglePrompt(
		ctx,
		c.llm,
		prompt,
		llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			fmt.Print(string(chunk))
			return nil
		}),
	)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get LLM response: %w", err)
	}
	fmt.Println("\n")

	// Process the response
	selectedFiles := []string{}
	selectedSize := int64(0)

	for _, file := range strings.Split(completion, "\n") {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}

		// Extract just the filepath if the LLM included the size
		if idx := strings.Index(file, " ("); idx != -1 {
			file = file[:idx]
		}

		if repoFile, exists := files[file]; exists {
			if selectedSize+repoFile.Size > int64(maxSize) {
				fmt.Printf("Skipping %s: would exceed size limit\n", file)
				continue
			}
			selectedFiles = append(selectedFiles, file)
			selectedSize += repoFile.Size
			fmt.Printf("Selected: %s (%d bytes)\n", file, repoFile.Size)
		} else {
			fmt.Printf("Warning: File not found: %s\n", file)
		}
	}

	if len(selectedFiles) == 0 {
		return nil, 0, fmt.Errorf("no files were selected within size constraints")
	}

	fmt.Printf("\nTotal selected size: %d bytes (%.2f%% of limit)\n",
		selectedSize, float64(selectedSize)/float64(maxSize)*100)

	return selectedFiles, selectedSize, nil
}

func (c *Client) GenerateDocumentation(files map[string]string) (string, error) {
	// TODO: Implement documentation generation logic
	return "", fmt.Errorf("not implemented")
}
