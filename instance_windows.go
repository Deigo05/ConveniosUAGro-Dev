//go:build windows

package main

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// instanceMutex queda abierto mientras viva el proceso: Windows lo libera al
// cerrarse la app, aunque se cierre de forma anormal.
var instanceMutex windows.Handle

const instanceMutexName = `Local\ConveniosUAGro-instancia-unica`

// acquireInstance toma el candado de instancia única. Con wait > 0 espera a que
// la instancia anterior lo suelte (tras una actualización, la versión vieja
// tarda un momento en cerrarse). Devuelve false si otra ventana sigue abierta.
func acquireInstance(wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	name, _ := windows.UTF16PtrFromString(instanceMutexName)
	for {
		h, err := windows.CreateMutex(nil, false, name)
		if err == nil {
			instanceMutex = h
			return true
		}
		if h != 0 {
			windows.CloseHandle(h)
		}
		if err != windows.ERROR_ALREADY_EXISTS {
			// Sin candado no se puede saber; es mejor abrir que no abrir.
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(250 * time.Millisecond)
	}
}

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW   = user32.NewProc("FindWindowW")
	procShowWindow    = user32.NewProc("ShowWindow")
	procSetForeground = user32.NewProc("SetForegroundWindow")
	procMessageBoxW   = user32.NewProc("MessageBoxW")
)

// focusExisting trae al frente la ventana que ya está abierta.
func focusExisting() {
	title, _ := windows.UTF16PtrFromString(windowTitle)
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return
	}
	const swRestore = 9
	procShowWindow.Call(hwnd, swRestore)
	procSetForeground.Call(hwnd)
}

// showFatal muestra el error en un cuadro de diálogo nativo: la app no tiene
// consola, así que sin esto no abriría y no diría por qué.
func showFatal(message string) {
	text, _ := windows.UTF16PtrFromString(message)
	caption, _ := windows.UTF16PtrFromString(windowTitle)
	const mbOK, mbIconError = 0x0, 0x10
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(caption)), mbOK|mbIconError)
}
