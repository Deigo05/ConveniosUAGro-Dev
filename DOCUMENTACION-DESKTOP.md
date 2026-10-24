# Convenios UAGro Desktop — Documentación técnica

**Versión:** 1.2.3  
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
│   │   └── uagro/             Catálogo de facultades y sedes UAGro
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
- `etiquetas`: `ambito`, `estado`, `ciudad`, `facultad`, `tema`; `protegida` para las base. `etiquetas.estado` guarda el estado de cada ciudad. `convenios.facultad_ciudad` guarda la sede de la facultad, separada de la ciudad de la dependencia.
- `convenios.no_es_convenio`: 0 normal, 1 la IA (o el usuario) lo marcó como «no es convenio», 2 el usuario confirmó que sí lo es; `SetAIConvenioFlag` nunca cambia un 2.
- `meta(clave, valor)`: marcas de reparaciones que se ejecutan una sola vez (por ejemplo `reparacion_etiquetas_obligatorias`).
- `convenio_etiquetas`: N:M con cascada.
- `transcripcion_paginas`: texto por página, PK `(convenio_id, pagina)`.
- `convenios.dependencia_corta` (siglas) y `convenios.tipo_documento` (catálogo `store.TiposDocumento`): los extrae la IA, se editan en la ficha y forman el nombre de descarga.
- `convenios.huella`: SimHash de 64 bits del texto (grupos de 3 palabras); `NULL` cuando el texto cambió y falta recalcularla, 0 si hay menos de 80 palabras.
- `convenios.reemplazado_por`: id de la versión que renueva a esta (`ON DELETE SET NULL`). La versión vigente tiene `NULL`; el listado y las estadísticas solo cuentan vigentes salvo `incluir_anteriores`.
- `convenio_duplicados(convenio_a < convenio_b, motivo)`: pares por revisar con motivo `archivo` (mismo SHA-256), `texto` (huellas a ≤8 bits y contraparte compatible), `datos` (misma contraparte, fecha, tipo y facultad) o `renovacion` (misma contraparte, tipo y facultad con otra fecha; solo entre vigentes y solo con el anterior más reciente). `ON DELETE CASCADE`: al borrar uno, la marca del otro desaparece. `store.RefreshDuplicates` recalcula todo en memoria después de cada cambio (importar, guardar, etiquetas, IA, transcripción, borrar) y al arrancar.
- `duplicados_descartados`: pares que el usuario marcó como distintos; no se vuelven a proponer.
- `convenios.busqueda`: firmantes, cargos, facultad, sede, vigencia y nombres de etiquetas, mantenida por triggers.
- `convenios_fts` (definido en `search.sql`, se crea después de las migraciones): FTS5 propio sobre dependencia, archivo, notas, texto y `busqueda`. Los triggers borran por `rowid` e insertan la fila actual; `ensureSearchIndex` lo reconstruye si el conteo no coincide.
- `PRAGMA user_version = 6`. Las migraciones agregan transcripción, firmantes/facultad, tipo `facultad`, `facultad_ciudad` y (v6) `busqueda` con el índice nuevo.

Búsqueda: términos con prefijo y frases entre comillas (AND); si no hay resultados se repite con OR y se marca `aproximado`. El orden `relevancia` pone primero, con `relevanceTiers`, lo buscado como frase en `dependencia` (la última palabra como prefijo), luego la frase exacta en cualquier columna, la frase con la última palabra como prefijo y todas las palabras completas, sin prefijo; después ordena por `bm25` con peso 10 para `dependencia`. Sin esos grupos el orden era casi al azar: `bm25` casi anula las palabras que están en más de la mitad de los convenios, y como prefijo «par» y «3» coinciden con «para» y «39070». Cada fila trae un `fragmento` con la coincidencia entre los caracteres 0x02/0x03.

Las etiquetas base son:

- 2 de ámbito: Nacional, Internacional.
- 32 estados de México.
- Se insertan con `INSERT OR IGNORE` en cada arranque.
- No se pueden editar ni borrar.
- Además se crea la etiqueta protegida de facultad **Convenio Abierto** (`#00796B`). `store.MarkOpenAgreement` la asigna a convenios analizados o clasificados sin facultad ni otra etiqueta de facultad; `UnmarkOpenAgreement` la retira cuando se captura una facultad.

Reglas de ubicación (`consistentTags`, en `tags.go`):

