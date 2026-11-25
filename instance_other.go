//go:build !windows

package main

import (
	"fmt"
	"os"
	"time"
)

// Fuera de Windows (desarrollo y pruebas) no hay candado de instancia.
func acquireInstance(wait time.Duration) bool { return true }

func focusExisting() {}

func showFatal(message string) { fmt.Fprintln(os.Stderr, message) }
