package main

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"

	"timetable/internal/db"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails-bound backend.
type App struct {
	ctx   context.Context
	store *db.Store
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// Greet is a sanity endpoint.
func (a *App) Greet(name string) string {
	return "Hello " + name + ", welcome to Timetable!"
}

// SaveFile writes a base64-encoded payload (e.g. a generated PDF) to disk at
// the path chosen by the user via the native Save dialog.
func (a *App) SaveFile(path string, b64 string) error {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SaveFileWithDialog shows a native save dialog and returns the chosen path
// (empty string if cancelled). Uses Wails v2 runtime.SaveFileDialog.
func (a *App) SaveFileWithDialog(defaultName string) (string, error) {
	return wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:                "Сохранить файл",
		DefaultFilename:      defaultName,
		CanCreateDirectories: true,
	})
}

// SaveExport writes a base64 payload (e.g. a generated PDF/CSV) directly into
// the user's Downloads folder under the given filename. This avoids the native
// GTK/xdg-desktop-portal save dialog, which is unreliable inside an AppImage on
// Wayland. Returns the absolute path the file was written to.
func (a *App) SaveExport(filename string, b64 string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	dir := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		dir = home
	}
	name := filepath.Base(strings.TrimSpace(filename))
	if name == "" || name == "." || name == string(os.PathSeparator) {
		name = "export"
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
