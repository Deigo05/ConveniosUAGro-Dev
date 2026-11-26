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

/* ---------- Recuadros ---------- */

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

/* ---------- Párrafos (encontrado en el entorno de prueba) ---------- */

test('un salto de párrafo termina la oración aunque falte el punto', () => {
  const ix = R.buildPageIndex([it('lo firman el 15 de marzo de 2024', 72, 300, true), it('Por la UAGro', 72, 270)]);
  const c = R.findClosings(ix);
  assert.equal(ix.text.slice(c[0].start, c[0].end), 'lo firman el 15 de marzo de 2024');
});

test('un renglón separado por un espacio de párrafo no se une al recuadro', () => {
  const ix = R.buildPageIndex([{ str: 'veinticuatro', transform: [10, 0, 0, 10, 72, 300], width: 60, height: 10 }, { str: 'Por la UAGro', transform: [10, 0, 0, 10, 72, 279], width: 60, height: 10 }]);
  const a = ix.text.indexOf('veinticuatro');
  assert.equal(R.matchRects(ix, a, ix.text.indexOf('uagro') + 5), null);
});

test('con una medida de texto, el recuadro usa el ancho real de las letras', () => {
  // Mayúsculas miden el doble: «AB cd» = 2+2+1+1+1 = 7 unidades en 70 pt.
  const ix = R.buildPageIndex([{ str: 'AB cd', transform: [10, 0, 0, 10, 0, 0], width: 70, height: 10 }]);
  const measure = (item, s) => [...s].reduce((n, ch) => n + (/[A-Z]/.test(ch) ? 2 : 1), 0);
  const a = ix.text.indexOf('cd');
  const r = R.matchRects(ix, a, a + 2, measure);
  assert.deepEqual([r[0].x0, r[0].x1], [50, 70]);
});

/* ---------- Correcciones de la revisión final ---------- */

// Renglones de un párrafo a una distancia fija entre sí.
const lines = (texts, step, y0 = 700) => texts.map((t, i) => it(t, 72, y0 - i * step, true));

test('interlineado de 1.5 (1.83 veces la letra): los renglones no son párrafos', () => {
  const ix = R.buildPageIndex(lines(['Las partes acuerdan colaborar', 'lo firman en Chilpancingo a los', 'quince días del mes de marzo', 'de dos mil veinticuatro.'], 18.3));
  const c = R.findClosings(ix);
  assert.equal(c.length, 1);
  assert.equal(R.matchRects(ix, c[0].start, c[0].end).length, 3);
});

test('con renglones normales, un hueco de firmas a 2.5 renglones sigue cortando', () => {
  const items = lines(['uno dos tres', 'cuatro cinco seis', 'lo firman el 15 de marzo de 2024'], 11.5);
  items.push(it('Por la UAGro', 72, 700 - 2 * 11.5 - 25));
  const ix = R.buildPageIndex(items);
  const c = R.findClosings(ix);
  assert.equal(ix.text.slice(c[0].start, c[0].end), 'lo firman el 15 de marzo de 2024');
});

test('una vigencia distinta no cae como «parte de la cláusula» sobre otra cláusula', () => {
  const term = 'pudiendo darse por terminado anticipadamente por cualquiera de las partes mediante aviso por escrito con treinta días de anticipación';
  const pages = [pg(4, 'DÉCIMA PRIMERA. TERMINACIÓN. El convenio podrá ' + term + ' sin responsabilidad.')];
  const vig = 'El presente convenio tendrá vigencia de tres años a partir de su firma, ' + term;
  assert.deepEqual(R.findClause(pages, vig), []);
});

test('los tratamientos (Lic., Dr.) no parten el cierre', () => {
  const ix = idx('Leído que fue el presente convenio, lo firman el Lic. Juan Pérez y el Dr. Javier Saldaña el 15 de marzo de 2024.');
  assert.equal(R.findClosings(ix).length, 1);
});

test('una coincidencia de la columna derecha al renglón de abajo en la izquierda no lleva recuadro', () => {
  const ix = R.buildPageIndex([it('Por la UAGro', 72, 200), it('Por la Contraparte', 330, 200, true), it('Dr. Javier', 72, 186)]);
  assert.equal(R.matchRects(ix, ...find(ix, 'contraparte dr javier')), null);
});

test('un párrafo con sangría en el primer renglón sí lleva recuadro', () => {
  const ix = R.buildPageIndex([it('DÉCIMA. El presente', 108, 300, true), it('convenio tendrá vigencia', 72, 286)]);
  assert.equal(R.matchRects(ix, ...find(ix, 'presente convenio')).length, 2);
});

/* ---------- Marcas de fecha y plazo ---------- */

const marks = (s, kind, from, to) => {
  const ix = idx(s);
  return R.findMarks(ix, from ?? 0, to ?? ix.text.length, kind).map(m => ix.text.slice(m.start, m.end));
};

test('firma: fecha en letra con números entre paréntesis y año en letra', () => {
  assert.deepEqual(
    marks('a los veinticinco (25) días del mes de noviembre del año dos mil veinticinco (2025).', 'firma'),
    ['veinticinco 25 dias del mes de noviembre del ano dos mil veinticinco 2025']);
});

test('firma: fecha en número con año', () => {
  assert.deepEqual(marks('Chilpancingo, Gro., 25 de noviembre de 2025', 'firma'), ['25 de noviembre de 2025']);
});

test('firma: fecha con diagonales', () => {
  assert.deepEqual(marks('firmado el 25/11/2025', 'firma'), ['25 11 2025']);
});

test('firma: los plazos no se marcan', () => {
  assert.deepEqual(marks('por tres (3) años, el 1o. de enero de 2025', 'firma'), ['1o de enero de 2025']);
});

test('vigencia: plazo y fecha límite', () => {
  assert.deepEqual(
    marks('tendrá una vigencia de tres (3) años y estará vigente hasta el 31 de diciembre de 2027.', 'vigencia'),
    ['tres 3 anos', '31 de diciembre de 2027']);
});

test('vigencia: plazo en meses e indefinido', () => {
  assert.deepEqual(marks('una vigencia de 36 meses', 'vigencia'), ['36 meses']);
  assert.deepEqual(marks('tendrá vigencia por tiempo indefinido', 'vigencia'), ['tiempo indefinido']);
  assert.deepEqual(marks('su vigencia será indefinida', 'vigencia'), ['indefinida']);
});

test('vigencia: treinta y uno en letra', () => {
  assert.deepEqual(marks('hasta el treinta y uno de diciembre de dos mil veintisiete', 'vigencia'),
    ['treinta y uno de diciembre de dos mil veintisiete']);
});

test('vigencia: los avisos de terminación no se marcan', () => {
  assert.deepEqual(marks('dando aviso con 30 días naturales de anticipación', 'vigencia'), []);
  assert.deepEqual(marks('dando aviso con tres meses de anticipación', 'vigencia'), []);
  assert.deepEqual(marks('avisando dos meses antes de su terminación', 'vigencia'), []);
});

test('las marcas no salen del tramo pedido', () => {
  const s = 'el 1 de enero de 2020. Lo firman a los 25 días del mes de noviembre de 2025';
  const ix = idx(s);
  const from = ix.text.indexOf('lo firman');
  assert.deepEqual(marks(s, 'firma', from), ['25 dias del mes de noviembre de 2025']);
  const cut = ix.text.indexOf(' de 2025');
  assert.deepEqual(marks(s, 'firma', from, cut), ['25 dias del mes de noviembre']);
});
