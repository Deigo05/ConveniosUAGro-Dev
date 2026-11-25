package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// maxLogSize es el tamaño a partir del cual el log se rota a convenios.log.1.
const maxLogSize = 1 << 20

// setupLog escribe el log también en <carpeta de datos>/convenios.log. En una
// app de ventana de Windows la salida estándar no se ve, y sin archivo los
// errores que solo se registran se perderían.
func setupLog(dir string) {
	path := filepath.Join(dir, "convenios.log")
	if st, err := os.Stat(path); err == nil && st.Size() > maxLogSize {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
}
