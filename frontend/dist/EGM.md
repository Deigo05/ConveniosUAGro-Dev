# frontend/dist — Interfaz de Convenios UAGro

## Overview

Esta carpeta es la interfaz de la app de escritorio (Wails v2). **Aunque se llame `dist`, es código fuente escrito a mano, no el resultado de una compilación.** `wails.json` no tiene comandos de compilación del frontend (`frontend:install` y `frontend:build` están vacíos) y Go la incrusta tal cual con `//go:embed all:frontend/dist` (`main.go`). Si cambias algo aquí, cambia la app.

Qué hay en la carpeta:

- `index.html`: el esqueleto con todas las vistas y diálogos.
- `app.js`: unas 3,200 líneas de JavaScript plano (`'use strict'`, sin módulos ni bundler). Todas sus funciones y constantes son globales de un solo script.
- `styles.css`: los estilos (no se documentan aquí).
- `vendor/`: PDF.js de terceros (`pdf.min.js`, `pdf.worker.min.js`). No se edita.
- `assets/`: imágenes (`escudo.png`).

`app.js` no guarda nada por su cuenta. Todo dato pasa por los métodos de `App` en `app.go`, que se llaman con `call('Metodo', ...)`. Los PDF los sirve el middleware de Go en la ruta local `/pdf/{id}`. La interfaz cubre: lista de convenios con filtros y búsqueda, ficha de detalle con formulario y etiquetas, análisis con IA (transcripción de páginas escaneadas, extracción de datos, verificación de dudas), importación y exportación, archivados, duplicados y versiones, etiquetas, ajustes y actualizaciones.

## Interface

**Puente con Go y arranque**

- `api()` devuelve `window.go.main.App` y lanza un error si no existe. `call(name, ...args)` lanza `'Método no disponible: ' + name` si el método no existe [app.js:api] [app.js:call]
- `DOMContentLoaded` dispara `init`, que conecta los eventos, apunta el worker de PDF.js a `vendor/pdf.worker.min.js` y llama a `GetBootstrap`. Después corre `applyBootstrap`, `loadList`, `setupRuntime` y `retryFailedOnStart`, y programa la revisión de actualizaciones [app.js:init] [app.js:applyBootstrap]
- Métodos de Go que usa la interfaz, todos verificados en `app.go`:
  - Datos: `GetBootstrap`, `ListConvenios`, `GetConvenio`, `SaveConvenio`, `SaveConvenioTags`, `SetNoConvenio`, `DeleteConvenios`, `ArchiveConvenios`, `UnarchiveConvenios`
  - Etiquetas: `AcceptSuggestions`, `DiscardSuggestions`, `CreateTag`, `UpdateTag`, `DeleteTag`
  - Importar y exportar: `ChoosePDFs`, `CheckImport`, `ImportPDFs`, `ChooseImportFile`, `ImportConvenios`, `ExportConvenios`, `ExportConveniosCSV`
  - Descargas: `DownloadPDF`, `DownloadPDFs`, `DownloadPDFsFiltered`, `DownloadTranscription`
  - IA: `GetTranscription`, `BeginTranscription`, `ResumeTranscription`, `TranscribePage`, `FinishTranscription`, `FailTranscription`, `ExtractAI`, `PagesToVerify`, `VerifyAI`, `RetryableError`, `RetryCandidates`
  - Duplicados y versiones: `ListDuplicates`, `DismissDuplicate`, `LinkVersion`, `UnlinkVersion`
  - Ajustes y sistema: `SaveSettings`, `BackupData`, `OpenDataFolder`, `CheckUpdate`, `ApplyUpdate`, `OpenReleasePage`

  [app.js:call] [app.js:setArchived]
- Eventos de Wails: `runtime.OnFileDrop` va a `handleDroppedFiles`. `EventsOn('update:progress')` va a `onUpdateProgress`. `EventsOn('duplicates:refreshed')` recarga los datos de la vista actual. Go emite esos dos eventos con `EventsEmit` en `app.go` [app.js:setupRuntime]

