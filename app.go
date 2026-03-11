package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx          context.Context
	initialFile  string
	initialDir   string
	currentFile  string
	watcher      *fsnotify.Watcher
	watcherMutex sync.Mutex
}

// NewApp creates a new App application struct
func NewApp(initialPath string) *App {
	app := &App{}

	if initialPath != "" {
		absPath, err := filepath.Abs(initialPath)
		if err == nil {
			info, err := os.Stat(absPath)
			if err == nil {
				if info.IsDir() {
					app.initialDir = absPath
				} else {
					app.initialFile = absPath
					app.initialDir = filepath.Dir(absPath)
				}
			}
		}
	}

	// If no dir is set, use current working directory
	if app.initialDir == "" {
		cwd, err := os.Getwd()
		if err == nil {
			app.initialDir = cwd
		}
	}

	return app
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Initialize fsnotify watcher
	watcher, err := fsnotify.NewWatcher()
	if err == nil {
		a.watcher = watcher
		go a.watchFileChanges()
	} else {
		fmt.Printf("Error creating watcher: %v\n", err)
	}
}

func (a *App) shutdown(ctx context.Context) {
	if a.watcher != nil {
		a.watcher.Close()
	}
}

func (a *App) watchFileChanges() {
	for {
		select {
		case event, ok := <-a.watcher.Events:
			if !ok {
				return
			}
			// Only trigger on write events
			if event.Has(fsnotify.Write) {
				// Wait a tiny bit for the file system to finish the write operation
				time.Sleep(100 * time.Millisecond)
				a.reloadCurrentFile()
			}
		case err, ok := <-a.watcher.Errors:
			if !ok {
				return
			}
			fmt.Println("error:", err)
		}
	}
}

func (a *App) reloadCurrentFile() {
	a.watcherMutex.Lock()
	fileToLoad := a.currentFile
	a.watcherMutex.Unlock()

	if fileToLoad == "" {
		return
	}

	content, err := os.ReadFile(fileToLoad)
	if err == nil {
		runtime.EventsEmit(a.ctx, "markdown-updated", string(content))
	}
}

// InitializeFile is called by the frontend when it's ready.
// It sends the initial directory to the frontend.
// If an initial file was passed via CLI, it also loads it.
func (a *App) InitializeFile() {
	if a.initialDir != "" {
		runtime.EventsEmit(a.ctx, "set-initial-dir", a.initialDir)
	}

	if a.initialFile != "" {
		a.LoadFile(a.initialFile)
	}
}

// FileNode represents a file or directory in the tree
type FileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	IsDir    bool       `json:"isDir"`
	Children []FileNode `json:"children,omitempty"`
}

// GetDirectoryTree returns the files and directories inside the given path
func (a *App) GetDirectoryTree(dirPath string) ([]FileNode, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var nodes []FileNode
	for _, entry := range entries {
		// Basic filter to hide hidden files/folders (starting with .)
		if len(entry.Name()) > 0 && entry.Name()[0] == '.' {
			continue
		}

		fullPath := filepath.Join(dirPath, entry.Name())
		nodes = append(nodes, FileNode{
			Name:  entry.Name(),
			Path:  fullPath,
			IsDir: entry.IsDir(),
		})
	}

	// Sort nodes: directories first, then files. Sort case-insensitively by name.
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})

	return nodes, nil
}

// GetParentDir returns the parent directory of the given path
func (a *App) GetParentDir(dirPath string) string {
	return filepath.Dir(dirPath)
}

// OpenFile opens a file selection dialog.
func (a *App) OpenFile() {
	selection, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Markdown File",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Markdown Files (*.md;*.markdown)",
				Pattern:     "*.md;*.markdown",
			},
			{
				DisplayName: "All Files (*.*)",
				Pattern:     "*.*",
			},
		},
	})

	if err != nil {
		fmt.Printf("Error selecting file: %v\n", err)
		return
	}

	if selection != "" {
		a.LoadFile(selection)
	}
}

