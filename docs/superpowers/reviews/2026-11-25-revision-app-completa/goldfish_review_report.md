# Goldfish Annotated Review Report

Status: resolved (2026-11-25). Se aplicaron las 31 correcciones (auto accept), se añadieron al documento los datos de E1-E3 (unos 120 convenios hoy y la meta de 1000, medido 28 ms por recálculo con 1000; solo el usuario sube a main mediante Claude Code; una sola PC) y los defectos D1-D11 se corrigieron en la rama `version-1.3.1` (commit e5f6382). Las mejoras M1-M9 quedan pendientes.

**Target:** `EGM.md` (full file, líneas 1-73) — ronda 1

## Reviewer Status

| Reviewer | Estándar | Status |
| --- | --- | --- |
| `design_critic` (dc) | re-implementation-standard | completed (13 Major, 5 Nit) |
| `codebase_details` (cd) | re-implementation-standard | completed (7 Blocker, 13 Major, 8 Nit) |
| `zoom_out` (zo) | zoom-out-standard | completed (6 Major, 8 Nit) |

59 hallazgos en bruto; tras juntar los repetidos entre revisores quedan 46. Resumen de la clasificación: 31 PROPOSE FIX (corregir el documento), 1 CONTEST, 3 ESCALATE (necesitan un dato tuyo), 11 DECLINE (menores o solo de código).

Las correcciones de esta ronda solo tocan `EGM.md`. Los fallos del código que salieron y confirmé están juntos al final, en **Defectos del código confirmados**, para decidir aparte si van en una versión nueva.

## Hallazgos que necesitan tu conocimiento (ESCALATE)

