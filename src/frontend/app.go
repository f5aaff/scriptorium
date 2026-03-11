package main

import (
	"context"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx context.Context
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// SelectFolder opens a native directory picker dialog
func (a *App) SelectFolder() (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Folder",
	})
}
