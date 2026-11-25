# Revisión puntual de la vigencia: plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un botón «Ver en el documento» junto a Fecha de firma, Fecha de vigencia y Cláusula de vigencia que muestra, dentro de la ficha, la página del PDF con un recuadro sobre el cierre o la cláusula.

**Architecture:** La lógica de búsqueda va en un archivo nuevo de funciones puras, `frontend/dist/revision.js` (global `Revision` en el navegador, `module.exports` en Node), probado con `node --test`. `app.js` agrega el controlador de la vista de revisión: abre el PDF con PDF.js, lee el texto por página (o la transcripción en páginas escaneadas), llama a `Revision` y dibuja la página con los recuadros. No cambia Go ni la base de datos.

**Tech Stack:** JavaScript sin framework en `frontend/dist`, PDF.js ya incluido (`vendor/pdf.min.js`), Node 18+ solo para las pruebas (`node:test`, `node:assert`), Wails v2.

**Spec:** `docs/superpowers/specs/2026-11-25-revision-puntual-fechas-design.md`

## Global Constraints

- Rama `revision-fechas`. No se une a `main` sin preguntar al usuario: un push a `main` publica un release que llega a la PC.
- Sin red ni IA; sin cambios en Go, en la base de datos ni en los métodos publicados de `*App`. Se usan `GetTranscription(id)` y el PDF de `/pdf/<id>`.
- Página con texto = 30 o más caracteres que no son espacio en PDF.js (la misma regla de `transcribeConvention`).
- Dos estados: con recuadro solo si lo buscado aparece entero en fragmentos contiguos de PDF.js en una página con texto; todo lo demás, sin recuadro y con aviso. Nunca un recuadro aproximado.
- Las páginas transcritas nunca llevan recuadro y siempre muestran «Según la transcripción de la IA; compare con la imagen».
- Etiquetas exactas: «posible cierre», «cláusula completa», «parte de la cláusula».
- Avisos exactos: «No se encontró el cierre en el texto del documento», «No se encontró la cláusula en el texto del documento», «No hay cláusula de vigencia guardada», «Hay N páginas escaneadas sin transcribir» (singular: «Hay 1 página escaneada sin transcribir»), «No se pudo abrir el PDF para revisarlo».
- Texto del PDF o de la transcripción siempre con `textContent` o `esc()`, nunca concatenado en `innerHTML`.
- Textos de interfaz en español de México; «hacer clic», no «pulsar». Sin emojis.
- Commits con prefijo `feat:`/`fix:`/`docs:`/`test:`, sin acentos, con las líneas `Co-Authored-By` y `Claude-Session` de la sesión.

## Review Focus

- **Ligaduras y guiones de corte** («ﬁrman», «fir-» al final de renglón + «man»): deben encontrarse como «firman». Prueba en la tarea 1.
- **Firmas a dos columnas en la misma altura**: una coincidencia que salta de una columna a la otra no lleva recuadro. Prueba en la tarea 4.
- **Cambiar de campo o cerrar la vista mientras se busca**: no debe aparecer el resultado de la búsqueda anterior. Paso en la tarea 6.
- **Cláusula editada y sin guardar**: se busca lo que está escrito en `#f-vigencia-texto` en ese momento, no lo guardado. Paso en la tarea 6.
- **Página rotada (`/Rotate 90`) o PDF que PDF.js no puede abrir**: el recuadro cae sobre el texto rotado, y un PDF dañado muestra el aviso sin romper el visor de Edge. Paso en la tarea 6.

---

### Task 1: Base de `revision.js`: índice de página, normalización y transcripción

**Files:**
- Create: `frontend/dist/revision.js`
- Create: `frontend/tests/revision.test.js`
- Modify: `frontend/dist/index.html` (agregar `<script src="revision.js"></script>` entre `vendor/pdf.min.js` y `app.js`)
- Modify: `frontend/dist/app.js` (`transcribeConvention`: usar `Revision.pageChars(content.items) < Revision.MIN_PAGE_CHARS` en lugar del cálculo en línea)
- Modify: `.github/workflows/release.yml` (paso «Pruebas de la interfaz» después de «Pruebas»: `run: node --test frontend/tests/revision.test.js`)