- Una ciudad con estado agrega ese estado; un estado de México agrega «Nacional».
- `ReplaceTags` (cambios del usuario) rechaza con `TagConflictError` una ciudad con el estado de otra o un estado con «Internacional». `ReplaceTagsAuto` (IA y reparaciones) corrige en vez de rechazar: gana el estado de la ciudad.
- Una ciudad sin estado (de versiones anteriores) adopta el estado con el que se guarda; `BackfillCityStates` asigna al arrancar el estado más usado por sus convenios.
- `AcceptSuggestions` busca la ciudad con `findCity`: prefiere la del mismo estado y, si no existe, la crea con el estado sugerido.
- `EnsureAmbito` agrega «Nacional» cuando un convenio analizado no tiene ámbito; `applyAITags` lo aplica junto con la facultad salvo en archivos marcados como «no es convenio».

Catálogo de facultades (`internal/uagro`): cada unidad tiene nombre oficial, sede, alias y si es la sede principal. `Resolve` quita prefijos («Unidad Académica de», «Escuela Superior de», «UA»), siglas (EMVZ, FCA…) y sufijos («de la UAGro»), conserva el número y elige la sede: la que coincide con la ciudad, la única que existe o la principal. Las unidades no catalogadas se aceptan con su nombre normalizado; «Rectoría» y otras áreas administrativas devuelven `ok=false` y el convenio queda como «Convenio Abierto».

### Vigencias

`store.EstadoConvenio` devuelve:

- `indefinida`
- `sin_fecha`
- `vencido`
- `por_vencer` (≤90 días)
- `vigente`

Filtros del listado:

- `vigentes`: fecha futura/hoy o indefinida.
- `por_vencer`: vence en 90 días o menos.
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
| `SaveConvenioTags(id, ids)` | Reemplazar etiquetas y marcar revisado; rechaza ciudad y estado que no coinciden |
| `SetNoConvenio(id, noEs)` | Marcar como «no es convenio» o confirmar que sí lo es (agrega las etiquetas obligatorias) |
| `AcceptSuggestions(id)` | Crear ciudades/facultades y aplicar sugerencias |
| `DiscardSuggestions(id, remove)` | Descartar; opcionalmente dejar sin etiquetas |
| `DeleteConvenios(ids)` | Borrar filas y PDF |
| `DownloadPDF(id)` | Guardar como + copiar PDF con el nombre útil |
| `DownloadPDFs(ids)` | Copiar varios PDF a una carpeta con nombres útiles (sin sobrescribir) |
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
| `ExportConvenios(ids)` | Paquete `.zip` (manifest.json, convenios.json, pdfs/); todos si `ids` está vacío |
| `ExportConveniosCSV(ids, filter)` | CSV UTF-8 con BOM; usa el filtro si `ids` está vacío |
| `ChooseImportFile()` / `ImportConvenios(path)` | Importar paquete, respaldo ZIP o `convenios.db`; omite SHA-256 repetidos |
| `CheckImport(paths)` | SHA-256 de cada archivo antes de importar: ya registrado (`duplicate_id`) o repetido en la selección |
| `ListDuplicates()` | Pares por revisar con el resumen de ambos convenios |
| `DismissDuplicate(a, b)` | Marcar un par como distinto |
| `LinkVersion(older, newer)` / `UnlinkVersion(id)` | Unir o separar versiones del historial (evita ciclos) |
| `CheckUpdate()` | Consultar el release más reciente de GitHub |
| `ApplyUpdate()` | Respaldar base, descargar, verificar, reemplazar el exe y reiniciar (evento `update:progress`) |
| `OpenReleasePage(url)` | Abrir la página del release |

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
- `confirmDialog()`, `infoDialog()`, `showToast()`, `showLoading()`.
- `navigate()`: cambia de vista y pregunta si hay cambios sin guardar (`markDirty()`/`clearDirty()`).
- `exportPackage()`, `exportCSV()`, `importPackage()`; arrastrar y soltar con `runtime.OnFileDrop`.
- `checkForUpdates()`, `renderUpdate()`, `applyUpdate()`.

Los estilos están en `styles.css`; no hay dependencias externas ni CDN. PDF.js se sirve localmente desde `vendor/`.

## 7. IA opcional

`internal/ai` envía el texto del PDF a un endpoint OpenAI-compatible:

