# Convenios UAGro Desktop — Documentación técnica

**Versión:** 1.1.0  
**Estado:** aplicación Windows compilada, probada y entregada en la PC.  
**Base inicial:** vacía (se crean solo las etiquetas base al primer arranque).

Este documento es para retomar el proyecto en otra sesión. La guía para el usuario final está en `README.md` y `LEEME.txt`.

## 1. Qué es

Aplicación de escritorio local para registrar, buscar, clasificar y descargar los convenios de la UAGro. Sustituye el servidor web original por un ejecutable con:

- Interfaz HTML/CSS/JS dentro de WebView2.
- Backend Go con Wails v2.
- SQLite local con FTS5.
- Importación y extracción de texto de PDF.
- Visor PDF integrado y descarga con diálogo nativo.
- Etiquetas, filtros, vigencias, selección múltiple y respaldo ZIP.
- IA opcional (DeepSeek/OpenAI-compatible) para extraer datos de PDF con texto.
- Transcripción de páginas escaneadas con un modelo de visión; el texto por página se guarda en SQLite y se integra a FTS5.
- Filtro de clasificación pendiente para revisar etiquetas.

No hay login, usuarios, servidor propio ni datos de ejemplo.

## 2. Stack

| Componente | Tecnología |
|---|---|
| Lenguaje | Go 1.26+ |
| Escritorio | Wails v2.17 / WebView2 |
| Interfaz | HTML + CSS + JavaScript sin framework |
| Base de datos | SQLite mediante `modernc.org/sqlite` (Go puro, sin CGO) |
| Búsqueda | FTS5 `unicode61 remove_diacritics 2` |
| PDF | `github.com/ledongthuc/pdf` para texto; PDF.js para renderizar páginas escaneadas; WebView2 para visor |
| IA | HTTP a `/chat/completions` compatible con OpenAI (texto + visión) |
| Instalador | Wails + NSIS 3.13 |

## 3. Estructura

```text
convenios-desktop/
├── main.go                    Wails.Run, embed de frontend/dist
├── app.go                     API local, diálogos, servidor /pdf, configuración
├── internal/
│   ├── config/config.go       settings.json, clave IA, vista segura
│   ├── store/
│   │   ├── store.go           Open, rutas, esquema, etiquetas base
│   │   ├── schema.sql         tablas, FTS5 y triggers
│   │   ├── types.go           structs de la API
│   │   ├── tags.go            catálogo, asignación, sugerencias
│   │   ├── conventions.go     CRUD, filtros, FTS, vigencias, borrado
│   │   ├── transcription.go   páginas transcritas y reconstrucción de texto
│   │   └── backup.go          respaldo ZIP (base + PDF)
│   ├── pdfutil/pdf.go         validación, copia, texto, exportación
│   └── ai/ai.go               prompts, extracción y transcripción de imágenes
├── frontend/dist/
│   ├── index.html
│   ├── styles.css
│   ├── app.js
│   ├── vendor/pdf.min.js      PDF.js
│   ├── vendor/pdf.worker.min.js
│   └── assets/escudo.png
├── build/appicon.png
├── build/bin/ConveniosUAGro.exe
├── build/bin/Convenios UAGro-amd64-installer.exe
├── wails.json
└── go.mod / go.sum
```

## 4. Datos

Ruta en Windows:

```text
%APPDATA%\ConveniosUAGro\
├── convenios.db
├── pdfs\<32 hex>.pdf
├── respaldos\
└── settings.json
```

La variable `CONVENIOS_DATA_DIR` permite usar otra carpeta (pruebas). El ejecutable entregado no contiene datos.

### Esquema

- `convenios`: datos, archivo, `texto_pdf` original, `texto` final, estado IA, sugerencias, clasificación, estado de transcripción y fechas.
- `etiquetas`: `ambito`, `estado`, `ciudad`, `tema`; `protegida` para las base.
- `convenio_etiquetas`: N:M con cascada.
- `transcripcion_paginas`: texto por página, PK `(convenio_id, pagina)`.
- `convenios_fts`: FTS5 sobre dependencia, archivo, notas y texto, con triggers.
- `PRAGMA user_version = 2`. La migración v1→v2 agrega `texto_pdf`, el estado de transcripción y `transcripcion_paginas`.

Las etiquetas base son:

- 2 de ámbito: Nacional, Internacional.
- 32 estados de México.
- Se insertan con `INSERT OR IGNORE` en cada arranque.
- No se pueden editar ni borrar.

### Vigencias

