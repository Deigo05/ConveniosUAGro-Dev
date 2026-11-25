package main

import (
	"embed"
	"log"
	"path/filepath"
	"time"

	"convenios-uagro-desktop/internal/store"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

const windowTitle = "Convenios UAGro"

func main() {
	paths, err := store.DefaultPaths()
	if err == nil {
		setupLog(paths.Base)
	}
	// Una sola ventana a la vez: una segunda instancia abriría la misma base y
	// marcaría como interrumpidos los análisis de la primera. Tras actualizar,
	// se espera a que la versión anterior termine de cerrarse.
	wait := time.Duration(0)
	if hasArg(afterUpdateFlag) {
		wait = 30 * time.Second
	}
	if !acquireInstance(wait) {
		focusExisting()
		return
	}

	app, err := NewApp()
	if err != nil {
		fatal(err, paths)
	}

	err = wails.Run(&options.App{
		Title:             windowTitle,
		Width:             1320,
		Height:            860,
		MinWidth:          1040,
		MinHeight:         680,
		DisableResize:     false,
		Fullscreen:        false,
		Frameless:         false,
		StartHidden:       false,
		HideWindowOnClose: false,
		BackgroundColour:  &options.RGBA{R: 244, G: 246, B: 250, A: 1},
		AssetServer: &assetserver.Options{
			Assets:     assets,
			Middleware: app.middleware,
		},
		// Permite arrastrar PDF, paquetes o respaldos a la ventana; la interfaz
		// recibe las rutas con runtime.OnFileDrop.
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent:              false,
			WindowIsTranslucent:               false,
			DisableWindowIcon:                 false,
			DisableFramelessWindowDecorations: false,
		},
	})
	if err != nil {
		fatal(err, paths)
	}
}

// fatal registra el error y lo muestra antes de salir.
func fatal(err error, paths store.Paths) {
	log.Printf("error fatal: %v", err)
	msg := "Convenios UAGro no pudo abrirse:\n\n" + err.Error()
	if paths.Base != "" {
		msg += "\n\nEl detalle quedó en " + filepath.Join(paths.Base, "convenios.log") + "."
	}
	showFatal(msg)
	log.Fatal(err)
}
