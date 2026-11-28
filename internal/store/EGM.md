# store

## Overview
`internal/store` es la capa de persistencia de la app de escritorio ConveniosUAGro (módulo
`convenios-uagro-desktop`; la importa `app.go` en la raíz del repo). Guarda en una base SQLite local
(`modernc.org/sqlite`, sin cgo) los convenios, sus PDF, etiquetas, transcripciones por página,
pares de duplicados y la cadena de versiones (renovaciones). También se encarga de la búsqueda de
texto completo (FTS5), las migraciones de esquema, los respaldos, la exportación e importación de
paquetes `.zip` y CSV, y las reglas para clasificar con etiquetas y con IA. No hace llamadas de red: la IA
y la lectura de PDF viven en otros paquetes, y este solo guarda sus resultados. Depende de
`internal/pdfutil` (`SavedFile`, `MaxBytes`) y `internal/municipios` (`Oficial`).

## Interface

### Apertura, rutas y esquema
- `type Paths struct{Base, DB, PDFs, Backups, Config string}` y `DefaultPaths() (Paths, error)`: rutas bajo `os.UserConfigDir()/ConveniosUAGro` o bajo `CONVENIOS_DATA_DIR` (`convenios.db`, `pdfs/`, `respaldos/`, `settings.json`) [store.go:Paths] [store.go:DefaultPaths]
- `type Store` (campos no exportados `db`, `paths`, `dupMu`) y `Open(paths Paths) (*Store, error)`: abre o crea la base, aplica PRAGMAs, el esquema, las migraciones, el índice FTS y las etiquetas base [store.go:Store] [store.go:Open]
- `(*Store) Close() error`, `Paths() Paths`, `DB() *sql.DB`, `String() string` [store.go:Close] [store.go:Paths] [store.go:DB] [conventions.go:String]
- `(*Store) SeedTags() error`: crea las etiquetas base protegidas [store.go:SeedTags]
- `Acronym(text string) string`: iniciales para la UI (hasta 3) o `"PDF"` si el texto viene vacío [store.go:Acronym]
- Constantes `ColorAmbito`, `ColorEstado`, `ColorCiudad`, `ColorFacultad`, `ColorTema`, `ColorAbierto`, `TagConvenioAbierto = "Convenio Abierto"` y `TagNacional = "Nacional"` [store.go:ColorAmbito] [store.go:TagConvenioAbierto] [store.go:TagNacional]
- DDL embebido (`//go:embed schema.sql search.sql`): tablas `convenios`, `etiquetas`, `convenio_etiquetas`, `convenio_duplicados`, `duplicados_descartados`, `meta`, `transcripcion_paginas`; tabla virtual `convenios_fts` (fts5, `unicode61 remove_diacritics 2`) con los disparadores `convenios_fts_ai/ad/au`, `convenios_busqueda_au`, `convenio_etiquetas_busqueda_ai/ad` y `etiquetas_busqueda_au`; índices `ix_convenios_sha`, `ix_convenios_fechas`, `ux_etiquetas_tipo_nombre`, `ix_convenio_etiquetas_etiqueta`, `ix_convenio_duplicados_b`, `ix_convenios_reemplazado` e `ix_convenios_archivado` [store.go:schemaFS] [schema.sql:convenios] [search.sql:convenios_fts]

### Tipos de datos y utilidades
- Tipos con etiquetas JSON para la UI: `Tag`, `Suggestion`, `ConventionSummary`, `TranscriptionStatus`, `ConventionDetail`, `RelatedRef`, `VersionRef`, `DuplicatePair`, `ListFilter`, `ListResult`, `Stats`, `ConventionInput`, `TagInput` y `AppInfo` [types.go:Tag] [types.go:ConventionDetail] [types.go:ListFilter] [types.go:AppInfo]
- `Today() string` (fecha local `2006-01-02`) e `IsDate(s string) bool` [types.go:Today] [types.go:IsDate]