**Interfaces:**
- Produces:
  - `Item = {str: string, hasEOL?: boolean, transform?: number[], width?: number, height?: number}` (la forma de `getTextContent().items` de PDF.js)
  - `PageIndex = {text: string, pos: Array<{item: number, offset: number}>, sentenceEnds: number[], items: Item[]}`. `text` está normalizado; `pos[i]` es el ítem y la posición dentro de `items[item].str` del primer carácter original que produjo `text[i]`; `sentenceEnds` son posiciones de `text` donde el original tenía un punto.
  - `Revision.buildPageIndex(items: Item[]): PageIndex`
  - `Revision.normalizeForSearch(s: string): string`, igual a `buildPageIndex([{str: s}]).text.trim()`
  - `Revision.pageChars(items: Item[]): number`, `Revision.MIN_PAGE_CHARS = 30`
  - `Revision.splitTranscription(texto: string): Map<number, string>`
  - `Revision.snippet(index: PageIndex, start: number, end: number): string`, el texto original (sin normalizar) que cubre `text[start..end)`

- [ ] **Step 1: Escribir las pruebas que fallan** en `frontend/tests/revision.test.js` (CommonJS, `const R = require('../dist/revision.js')`, `node:test` y `node:assert/strict`):

```js
test('normaliza acentos, mayúsculas, puntuación y espacios', () => {
  assert.equal(R.normalizeForSearch('  Días del MES, de  Marzo. '), 'dias del mes de marzo');
});
test('normaliza ligaduras', () => {
  assert.equal(R.normalizeForSearch('lo ﬁrman'), 'lo firman');
});
test('une el guion de corte entre renglones', () => {
  const idx = R.buildPageIndex([{ str: 'lo fir-', hasEOL: true }, { str: 'man en Chilpancingo' }]);
  assert.equal(idx.text.trim(), 'lo firman en chilpancingo');
});
test('el guion normal entre palabras se vuelve espacio', () => {
  assert.equal(R.normalizeForSearch('teórico-práctico'), 'teorico practico');
});
test('pos apunta al ítem y posición originales', () => {
  const idx = R.buildPageIndex([{ str: 'Hola ' }, { str: 'Mundo' }]);
  const i = idx.text.indexOf('mundo');
  assert.deepEqual(idx.pos[i], { item: 1, offset: 0 });
});
test('registra fin de oración en los puntos', () => {
  const idx = R.buildPageIndex([{ str: 'Uno dos. Tres' }]);
  assert.deepEqual(idx.sentenceEnds, [idx.text.indexOf(' tres')]);
});
test('pageChars cuenta solo caracteres que no son espacio', () => {
  assert.equal(R.pageChars([{ str: 'a b ' }, { str: ' c' }]), 3);
});
test('splitTranscription separa por [Página N] e ignora el texto del PDF previo', () => {
  const m = R.splitTranscription('texto nativo\n\n[Página 3]\nuno\n\n[Página 7]\ndos');
  assert.deepEqual([...m.entries()], [[3, 'uno'], [7, 'dos']]);
});
test('snippet devuelve el texto original', () => {
  const idx = R.buildPageIndex([{ str: 'a los Quince días.' }]);
  const s = idx.text.indexOf('quince');
  assert.equal(R.snippet(idx, s, s + 'quince dias'.length), 'Quince días');
});
```

- [ ] **Step 2: Correr y ver que fallan**

Run: `cd ~/proyectos/ConveniosUAGro-Dev && ~/.cache/tg/node/bin/node --test frontend/tests/revision.test.js`
Expected: FAIL con `Cannot find module '../dist/revision.js'`

- [ ] **Step 3: Implementar `frontend/dist/revision.js`**

Un IIFE que construye el objeto `Revision` y al final hace `if (typeof module !== 'undefined' && module.exports) module.exports = Revision; else window.Revision = Revision;`. Comentarios en español, como el resto del proyecto. Reglas de `buildPageIndex` que las pruebas no fijan por completo:
- Recorre los caracteres de cada `item.str`. Cada carácter pasa por `normalize('NFKD')`, se le quitan las marcas `̀-ͯ` y se pasa a minúsculas. Las letras y dígitos resultantes se agregan a `text`. Cualquier otro carácter (espacio, salto, puntuación, guion) agrega un solo espacio si el último carácter de `text` no es espacio.
- Después de un ítem con `hasEOL`, se agrega un salto virtual (cuenta como espacio).
- Entre dos ítems sin `hasEOL` se agrega un espacio virtual solo si los dos tienen `transform` y la separación horizontal (`next.transform[4] - (prev.transform[4] + prev.width)`) es mayor que `0.15 * altura`, o si cambian de renglón (`|Δy| > 0.5 * altura`). Altura = `item.height || Math.abs(item.transform[3])`.
- Guion de corte: un `-` o `­` seguido solo de espacios y de un salto (real o virtual) y luego de una letra minúscula se descarta junto con el salto.
- Un `.` registra `text.length` en `sentenceEnds` (sin duplicar el valor anterior).
- `pos` recibe una entrada por cada carácter agregado a `text`, incluidos los espacios; los virtuales apuntan al final del ítem anterior.