`store.EstadoConvenio` devuelve:

- `indefinida`
- `sin_fecha`
- `vencido`
- `por_vencer` (≤90 días)
- `vigente`

Filtros del listado:

- `vigentes`: fecha futura/hoy o indefinida.
- `no_vigentes`: solo vencidos.
- `sin_fecha`: sin fecha e no indefinida.

## 5. API Wails

El frontend llama `window.go.main.App.<Método>`.

| Método | Función |
|---|---|
| `GetBootstrap` | Etiquetas, stats, configuración y datos de la app |
| `ListConvenios(filter)` | Búsqueda, etiquetas, vigencia, orden y página |
| `GetConvenio(id)` | Ficha completa + sugerencias |
| `SaveConvenio(input)` | Guardar captura manual |
| `ChoosePDFs()` | Diálogo nativo de PDF |
| `ImportPDFs(paths)` | Copiar/registrar PDF y extraer texto |
| `SaveConvenioTags(id, ids)` | Reemplazar etiquetas y marcar revisado |
| `AcceptSuggestions(id)` | Crear ciudades y aplicar sugerencias |
| `DiscardSuggestions(id, remove)` | Descartar; opcionalmente dejar sin etiquetas |
| `DeleteConvenios(ids)` | Borrar filas y PDF |
| `DownloadPDF(id)` | Guardar como + copiar PDF |
| `BeginTranscription(id, total)` | Reiniciar la transcripción y fijar el total de páginas |
| `TranscribePage(id, page, imageBase64)` | Enviar una página al modelo de visión y guardar el texto |
| `FailTranscription(id, msg)` | Registrar un error de transcripción |
| `FinishTranscription(id)` | Cerrar el trabajo y reconstruir FTS |
| `GetTranscription(id)` | Estado, avance y texto |
| `DownloadTranscription(id)` | Guardar el texto como `.txt` |
| `GetTags` / `CreateTag` / `UpdateTag` / `DeleteTag` | Catálogo |
| `GetSettings` / `SaveSettings` | IA, clave y modelos |
| `ExtractAI(id)` | Extraer datos y sugerencias |
| `BackupData()` | Guardar ZIP |
| `OpenDataFolder()` | Abrir carpeta de datos |

El servidor de assets de Wails incluye un middleware para `/pdf/<id>`; solo acepta IDs numéricos y sirve desde `data/pdfs`.

## 6. Interfaz

`frontend/dist/index.html` contiene las vistas:

- `#view-convenios`: stats, búsqueda, filtros, tabla y lote.
- `#view-detail`: PDF, formulario, etiquetas, sugerencias, IA.
- `#view-tags`: catálogo y creación/edición.
- `#view-settings`: IA, clave, carpeta y respaldo.

`app.js` no usa framework. Funciones importantes:

- `loadList()` / `renderList()`: filtros, pendientes y tabla.
- `openDetail()` / `renderDetail()`: ficha; el PDF se carga con `/pdf/<id>`.
- `saveConvenio()`, `saveTags()`, `acceptSuggestions()`.
- `openImport()` / `confirmImport()`.
- `downloadPDF()`.
- `renderTagFilter()`, `renderTagSelector()`.
- `renderTranscription()` / `transcribeCurrent()`: PDF.js detecta páginas sin texto, las renderiza a JPEG y las envía una por una.
- `confirmDialog()`, `showToast()`, `showLoading()`.

Los estilos están en `styles.css`; no hay dependencias externas ni CDN. PDF.js se sirve localmente desde `vendor/`.

## 7. IA opcional

`internal/ai` envía el texto del PDF a un endpoint OpenAI-compatible:

- URL por defecto: `https://api.deepseek.com`.
- Modelo por defecto: `deepseek-flash`.
- Temperatura 0.1, `response_format=json_object`, timeout 150 s.
- Valida fechas, duración, vigencia y etiquetas contra el catálogo.
- Solo crea ciudades nuevas; los temas deben existir.
- Máximo 8 etiquetas; una de ámbito, una de estado, hasta 3 ciudades y 4 temas.
- La extracción de datos no procesa PDF escaneados sin texto.

### Transcripción

- `TranscribeImage` usa el **modelo de visión** configurado (`vision_model`), no el modelo de texto.
- El prompt pide transcripción literal, sin resumen, con marcas `[ilegible]`, `[Firma]`, `[Sello]`.
- La imagen se envía como JPEG base64 en `image_url`.
- Si el modelo no acepta imágenes, se muestra un error que pide revisar el modelo de visión.
- El texto se guarda por página; cada guardado reconstruye `convenios.texto` con `texto_pdf` + `[Página N]` y actualiza FTS5.
- Si la app se cierra, `Open` marca las transcripciones `en_proceso` como `pendiente` para reintentar.

