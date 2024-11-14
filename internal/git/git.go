package git

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/boyter/gocodewalker"
	"github.com/go-git/go-git/v5"
)

type Repository struct {
	User string
	Repo string
	Tag  string
	Path string
}

type RepoFile struct {
	Path    string
	Size    int64
	Content string
}

// Common binary file signatures (magic numbers)
var binarySignatures = [][]byte{
	{0x7F, 0x45, 0x4C, 0x46}, // ELF
	{0x4D, 0x5A},             // DOS MZ executable
	{0x50, 0x4B, 0x03, 0x04}, // ZIP
	{0x1F, 0x8B},             // GZIP
	{0x89, 0x50, 0x4E, 0x47}, // PNG
	{0xFF, 0xD8, 0xFF},       // JPEG
	{0x47, 0x49, 0x46, 0x38}, // GIF
	{0x42, 0x4D},             // BMP
	{0x25, 0x50, 0x44, 0x46}, // PDF
}

// isBinaryFile checks if a file is binary using multiple heuristics
func isBinaryFile(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	// Read first 512 bytes for analysis
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err.Error() != "EOF" {
		return false, err
	}
	buf = buf[:n]

	// 1. Check file signatures
	for _, signature := range binarySignatures {
		if bytes.HasPrefix(buf, signature) {
			return true, nil
		}
	}

	// 2. Check for zero bytes (common in binary files)
	if bytes.Contains(buf, []byte{0x00}) {
		return true, nil
	}

	// 3. Calculate entropy of the content
	// High entropy often indicates compression or encryption
	entropy := calculateEntropy(buf)
	if entropy > 7.0 {
		return true, nil
	}

	// 4. Check character distribution
	textChars := 0
	for _, b := range buf {
		if (b >= 32 && b <= 126) || // Printable ASCII
			(b >= 9 && b <= 13) { // Tab, LF, VT, FF, CR
			textChars++
		}
	}

	// If less than 70% of content is text characters, likely binary
	if float64(textChars)/float64(len(buf)) < 0.7 {
		return true, nil
	}

	return false, nil
}

// calculateEntropy calculates Shannon entropy of data
func calculateEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}

	// Calculate frequency of each byte
	freq := make(map[byte]int)
	for _, b := range data {
		freq[b]++
	}

	// Calculate entropy
	var entropy float64
	for _, count := range freq {
		p := float64(count) / float64(len(data))
		entropy -= p * math.Log2(p)
	}

	return entropy
}

func ParseRepoPath(path string) (*Repository, error) {
	parts := strings.Split(path, "@")
	repoPath := parts[0]
	tag := ""
	if len(parts) > 1 {
		tag = parts[1]
	}

	repoParts := strings.Split(repoPath, "/")
	if len(repoParts) != 2 {
		return nil, fmt.Errorf("invalid repository path format. Expected user/repo[@tag]")
	}

	return &Repository{
		User: repoParts[0],
		Repo: repoParts[1],
		Tag:  tag,
	}, nil
}

// git.go
func (r *Repository) Clone() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not get home directory: %w", err)
	}

	r.Path = filepath.Join(homeDir, ".repocontext", r.User, r.Repo)

	// Check if repository already exists
	if _, err := os.Stat(r.Path); err == nil {
		fmt.Printf("Repository already exists at %s, using existing clone\n", r.Path)
		return nil
	}

	if err := os.MkdirAll(r.Path, 0755); err != nil {
		return fmt.Errorf("could not create repository directory: %w", err)
	}

	url := fmt.Sprintf("https://github.com/%s/%s.git", r.User, r.Repo)
	_, err = git.PlainClone(r.Path, false, &git.CloneOptions{
		URL:      url,
		Progress: os.Stdout,
		Depth:    1, // Ensure shallow clone
	})
	if err != nil {
		// Clean up the directory if clone fails
		os.RemoveAll(r.Path)
		return fmt.Errorf("could not clone repository: %w", err)
	}

	return nil
}

func (r *Repository) GetFiles() (map[string]*RepoFile, error) {
	fileListQueue := make(chan *gocodewalker.File, 100)
	files := make(map[string]*RepoFile)

	fileWalker := gocodewalker.NewFileWalker(r.Path, fileListQueue)

	// Error handler that continues on error
	errorHandler := func(e error) bool {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", e)
		return true
	}
	fileWalker.SetErrorHandler(errorHandler)

	// Start walking in a goroutine
	go fileWalker.Start()

	// Collect files
	for f := range fileListQueue {
		// Get file info
		info, err := os.Stat(f.Location)
		if err != nil {
			continue
		}

		// Skip directories
		if info.IsDir() {
			continue
		}

		// Check if file is binary
		isBinary, err := isBinaryFile(f.Location)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not check if file is binary %s: %v\n", f.Location, err)
			continue
		}

		if isBinary {
			continue
		}

		// Get relative path
		relPath, err := filepath.Rel(r.Path, f.Location)
		if err != nil {
			continue
		}

		files[relPath] = &RepoFile{
			Path: relPath,
			Size: info.Size(),
		}
	}

	return files, nil
}

// ReadFileContents reads the actual content of selected files
func (r *Repository) ReadFileContents(files map[string]*RepoFile) error {
	for _, file := range files {
		content, err := ioutil.ReadFile(filepath.Join(r.Path, file.Path))
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", file.Path, err)
		}
		file.Content = string(content)
	}
	return nil
}

func (r *Repository) GetCurrentCommitHash() (string, error) {
	repo, err := git.PlainOpen(r.Path)
	if err != nil {
		return "", fmt.Errorf("failed to open repository: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD reference: %w", err)
	}

	return head.Hash().String(), nil
}