- [ ] **Step 4: Correr y ver que pasan**

Run: `~/.cache/tg/node/bin/node --test frontend/tests/revision.test.js`
Expected: PASS, 9 pruebas

- [ ] **Step 5: Conectar en la app y en CI**

Agregar el `<script>` en `index.html`, reemplazar el cálculo de caracteres en `transcribeConvention` por `Revision.pageChars` y agregar el paso de CI. Verificar que `node --check frontend/dist/app.js` no marca error.

- [ ] **Step 6: Commit**

```bash
git add frontend/dist/revision.js frontend/tests/revision.test.js frontend/dist/index.html frontend/dist/app.js .github/workflows/release.yml
git commit -m "feat: indice de texto por pagina para la revision puntual"
```

### Task 2: Ubicar el cierre (`findClosings`)

**Files:**
- Modify: `frontend/dist/revision.js`
- Test: `frontend/tests/revision.test.js`

**Interfaces:**
- Consumes: `buildPageIndex`, `PageIndex` (tarea 1)
- Produces: `Revision.CLOSING_FORMULAS: string[]`; `Revision.findClosings(index: PageIndex): Array<{start: number, end: number}>`, en orden del documento, una entrada por oración.

- [ ] **Step 1: Escribir las pruebas que fallan**

```js
const idx = s => R.buildPageIndex([{ str: s }]);
const covered = (ix, c) => ix.text.slice(c.start, c.end);

test('encuentra el cierre con fecha en palabras', () => {
  const ix = idx('Cláusula final. Leído que fue, lo firman por duplicado en Chilpancingo, Guerrero, a los quince días del mes de marzo de dos mil veinticuatro. Por la UAGro');
  const c = R.findClosings(ix);
  assert.equal(c.length, 1);
  assert.match(covered(ix, c[0]), /^lo firman por duplicado .* dos mil veinticuatro$/);
});
test('incluye la fecha que va antes de la fórmula en la misma oración', () => {
  const ix = idx('En Acapulco, el 15 de marzo de 2024, lo firman las partes.');
  assert.match(covered(ix, R.findClosings(ix)[0]), /^15 de marzo de 2024 lo firman las partes$/);
});
test('acepta la anotación de firmas y la fecha en números', () => {
  const ix = idx('Las firmas que anteceden corresponden al convenio de fecha 15/03/2024.');
  assert.equal(R.findClosings(ix).length, 1);
});
test('sin fecha no es candidato', () => {
  assert.deepEqual(R.findClosings(idx('Para constancia de lo anterior, lo firman las partes.')), []);
});
test('una oración con dos fórmulas da un solo candidato', () => {
  assert.equal(R.findClosings(idx('Lo firman a los dos días del mes de enero de 2020.')).length, 1);
});
test('un nombramiento con fecha y fórmula sale como candidato aparte', () => {
  const ix = idx('Nombrado el día 3 del mes de enero de 2019, firmado en Chilpancingo. Lo firman a los 15 días del mes de marzo de 2024.');
  assert.equal(R.findClosings(ix).length, 2);
});
test('el candidato no pasa de 80 palabras', () => {
  const ix = idx('lo firman ' + 'palabra '.repeat(200) + 'el 1 de enero de 2020.');
  const c = R.findClosings(ix);
  assert.ok(c.length === 0 || covered(ix, c[0]).split(' ').length <= 80);
});
```

- [ ] **Step 2: Correr y ver que fallan**

Run: `~/.cache/tg/node/bin/node --test frontend/tests/revision.test.js`
Expected: FAIL, `R.findClosings is not a function`

- [ ] **Step 3: Implementar `findClosings(index)`**