**Estado y constantes**

- El objeto global `state` guarda:
  - `view`, `tags`, `stats`, `config` y `app`
  - `filters` (`query`, `tag_ids`, `vigencia`, `solo_pendientes`, `solo_no_convenios`, `incluir_anteriores`, `order`, `dir`, `page`) y `result`
  - la selección `selected`, el convenio abierto `current`, y `dirty`/`formId` para cambios sin guardar
  - la cola de IA: `autoQueue`, `autoActive`, `autoRunning`, `transcribing`, `activity`
  - la actualización disponible: `update`
  - duplicados (`dups`, `dupCompare`) y archivados (`arch`)

  [app.js:state]
- Constantes: `TYPE_ORDER`, `TYPE_LABEL`, `PALETTE`, `EXCLUSIVE_TYPES`, `UPDATE_INTERVAL_MS` (6 h), `AUTO_FILES_AT_ONCE` (3), `PAGES_AT_ONCE` (10), `PAGE_RETRY_DELAYS_MS`, `FILE_RETRY_DELAYS_MS`, `MAX_IMPORT` (60) y `MOTIVO` (motivos de duplicado) [app.js:TYPE_ORDER] [app.js:EXCLUSIVE_TYPES] [app.js:MAX_IMPORT] [app.js:MOTIVO]

**Funciones por área**

- Lista y filtros: `loadList`, `renderList`, `rowHTML`, `rowChipsHTML` (3 etiquetas visibles y «+N»), `vigenciaCellHTML` (estado, fecha y plazo en una celda), `aiButtonHTML`, `snippetHTML`, `stateBadge`, `relativeVigencia`, `renderTagFilter`, `clearFilters`, `scheduleLoad`, `syncProcessingPoll` y la selección con `bulkDelete` [app.js:loadList] [app.js:renderList] [app.js:bulkDelete]
- Ficha de detalle:
  - formulario y vista: `openDetail`, `renderDetail`, `fillForm`, `saveConvenio`, `navigate` (devuelve booleano), `markDirty`/`clearDirty`
  - etiquetas: `renderTagSelector`, `onTagSelectorChange`, `syncLocation`, `saveTags`, `acceptSuggestions`, `discardSuggestions`, `leaveWithoutTags`
  - estado del documento: `setNoConvenio`, `deleteCurrent`

  [app.js:openDetail] [app.js:saveConvenio] [app.js:navigate]
- IA:
  - transcripción y extracción: `transcribeConvention(id, {auto, confirm})`, `transcribeCurrent`, `extractAI`, `verifyDoubts`
  - render de páginas: `renderPageJPEG`
  - cola automática: `startAutoAI`, `runAutoAIQueue`, `processAutoFile`, `processAutoFileOnce`, `analyzeWithAI`
  - reintentos: `withRetries`, `isRetryable`, `retryFailedOnStart`
  - cupo de páginas: `acquirePageSlot`/`releasePageSlot`

  [app.js:transcribeConvention] [app.js:runAutoAIQueue] [app.js:withRetries] [app.js:analyzeWithAI]
- Importar, exportar y descargar:
  - importar: `openImport`, `openImportWith`, `confirmImport`, `renderImportChecks`, `importPackage`, `handleDroppedFiles`
  - exportar: `exportPackage`, `exportCSV`
  - descargar: `downloadResults`, `downloadPDF`, `downloadPDFs`, `downloadTranscription`

  [app.js:confirmImport] [app.js:importPackage] [app.js:downloadResults]