### Convenios (CRUD, listado y búsqueda)
- `Insert(f pdfutil.SavedFile) (int64, error)`, `DuplicateID(sha string, exclude int64) (int64, bool, error)` [conventions.go:Insert] [conventions.go:DuplicateID]
- `List(f ListFilter) (ListResult, error)`: búsqueda FTS, filtros por etiqueta y vigencia, orden y paginación; si no hay resultados con todas las palabras, repite la búsqueda con cualquiera de ellas y marca `Aproximado` [conventions.go:List]
- `Get(id) (ConventionDetail, error)`, `Update(ConventionInput) error`, `DeleteMany(ids) ([]string, error)`, `PDF(id) (path, original string, err error)`, `Stats() (Stats, error)` [conventions.go:Get] [conventions.go:Update] [conventions.go:DeleteMany] [conventions.go:PDF] [conventions.go:Stats]
- `EstadoConvenio(fecha string, indefinida bool) string`: devuelve `indefinida`, `sin_fecha`, `vencido`, `por_vencer` o `vigente` [conventions.go:EstadoConvenio]
- Resultados de la IA: `SetFacultyCity`, `SetIAStatus`, `SetAIResult`, `SetReviewed`, `TextForAI` y `ExistingTags` [conventions.go:SetAIResult] [conventions.go:SetIAStatus] [conventions.go:TextForAI]
- `SetArchived(ids []int64, archived bool) (int, error)`: archiva o restaura [archive.go:SetArchived]
- `FilterIDs(f ListFilter) ([]int64, error)`: todos los ids que cumplen un filtro [csv.go:FilterIDs]

### Etiquetas
- `NormalizeKey(s string) string`: clave sin acentos y en minúsculas [tags.go:NormalizeKey]
- `ListTags`, `CreateTag(TagInput)`, `EnsureTag(tipo, nombre)`, `UpdateTag(TagInput)`, `DeleteTag(id)`, `DeleteTagIfUnused(id)`, `TagByID(id)`, `TagsForConvention(id)` y `TagsForIDs(ids)` [tags.go:CreateTag] [tags.go:UpdateTag] [tags.go:DeleteTag] [tags.go:TagsForIDs]
- `ReplaceTags(convenioID, ids, markReviewed)` (modo estricto) y `ReplaceTagsAuto(convenioID, ids)` (corrige los conflictos en vez de rechazarlos); `type TagConflictError` [tags.go:ReplaceTags] [tags.go:ReplaceTagsAuto] [tags.go:TagConflictError]
- Sugerencias de la IA: `SetSuggestions`, `ParseSuggestions(raw) []Suggestion`, `AcceptSuggestions`, `MarkTagsReviewed` y `DiscardSuggestions(id, remove)` [tags.go:AcceptSuggestions] [tags.go:ParseSuggestions] [tags.go:DiscardSuggestions]
- `UnifyCityNames() (int, error)`: pasa las ciudades al nombre oficial del municipio [cities.go:UnifyCityNames]
- Facultad: `HasFacultyTag`, `MarkOpenAgreement`, `UnmarkOpenAgreement` y `OpenAgreementCandidates` [faculty.go:MarkOpenAgreement] [faculty.go:OpenAgreementCandidates]
- `EnsureAmbito(id)`, `BackfillCityStates()` [review.go:EnsureAmbito] [review.go:BackfillCityStates]

### Transcripción y correcciones
- `BeginTranscription(id, total)`, `SaveTranscriptionPage(id, page, texto) (TranscriptionStatus, error)`, `FailTranscription`, `FinishTranscription` y `GetTranscription` [transcription.go:BeginTranscription] [transcription.go:SaveTranscriptionPage] [transcription.go:FinishTranscription] [transcription.go:GetTranscription]
- `type NameCorrection`, `HasTranscription`, `TranscriptionPage`, `ApplyNameCorrections`, `ApplyPageCorrections` y `CloneAnalysis(src, dst) (bool, error)` [analysis.go:NameCorrection] [analysis.go:ApplyNameCorrections] [analysis.go:ApplyPageCorrections] [analysis.go:CloneAnalysis]
- Reintentos: `MsgAnalisisInterrumpido`, `MaxRetryRounds = 3`, `RetryableError(msg) bool`, `ClearRetries(id)`, `RetryCandidates() ([]int64, error)` y `ResumeTranscription(id, total) ([]int, error)` [retry.go:MaxRetryRounds] [retry.go:RetryableError] [retry.go:RetryCandidates] [retry.go:ResumeTranscription]

