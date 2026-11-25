# pdfutil

## Overview
Guarda y analiza los PDF de los convenios sin dependencias nativas (sin CGO), usando `github.com/ledongthuc/pdf`. Valida el archivo de origen, lo copia a la carpeta de datos con un nombre aleatorio, calcula su SHA-256, extrae el texto y marca si parece escaneado. También exporta una copia a la ruta que elija el usuario. Lo usan `app.go` e `internal/store`.

## Interface
- `MaxBytes = 100 << 20` (100 MB), `MaxPages = 2000`, `ScannedThreshold = 200` [pdf.go:MaxBytes] [pdf.go:MaxPages] [pdf.go:ScannedThreshold]
- `type SavedFile`: `NombreGuardado`, `NombreOriginal`, `Bytes`, `SHA256`, `Paginas`, `PaginasSinTexto`, `Texto` y `Escaneado` [pdf.go:SavedFile]
- `func Import(src, destDir string) (SavedFile, error)`: valida, copia y extrae el texto de un PDF [pdf.go:Import]
- `func Export(src, dst string) error`: copia un PDF a la ruta elegida por el usuario [pdf.go:Export]

## Invariants
- `Import` rechaza un origen que no se puede leer, una carpeta, un archivo vacío o uno mayor que `MaxBytes`. El tamaño se revisa con `os.Stat` antes de copiar, y la copia en sí no tiene límite [pdf.go:Import]
- `Import` exige que los primeros 5 bytes sean `%PDF-` («el archivo no es un PDF válido») [pdf.go:Import]
- `SHA256` es el hash en hexadecimal (minúsculas) de los bytes copiados, calculado en la misma pasada que la escritura. `Bytes` es lo que realmente se copió [pdf.go:Import]
- `NombreGuardado` son 16 bytes de `crypto/rand` en hexadecimal más `.pdf`. El nombre original nunca se usa en disco [pdf.go:randomName]
- `NombreOriginal` es el nombre base del origen (las `\` se tratan como `/`), sin caracteres de control y sin espacios a los lados, cortado a 180 runas. Si queda vacío, se usa `convenio.pdf` [pdf.go:cleanName] [pdf.go:reControl]
- Si falla la extracción de texto (error de la librería, pánico recuperado, cero páginas o más de `MaxPages`), `Import` no devuelve error: el archivo se conserva con `Paginas=0`, `Texto=""` y `Escaneado=true` [pdf.go:Import] [pdf.go:extractText]
- `extractText` recupera los pánicos de la librería y los convierte en el error «no se pudo leer el PDF» [pdf.go:extractText]
- Una página cuenta en `PaginasSinTexto` si es nula, si falla `GetPlainText` o si tiene menos de 30 caracteres visibles. Los textos de las páginas se unen con `\n\n` [pdf.go:extractText]
- `Escaneado` vale `true` cuando el documento completo tiene menos de `ScannedThreshold` caracteres visibles, contando como visibles las runas que no son espacio ni control [pdf.go:Import] [pdf.go:visibleChars]
- `Export` agrega `.pdf` si la extensión de `dst` no lo es (sin importar mayúsculas). No revisa el encabezado, el tamaño ni la ruta [pdf.go:Export]

## Side Effects
- `Import` crea `destDir` con `os.MkdirAll(..., 0o700)`, escribe un temporal `.import-*.pdf` con `os.CreateTemp` y lo renombra a `destDir/<32 hex>.pdf`. Un `os.Remove` diferido borra el temporal si algo falla antes del renombrado [pdf.go:Import]
- `Import` abre el origen solo para lectura, y `extractText` vuelve a abrir el archivo ya guardado [pdf.go:Import] [pdf.go:extractText]
- `Export` crea o trunca `dst` con `os.Create` y sobrescribe sin preguntar. No crea carpetas, y si la copia falla deja el archivo a medias [pdf.go:Export]

## Verification
- Run: `go test ./internal/pdfutil/...`