- `CLOSING_FORMULAS` (normalizadas): `'dias del mes de'`, `'dia del mes de'`, `'lo firman'`, `'la firman'`, `'firman por'`, `'firman de conformidad'`, `'firmado en'`, `'firmada en'`, `'se firma'`, `'suscriben'`, `'para constancia'`, `'firmas que anteceden'`.
- Una búsqueda con límite de palabra (helper `occurrences(text, phrase)`: el carácter anterior y el siguiente son inicio, fin o espacio) da cada aparición de fórmula. La oración va del `sentenceEnds` anterior (o 0) al siguiente (o el final del texto), sin espacios en los bordes.
- Fecha = `/\b(?:\d{1,2} (?:de )?)?(?:enero|febrero|marzo|abril|mayo|junio|julio|agosto|septiembre|setiembre|octubre|noviembre|diciembre)\b/` (el día en números, si va pegado, entra en la fecha) o `/\b\d{1,2} \d{1,2} \d{2,4}\b/`, dentro de la oración. Sin fecha, no hay candidato.
- `start` = la menor posición entre la primera fórmula de la oración y la primera fecha. `end` = el fin de la oración, recortado a 80 palabras desde `start`. Una sola entrada por oración.

- [ ] **Step 4: Correr y ver que pasan**

Run: `~/.cache/tg/node/bin/node --test frontend/tests/revision.test.js`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/dist/revision.js frontend/tests/revision.test.js
git commit -m "feat: ubicar el cierre del convenio en el texto de una pagina"
```

### Task 3: Ubicar la cláusula (`findClause`)

**Files:**
- Modify: `frontend/dist/revision.js`
- Test: `frontend/tests/revision.test.js`

**Interfaces:**
- Consumes: `buildPageIndex`, `normalizeForSearch`, `occurrences` (tareas 1 y 2)
- Produces: `Revision.MIN_CLAUSE_WORDS = 12`; `Revision.findClause(pages: Array<{page: number, index: PageIndex}>, clause: string): Array<{page: number, start: number, end: number, complete: boolean}>`

- [ ] **Step 1: Escribir las pruebas que fallan**

```js
const CL = 'El presente convenio tendrá una vigencia de tres años contados a partir de la fecha de su firma y podrá renovarse por acuerdo de las partes';
const pg = (page, s) => ({ page, index: R.buildPageIndex([{ str: s }]) });

test('cláusula completa', () => {
  const r = R.findClause([pg(1, 'Proemio.'), pg(5, 'DÉCIMA. ' + CL + '. UNDÉCIMA.')], CL);
  assert.deepEqual(r.map(x => [x.page, x.complete]), [[5, true]]);
});
test('cláusula recortada por la IA: tramo largo y único', () => {
  const r = R.findClause([pg(5, 'DÉCIMA. ' + CL.replace('y podrá renovarse', 'pudiendo renovarse'))], CL);
  assert.equal(r.length, 1);
  assert.equal(r[0].complete, false);
  assert.ok(r[0].end - r[0].start >= R.normalizeForSearch('El presente convenio tendrá una vigencia de tres años contados a partir de la fecha de su firma').length);
});
test('cláusula que cruza de página: el tramo más largo en una página', () => {
  const words = CL.split(' ');
  const r = R.findClause([pg(5, words.slice(0, 16).join(' ')), pg(6, words.slice(16).join(' '))], CL);
  assert.deepEqual(r.map(x => [x.page, x.complete]), [[5, false]]);
});
test('tramo repetido en el documento no cuenta', () => {
  const frase = 'contados a partir de la fecha de su firma y podrá renovarse por acuerdo de las partes';
  const r = R.findClause([pg(2, frase), pg(5, frase)], 'Su vigencia será indefinida, ' + frase + ' x');
  assert.deepEqual(r, []);
});
test('menos de 12 palabras: solo coincidencia completa', () => {
  assert.deepEqual(R.findClause([pg(1, 'vigencia de tres años')], 'una vigencia de tres años y medio'), []);
});
test('cláusula vacía', () => {
  assert.deepEqual(R.findClause([pg(1, CL)], '  '), []);
});
```

- [ ] **Step 2: Correr y ver que fallan**

Run: `~/.cache/tg/node/bin/node --test frontend/tests/revision.test.js`
Expected: FAIL, `R.findClause is not a function`

- [ ] **Step 3: Implementar `findClause(pages, clause)`**

- `q = normalizeForSearch(clause)`; si está vacía, `[]`.
- Coincidencia completa: todas las apariciones de `q` (con límite de palabra) en cada página, como `complete: true`. Si hay alguna, se devuelven en orden de página.
- Si no, y `q` tiene menos de `MIN_CLAUSE_WORDS` palabras, `[]`.
- Si no, para `L` desde `palabras − 1` hasta `MIN_CLAUSE_WORDS`, y para cada inicio `i`: la frase `palabras[i..i+L)` se cuenta en todas las páginas. La primera que aparece exactamente una vez se devuelve como `[{…, complete: false}]`. Si ninguna, `[]`.

- [ ] **Step 4: Correr y ver que pasan**

Run: `~/.cache/tg/node/bin/node --test frontend/tests/revision.test.js`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/dist/revision.js frontend/tests/revision.test.js
git commit -m "feat: ubicar la clausula de vigencia completa o su tramo unico"
```

