// Pruebas de las funciones puras de la revisión puntual (frontend/dist/revision.js).
// Se corren con: node --test frontend/tests/revision.test.js
const test = require('node:test');
const assert = require('node:assert/strict');
const R = require('../dist/revision.js');

/* ---------- Índice de página y normalización ---------- */

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

test('el punto de una abreviatura seguida de minúscula no termina la oración', () => {
  const idx = R.buildPageIndex([{ str: 'en Chilpancingo, Gro., a los quince días.' }]);
  assert.deepEqual(idx.sentenceEnds, [idx.text.trimEnd().length]);
});

/* ---------- Cierre ---------- */

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

/* ---------- Cláusula ---------- */

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