### Duplicados y versiones
- Constantes `MotivoArchivo`, `MotivoTexto`, `MotivoDatos` y `MotivoRenovacion`; `RefreshDuplicates()`, `DismissDuplicate(a, b)` y `ListDuplicates() ([]DuplicatePair, error)` [duplicates.go:MotivoArchivo] [duplicates.go:RefreshDuplicates] [duplicates.go:DismissDuplicate] [duplicates.go:ListDuplicates]
- `LinkVersion(older, newer)`, `UnlinkVersion(id)` y `History(id) ([]VersionRef, int64, error)` [versions.go:LinkVersion] [versions.go:UnlinkVersion] [versions.go:History]

### Cola de procesamiento
- `cola_procesamiento` (id autoincremental, `convenio_id` único con `ON DELETE CASCADE`) guarda los convenios pendientes de transcribir y analizar; el orden es el del `id`. Sin subir `user_version` [schema.sql:cola_procesamiento]
- `EnqueueForAI(ids)`, `Dequeue(id)`, `QueuedIDs()`, `QueueCount()`, `PruneQueue()` y `ClearQueue()` [queue.go:EnqueueForAI] [queue.go:QueuedIDs]
- `EnqueueForAI` inserta con `INSERT OR IGNORE … SELECT id FROM convenios`: un id repetido no se duplica y uno inexistente no falla. `PruneQueue` borra las filas de convenios ya resueltos (`ia_estado` `ok` o `parcial`) o archivados, y conserva los que quedaron con error para que el usuario decida [queue.go:PruneQueue]

### Fechas por confirmar
- `RefreshDateChecks()` vuelve a comprobar con `fechas.Check` los convenios cuyo `dateCheckKey` cambió (actualizado_en, fechas, indefinida, calculada y largo del texto y de la cláusula). `rebuildText` borra su fila de `revision_fechas_hechas` para forzarlo. `DateReviews(id)`, `HasPendingFirma(id)`, `ConfirmDate(id, campo)`; `ListFilter.SoloFechasPorConfirmar`, `Stats.FechasPorConfirmar`, `ConventionSummary.FechasPorConfirmar` y `ConventionDetail.RevisionFechas` [date_checks.go:RefreshDateChecks] [date_checks.go:ConfirmDate] [transcription.go:rebuildText]
- Una fila de `revision_fechas` existe solo si la fecha no coincide con el texto. Está por confirmar mientras `confirmada` sea NULL o distinta de `valor`, así que la confirmación del usuario se pierde si la fecha cambia. Las dos tablas se crean con `CREATE TABLE IF NOT EXISTS` sin subir el esquema: una versión anterior de la app las ignora [schema.sql:revision_fechas] [date_checks.go:checkDates]

### Revisión y mantenimiento
- `Flag(name)` y `SetFlag(name)`: marcas en `meta` para las reparaciones que se aplican una sola vez [review.go:Flag] [review.go:SetFlag]
- `SetNoConvenio`, `SetAIConvenioFlag`, `IsFlaggedNoConvenio`, `FlagNoConveniosFromNotes` y `AnalyzedConventions` [review.go:SetNoConvenio] [review.go:SetAIConvenioFlag] [review.go:FlagNoConveniosFromNotes] [review.go:AnalyzedConventions]

### Respaldos, exportación e importación
- `BackupZip(dst)` (base + PDF + `LEEME.txt`) y `SnapshotDB(dst)` (solo la base, con `VACUUM INTO`) [backup.go:BackupZip] [backup.go:SnapshotDB]
- `ExportCSV(dst, ids) (int, error)` [csv.go:ExportCSV]
- `PackageFormat = "convenios-uagro"`, `PackageTag`, `PackagePage`, `PackageRecord`, `ImportSummary`; `ExportPackage(dst, ids, appVersion) (int, error)` e `ImportPackage(src) (ImportSummary, error)` (acepta un paquete `.zip`, un respaldo `.zip` con `convenios.db` o un `.db` suelto) [package.go:PackageFormat] [package.go:ExportPackage] [package.go:ImportPackage]
- Nombres de descarga: `TiposDocumento`, `NormalizeTipoDocumento`, `FileName(ConventionSummary)` y `UniqueFileName(name, used)` [filename.go:TiposDocumento] [filename.go:NormalizeTipoDocumento] [filename.go:FileName] [filename.go:UniqueFileName]

