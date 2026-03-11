package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

// App struct
type App struct {
	ctx             context.Context
	initialFile     string
	initialDir      string
	currentFile     string
	currentEncoding string
	isPrettyPrint   bool
	watcher         *fsnotify.Watcher
	watcherMutex    sync.Mutex
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

	// Handle native file drops
	runtime.OnFileDrop(ctx, func(x, y int, paths []string) {
		runtime.EventsEmit(ctx, "custom-file-drop", paths)
	})

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

// OpenDirectory opens a directory selection dialog and updates the tree view.
func (a *App) OpenDirectory() {
	selection, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "フォルダを選択",
	})
	if err != nil {
		fmt.Printf("Error selecting directory: %v\n", err)
		return
	}
	if selection != "" {
		runtime.EventsEmit(a.ctx, "directory-opened", selection)
	}
}

func getMimeType(ext string) string {
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".svg":
		return "image/svg+xml"
	default:
		return ""
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

	mimeType := getMimeType(ext)
	if mimeType != "" {
		base64Str := base64.StdEncoding.EncodeToString(content)
		payload = fmt.Sprintf("![%s](data:%s;base64,%s)", filepath.Base(absPath), mimeType, base64Str)
	} else if isBinary {
		payload = fmt.Sprintf("# Unsupported File\n\n`%s` appears to be a binary file and cannot be displayed as text.", filepath.Base(absPath))
	} else {
		// Decode content based on current encoding
		decodedStr, err := a.decodeContent(content, a.currentEncoding)
		if err != nil {
			fmt.Printf("Failed to decode content with %s: %v\n", a.currentEncoding, err)
			decodedStr = string(content) // Fallback to raw string
		}

		if ext == ".md" || ext == ".markdown" {
			// Simple logic to resolve relative image paths in markdown
			// This finds ![]() and tries to replace path with base64
			lines := strings.Split(decodedStr, "\n")
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
							imgMime := getMimeType(imgExt)
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
			
			displayContent := decodedStr
			if lang == "json" && a.isPrettyPrint {
				var jsonObj interface{}
				if err := json.Unmarshal([]byte(decodedStr), &jsonObj); err == nil {
					if pretty, err := json.MarshalIndent(jsonObj, "", "  "); err == nil {
						displayContent = string(pretty)
					}
				}
			}
			
			payload = fmt.Sprintf("```%s\n%s\n```", lang, displayContent)
		}
	}

	// Send content to frontend
	runtime.EventsEmit(a.ctx, "markdown-updated", payload)
}

// ChangeEncoding updates the current encoding and reloads the current file if possible.
func (a *App) ChangeEncoding(encoding string) {
	a.currentEncoding = encoding
	if a.currentFile != "" {
		a.LoadFile(a.currentFile)
	}
}

// SetPrettyPrint updates the pretty print flag and reloads the current file if it's a JSON.
func (a *App) SetPrettyPrint(pretty bool) {
	a.isPrettyPrint = pretty
	if a.currentFile != "" && strings.ToLower(filepath.Ext(a.currentFile)) == ".json" {
		a.LoadFile(a.currentFile)
	}
}

func (a *App) decodeContent(content []byte, encodingName string) (string, error) {
	if encodingName == "" || strings.ToUpper(encodingName) == "UTF-8" {
		return string(content), nil
	}

	var enc transform.Transformer
	switch strings.ToUpper(encodingName) {
	case "SHIFT-JIS":
		enc = japanese.ShiftJIS.NewDecoder()
	case "EUC-JP":
		enc = japanese.EUCJP.NewDecoder()
	case "ISO-2022-JP":
		enc = japanese.ISO2022JP.NewDecoder()
	default:
		return string(content), nil
	}

	reader := transform.NewReader(bytes.NewReader(content), enc)
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
