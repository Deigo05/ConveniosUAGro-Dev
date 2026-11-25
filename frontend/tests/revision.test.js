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