## Invariants

### Conexión y esquema
- Una sola conexión (`SetMaxOpenConns(1)`); PRAGMAs `journal_mode=WAL`, `foreign_keys=ON` y `busy_timeout=5000`. Por eso las etiquetas se resuelven **antes** de abrir una transacción: dentro de ella no se puede usar otra consulta del `Store` [store.go:Open] [package.go:insertRecord]
- `schemaVersion = 7`: si `user_version` es mayor, `Open` falla. Una base nueva se marca directo como v7; una vieja pasa por la cadena `migrateV2`…`migrateV7` en orden (con v6 se corre antes `repairV6`). Cada migración usa su propia transacción y fija `user_version` dentro de ella [store.go:schemaVersion] [store.go:Open] [store.go:migrateV4] [store.go:repairV6]
- `search.sql` se ejecuta después de las migraciones porque depende de la columna `busqueda`; `busquedaExpr` tiene que coincidir con la expresión de los disparadores de `search.sql` [store.go:busquedaExpr] [search.sql:convenios_busqueda_au]
- `ensureSearchIndex` reconstruye `convenios_fts` solo si su conteo no coincide con el de `convenios` [store.go:ensureSearchIndex]
- Al abrir, `transcripcion_estado='en_proceso'` pasa a `pendiente` e `ia_estado='pendiente'` pasa a `error` con `MsgAnalisisInterrumpido` [store.go:Open]
- Restricciones SQL: `archivo_guardado UNIQUE`; `(tipo, nombre)` único en `etiquetas`, con `nombre COLLATE NOCASE`; `tipo IN ('ambito','estado','ciudad','tema','facultad')`; `ia_estado` y `transcripcion_estado` limitados por CHECK; los pares de duplicados se guardan con `convenio_a < convenio_b` [schema.sql:convenios] [schema.sql:etiquetas] [schema.sql:convenio_duplicados]

### Validación de datos
- `IsDate`: la cadena vacía es válida; si no, formato `2006-01-02` con año entre 1950 y 2100. `Update` y `SetAIResult` rechazan fechas inválidas [types.go:IsDate] [conventions.go:Update] [conventions.go:SetAIResult]
- Nombre de etiqueta `^[^\x00-\x1f\x7f]{1,80}$`; color `^#[0-9A-Fa-f]{6}$`, o `colorForType` si viene vacío [store.go:validateTagName] [store.go:validateColor] [tags.go:CreateTag]
- `PDF` solo acepta nombres `^[0-9a-f]{32}\.pdf$` y rutas que queden directamente dentro de `paths.PDFs` (evita path traversal) [conventions.go:reSavedName] [conventions.go:PDF]
- Límites de lote: `DeleteMany` toma como máximo 200 ids distintos e ignora el resto sin dar error; `SetArchived` descarta ids ≤ 0 y repetidos, y falla si la lista queda vacía o pasa de 500 [conventions.go:maxBulkDelete] [archive.go:maxBulkArchive]

### Listado
- `pageSize = 40`; la página se acota entre 1 y el total. Por defecto se excluyen los archivados y las versiones anteriores. Entre tipos de etiqueta el filtro es AND y dentro de un mismo tipo es OR [conventions.go:pageSize] [conventions.go:list] [conventions.go:tagGroups]
- Búsqueda: una palabra suelta se busca como prefijo y el texto entre comillas como frase; se descartan stopwords (salvo que solo haya stopwords). Límites: 20 fragmentos, 12 palabras por frase y 10 términos [conventions.go:searchTerms] [conventions.go:stopwords]
- El orden solo admite `dependencia`, `fecha_firma`, `fecha_vigencia`, `archivado_en` e `id`; por defecto `c.id DESC` [conventions.go:orderSQL]
- `EstadoConvenio`: `por_vencer` significa que vence en 90 días o menos [conventions.go:EstadoConvenio]