### Task 4: De la coincidencia al recuadro (`matchRects`)

**Files:**
- Modify: `frontend/dist/revision.js`
- Test: `frontend/tests/revision.test.js`

**Interfaces:**
- Consumes: `PageIndex` (tarea 1)
- Produces: `Revision.matchRects(index: PageIndex, start: number, end: number): Array<{x0: number, y0: number, x1: number, y1: number}> | null`, en coordenadas de PDF (sin escalar), un rectángulo por renglón; `null` = sin recuadro.

- [ ] **Step 1: Escribir las pruebas que fallan**

```js
// Ítem de 10 pt de alto; ancho = 5 pt por carácter.
const it = (str, x, y, eol = false) => ({ str, transform: [10, 0, 0, 10, x, y], width: str.length * 5, height: 10, hasEOL: eol });
const find = (ix, s) => { const a = ix.text.indexOf(s); return [a, a + s.length]; };

test('un renglón: un rectángulo que cubre solo lo encontrado', () => {
  const ix = R.buildPageIndex([it('a los quince dias', 100, 700)]);
  const r = R.matchRects(ix, ...find(ix, 'quince'));
  assert.equal(r.length, 1);
  assert.deepEqual(r[0], { x0: 130, y0: 697.5, x1: 160, y1: 710 });
});
test('dos renglones seguidos: dos rectángulos', () => {
  const ix = R.buildPageIndex([it('lo firman en', 100, 700, true), it('Chilpancingo', 72, 686)]);
  assert.equal(R.matchRects(ix, ...find(ix, 'firman en chilpancingo')).length, 2);
});
test('dos columnas en la misma altura: sin recuadro', () => {
  const ix = R.buildPageIndex([it('Por la UAGro', 72, 200), it('Por la Contraparte', 330, 200)]);
  assert.equal(R.matchRects(ix, ...find(ix, 'uagro por la contraparte')), null);
});
test('el texto sube en la página: sin recuadro', () => {
  const ix = R.buildPageIndex([it('final de columna', 72, 100, true), it('inicio de otra', 330, 700)]);
  assert.equal(R.matchRects(ix, ...find(ix, 'columna inicio')), null);
});
test('texto rotado: sin recuadro', () => {
  const ix = R.buildPageIndex([{ str: 'lo firman', transform: [0, 10, -10, 0, 50, 50], width: 45, height: 10 }]);
  assert.equal(R.matchRects(ix, ...find(ix, 'firman')), null);
});
test('ítem sin posición (transcripción): sin recuadro', () => {
  const ix = R.buildPageIndex([{ str: 'lo firman' }]);
  assert.equal(R.matchRects(ix, ...find(ix, 'firman')), null);
});
```

- [ ] **Step 2: Correr y ver que fallan**

Run: `~/.cache/tg/node/bin/node --test frontend/tests/revision.test.js`
Expected: FAIL, `R.matchRects is not a function`

- [ ] **Step 3: Implementar `matchRects(index, start, end)`**

- Ítems cubiertos: de `pos[start].item` a `pos[end − 1].item`. Si alguno no tiene `transform` o tiene `transform[1] !== 0 || transform[2] !== 0`, `null`.
- Recorrerlos en orden con `h = height || |transform[3]|`, `x = transform[4]`, `y = transform[5]`:
  - mismo renglón si `|y − prevY| ≤ 0.5h` y `−0.5h ≤ x − prevX1 ≤ 3h`;
  - renglón siguiente si `0.5h < prevY − y ≤ 2.5h` y `x ≤ inicioDelRenglónActual + 3h`;
  - cualquier otro caso, `null`.
- Por renglón: `x0` y `x1` interpolan dentro del ítem en proporción a `offset / str.length`; `y0 = y − 0.25h`, `y1 = y + h`. El `end` usa el carácter `end − 1` (inclusive).