- URL por defecto: `https://api.deepseek.com`.
- Modelo por defecto: `deepseek-flash`.
- Temperatura 0.1, `response_format=json_object`, timeout 150 s.
- Valida fechas, duración, vigencia y etiquetas contra el catálogo.
- Solo crea ciudades y facultades nuevas; los temas deben existir.
- Máximo 8 etiquetas; una de ámbito, una de estado, una ciudad, una facultad y hasta 4 temas.
- La extracción de datos no procesa PDF escaneados sin texto, salvo que ya estén transcritos.
- Nivel de Ajustes `low`/`medium`/`high` (Rápido, Detallado, Máximo; predeterminado `low`). En DeepSeek se envía como `reasoning_effort` `low`/`high`/`max`; visión y búsqueda de ubicación usan `low`.
- El prompt pide `es_convenio` y `domicilio` de la contraparte. `enrichLocation` busca en DuckDuckGo si falta ciudad o estado, si la ciudad sería nueva o si el domicilio trae `[?]`; las consultas van de una en una con 2.5 s de separación, se guardan en caché y se pausan 15 min si aparece el reto anti-robots.
- Consultas de ubicación: primero `locationQuery` (razón social, o siglas si es muy larga, más domicilio o `lugar_firma`); si `confirmLocation` no confirma nada, `brandQuery` (hasta 3 `marcas` de logotipos y encabezados, más domicilio o lugar de firma). `confirmLocation` envía razón social, marcas, representante, domicilio y lugar de firma, y solo acepta resultados que sean claramente la misma entidad (con un nombre común exige otro dato coincidente).
- Si la ubicación cambia con la web y no hay domicilio en el documento (o cambia el estado leído), `enrichLocation` agrega a `notas` «Ubicación de la contraparte tomada de internet (no consta en el documento): …; revísela». Si solo corrige un municipio de un domicilio que sí consta, no avisa.
- La regla 6 del prompt pide estado y ciudad solo si el documento dice dónde está la contraparte; el lugar de firma y los domicilios de la UAGro no bastan (el lugar de firma solo se usa como pista en la búsqueda).
- `searchWeb` devuelve `ok=false` si no pudo consultar (sin conexión o con el reto anti-robots) y `enrichLocation` devuelve `unavailable`. `noteMissingLocation` explica en `notas` por qué un convenio nacional queda sin estado: no consta ni se confirmó en internet, no se pudo consultar internet (volver a extraer más tarde) o la búsqueda web está desactivada.
- `chat` reintenta a los 3 s y 10 s ante 429, 5xx o errores de red, con un cliente HTTP compartido.
- `CompactText` reduce espacios y conserva inicio, párrafos con palabras clave y final (máx. 32 000 caracteres).
- El prompt pide `renovacion: {es, fecha_anterior}`; si el documento declara que renueva a otro, `linkRenewal` une la versión con el candidato de renovación más antiguo que coincide (o el de esa fecha).
- Con texto transcrito el prompt pide `correcciones` de nombres; `store.ApplyNameCorrections` solo las aplica si ambos nombres tienen dos o más palabras, se parecen (≥0.6) y la forma correcta aparece en el texto.
- El mensaje de usuario empieza con la fecha de registro (`Client.ReferenceDate`, de `creado_en`); queda fuera del prompt de sistema para no romper la caché. `checkSignatureDate` agrega una nota si `fecha_firma` es posterior.
- Reglas de escaneados: firmantes del bloque final comparados con proemio y declaraciones (misma parte y cargo ⇒ misma persona, se prefiere la forma del proemio); `[Tachado: …]` no vale; si cierre y anotación de firmas difieren, gana la anotación con nota. `lugar_firma` alimenta `locationQuery` cuando falta el domicilio.
- **Dudas de lectura**: con texto transcrito el prompt pide `dudas` (máx. 3: páginas, dato y lecturas). `App.extract` las guarda en memoria; la interfaz llama a `PagesToVerify` (máx. 3 páginas transcritas), renderiza esas páginas a 2400 px (`renderPageJPEG`, escala ≤2.2, calidad 0.9) y llama a `VerifyAI`. `Client.VerifyPage` manda imagen, transcripción de la página y dudas al modelo de visión (`low`, JSON); `relevantCorrections` descarta lo que no comparte palabras con las lecturas dudosas y `store.ApplyPageCorrections` reemplaza solo en esa página (original de ≤12 palabras presente en la página, reemplazo de ≤160 caracteres). Si hubo cambios, se vuelve a extraer una vez (sin guardar dudas) con la nota «Verificado con la imagen: …».

### Transcripción

