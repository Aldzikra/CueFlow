package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	cueapp "github.com/Aldzikra/cueflow/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	svc := cueapp.New()
	app := application.New(application.Options{
		Name:        "CueFlow",
		Description: "Voice-driven teleprompter",
		Services:    []application.Service{application.NewService(svc)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
	})

	editor := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:   "editor",
		Title:  "CueFlow",
		Width:  900,
		Height: 700,
		URL:    "/",
	})
	editor.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) { app.Quit() })

	// Overlay: frameless, transparent, always on top. On Wayland run with
	// GDK_BACKEND=x11 (XWayland), otherwise always-on-top is ignored.
	overlay := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "overlay",
		Title:            "CueFlow Overlay",
		Width:            900,
		Height:           320,
		URL:              "/#overlay",
		Frameless:        true,
		AlwaysOnTop:      true,
		Hidden:           true,
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		Linux:            application.LinuxWindow{WindowIsTranslucent: true},
	})
	cueapp.Attach(svc, app, overlay)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