// LoadFile reads the specified file, sends its content to the frontend,
// and sets up the file watcher for it.
func (a *App) LoadFile(filePath string) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		fmt.Printf("Failed to get absolute path: %v\n", err)
		return
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		fmt.Printf("Failed to read file: %v\n", err)
		// Send error message to frontend as markdown
		runtime.EventsEmit(a.ctx, "markdown-updated", fmt.Sprintf("# Error\nFailed to read file: `%s`\n\n```\n%v\n```", absPath, err))
		return
	}

	// Update current file and set up watcher
	a.watcherMutex.Lock()
	if a.currentFile != "" && a.watcher != nil {
		a.watcher.Remove(a.currentFile)
	}
	a.currentFile = absPath
	if a.watcher != nil {
		a.watcher.Add(absPath)
	}
	a.watcherMutex.Unlock()

	// Update window title
	runtime.WindowSetTitle(a.ctx, fmt.Sprintf("MashDiewer - %s", filepath.Base(absPath)))

	// Check if file is binary
	isBinary := false
	checkLen := len(content)
	if checkLen > 512 {
		checkLen = 512
	}
	if bytes.IndexByte(content[:checkLen], 0) != -1 {
		isBinary = true
	}

	var payload string
	ext := strings.ToLower(filepath.Ext(absPath))

	// Handle images
	isImage := false
	var mimeType string
	switch ext {
	case ".png":
		isImage = true
		mimeType = "image/png"
	case ".jpg", ".jpeg":
		isImage = true
		mimeType = "image/jpeg"
	case ".gif":
		isImage = true
		mimeType = "image/gif"
	case ".webp":
		isImage = true
		mimeType = "image/webp"
	case ".bmp":
		isImage = true
		mimeType = "image/bmp"
	case ".svg":
		isImage = true
		mimeType = "image/svg+xml"
	}

	if isImage {
		base64Str := base64.StdEncoding.EncodeToString(content)
		payload = fmt.Sprintf("![%s](data:%s;base64,%s)", filepath.Base(absPath), mimeType, base64Str)
	} else if isBinary {
		payload = fmt.Sprintf("# Unsupported File\n\n`%s` appears to be a binary file and cannot be displayed as text.", filepath.Base(absPath))
	} else {
		if ext == ".md" || ext == ".markdown" {
			markdownContent := string(content)

			// Simple logic to resolve relative image paths in markdown
			// This finds ![]() and tries to replace path with base64
			lines := strings.Split(markdownContent, "\n")
			for i, line := range lines {
				if strings.Contains(line, "![") && strings.Contains(line, "](") && strings.Contains(line, ")") {
					startIdx := strings.Index(line, "](") + 2
					endIdx := strings.Index(line[startIdx:], ")") + startIdx
					imgPath := line[startIdx:endIdx]

					// If it's a relative path and not a URL
					if !strings.HasPrefix(imgPath, "http") && !strings.HasPrefix(imgPath, "data:") {
						fullImgPath := filepath.Join(filepath.Dir(absPath), imgPath)
						imgData, err := os.ReadFile(fullImgPath)
						if err == nil {
							imgExt := strings.ToLower(filepath.Ext(fullImgPath))
							var imgMime string
							switch imgExt {
							case ".png":
								imgMime = "image/png"
							case ".jpg", ".jpeg":
								imgMime = "image/jpeg"
							case ".gif":
								imgMime = "image/gif"
							case ".webp":
								imgMime = "image/webp"
							case ".svg":
								imgMime = "image/svg+xml"
							}

							if imgMime != "" {
								imgBase64 := base64.StdEncoding.EncodeToString(imgData)
								newDataURI := fmt.Sprintf("data:%s;base64,%s", imgMime, imgBase64)
								lines[i] = strings.Replace(line, imgPath, newDataURI, 1)
							}
						}
					}
				}
			}
			payload = strings.Join(lines, "\n")
		} else {
			// Wrap raw text in a Markdown code block so highlight.js handles it
			lang := strings.TrimPrefix(ext, ".")
			payload = fmt.Sprintf("```%s\n%s\n```", lang, string(content))
		}
	}

	// Send content to frontend
	runtime.EventsEmit(a.ctx, "markdown-updated", payload)
}