- `TranscribeImage` usa el **modelo de visión** configurado (`vision_model`), no el modelo de texto.
- El prompt pide transcripción literal, sin resumen, nombres letra por letra, `[?]` en letras dudosas y marcas `[ilegible]`, `[Firma]`, `[Sello: texto]`, `[Tachado: texto]` y `[Manuscrito: texto]`.
- La imagen se envía como JPEG base64 en `image_url`.
- Si el modelo no acepta imágenes, se muestra un error que pide revisar el modelo de visión.
- El texto se guarda por página; cada guardado reconstruye `convenios.texto` con `texto_pdf` + `[Página N]` y actualiza FTS5.
- Si la app se cierra, `Open` marca las transcripciones `en_proceso` como `pendiente` para reintentar.

La clave se guarda en `settings.json`; la UI solo devuelve una pista (`has_api_key`, `api_key_hint`).

## 8. PDF

- Se valida cabecera `%PDF-`, tamaño (100 MB) y páginas (2000).
- Se copia con nombre aleatorio de 32 hex + `.pdf`.
- Se calcula SHA-256; duplicados se avisan pero se permiten y copian los datos del original (`CloneAnalysis`).
- `ledongthuc/pdf` extrae texto por página; si falla, el PDF se conserva y se marca escaneado.
- `es_escaneado` si hay menos de 200 caracteres visibles.
- `paginas_sin_texto` cuenta páginas con menos de 30 caracteres; alimenta el estado inicial de transcripción.
- El visor y la descarga usan el archivo original; el nombre de descarga lo calcula `store.FileName` al leer el convenio (`archivo_nombre`): `AAAA-MM-DD Tipo - Contraparte - Facultad.pdf`, con la etiqueta de facultad (o «Convenio Abierto»), siglas o la primera contraparte recortada, sin caracteres no válidos en Windows y con máximo 150 caracteres. Sin contraparte se usa el nombre original.
- La transcripción ocurre en la interfaz: PDF.js detecta páginas sin texto y las renderiza a 1600 px como máximo; Go las envía al modelo de visión con reintentos de tokens. La interfaz procesa 3 archivos a la vez (`AUTO_FILES_AT_ONCE`) y comparte un cupo de 10 páginas simultáneas entre todos (`PAGES_AT_ONCE`, `acquirePageSlot`).
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

Compilación con datos de versión (lo hace el flujo de GitHub):

```powershell
wails build -s -skipbindings -clean -platform windows/amd64 -ldflags "-X main.buildCommit=<sha> -X main.buildDate=<RFC3339>"
```

`VERSION`, `wails.json` y `appVersion` en `app.go` deben coincidir (`TestVersionFilesMatch`).

## 10. Pruebas

```bash
go test ./internal/...
SAMPLE_PDF=/ruta/al/pdf go test ./internal/pdfutil -run TestImportSample -v
```

Cubierto:

- Esquema, etiquetas base, estado vacío.
- CRUD, FTS, filtro por etiqueta, filtro de vigencia.
- Sugerencias, creación de ciudades y facultades, protección de etiquetas base.
- `SetAIResult` y respaldo ZIP.
- Extracción de PDF y exportación.
- Fechas en español, interpretación de IA y validación.
- Transcripción: actualización de FTS5 y búsqueda del texto transcrito.
- Petición de visión: modelo, autorización y `image_url` con JPEG base64.
- Migración v1→v6 y v5→v6, búsqueda por firmantes/etiquetas, aproximada y fragmentos; el nombre buscado sale primero aunque sus palabras sean prefijo de otras comunes (`TestSearchRanksNameFirst`).
- Convenio Abierto, unificación de nombres, copia de análisis de duplicados.
- Ubicación por web sin salir a internet (caché precargada y modelo simulado): búsqueda por marcas cuando la razón social no aparece, aviso de ubicación tomada de internet, sin aviso al corregir un domicilio que consta y sin ubicación cuando no hay evidencia.
- Revisión de dudas con la imagen (`TestVerifyDoubtsWithImage`): corrige solo la página dudosa, vuelve a extraer y no repite el ciclo; `ApplyPageCorrections`, `parseDoubts`, `relevantCorrections`, fecha de firma posterior al registro y lugar de firma en la búsqueda de ubicación.
- Exportar/importar paquete y respaldo, CSV.
- Actualizaciones: comparación de versiones, release nuevo/igual, descarga con verificación e instalación.

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
├── ConveniosUAGro-1.1.6-win64.zip
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