- Archivados: `setArchived`, `loadArchived`, `renderArchived`, `bindArchivedEvents` [app.js:setArchived] [app.js:loadArchived]
- Duplicados y versiones: `loadDuplicates`, `renderDuplicates`, `onDuplicatesClick`, `toggleDupCompare`, `renderRelations`, `onRelationsClick`, `searchVersions`, `linkVersions`, `olderNewer` [app.js:onDuplicatesClick] [app.js:linkVersions] [app.js:onRelationsClick]
- Etiquetas, ajustes y actualizaciones: `renderTagsView`, `createTag`, `saveTagEdit`, `deleteTag`, `renderSettings`, `saveSettings`, `clearKey`, `backupData`, `refreshData`, `checkForUpdates`, `applyUpdate`, `dismissUpdate` [app.js:createTag] [app.js:saveSettings] [app.js:checkForUpdates] [app.js:applyUpdate]
- Utilidades de interfaz: `$`, `$$`, `confirmDialog` (devuelve `Promise<boolean>`), `infoDialog`, `showToast`, `showLoading`, `setButtonBusy`, `esc`, `errorText`, `safeColor`, `chipHTML` (fondo tenue con `tint` y texto con `readableOn`), `normalize`, `formatDate`/`formatDateTime`/`formatLocalDateTime` (`es-MX`) [app.js:confirmDialog] [app.js:esc] [app.js:safeColor]

**Plantilla `index.html`**

- Vistas (`section.view`): `#view-convenios` (la inicial), `#view-detail`, `#view-archived`, `#view-dups`, `#view-tags` y `#view-settings`. `showView` las alterna según `state.view`: `convenios`, `detalle`, `archivados`, `duplicados`, `etiquetas` o `ajustes` [index.html:section.view] [app.js:showView]
- Diálogos: `#dialog-import`, `#dialog-version`, `#dialog-confirm`, `#dialog-tag` y `#dialog-update`. Capas: `#drop-overlay`, `#toasts` y `#loading` [index.html:dialog]
- Orden de carga: `styles.css` en `<head>`. Al final de `<body>` van `vendor/pdf.min.js` y luego `app.js`, como scripts normales, no módulos [index.html:script]

## Invariants

**Conexión entre `index.html` y `app.js`**

- `app.js` encuentra los elementos por su `id` con `$()`/`$$()`. Si cambias un `id` o un atributo `data-*` en `index.html`, cambia también `app.js`, o el botón deja de funcionar sin ningún error [app.js:$] [index.html:section.view]
- `pdf.min.js` tiene que cargarse antes que `app.js`. Sin `window.pdfjsLib`, la transcripción y la verificación de dudas se niegan a correr [index.html:script] [app.js:transcribeConvention] [app.js:verifyDoubts]
- Cada método llamado con `call('X')` tiene que existir como `func (a *App) X` en `app.go`. Si falta, solo falla al usarse, con `Método no disponible` [app.js:call]

**Seguridad del HTML**

- Todo texto del usuario o de Go pasa por `esc()` antes de entrar en `innerHTML`.
- Los colores de etiqueta pasan por `safeColor()`, que solo acepta `#rrggbb` y si no usa `#3A5A9C`.
- `snippetHTML` escapa primero y solo después cambia las marcas `\u0002`/`\u0003` del servidor por `<mark>`.

[app.js:esc] [app.js:safeColor] [app.js:snippetHTML] [app.js:rowHTML]

**Cambios sin guardar**

- `navigate` pregunta antes de salir de `detalle` si `state.dirty` es verdadero. Si el usuario cancela, devuelve `false` y la vista no cambia. Ctrl/Cmd+S guarda el formulario [app.js:navigate] [app.js:bindEvents]
- `renderDetail` no sobrescribe el formulario con datos refrescados mientras haya cambios sin guardar del mismo convenio. `openOther` también pregunta antes de salir [app.js:renderDetail] [app.js:openOther]

**Reglas de etiquetas**

- Los tipos `ambito`, `estado`, `ciudad` y `facultad` admiten una sola etiqueta. `tema` admite varias [app.js:EXCLUSIVE_TYPES] [app.js:onTagSelectorChange]
- `syncLocation` aplica estas reglas de ubicación:
  - Una ciudad marca su estado y el ámbito «Nacional».
  - Un estado quita las ciudades de otros estados.
  - Un ámbito distinto de «Nacional» quita todos los estados y ciudades.
  - Desmarcar un estado o «Nacional» quita lo que depende de él.

  [app.js:syncLocation]