- [ ] **Step 4: Correr y ver que pasan**

Run: `~/.cache/tg/node/bin/node --test frontend/tests/revision.test.js`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/dist/revision.js frontend/tests/revision.test.js
git commit -m "feat: recuadros por renglon solo para fragmentos contiguos"
```

### Task 5: Botones en la ficha y panel de revisión (marcado y estilo)

**Files:**
- Modify: `frontend/dist/index.html:262-273` (tarjeta del PDF) y `:344-363` (sección Vigencia)
- Modify: `frontend/dist/styles.css`

**Interfaces:**
- Produces (ids que usa la tarea 6):
  - Botones `button.field-locate[data-review="firma"|"vigencia"|"clausula"]`, cada uno `type="button"`, `title="Ver en el documento"`, `aria-label="Ver en el documento: <Campo>"`, con un SVG de 16 px `aria-hidden="true"` (lupa sobre una hoja, mismo trazo que los `ICON_*` de `app.js`).
  - Contenedor `#review` (hermano de `.pdf-frame-wrap`, `hidden` al inicio) con: `#review-title`, `#review-meta`, `#review-prev`, `#review-count`, `#review-next`, `#review-notice` (`role="status"`), `#review-snippet`, `.review-canvas-wrap` > `canvas#review-canvas` + `div#review-boxes`, `#review-page-prev`, `#review-page`, `#review-page-next`, `#review-zoom-out`, `#review-zoom-in` y `#review-close` («Volver al PDF completo»).
  - Clase `.field.is-reviewing` para marcar el campo en revisión.

- [ ] **Step 1: Cambiar los tres campos de Vigencia**

Cada `<label class="field">` de Fecha de firma, Fecha de vigencia y Cláusula de vigencia pasa a `<div class="field">` con `<div class="field__head"><label for="f-…">Texto</label><button …></div>` y luego el control. El botón queda fuera del `<label>`. Se conservan dentro de la etiqueta el badge `#f-calculada` y el `<small>` «(texto del convenio)».

- [ ] **Step 2: Agregar el panel `#review`** con los ids de arriba, todos los botones `type="button"`, y estilo en `styles.css`:
  - `.field__head` en una fila, con la etiqueta igual que `.field > span`.
  - `.field-locate` de 24 px, sin fondo, color `--titulo` al pasar el cursor.
  - `.field.is-reviewing` con un borde izquierdo de 3 px en `--oro`.
  - `#review` ocupa el alto de `.pdf-frame-wrap`; `.review-canvas-wrap` con `overflow: auto` y posición relativa.
  - Recuadros: `#review-boxes > div` con posición absoluta, borde de 2 px en `--oro` y un fondo del mismo color al 15 %.

- [ ] **Step 3: Verificar el marcado**

Run: copiar `frontend/dist/*` a `/tmp/claude-1000/-home-diego-admin/c835bee7-b2dd-45f6-a9a1-597d5d21cdbb/scratchpad/h/site/` (sin pisar `stub.js`), servir con `python3 -m http.server 8765` desde esa carpeta y abrir una ficha con playwright-cli (desde `~/admin`).
Expected: los tres íconos junto a sus etiquetas. Hacer clic en el texto de la etiqueta pone el foco en el campo y no abre nada. Captura revisada a mano.

- [ ] **Step 4: Commit**

```bash
git add frontend/dist/index.html frontend/dist/styles.css
git commit -m "feat: botones de revision junto a la vigencia y panel de revision"
```

### Task 6: Controlador de la vista de revisión y verificación en el entorno de prueba

**Files:**
- Modify: `frontend/dist/app.js` (sección nueva `/* ---------- Revisión puntual ---------- */`; `bindEvents`; `showView`; `renderDetail`; los cinco puntos que cambian el texto del convenio)
- Fuera del repo: `/tmp/claude-1000/-home-diego-admin/c835bee7-b2dd-45f6-a9a1-597d5d21cdbb/scratchpad/h/site/stub.js` y `.../site/pdf/<id>`

