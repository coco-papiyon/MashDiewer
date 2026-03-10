package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	return nodes, nil
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

	// Send content to frontend
	runtime.EventsEmit(a.ctx, "markdown-updated", string(content))
}