- Al crear una etiqueta, el estado solo se envía si `tipo === 'ciudad'`. Al editar, el tipo no se puede cambiar. Las etiquetas `protegida` no muestran botones para editar ni borrar [app.js:createTag] [app.js:openTagDialog] [app.js:renderTagsView]

**Carga de la lista**

- `loadList` nunca corre dos veces a la vez. Si la llaman durante una carga, marca `listReload` y vuelve a cargar al terminar. Archivados sigue la misma regla [app.js:loadList] [app.js:loadArchived]
- Cualquier cambio de filtro regresa a la página 1.
- Con búsqueda activa, el orden `id` pasa a `relevancia`, y al borrar la búsqueda regresa a `id`.
- `dependencia` se ordena ascendente y todo lo demás descendente.

[app.js:syncRelevanceOrder] [app.js:clearFilters] [app.js:bindEvents]

**IA: costo y concurrencia**

- No hay IA sin `config.ai_enabled && config.has_api_key`. Aplica a `startAutoAI`, `transcribeConvention`, `retryFailedOnStart` y `analyzeWithAI` [app.js:startAutoAI] [app.js:transcribeConvention] [app.js:retryFailedOnStart] [app.js:analyzeWithAI]
- Concurrencia:
  - Máximo `AUTO_FILES_AT_ONCE` (3) archivos a la vez en la cola automática.
  - Máximo `PAGES_AT_ONCE` (10) páginas enviadas a la vez entre todos los documentos (semáforo global `pageSlots`, atiende en orden de llegada y libera en `finally`).
  - Un documento no se transcribe dos veces a la vez (`state.transcribing`).

  [app.js:runAutoAIQueue] [app.js:pageSlots] [app.js:transcribeConvention]
- Reintentos: cada página se reintenta hasta 2 veces (5 s y 15 s) y cada archivo completo hasta 2 veces (30 s y 90 s). Qué error se reintenta lo decide Go con `RetryableError`. Si esa llamada falla, el error no se reintenta. El tope de 3 aperturas seguidas por convenio lo aplica Go, no el JS [app.js:PAGE_RETRY_DELAYS_MS] [app.js:FILE_RETRY_DELAYS_MS] [app.js:withRetries] [app.js:isRetryable]
- Orden de la transcripción:
  1. `GetTranscription`
  2. `ResumeTranscription` si se retoma una incompleta (solo se envían las páginas que faltan), o `BeginTranscription` si se empieza de cero
  3. `TranscribePage` por cada página pendiente
  4. `FinishTranscription`
  5. `ExtractAI`
  6. `verifyDoubts`

  Si algo falla antes de guardar todas las páginas, se llama a `FailTranscription`. Un fallo de extracción después de `FinishTranscription` no marca la transcripción como fallida [app.js:transcribeConvention]
- Una página con menos de 30 caracteres de texto (sin contar espacios) se considera escaneada. Una transcripción `completa` no se repite en automático [app.js:transcribeConvention] [app.js:processAutoFileOnce]
- PDF.js decide por su cuenta qué páginas son escaneadas, y puede no coincidir con lo que marcó Go al importar. Si no encuentra ninguna, `transcribeConvention` devuelve `'sin_paginas'`; la cola automática da la transcripción por completa (`BeginTranscription(id, 0)`) y sigue con `ExtractAI`. Sin texto, `ExtractAI` deja el error en la ficha [app.js:processAutoFileOnce] [app.js:transcribeConvention]
- Al arrancar, si `GetBootstrap` trae `aviso` (por ejemplo, ajustes dañados), se muestra como aviso de error. Si la búsqueda de actualizaciones falla, la insignia dice «No se pudo revisar» y el detalle queda en su `title` [app.js:renderUpdate]
- Tamaño de las imágenes: las páginas se renderizan a 1600 px y JPEG 0.82, y para verificar dudas a 2400 px y JPEG 0.9 [app.js:renderPageJPEG] [app.js:verifyDoubts]