### Etiquetas
- Las etiquetas protegidas (las de `SeedTags`: ámbitos, los 32 estados y `Convenio Abierto`) no se pueden editar ni borrar; `CreateTag` solo crea `ciudad`, `tema` o `facultad`, y siempre con `protegida=0` [store.go:SeedTags] [tags.go:UpdateTag] [tags.go:DeleteTag] [tags.go:CreateTag]
- Un convenio lleva como máximo una etiqueta de cada tipo `ambito`, `estado`, `ciudad` y `facultad` (gana la primera en el orden pedido); `tema` no tiene límite [tags.go:consistentTags]
- Reglas de ubicación: una ciudad implica el estado de su ciudad y un estado implica el ámbito `Nacional`. Si hay conflicto, `ReplaceTags` devuelve `TagConflictError` y `ReplaceTagsAuto` corrige la combinación [tags.go:consistentTags] [tags.go:TagConflictError]
- El estado de una ciudad debe ser uno de los 32 de `estadosMx`; nombre y estado pasan por `officialCity` (catálogo de `internal/municipios`) [tags.go:estadoCanonico] [cities.go:officialCity]
- `ParseSuggestions` solo conserva tipos válidos con nombre no vacío y recorta el nombre a 80 runas; `AcceptSuggestions` crea ciudades y facultades, pero `ambito`, `estado` y `tema` solo se aceptan si la etiqueta ya existe [tags.go:ParseSuggestions] [tags.go:AcceptSuggestions]
- `no_es_convenio` admite 0 (normal), 1 (dudoso según la IA) y 2 (confirmado por el usuario); `SetAIConvenioFlag` nunca pisa el 2 [review.go:convenioNormal] [review.go:SetAIConvenioFlag]

### Transcripción, reintentos y análisis
- `BeginTranscription` rechaza `total < 0`; con `total == 0` el estado queda `completa`. `SaveTranscriptionPage` exige `page >= 1` y texto no vacío, y marca `completa` cuando las páginas guardadas llegan al total [transcription.go:BeginTranscription] [transcription.go:SaveTranscriptionPage]
- `rebuildText` arma `texto` como `texto_pdf` + bloques `[Página N]` y pone `huella=NULL` para que se recalcule [transcription.go:rebuildText]
- `RetryCandidates` excluye los archivados, los `permanentErrors` y los convenios con `MaxRetryRounds` rondas o más (contador en `meta`, clave `reintento:<id>`); `SetAIResult` reinicia el contador [retry.go:RetryCandidates] [retry.go:permanentErrors] [retry.go:retryKey] [conventions.go:SetAIResult]
- Las correcciones de nombres nunca introducen un nombre nuevo: piden al menos 2 palabras, similitud ≥ `minNameSimilarity` (0.6) y que la forma correcta ya aparezca en el texto [analysis.go:minNameSimilarity] [analysis.go:ApplyNameCorrections]
- `CloneAnalysis` solo copia si el origen tiene `ia_estado` `ok` o `parcial`, o una transcripción `completa` [analysis.go:CloneAnalysis]

### Duplicados y versiones
- Huella SimHash de 64 bits que exige al menos 80 palabras; una huella calculada nunca vale 0. Se considera texto similar con una distancia de Hamming ≤ 8 [duplicates.go:fingerprint] [duplicates.go:fingerprintWords] [duplicates.go:fingerprintMaxDist]
- `RefreshDuplicates` corre bajo `dupMu`; omite los pares de una misma familia de versiones y los descartados; una renovación solo se busca entre versiones vigentes [duplicates.go:RefreshDuplicates]
- Cadena de versiones: la versión anterior apunta con `reemplazado_por` a la que la sustituye y la vigente lo tiene en NULL. `LinkVersion` rechaza `older == newer` y los ciclos; los recorridos se cortan en `maxVersionDepth = 1000` [versions.go:LinkVersion] [versions.go:maxVersionDepth]

