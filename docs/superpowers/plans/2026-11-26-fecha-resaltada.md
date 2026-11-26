# Fecha resaltada Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Dentro del recuadro de «Ver en el documento», marcar con resaltador la fecha (firma) o las fechas y plazos (vigencia y cláusula).

**Architecture:** Una función pura nueva en `revision.js` encuentra los tramos dentro del cierre o de la cláusula ya ubicados. `app.js` convierte cada tramo en rectángulos con `matchRects` y los dibuja encima del recuadro.

**Tech Stack:** JS sin compilación (Wails, frontend/dist), PDF.js 3.11.174, node:test.

**Spec:** `docs/superpowers/specs/2026-11-26-fecha-resaltada-design.md`

## Global Constraints

- Sin cambios en Go, en la base de datos ni en el análisis de la IA.
- Nunca un marcador fuera del recuadro ni aproximado: si `matchRects` devuelve null para una marca, esa marca no se dibuja.
- Plazos: solo en años o meses, o indefinidos. No se marca uno seguido de «de anticipación» o «antes».
- Versión 1.4.1 en `VERSION`, en `appVersion` (app.go) y en `productVersion` (wails.json), con su entrada en CHANGELOG.

## Review Focus

- Fecha con número entre paréntesis («veinticinco (25)», «dos mil veinticinco (2025)»): la marca cubre las palabras y el número.
- El aviso de terminación («con 30 días naturales de anticipación», «tres meses antes») dentro de la cláusula no se marca.
- El cierre cortado a 80 palabras: no hay marcas fuera del tramo.
- Una fecha que cruza renglón: dos rectángulos de marca, o ninguno si no son contiguos.
- Cambio de tamaño de ventana o de zoom: las marcas se vuelven a dibujar en su lugar, igual que el recuadro.

---

### Task 1: `findMarks` en revision.js

**Files:**
- Modify: `frontend/dist/revision.js` (sección nueva «Marcas» antes de «Recuadros»; exportar `findMarks`)
- Test: `frontend/tests/revision.test.js`

**Interfaces:**
- Produces: `findMarks(index, start, end, kind) -> [{start, end}]`, con `kind` igual a `'firma'` o `'vigencia'`. Los tramos están dentro de `[start, end)`, ordenados, sin traslapes y sin espacios en los bordes.

- [ ] **Step 1: Pruebas que fallan.** Con `marks(s, kind) = R.findMarks(idx(s), 0, idx(s).text.length, kind).map(m => texto)`:
  - `'firma'` en «a los veinticinco (25) días del mes de noviembre del año dos mil veinticinco (2025).» da `['veinticinco 25 dias del mes de noviembre del ano dos mil veinticinco 2025']`.
  - «Chilpancingo, Gro., 25 de noviembre de 2025» da `['25 de noviembre de 2025']`.
  - «el 25/11/2025» da `['25 11 2025']`.
  - `'firma'` no marca plazos: «por tres (3) años, el 1o. de enero de 2025» da `['1o de enero de 2025']`.
  - `'vigencia'` en «tendrá una vigencia de tres (3) años … hasta el 31 de diciembre de 2027» da `['tres 3 anos', '31 de diciembre de 2027']`.
  - «36 meses» da `['36 meses']`; «por tiempo indefinido» da `['tiempo indefinido']`.
  - «con 30 días naturales de anticipación» y «con tres meses de anticipación» dan `[]`.
  - El rango limita: con `start`/`end` en medio del texto, ninguna marca sale de él.
- [ ] **Step 2:** `node --test frontend/tests/revision.test.js` falla con «findMarks is not a function».
- [ ] **Step 3: Implementar.** Expresiones sobre el texto normalizado:
  - Número en palabras: de uno/un/primero a treinta, decenas y «y» (treinta y uno), o bien `\d{1,2}o?`, seguido opcionalmente de su número entre paréntesis.
  - Fecha: [día][« dias»][« del mes»]« de » mes [« de|del»[« ano»] año], donde el año es `\d{4}` o «dos mil …» con su número opcional. También `\d{1,2} \d{1,2} \d{2,4}`.
  - Plazo: número + `anos?|mes|meses`, o `(tiempo )?(indefinid|indeterminad)[ao]`. Se descarta si lo siguen « de anticipacion» o « antes».
  - Primero las fechas y luego los plazos que no se traslapan.
- [ ] **Step 4:** la suite pasa completa.
- [ ] **Step 5:** commit `feat: marcas de fecha y plazo dentro del recuadro`.

### Task 2: Dibujar las marcas en la vista

**Files:**
- Modify: `frontend/dist/app.js` (`reviewCandidate`, `renderReviewHead`, `drawReviewPage`)
- Modify: `frontend/dist/styles.css` (`#review-boxes`)

**Interfaces:**
- Consumes: `Revision.findMarks`, `Revision.matchRects`, `Revision.snippet`.
- Produces: el candidato lleva `marks: [[rect…]…]`; en las transcripciones, `snippetParts: [{text, mark}]`.

- [ ] **Step 1:** `reviewCandidate(page, data, match, label, kind)`.
  - Si hay `rects`, `marks` son los `matchRects` de cada tramo de `findMarks`, sin los null.
  - En una transcripción, `snippetParts` alterna el texto normal y el marcado con `Revision.snippet` por tramo.
  - Llamadas: `'firma'` para el cierre y `'vigencia'` para los campos vigencia y cláusula.
- [ ] **Step 2:** `drawReviewPage`.
  - El recuadro lleva la clase `review-region` y cada marca un `div.review-mark`.
  - Con marcas, `#review-boxes` lleva `has-marks` y el desplazamiento se centra en la primera marca.
  - `renderReviewHead` arma el fragmento con `<mark>` usando nodos de texto, nunca innerHTML con el texto del PDF.
- [ ] **Step 3:** CSS.
  - `.review-mark`: fondo amarillo de resaltador, `mix-blend-mode: multiply`, sin borde.
  - `#review-boxes.has-marks .review-region`: borde y fondo más tenues.
  - `#review-snippet mark`: mismo color.
- [ ] **Step 4:** prueba visual con playwright-cli sobre la página de pruebas: cierre con fecha en letra y número, cláusula con plazo y aviso, transcripción, cambio de tamaño de ventana.
- [ ] **Step 5:** commit `feat: la vista marca la fecha dentro del recuadro`.

### Task 3: Versión 1.4.1

- [ ] Subir la versión en los tres archivos y agregar la entrada de CHANGELOG.
- [ ] Actualizar la sección de revisión puntual en `frontend/dist/EGM.md`.
- [ ] `~/.local/go/bin/go test ./...` y la suite de node pasan.
- [ ] Commit `docs: version 1.4.1`.
