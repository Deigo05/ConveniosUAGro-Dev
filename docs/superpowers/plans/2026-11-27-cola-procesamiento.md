# Cola de procesamiento con pausa y reanudación Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Importar hasta 200 convenios y dejar la cola de IA en pausa, incluso cerrando la app, para continuarla después.

**Architecture:** Una tabla `cola_procesamiento` (sin subir el esquema) guarda los ids pendientes. El frontend pausa en el siguiente límite de página; al abrir, muestra «N en cola · Continuar» y no procesa nada solo.

**Tech Stack:** Go 1.26, SQLite, Wails v2, JS sin compilación.

**Spec:** `docs/superpowers/specs/2026-11-27-cola-procesamiento-design.md`

## Global Constraints

- Sin subir `PRAGMA user_version`: la tabla se crea con `CREATE TABLE IF NOT EXISTS`.
- Nada procesa solo al abrir: el usuario pulsa «Continuar».
- Pausar no es error: no se llama a `FailTranscription` ni se suman rondas de reintento.
- Límite de importación: 200 en `MAX_IMPORT` (app.js) y en `ImportPDFs` (app.go).
- Versión 1.6.0 en los tres archivos, con su entrada en CHANGELOG.

## Review Focus

- Cerrar la app con archivos en cola: al abrir siguen todos y el aviso los cuenta.
- Borrar un convenio de la cola: la fila desaparece sola (cascade).
- Pausar a media transcripción: las páginas ya escritas se conservan y no se marca error.
- Archivo que falla de verdad: sale de la cola, queda con su error y no se repite solo.
- Importar 200 con IA apagada: se importan y quedan sin cola ni avisos.

---

### Task 1: Tabla y API de la cola en el store

**Files:**
- Modify: `internal/store/schema.sql` (al final)
- Create: `internal/store/queue.go`
- Test: `internal/store/queue_test.go`

**Interfaces:**
- Produces: `EnqueueForAI(ids []int64) error`, `Dequeue(id int64) error`, `QueuedIDs() ([]int64, error)`, `QueueCount() (int, error)`, `PruneQueue() error`, `ClearQueue() error`.

- [ ] **Step 1: Pruebas que fallan.**
  - Encolar 2 con `EnqueueForAI` da `QueuedIDs` en orden de encolado y `QueueCount`=2.
  - Encolar dos veces el mismo id no lo duplica.
  - `Dequeue` quita solo ese.
  - Borrar el convenio (SQL `DELETE FROM convenios`) quita la fila.
  - `PruneQueue` quita filas de convenios con `ia_estado='ok'` o archivados y conserva los `error`.
  - Cerrar y reabrir la base conserva la cola.
- [ ] **Step 2:** `go test ./internal/store/ -run Queue` falla.
- [ ] **Step 3:** Crear la tabla del diseño y las funciones con `INSERT OR IGNORE`, `DELETE`, `SELECT … ORDER BY id`.
- [ ] **Step 4:** Pasan.
- [ ] **Step 5:** commit.

### Task 2: Importar 200 y métodos de la app

**Files:**
- Modify: `app.go` (`ImportPDFs`, `Bootstrap`, `startup`, nuevos métodos)
- Test: `app_test.go`

**Interfaces:**
- Produces: `Bootstrap.ColaPendientes int` (`cola_pendientes`), `QueuePending() ([]int64, error)`, `EnqueueAI(ids []int64) error`, `DequeueAI(id int64) error`, `ClearQueue() error`.

- [ ] **Step 1:** Pruebas: `ImportPDFs` con 201 rutas acepta 200; `GetBootstrap` informa `ColaPendientes` tras `EnqueueAI`; `startup` limpia la cola de convenios resueltos.
- [ ] **Step 2:** fallan.
- [ ] **Step 3:** Implementar con `store`.
- [ ] **Step 4:** pasan.
- [ ] **Step 5:** commit.

### Task 3: Interfaz de la cola y la pausa

**Files:**
- Modify: `frontend/dist/index.html` (botones del aviso)
- Modify: `frontend/dist/app.js`
- Modify: `frontend/dist/styles.css` si hace falta

**Interfaces:**
- Consumes: `QueuePending`, `EnqueueAI`, `DequeueAI`, `ClearQueue`, `Bootstrap.cola_pendientes`.

- [ ] `MAX_IMPORT = 200`.
- [ ] `startAutoAI` encola con `EnqueueAI` y procesa; `state.autoPaused`, `#processing-pause`, `#processing-resume`, `#processing-clear`.
- [ ] `transcribeConvention` devuelve `'pausado'` si se pidió pausa antes de una página; no llama a `FailTranscription`; el worker devuelve el id al frente de la cola.
- [ ] Al terminar o fallar de verdad un archivo, `DequeueAI`.
- [ ] Se quita el reintento automático al abrir; «Continuar» usa `QueuePending` + `RetryCandidates`.
- [ ] El aviso muestra «N en cola · Continuar · Quitar» al abrir con la IA configurada.
- [ ] Verificación manual con playwright-cli y el stub.

### Task 4: Versión 1.6.0 y documentación

- [ ] CHANGELOG, `EGM.md` (métodos publicados), `internal/store/EGM.md`, `frontend/dist/EGM.md`.
- [ ] `VERSION`, `appVersion`, `productVersion` a 1.6.0.
- [ ] `go test ./...` y `node --test frontend/tests/revision.test.js` pasan.