### Exportación e importación
- `ImportPackage` acepta solo `.zip` o `.db` y no lee `manifest.json`: distingue el tipo por la presencia de `convenios.json` o `convenios.db` [package.go:ImportPackage]
- Límites: `convenios.json` hasta 512 MiB, `convenios.db` extraído hasta 4 GiB, cada PDF hasta `pdfutil.MaxBytes` (100 MB) y con firma `%PDF-`; las rutas de PDF deben cumplir `^pdfs/[0-9a-f]{32}\.pdf$`; se guardan como máximo 50 errores [package.go:ImportPackage] [package.go:extractZipFile] [package.go:copyImportedPDF] [package.go:rePackagePDF] [package.go:maxImportErrs]
- Se deduplica por SHA-256 (el declarado y el recalculado). El PDF importado recibe un nombre aleatorio nuevo y los estados inválidos o transitorios se normalizan [package.go:importRecords] [package.go:insertRecord]
- `importFromDB` rechaza importar la base que está en uso y trabaja sobre una copia [package.go:importFromDB]
- `ExportCSV` antepone `'` a las celdas que empiezan con `= + - @` (evita fórmulas inyectadas) y escribe BOM UTF-8 [csv.go:csvSafe] [csv.go:ExportCSV]
- `UniqueFileName` compara sin distinguir mayúsculas (como Windows); `FileName` limita el nombre base a 150 runas [filename.go:UniqueFileName] [filename.go:FileName]

## Side Effects
- Lee `CONVENIOS_DATA_DIR` y `os.UserConfigDir()`; crea `Base`, `pdfs/` y `respaldos/` con permisos 0700 [store.go:DefaultPaths]
- Abre o crea el archivo SQLite con WAL (genera `-wal` y `-shm`); `Open` ejecuta DDL, migraciones, reconstrucción del FTS, `INSERT OR IGNORE` de etiquetas base y los UPDATE de recuperación [store.go:Open] [store.go:SeedTags]
- Los disparadores de `search.sql` reescriben `convenios_fts` y `convenios.busqueda` en cada cambio a `convenios`, `convenio_etiquetas` o al nombre de una etiqueta [search.sql:convenios_fts_au] [search.sql:convenio_etiquetas_busqueda_ai]
- Borrado en cascada por FK (`convenio_etiquetas`, duplicados, `transcripcion_paginas`) y `reemplazado_por` a NULL al borrar un convenio [schema.sql:convenio_etiquetas] [schema.sql:convenios]
- `DeleteMany` **no** borra los PDF del disco: devuelve sus nombres para que los borre quien llama [conventions.go:DeleteMany]
- Escrituras SQLite en `convenios`, `etiquetas`, `convenio_etiquetas`, `transcripcion_paginas`, `convenio_duplicados`, `duplicados_descartados` y `meta` a lo largo de la API mutadora; casi todas tocan `actualizado_en` [conventions.go:Update] [tags.go:writeTags] [transcription.go:SaveTranscriptionPage] [duplicates.go:RefreshDuplicates] [review.go:SetFlag]
- Escrituras fuera de una transacción común: `consistentTags`/`findCity` pueden fijar `etiquetas.estado` incluso durante una validación; `applyCityState`, `AcceptSuggestions` e `insertRecord` pueden crear etiquetas que se quedan aunque el paso siguiente falle [tags.go:consistentTags] [tags.go:applyCityState] [tags.go:AcceptSuggestions] [package.go:insertRecord]
- `LinkVersion` y `UnlinkVersion` llaman a `RefreshDuplicates`, que toma `dupMu`, rellena `huella` y reescribe completa `convenio_duplicados` [versions.go:LinkVersion] [versions.go:UnlinkVersion] [duplicates.go:fillFingerprints]
- `BackupZip`: un `VACUUM INTO` a `respaldos/.backup-<nanos>.db` (se borra al final), un zip escrito en `dst.tmp` y renombrado a `dst` [backup.go:BackupZip]
- `SnapshotDB` borra el `dst` existente y escribe la copia con `VACUUM INTO` [backup.go:SnapshotDB]
- `ExportPackage` y `ExportCSV` escriben en `dst.tmp` y lo renombran a `dst` (el temporal se borra si algo falla) [package.go:ExportPackage] [csv.go:ExportCSV]
- `ImportPackage` crea carpetas temporales (`convenios-import-*`, `convenios-db-*`) que se borran al terminar; escribe `.import-*.pdf` en `pdfs/`, lo renombra a `<hex>.pdf` y lo borra si resulta duplicado o si la inserción falla; `importHistory` llama a `LinkVersion` e ignora sus errores [package.go:ImportPackage] [package.go:importFromDB] [package.go:copyImportedPDF] [package.go:importHistory]
- Sin red, sin goroutines propias; el único candado explícito es `dupMu` [store.go:Store] [duplicates.go:RefreshDuplicates]

## Verification
- Run: `go test ./internal/store/...` (desde la raíz del repo)