La clave se guarda en `settings.json`; la UI solo devuelve una pista (`has_api_key`, `api_key_hint`).

## 8. PDF

- Se valida cabecera `%PDF-`, tamaño (100 MB) y páginas (2000).
- Se copia con nombre aleatorio de 32 hex + `.pdf`.
- Se calcula SHA-256; duplicados se avisan pero se permiten.
- `ledongthuc/pdf` extrae texto por página; si falla, el PDF se conserva y se marca escaneado.
- `es_escaneado` si hay menos de 200 caracteres visibles.
- `paginas_sin_texto` cuenta páginas con menos de 30 caracteres; alimenta el estado inicial de transcripción.
- El visor y la descarga usan el archivo original.
- La transcripción ocurre en la interfaz: PDF.js detecta páginas sin texto y las renderiza a 1800 px como máximo; Go las envía al modelo de visión.
- No hay OCR local; se depende del servicio externo.

## 9. Compilar

En Windows (recomendado):

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0
cd C:\ruta\convenios-desktop
wails build -s -skipbindings -clean -platform windows/amd64
```

`-s` omite el build del frontend porque `frontend/dist` ya está incluido. Para instalar NSIS:

```powershell
winget install --id NSIS.NSIS -e
wails build -s -skipbindings -clean -platform windows/amd64 -nsis -installscope user
```

Salidas:

```text
build/bin/ConveniosUAGro.exe
build/bin/Convenios UAGro-amd64-installer.exe
```

## 10. Pruebas

```bash
go test ./internal/...
SAMPLE_PDF=/ruta/al/pdf go test ./internal/pdfutil -run TestImportSample -v
```

Cubierto:

- Esquema, etiquetas base, estado vacío.
- CRUD, FTS, filtro por etiqueta, filtro de vigencia.
- Sugerencias, creación de ciudades, protección de etiquetas base.
- `SetAIResult` y respaldo ZIP.
- Extracción de PDF y exportación.
- Fechas en español, interpretación de IA y validación.
- Transcripción: actualización de FTS5 y búsqueda del texto transcrito.
- Petición de visión: modelo, autorización y `image_url` con JPEG base64.

Prueba manual realizada:

- Instalador per-user: funcionó.
- App instalada: arranca, base vacía, estado vacío.
- Lista con un PDF real: dependencia, fechas, etiquetas y estado.
- Ficha: visor PDF de WebView2 mostró el documento.
- Descarga: diálogo nativo guardó el PDF (1,269 bytes de prueba).
- Visual con Playwright sobre un mock de la API: sin errores JS.
- Flujo completo de transcripción en Chromium con PDF.js y una API simulada: 3 páginas, barra 3/3, estado "Completa" y búsqueda del texto transcrito con resultado.

## 11. Limitaciones y notas

- Solo Windows 64 bits está empaquetado; el código es portable.
- Requiere WebView2; Windows 11 lo incluye.
- La transcripción usa el servicio externo: requiere clave, internet, un modelo de visión y puede tener costo.
- La transcripción se controla desde la ficha; no hay cola en segundo plano al cerrar la app. Las páginas ya guardadas se conservan.
- Sin multiusuario, login ni permisos (decisión de producto para una app personal).
- La IA es opcional y externa; puede tener costo.
- La clave se guarda sin cifrado adicional, en `settings.json` con permisos locales.
- El ejecutable no está firmado; SmartScreen puede avisar.
- El formato visible de `<input type="date">` depende del idioma de Windows; el valor interno es ISO.
- El respaldo ZIP incluye base y PDF; restaurar copiando a `%APPDATA%\ConveniosUAGro`.

## 12. Entrega

En el escritorio de Windows:

```text
C:\Users\PC\Desktop\ConveniosUAGro\
├── ConveniosUAGro.exe
├── Convenios UAGro-amd64-installer.exe
├── ConveniosUAGro-1.1.0-win64.zip
├── LEEME.txt
├── README.md
├── codigo-fuente.zip
└── SHA256.txt
```

También quedó instalado en:

```text
C:\Users\PC\AppData\Local\Programs\Convenios UAGro\Convenios UAGro.exe
```

con accesos directos en el escritorio y en el menú Inicio. La carpeta de datos del usuario se borró después de las pruebas para que el primer arranque sea vacío.