**Interfaces:**
- Consumes: `Revision.buildPageIndex`, `pageChars`, `MIN_PAGE_CHARS`, `splitTranscription`, `findClosings`, `findClause`, `matchRects`, `snippet` (tareas 1-4); los ids de la tarea 5.
- Produces:
  - `state.review = {open: false, id: null, field: null, doc: null, pages: new Map(), transcription: null, candidates: [], pos: 0, page: 1, zoom: 1, token: 0, scanning: false, untranscribed: 0, notice: ''}`
  - `openReview(field: 'firma'|'vigencia'|'clausula'): Promise<void>`
  - `closeReview(): void`: oculta `#review`, muestra `.pdf-frame-wrap`, quita `.is-reviewing`, hace `doc.destroy()` y reinicia `state.review` (incrementa `token`).
  - `closeReviewFor(id: number): void`: `closeReview()` si la vista está abierta para ese `id`.
  - `textCall(method: string, id: number, ...args): Promise<any>`: `call(method, id, ...args)` seguido de `closeReviewFor(id)`.

- [ ] **Step 1: Ganchos de ciclo de vida**

- Reemplazar por `textCall` las llamadas a `call('ExtractAI', …)` (las tres), `call('VerifyAI', …)` en `verifyDoubts` y `call('FinishTranscription', …)`.
- En `showView`, si `view !== 'detalle'`, `closeReview()`.
- Al inicio de `renderDetail`, si `state.review.open && state.review.id !== c.id`, `closeReview()`.
- En `bindEvents`: clic en `.field-locate` → `openReview(btn.dataset.review)`, y los botones del panel.

- [ ] **Step 2: Implementar `openReview` y la búsqueda**

- Cada llamada incrementa `token` y lo guarda; tras cada `await` compara y, si cambió, sale sin tocar la pantalla.
- Al abrir: mostrar `#review`, ocultar `.pdf-frame-wrap` (el iframe no se recarga), marcar `.is-reviewing` y mostrar «Buscando…».
- Si no hay `doc`, `pdfjsLib.getDocument({url: '/pdf/' + id})`; si falla, aviso «No se pudo abrir el PDF para revisarlo» y la vista sin página.
- `transcription` se carga una vez con `splitTranscription((await call('GetTranscription', id)).texto)`.
- Por página, se guarda en `pages` `{index, source}`: con `pageChars ≥ MIN_PAGE_CHARS`, índice de los ítems de PDF.js con `source: 'pdf'`; si no, y hay transcripción de esa página, `buildPageIndex([{str: texto}])` con `source: 'transcripcion'`; si no, `source: null` y suma a `untranscribed`.
- **firma:** recorre de la última página a la primera, agrega los `findClosings` de cada página del final al principio de la página y, con el primer candidato, dibuja sin esperar a que termine (contador «1 de 1…» mientras `scanning`).
- **vigencia y clausula:** con `#f-vigencia-texto.value` vacío, aviso «No hay cláusula de vigencia guardada». Si no, lee todas las páginas y llama a `findClause`.
- Cada candidato es `{page, rects, label, source, snippet}`:
  - `rects` = `matchRects` solo si `source === 'pdf'`; si no, `null`.
  - `snippet` se llena solo en las transcritas.
  - `label` es «posible cierre», «cláusula completa» o «parte de la cláusula».
- Sin candidatos: aviso de la lista de Global Constraints; si `untranscribed > 0`, se le agrega « Hay N páginas escaneadas sin transcribir.». La vista queda en la página 1.

- [ ] **Step 3: Implementar el dibujo y los controles**

- `#review-title`: «Revisando: <Campo>»; `#review-meta`: «Página P de N · <label>».
- `#review-snippet` (con `textContent`) y el aviso de transcripción solo cuando `source === 'transcripcion'`.
- Escala = `(ancho de .review-canvas-wrap / viewport a escala 1) × zoom`, con zoom en `[0.75, 1, 1.25, 1.5, 2]`. Canvas a `devicePixelRatio`; cancelar el `renderTask` anterior.
- Cada rect con `viewport.convertToViewportRectangle([x0, y0, x1, y1])`, como `div` absoluto en `#review-boxes`, y llevar el primero a la vista con `scrollIntoView({block: 'center'})`.
- «anterior / siguiente» recorren los candidatos. Las flechas de página cambian de página sin recuadros. «Volver al PDF completo» → `closeReview()`. Hacer clic en el ícono de otro campo con la vista abierta reutiliza `doc` y `pages`.

- [ ] **Step 4: Preparar el entorno de prueba (fuera del repo)**