**Confirmaciones**

- Se pide confirmación antes de:
  - borrar convenios o etiquetas, o quedarse sin etiquetas
  - quitar la clave de la API
  - transcribir a mano (el aviso dice cuántas páginas y que tiene costo)
  - importar un paquete
  - vincular o desvincular versiones, o borrar un duplicado
  - aplicar una actualización

  [app.js:bulkDelete] [app.js:deleteCurrent] [app.js:deleteTag] [app.js:leaveWithoutTags] [app.js:clearKey] [app.js:importPackage] [app.js:linkVersions] [app.js:onDuplicatesClick] [app.js:applyUpdate]
- Archivar o desarchivar y descartar un par duplicado no piden confirmación, porque se deshacen con un clic [app.js:setArchived] [app.js:onDuplicatesClick]
- Solo puede haber un `confirmDialog` pendiente: uno nuevo resuelve el anterior con `false` [app.js:confirmDialog]

**Importación**

- Máximo `MAX_IMPORT` (60) PDF por tanda.
- Los repetidos (`duplicate_id` o `repetido`) se excluyen salvo que se marque «Importar de todos modos». Los que tienen error nunca se importan.
- Solo se aceptan archivos soltados `.pdf`, `.zip` o `.db`, y se ignoran si hay un diálogo abierto.

[app.js:openImportWith] [app.js:selectedImportPaths] [app.js:handleDroppedFiles]

**Otras reglas**

- Solo puede haber una comparación de PDF abierta en duplicados, porque cada visor consume memoria [app.js:toggleDupCompare]
- `olderNewer` ordena por `fecha_firma`. Si no hay fechas o son iguales, el de `id` menor se toma como el más antiguo [app.js:olderNewer]
- Actualizaciones: solo se revisan si `config.update_check !== false`, a los 4 s y después cada 6 h, y nunca dos revisiones a la vez. Ocultar el aviso aplica solo a esa versión y commit (`update-dismissed`) [app.js:init] [app.js:checkForUpdates] [app.js:renderUpdate] [app.js:updateKey]
- `saveSettings` envía `clear_api_key:false`: un campo de clave vacío conserva la clave guardada [app.js:saveSettings]

## Side Effects

- Escrituras en la base de datos por Go:
  - convenios: `SaveConvenio`, `SaveConvenioTags`, `SetNoConvenio`, `ArchiveConvenios`/`UnarchiveConvenios`, `DeleteConvenios` (también borra el PDF)
  - etiquetas: `AcceptSuggestions`, `DiscardSuggestions` (con `true` deja el convenio sin etiquetas y marcado como revisado), `CreateTag`, `UpdateTag`, `DeleteTag` (la quita de todos los convenios)
  - duplicados y versiones: `LinkVersion`, `UnlinkVersion`, `DismissDuplicate`
  - ajustes: `SaveSettings`

  [app.js:saveConvenio] [app.js:saveTags] [app.js:setNoConvenio] [app.js:setArchived] [app.js:bulkDelete] [app.js:deleteCurrent] [app.js:leaveWithoutTags] [app.js:deleteTag] [app.js:linkVersions] [app.js:onRelationsClick] [app.js:onDuplicatesClick] [app.js:saveSettings]