### [E1] [`design_critic`] ¿Cuántos convenios se espera tener? - Severity: Major
*   **Affected Scope:** [Líneas 60-68](EGM.md#L60-L68) (Global Invariants / Side Effects); `internal/store/duplicates.go:236-281`
*   **Discrepancy Description:** `RefreshDuplicates` compara todos los pares (O(n²)), reescribe la tabla completa y corre de forma síncrona tras casi cada cambio, sobre una sola conexión SQLite. El documento no da volumen esperado ni costo.
*   **Rationale:** Con unos cientos de convenios es imperceptible; con miles, cada guardado puede tardar.
*   ***AI Suggestion:*** ESCALATE
*   ***Justification:*** El volumen real (cuántos convenios hay y cuántos se esperan) no está en ningún archivo.
*   ***Proposed Edit:*** Agregar a Global Invariants una línea con el volumen soportado cuando me digas la cifra.

### [E2] [`design_critic`] ¿Quién puede subir a `main`? - Severity: Major
*   **Affected Scope:** [Línea 58](EGM.md#L58); `.github/workflows/release.yml`; `internal/update/update.go:288`
*   **Discrepancy Description:** Cualquier push a `main` termina instalado en las PC sin firma de código. El SHA-256 viaja en el mismo release que el binario (detecta corrupción, no manipulación) y, si falta, la verificación se omite.
*   **Rationale:** La seguridad de todas las instalaciones depende de quién tiene acceso de escritura al repositorio.
*   ***AI Suggestion:*** ESCALATE
*   ***Justification:*** Quién tiene acceso (solo tú, la deploy key de este equipo, alguien más) y si hay protección de rama no se puede leer en el repositorio.
*   ***Proposed Edit:*** Documentar el modelo de confianza con esos datos.

### [E3] [`design_critic`] ¿Se usa en varias PC y cómo se sincronizan? - Severity: Major
*   **Affected Scope:** [Líneas 4, 58](EGM.md#L4); `internal/store/package.go:339-394`
*   **Discrepancy Description:** El documento dice «las PC», pero cada PC tiene su propia base. Importar un paquete solo agrega convenios nuevos (por SHA-256): ediciones, etiquetas, archivados y borrados de otra PC nunca se propagan.
*   **Rationale:** Si varias personas trabajan en PC distintas, las bases divergen sin aviso.
*   ***AI Suggestion:*** ESCALATE
*   ***Justification:*** Cómo se usa realmente la app (una PC central o varias que intercambian paquetes) no está en el código.
*   ***Proposed Edit:*** Documentar el modelo de uso y el límite de la importación.

## Prioritized Findings & AI Suggestions

### [1] [`codebase_details`] La transcripción la hace Go, no la interfaz - Severity: Blocker
*   **Affected Scope:** [Línea 6](EGM.md#L6) (System Overview); `app.go:995-1013`
*   **Discrepancy Description:** El documento dice que la interfaz transcribe y Go solo guarda. En realidad la interfaz solo dibuja la página y envía el JPEG; `TranscribePage` llama al modelo de visión desde Go.
*   **Rationale:** Describe al revés quién habla con la IA.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado: `TranscribePage` crea `ai.New(...)` y llama a `client.TranscribeImage`.
*   ***Proposed Edit:*** Línea 6 → «La interfaz dibuja cada página escaneada y la envía como JPEG; Go la manda al modelo de visión (`TranscribePage`) y guarda el texto. La verificación de dudas (`VerifyAI`) funciona igual».

### [2] [`codebase_details`] Archivados y versiones anteriores se excluyen solo por omisión - Severity: Blocker
*   **Affected Scope:** [Línea 61](EGM.md#L61); `internal/store/conventions.go:240-256`; `frontend/dist/app.js` (casilla «Incluir versiones anteriores»)
*   **Discrepancy Description:** `IncluirAnteriores` muestra las versiones anteriores y `Archivados` cambia a la vista de archivados. Además el contador «No son convenios» sí cuenta versiones anteriores, mientras que su filtro las excluye.
*   **Rationale:** «Nunca aparecen» es falso; hay banderas para verlos.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado en `List` y en la consulta de `Stats`.
*   ***Proposed Edit:*** Línea 61 → «Por omisión, la lista principal, su buscador y sus contadores excluyen los archivados y las versiones anteriores; `IncluirAnteriores` y `Archivados` los muestran». El contador va a Defectos (D10).

### [3] [`codebase_details`, `design_critic`] Tiempos de la actualización mal dados - Severity: Blocker
*   **Affected Scope:** [Línea 44](EGM.md#L44); `internal/update/update.go:68, 238`; `app.go:1230, 1279`
*   **Discrepancy Description:** La descarga tiene un cliente HTTP de 15 min dentro de un contexto de 20 min; la comprobación hace 2 peticiones de 30 s dentro de 40 s.
*   **Rationale:** La cifra documentada no es la que corta.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Verificable en las líneas citadas.
*   ***Proposed Edit:*** «la comprobación, 2 peticiones de 30 s en un máximo de 40 s; la descarga, 15 min por petición en un máximo de 20 min».

### [4] [`codebase_details`, `design_critic`] La descripción de la clave de API es incorrecta - Severity: Blocker
*   **Affected Scope:** [Línea 62](EGM.md#L62); [diagrama, línea 22](EGM.md#L22); `internal/config/config.go:145, 189, 205-214`
*   **Discrepancy Description:** La interfaz sí recibe los últimos 4 caracteres (`APIKeyHint`). En Windows los permisos 0600 no aplican ACL: protege la ACL del perfil en `%APPDATA%`. `BaseURL` acepta `http://`, y al cambiarla la clave guardada se envía al nuevo servidor.
*   **Rationale:** Tres afirmaciones falsas sobre un dato sensible.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado en `ToView` y `Apply`.
*   ***Proposed Edit:*** Reescribir la línea 62 con estos tres hechos y cambiar «HTTPS» por «HTTP(S)» en el diagrama. Exigir `https://` va a Defectos como mejora.

### [5] [`codebase_details`] La regla de facultad no siempre se cumple - Severity: Blocker
*   **Affected Scope:** [Línea 47](EGM.md#L47); `app.go:244-248`; `internal/store/faculty.go:27-33`
*   **Discrepancy Description:** Si el campo Facultad cambia de una facultad reconocida a un texto no reconocido (p. ej. «Rectoría»), se conserva la etiqueta vieja y no queda «Convenio Abierto»: `MarkOpenAgreement` no hace nada si ya hay una etiqueta de facultad. Con sede vacía, la etiqueta es solo «<nombre>». Todas las llamadas descartan el error.
*   **Rationale:** El documento lo presenta como garantía.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado leyendo `SaveConvenio` y `MarkOpenAgreement`.
*   ***Proposed Edit:*** Reescribir la línea 47 con los casos reales. El caso «Rectoría» va a Defectos (D1).

### [6] [`codebase_details`] Faltan métodos en los grupos de la interfaz - Severity: Blocker
*   **Affected Scope:** [Línea 34](EGM.md#L34)
*   **Discrepancy Description:** Faltan `DeleteConvenios`, `SetNoConvenio`, `GetTranscription`, `DownloadTranscription` y `OpenDataFolder`, y los tipos de respuesta (`Bootstrap`, `ImportResult`, `ImportCheck`, `ExportResult`, `PageImage`).
*   **Rationale:** El grupo debe cubrir los 49 métodos.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Faltan de verdad.
*   ***Proposed Edit:*** Añadirlos al grupo correspondiente y listar los tipos con sus campos.

### [7] [`codebase_details`] Comportamiento detallado de cada método (tabla de firmas) - Severity: Blocker
*   **Affected Scope:** [Líneas 30-38](EGM.md#L30-L38)
*   **Discrepancy Description:** Pide una tabla con firma, entradas, salidas, errores literales y efectos de cada uno de los 49 métodos, y el detalle de `extract`, `linkRenewal`, `ImportPDFs`, etc.
*   **Rationale:** El estándar de reimplementación lo exige.
*   ***AI Suggestion:*** CONTEST
*   ***Justification:*** El documento de la raíz sigue la plantilla «Repository Root Blueprint» de `egm-bootstrap` (`skills/egm-bootstrap/references/templates.md`), cuya sección «Local Interface & Invariants» pide símbolos, invariantes y efectos, no una especificación por método. El revisor `zoom_out` (hallazgo 36) señala lo contrario: que la raíz ya es demasiado detallada. El estándar de reimplementación lo elegí yo para esta ronda; aplicado al pie de la letra convertiría el resumen en una copia de `app.go`. Los casos con consecuencias para el usuario (`ImportConvenios`, `SetNoConvenio`, `DeleteConvenios`) sí se agregan en los hallazgos 8, 11 y 18.
*   ***Proposed Edit:*** Ninguno.

### [8] [`codebase_details`, `zoom_out`] Secuencia de arranque incompleta - Severity: Major
*   **Affected Scope:** [Línea 66](EGM.md#L66); `app.go:117-133`; `internal/store/store.go:186-201`
*   **Discrepancy Description:** Falta `markOpenAgreements`. `repairRequiredTags` corre una sola vez en la vida de la base (bandera), no en cada arranque. Faltan el recálculo de duplicados en segundo plano, la limpieza del `.old`, la carga de etiquetas base y los reinicios de estados de `store.Open`. `ImportConvenios` corre un subconjunto distinto.
*   **Rationale:** El documento llama «idempotentes en cada arranque» a cosas que no lo son.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado en `startup`.
*   ***Proposed Edit:*** Listar la secuencia completa y en orden. La propuesta de convertir las reparaciones en migraciones va a Defectos como mejora (M1).

### [9] [`codebase_details`, `design_critic`] Un `settings.json` dañado se sobrescribe - Severity: Major
*   **Affected Scope:** [Líneas 46, 65](EGM.md#L46); `app.go:108-113`
*   **Discrepancy Description:** Si `config.Load` falla, `NewApp` guarda los valores iniciales encima: se pierden la clave y todos los ajustes sin avisar.
*   **Rationale:** Contradice la idea de que la configuración solo cambia si se guardó bien.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado.
*   ***Proposed Edit:*** Agregarlo a Global Side Effects. Corregirlo va a Defectos (D3).

### [10] [`codebase_details`, `design_critic`] «Nunca sobrescribe» es demasiado fuerte - Severity: Major
*   **Affected Scope:** [Línea 48](EGM.md#L48); `app.go:427-446, 1072, 1097, 1147, 1181`; `internal/pdfutil/pdf.go:192-201`
*   **Discrepancy Description:** `DownloadPDFs` solo evita los nombres que existían al empezar. Y en las demás descargas la extensión se agrega después del diálogo: si escribes «x», se sobrescribe «x.zip» sin confirmación.
*   **Rationale:** El invariante tiene excepciones reales.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado (`dst += ".zip"` tras el diálogo).
*   ***Proposed Edit:*** Matizar la línea 48. La extensión forzada va a Defectos (D7).

### [11] [`codebase_details`, `design_critic`] Borrado definitivo y topes silenciosos - Severity: Major
*   **Affected Scope:** [Líneas 40-41](EGM.md#L40-L41); `internal/store/conventions.go:18, 556-568`; `app.go:362-377, 433-455`
*   **Discrepancy Description:** `DeleteConvenios` borra filas y PDF sin papelera, recorta en silencio a 200 ids y puede dejar PDF huérfanos. `DownloadPDFs` informa éxito aunque fallen algunos archivos.
*   **Rationale:** La única operación destructiva no aparece en el documento.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado. Hoy la selección es por página (40), así que el tope de 200 no se alcanza desde la interfaz.
*   ***Proposed Edit:*** Agregar a Invariants: borrado irreversible, tope de 200, posibles PDF huérfanos, descarga con fallas parciales.

### [12] [`codebase_details`] Cancelar un diálogo no es un error - Severity: Major
*   **Affected Scope:** [Línea 42](EGM.md#L42); `app.go:400-426, 1068-1179`
*   **Discrepancy Description:** Al cancelar se devuelve un valor vacío con `err == nil`; la interfaz tiene que distinguirlo.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado.
*   ***Proposed Edit:*** Añadir a la línea 42.

### [13] [`codebase_details`] `OpenReleasePage` reemplaza, no rechaza - Severity: Major
*   **Affected Scope:** [Línea 50](EGM.md#L50); `app.go:1312-1319`
*   **Discrepancy Description:** Una dirección ajena se cambia por la de releases/latest y el método siempre devuelve `nil`.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado.
*   ***Proposed Edit:*** «abre la dirección solo si es del repositorio propio; si no, abre la página de la versión más reciente».

### [14] [`codebase_details`, `design_critic`] La actualización está mal descrita y no tiene reversión - Severity: Major
*   **Affected Scope:** [Líneas 49, 58-59, 68](EGM.md#L49); `app.go:1244-1309`; `internal/update/update.go:229-325`
*   **Discrepancy Description:** El orden real: copia de la base → descarga → validación (tamaño, SHA-256 de las notas o del `digest`, cabecera MZ) → exe→`.old` y `.new`→exe con reversión si falla → reinicio con `--after-update` y cierre a los 300 ms. Las copias `antes-de-actualizar-*.db` nunca se borran. El `.old` se borra al arrancar la versión nueva, así que no hay forma automática de volver atrás.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado en las líneas citadas.
*   ***Proposed Edit:*** Reescribir la línea 68 con el orden real y una línea sobre la falta de reversión. Las mejoras (exigir el hash, conservar binarios) van a Defectos (M3).

### [15] [`codebase_details`, `design_critic`, `zoom_out`] Publicación: cuándo una versión cuenta como nueva y la carrera al publicar - Severity: Major
*   **Affected Scope:** [Línea 58](EGM.md#L58); `.github/workflows/release.yml:15-17, 73-75`; `internal/update/update.go:101-103, 166-190`
*   **Discrepancy Description:** Con la misma versión, se compara el commit y la fecha. El flujo borra el release y luego lo crea, con `cancel-in-progress: true`: un push que cancele entre los dos pasos deja el repositorio sin release, y la app muestra «Al día» ante un 404.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado en `release.yml`.
*   ***Proposed Edit:*** Documentar las reglas y el modo de fallo. Corregir el flujo va a Defectos (D5).

### [16] [`codebase_details`, `zoom_out`] Las dudas de lectura viven solo en memoria - Severity: Major
*   **Affected Scope:** [Línea 45](EGM.md#L45); `app.go:59-62, 691-753`
*   **Discrepancy Description:** Se pierden al reiniciar. `VerifyAI` las consume antes de revisar que la IA esté activa: con la IA desactivada da error y las dudas desaparecen.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado (`takeDoubts` antes de la revisión de configuración).
*   ***Proposed Edit:*** Añadirlo a la línea 45. El orden va a Defectos (D9).

### [17] [`codebase_details`, `zoom_out`, `design_critic`] Los reintentos dependen del texto de los mensajes - Severity: Major
*   **Affected Scope:** [Línea 43](EGM.md#L43); `internal/store/retry.go:20-45`; `frontend/dist/app.js:1388-1406`
*   **Discrepancy Description:** Qué error se reintenta se decide buscando frases en español de los mensajes, algunos escritos en `app.js`. Cambiar una frase cambia el comportamiento sin que ninguna prueba lo note. Todo error desconocido se reintenta, hasta 3 rondas.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado.
*   ***Proposed Edit:*** Documentar los mensajes como contrato y el tope de 3 rondas. Pasar a errores con tipo va a Defectos (M2).

### [18] [`codebase_details`] La facultad elegida a mano se reemplaza - Severity: Major
*   **Affected Scope:** [Línea 47](EGM.md#L47); `app.go:320-334, 874-896`
*   **Discrepancy Description:** `SaveConvenioTags` guarda la facultad que elige el usuario, pero luego `applyFacultyTag` la cambia por la del campo Facultad, y `DeleteTagIfUnused` puede borrar del catálogo facultades creadas por el usuario.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado.
*   ***Proposed Edit:*** Añadirlo a la regla de facultad.

### [19] [`design_critic`] No hay bloqueo de instancia única - Severity: Major
*   **Affected Scope:** [Líneas 37, 46, 49](EGM.md#L37); `main.go:22-55`; `internal/store/store.go:192-201`
*   **Discrepancy Description:** Abrir una segunda ventana corre `store.Open`, que marca como interrumpidos los análisis y transcripciones en curso de la primera. Los candados de memoria no funcionan entre procesos.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado: `main.go` no usa `SingleInstanceLock`.
*   ***Proposed Edit:*** Documentar el supuesto de una sola instancia. Corregirlo va a Defectos (D4).

### [20] [`design_critic`] La IA sobrescribe las correcciones manuales - Severity: Major
*   **Affected Scope:** [Líneas 43-45](EGM.md#L43-L45); `app.go:534-631`; `internal/store/conventions.go:516-548`
*   **Discrepancy Description:** `SetAIResult` reemplaza todos los campos sin comparar versión. Si el usuario guarda la ficha mientras la IA trabaja, uno de los dos cambios se pierde.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado; la 1.3.0 ya avisa al reanalizar desde la lista, pero no cubre la edición simultánea.
*   ***Proposed Edit:*** Documentarlo. Bloquear la edición durante el análisis va a Defectos (M4).

### [21] [`design_critic`, `codebase_details`] Los fallos de arranque y los logs no se ven - Severity: Major
*   **Affected Scope:** [Línea 59](EGM.md#L59); `main.go:19, 57`
*   **Discrepancy Description:** `log.Fatal` en una app de ventana de Windows: con una base más nueva, una migración fallida o la base bloqueada, la app no abre y no muestra ningún mensaje. Los `log.Printf` también se pierden.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado.
*   ***Proposed Edit:*** Documentar lo que ve el usuario. Mostrar un diálogo o escribir un log va a Defectos (D6).

### [22] [`design_critic`, `codebase_details`] Qué datos salen del equipo - Severity: Major
*   **Affected Scope:** [Líneas 4, 67](EGM.md#L4); `internal/config/config.go:64`; `internal/ai/websearch.go`
*   **Discrepancy Description:** Con la IA activa se envían el texto completo y las imágenes de las páginas (nombres, firmas) a un tercero. La búsqueda web viene activada y manda nombres y domicilios de contrapartes a DuckDuckGo. Los resultados web entran al prompt (riesgo de inyección, mitigado en parte por la validación contra el catálogo).
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Un lector humano necesita saberlo.
*   ***Proposed Edit:*** Agregar a Global Side Effects qué datos salen y hacia dónde.

### [23] [`design_critic`] Respaldos: retención y restauración - Severity: Major
*   **Affected Scope:** [Línea 65](EGM.md#L65); `internal/store/backup.go:61, 100-106`
*   **Discrepancy Description:** El único respaldo automático es la copia previa a cada actualización, sin PDF y sin depurar nunca. La restauración es manual y no indica borrar `-wal` y `-shm`.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado.
*   ***Proposed Edit:*** Documentar retención y el procedimiento seguro de restauración. La depuración va a Defectos (M5).

### [24] [`zoom_out`] La detección de páginas escaneadas está duplicada - Severity: Major
*   **Affected Scope:** [Línea 6](EGM.md#L6); `internal/pdfutil/pdf.go:104-148`; `frontend/dist/app.js:1449-1462, 1546-1557`
*   **Discrepancy Description:** Go (ledongthuc/pdf) y la interfaz (PDF.js) deciden por separado qué páginas son escaneadas. Si Go marca el PDF como escaneado pero PDF.js encuentra texto en todas las páginas, la transcripción no hace nada y la cola se salta la extracción: el convenio queda sin analizar y sin aviso.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado leyendo `processAutoFileOnce` y `transcribeConvention`.
*   ***Proposed Edit:*** Documentarlo. Corregirlo va a Defectos (D2).

### [25] [`zoom_out`] Código y funciones sin uso - Severity: Major
*   **Affected Scope:** [Líneas 34-35](EGM.md#L34-L35); `internal/ai/ai.go:524-580`; `app.go:336-359, 488-508, 1346-1349`
*   **Discrepancy Description:** Sin uso: `ai.Answer`, `GetTags`, `GetSettings`, `Health`, y en `store` `Acronym`, `SetReviewed`, `UnmarkOpenAgreement`. El panel «Aceptar / No usar sugerencias» solo aparece si falla la aceptación automática.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Confirmado.
*   ***Proposed Edit:*** Mencionarlos en el documento. Quitarlos va a Defectos (M6).

### [26] [`zoom_out`, `codebase_details`] Las reglas de etiquetas obligatorias y `tagMu` - Severity: Major
*   **Affected Scope:** [Líneas 46-47](EGM.md#L46-L47); `app.go:55-57, 354-358, 1213-1215`; `internal/store/tags.go:170-240`
*   **Discrepancy Description:** `tagMu` no cubre todas las rutas que cambian etiquetas: `DiscardSuggestions(remove)`, `CreateTag`, `UpdateTag`, `DeleteTag`, `CloneAnalysis` y las reparaciones de `ImportConvenios`. Las reglas están repartidas entre `app.go` y `store`.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** El código contradice su propio comentario.
*   ***Proposed Edit:*** Línea 46 → indicar qué rutas toman el candado. Lo demás va a Defectos (D8).

### [27] [`design_critic`] Sin política de seguridad de contenido en la interfaz - Severity: Nit
*   **Affected Scope:** [Líneas 31-35](EGM.md#L31-L35); `frontend/dist/index.html`
*   **Discrepancy Description:** Cualquier inyección en la WebView tendría acceso a los 49 métodos. `esc()` se usa de forma consistente, pero falta una CSP.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Vale una línea.
*   ***Proposed Edit:*** Una línea sobre la frontera de confianza. Agregar la CSP va a Defectos (M7).

### [28] [`design_critic`] Límite de consultas a GitHub - Severity: Nit
*   **Affected Scope:** [Línea 67](EGM.md#L67)
*   **Discrepancy Description:** Cada comprobación hace 2 consultas anónimas (60 por hora por IP). Muchas PC detrás del mismo NAT pueden recibir 403, que la interfaz muestra como «Sin conexión».
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Afecta a una universidad con red compartida.
*   ***Proposed Edit:*** Una línea en Global Side Effects.

### [29] [`codebase_details`] Falta la flecha `ai → uagro` en el diagrama - Severity: Nit
*   **Affected Scope:** [Líneas 19-21](EGM.md#L19-L21)
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** `internal/ai` importa `uagro`.
*   ***Proposed Edit:*** Agregar `ai --> uagro`.

### [30] [`codebase_details`, `zoom_out`] `Health` no lo usa ninguna prueba - Severity: Nit
*   **Affected Scope:** [Línea 35](EGM.md#L35); `app.go:1346-1349`
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Ninguna prueba ni script lo llama; además expone la ruta de datos.
*   ***Proposed Edit:*** Quitar «existe para pruebas automatizadas»; decir que no se usa.

### [31] [`codebase_details`] Detalles de la carpeta de datos - Severity: Nit
*   **Affected Scope:** [Línea 65](EGM.md#L65)
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** `respaldos/` también guarda las copias `antes-de-actualizar-*.db`; fuera de Windows se usa `os.UserConfigDir()`.
*   ***Proposed Edit:*** Añadirlo a la línea 65.

### [32] [`zoom_out`] La versión fija «1.3.0» envejece - Severity: Nit
*   **Affected Scope:** [Línea 38](EGM.md#L38)
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Cambia con cada versión.
*   ***Proposed Edit:*** «la versión está en `appVersion`» sin el número.

### [33]-[43] Menores sin cambio (DECLINE)
*   `codebase_details` N3: detalles de la ruta `/pdf/<id>` (cabeceras, rangos). Nivel de componente.
*   `codebase_details` N7: la prueba de versión depende del espaciado de `wails.json`. Solo código.
*   `codebase_details` N8: utilidades de las pruebas raíz sin documentar. Solo pruebas.
*   `zoom_out` 7: la versión en tres archivos podría inyectarse al compilar. Mejora de código (M8).
*   `zoom_out` 8: la raíz es demasiado detallada. Choca con `codebase_details`; se conservan los números que son invariantes.
*   `zoom_out` 9: límite de 60 PDF escrito en Go y en JS; SHA-256 calculado dos veces al importar. Solo código.
*   `zoom_out` 10: la validación de «IA activa y con clave» se repite 6 veces. Solo código (M2).
*   `zoom_out` 12: `app.go` escribe SQL directo. Solo código.
*   `zoom_out` 13: la lógica de «hay filtros activos» y los 90 días de «por vencer» están repetidos y ya divergen. Solo código (M9).
*   `zoom_out` 14: pausas fijas al actualizar. Cubierto por 14 y 19.
*   `design_critic` 15: carrera al exportar. Cubierto por 10.

## Defectos del código confirmados (fuera del documento)

Verificados en el código. No se corrigen en esta revisión: son candidatos para una versión nueva.

| Id | Defecto | Dónde | Efecto para el usuario |
| --- | --- | --- | --- |
| D1 | Cambiar Facultad de una reconocida a un texto no reconocido conserva la etiqueta vieja | `app.go:244-248`, `internal/store/faculty.go:27-33` | Etiqueta de facultad equivocada |
| D2 | Go marca escaneado y PDF.js encuentra texto: no se extraen datos | `frontend/dist/app.js:1449-1462, 1554-1557` | Convenio sin analizar, sin aviso |
| D3 | `settings.json` dañado se reemplaza | `app.go:108-113` | Se pierde la clave de API |
| D4 | Sin instancia única | `main.go` | Una segunda ventana interrumpe el análisis de la primera |
| D5 | Publicación borra y luego crea, con cancelación | `.github/workflows/release.yml:15-17, 73-75` | Puede quedar sin release; las PC muestran «Al día» |
| D6 | `log.Fatal` sin mensaje | `main.go:19, 57` | La app no abre y no dice por qué |
| D7 | La extensión se agrega después del diálogo | `app.go:1072, 1097, 1147, 1181` | Sobrescribe un archivo sin preguntar |
| D8 | `tagMu` no cubre todas las rutas | `app.go:354-358, 1213-1215` | Posible pérdida de un cambio de etiquetas |
| D9 | `VerifyAI` consume las dudas antes de revisar la IA | `app.go:691-700` | Se pierden las dudas con la IA desactivada |
| D10 | El contador «No son convenios» cuenta versiones anteriores | `internal/store/conventions.go` (`Stats`) | El número no coincide con la lista |
| D11 | El comentario dice que «UAGro-FT» se reconoce y no es así | `internal/uagro/facultades.go:23` | Comentario engañoso |

Mejoras propuestas por los revisores (no son fallos): M1 reparaciones de arranque como migraciones; M2 errores con tipo en vez de texto; M3 exigir el hash y conservar binarios anteriores; M4 bloquear la edición durante el análisis; M5 depurar copias viejas; M6 quitar código sin uso; M7 agregar CSP; M8 versión inyectada al compilar; M9 unificar «filtros activos» y los 90 días.