- En `stub.js`, `GetTranscription(id)` devuelve `{estado: 'completa', texto}` por convenio; `GetConvenio` incluye `vigencia_texto` acorde.
- Generar en `site/pdf/<id>` (sin extensión) con `soffice --headless --convert-to pdf` a partir de HTML, `pdftoppm` + `soffice` para páginas solo imagen y `pdfunite` para unirlas:
  1. convenio con texto y cierre en palabras;
  2. nombramiento con fecha en la página 1 y cierre al final;
  3. cláusula recortada (la de `vigencia_texto` difiere al final);
  4. cláusula que cruza de página;
  5. escaneado transcrito;
  6. escaneado sin transcribir;
  7. mixto;
  8. firmas a dos columnas;
  9. sin cláusula guardada;
  10. página con `/Rotate 90` (agregar `/Rotate 90` al diccionario de la página con `sed`; PDF.js reconstruye la tabla xref).
- Si la carpeta del entorno se borró, recrearla con el mismo esquema: copia de `frontend/dist` más un `stub.js` que simule `window.go.main.App` con un `Proxy`.

- [ ] **Step 5: Verificar con playwright-cli** (desde `~/admin`, servidor en 8765), con captura por caso, revisadas a mano:
  - Casos 1 y 2: «posible cierre», recuadro sobre el cierre; en el caso 2, «1 de 2» y el segundo candidato es el nombramiento.
  - Casos 3 y 4: «parte de la cláusula», recuadro sobre el tramo.
  - Caso 5: página sin recuadro, frase transcrita arriba y el aviso de transcripción.
  - Caso 6: «No se encontró el cierre…» más «Hay N páginas escaneadas sin transcribir.».
  - Caso 8: si el cierre está en el bloque a dos columnas, sin recuadro.
  - Caso 9: «No hay cláusula de vigencia guardada».
  - Caso 10: recuadro sobre el texto rotado.
  - **Review Focus:**
    - Escribir en `#f-vigencia-texto` sin guardar y hacer clic en el ícono: busca el texto nuevo.
    - Hacer clic en Fecha de firma e inmediatamente en Cláusula: solo se ve el resultado de la cláusula.
    - Apuntar `/pdf/<id>` a un archivo dañado: aviso «No se pudo abrir el PDF para revisarlo», y «Volver al PDF completo» regresa al iframe.
    - Volver a la lista y abrir otro convenio: la vista está cerrada.
  - Criterio: ningún recuadro sobre un lugar equivocado.

- [ ] **Step 6: Correr las pruebas**

Run: `~/.cache/tg/node/bin/node --test frontend/tests/revision.test.js && ~/.local/go/bin/go test ./...`
Expected: PASS en las dos

- [ ] **Step 7: Commit**

```bash
git add frontend/dist/app.js
git commit -m "feat: vista de revision puntual de la vigencia en la ficha"
```

### Task 7: Documentación y versión 1.4.0

**Files:**
- Modify: `frontend/dist/EGM.md` (Overview e Interface: `revision.js` y su global `Revision`; Invariants: los dos estados del recuadro, nunca recuadro en transcritas, destruir el documento de PDF.js al cerrar la vista; Verification: `node --test frontend/tests/revision.test.js`)
- Modify: `EGM.md` (sección de verificación: las pruebas de la interfaz corren en CI)
- Modify: `CHANGELOG.md` (entrada `## [1.4.0] - <fecha>` con «### Agregado»: qué hace el botón, qué busca cada campo, que en páginas escaneadas solo indica la página y que funciona sin volver a analizar)
- Modify: `VERSION`, `app.go:33` (`appVersion`), `wails.json:15` (`productVersion`) → `1.4.0`

- [ ] **Step 1: Escribir la documentación y subir la versión** con el estilo de la entrada 1.3.1 del CHANGELOG (frases cortas, negritas al inicio de cada punto).

- [ ] **Step 2: Verificar**

Run: `~/.local/go/bin/go test ./... && ~/.cache/tg/node/bin/node --test frontend/tests/revision.test.js && grep -c 1.4.0 VERSION app.go wails.json`
Expected: PASS en las dos y `1` en cada archivo

- [ ] **Step 3: Commit**

```bash
git add frontend/dist/EGM.md EGM.md CHANGELOG.md VERSION app.go wails.json
git commit -m "docs: revision puntual de la vigencia, version 1.4.0"
```

No se une a `main` en este plan: al terminar se pregunta al usuario. La prueba con unos 12 convenios reales en la PC (criterio: ningún recuadro sobre un lugar equivocado) la hace el usuario con el ejecutable de la rama o del release, según decida al unir.
