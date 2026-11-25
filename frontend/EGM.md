# frontend

## Overview
Contiene la interfaz de escritorio. Solo `dist/` tiene contenido versionado: es código escrito a mano (HTML, CSS y JS sin compilar) que Go incrusta en el ejecutable con `//go:embed all:frontend/dist` en `main.go`. `frontend/wailsjs/` y `frontend/node_modules/` están en `.gitignore` y no forman parte del proyecto.

## Subdirectories & Boundaries
- dist: [./dist/EGM.md](./dist/EGM.md) - Interfaz completa (`index.html`, `app.js`, `revision.js`, `styles.css`, PDF.js en `vendor/`). Llama a los métodos de Go con `call('Metodo', ...)` → `window.go.main.App` y escucha los eventos `update:progress` y `duplicates:refreshed`.
- tests: pruebas de Node (`node --test frontend/tests/revision.test.js`) para las funciones puras de `dist/revision.js`. No se incrustan en el ejecutable.
