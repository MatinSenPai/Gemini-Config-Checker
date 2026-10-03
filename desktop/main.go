// Desktop mode: native window (Wails) around the same UI and scan engine as the web build.
package main

import (
	"context"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wr "github.com/wailsapp/wails/v2/pkg/runtime"

	"gemini-config-checker/internal/buildinfo"
	"gemini-config-checker/internal/scan"
	"gemini-config-checker/internal/ui"
)

// App methods are exposed to the frontend as window.go.main.App.*
type App struct{ ctx context.Context }

func (a *App) Start(r scan.Request) error { return scan.Start(r) }
func (a *App) Stop()                      { scan.Stop() }
func (a *App) Version() string            { return buildinfo.Version }
func (a *App) Clear()                     { scan.Clear() }
func (a *App) Login() error               { return scan.StartLogin() }
func (a *App) CancelLogin()               { scan.CancelLogin() }
func (a *App) Logout() error              { return scan.Logout() }
func (a *App) Status() scan.Status        { return scan.Snapshot() }

// Chain returns a standalone xray config for base -> free (see scan.ChainJSON).
func (a *App) Chain(r scan.ChainRequest) (string, error) { return scan.ChainJSON(r) }

func (a *App) DefaultProfile() scan.Profile                     { return scan.DefaultProfile() }
func (a *App) ExtractProfile(base string) (scan.Profile, error) { return scan.ExtractProfile(base) }

// SaveText asks where to save, then writes. Cancelling the dialog is not an error.
func (a *App) SaveText(name, text string) (bool, error) {
	p, err := wr.SaveFileDialog(a.ctx, wr.SaveDialogOptions{DefaultFilename: name})
	if err != nil || p == "" {
		return false, err
	}
	return true, os.WriteFile(p, []byte(text), 0o600)
}

func main() {
	app := &App{}
	err := wails.Run(&options.App{
		Title:            "Gemini Config Checker",
		Width:            1120,
		Height:           780,
		MinWidth:         920,
		MinHeight:        640,
		BackgroundColour: &options.RGBA{R: 10, G: 10, B: 10, A: 255},
		AssetServer:      &assetserver.Options{Assets: ui.FS},
		// Only here, never in main(): `wails build` runs main() to generate bindings and must not spawn a browser.
		OnStartup: func(ctx context.Context) { app.ctx = ctx; scan.RestoreAtStartup() },
		Bind:      []any{app},
	})
	if err != nil {
		println("error:", err.Error())
	}
}