- Servicio externo de IA, a través de Go: `TranscribePage` envía la imagen de la página en JPEG base64, `ExtractAI` envía el texto y `VerifyAI` envía imágenes de las páginas dudosas. Cada llamada puede tener costo con el proveedor configurado. Por defecto el proveedor es `https://api.deepseek.com` con el modelo `deepseek-flash` [app.js:transcribeConvention] [app.js:extractAI] [app.js:verifyDoubts] [app.js:renderSettings]
- Estado de la transcripción en Go: `BeginTranscription`, `ResumeTranscription`, `FinishTranscription`, `FailTranscription` [app.js:transcribeConvention]
- `clearKey` borra la clave guardada (`clear_api_key:true`) y desactiva la IA [app.js:clearKey]
- Archivos y sistema operativo:
  - Selectores nativos: `ChoosePDFs` y `ChooseImportFile`.
  - Copia de PDF al almacén de la app: `ImportPDFs`. Importación de paquete o respaldo: `ImportConvenios`.
  - Escritura de archivos: `ExportConvenios`, `ExportConveniosCSV`, `DownloadPDF(s)`, `DownloadPDFsFiltered`, `DownloadTranscription` y `BackupData` (ZIP).
  - Apertura fuera de la app: `OpenDataFolder` abre la carpeta de datos y `OpenReleasePage` abre la página del release.

  [app.js:openImport] [app.js:confirmImport] [app.js:importPackage] [app.js:exportPackage] [app.js:exportCSV] [app.js:downloadResults] [app.js:backupData] [app.js:openDataFolder] [app.js:bindEvents]
- `ApplyUpdate` descarga e instala la actualización. Según el aviso, respalda la base y reinicia la app. El progreso llega por el evento `update:progress` [app.js:applyUpdate] [app.js:onUpdateProgress]
- Al importar PDF nuevos, sin error y que no sean clones, arranca la cola de IA automática. Al abrir la app se reintentan los convenios que devuelve `RetryCandidates` [app.js:confirmImport] [app.js:retryFailedOnStart]
- Temporizadores:
  - revisión de actualizaciones: `setTimeout` de 4 s y `setInterval` de 6 h
  - recarga de la lista cada 3 s mientras haya convenios en proceso, solo en la vista `convenios`
  - esperas al buscar: 240 ms en la búsqueda, 180 ms en el filtro de etiquetas y 250 ms en la búsqueda de versiones
  - esperas entre reintentos (`sleep`)

  [app.js:init] [app.js:syncProcessingPoll] [app.js:scheduleLoad] [app.js:renderTagFilter] [app.js:bindDuplicateEvents] [app.js:sleep]
- `localStorage` guarda solo la clave `update-dismissed`, y todo acceso va dentro de `try/catch` [app.js:dismissUpdate] [app.js:storageGet] [app.js:storageSet]
- PDF.js carga los PDF desde `/pdf/{id}`, y la vista de duplicados los muestra en `iframe`. Los documentos PDF.js se destruyen al terminar [app.js:transcribeConvention] [app.js:dupCompareHTML]
- Escuchas globales: `keydown` para Escape, Ctrl/Cmd+S, `/` y Ctrl/Cmd+N; `unhandledrejection` muestra un aviso; `dragenter`/`dragleave`/`drop` muestran la capa de soltar archivos [app.js:bindEvents] [app.js:setupRuntime]

## Verification

- Run: `go test ./...` (desde la raíz del repositorio). Estas pruebas cubren el backend de Go e incluyen `TestVersionFilesMatch`, que exige que `VERSION`, `appVersion` y `productVersion` de `wails.json` coincidan.
- No hay pruebas automáticas del frontend. Los cambios en `app.js` o `index.html` se comprueban a mano en la app (por ejemplo con `wails dev`). Revisa la vista afectada, la consola del WebView y que cada `id` que usa `app.js` siga existiendo en `index.html`.
- Estilo: títulos con serif de Windows (`--titulo`: Cambria o Georgia, sin fuentes incrustadas) y el oro institucional (`--oro`) solo como acento (sección activa y línea bajo la marca). La lista abre la ficha al hacer clic en la fila o en el nombre (un botón, para usarla con teclado); las acciones de la fila son botones de ícono con `title` y `aria-label` [styles.css:root] [app.js:rowHTML]
