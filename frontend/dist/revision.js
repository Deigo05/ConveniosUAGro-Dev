// Revisión puntual: funciones puras para ubicar en el texto de cada página el
// cierre del convenio y la cláusula de vigencia, y para convertir lo encontrado
// en recuadros. No tocan la pantalla: app.js las usa con el texto que da PDF.js
// o con la transcripción guardada. En Node se cargan con require() para las
// pruebas (frontend/tests/revision.test.js).
(function () {
  'use strict';

  // Página con texto: la misma regla que transcribeConvention usa para decidir
  // qué páginas están escaneadas.
  const MIN_PAGE_CHARS = 30;

  const isWordChar = c => /[\p{L}\p{N}]/u.test(c);

  function itemHeight(item) {
    return item.height || Math.abs((item.transform || [])[3] || 0);
  }

  // Entre dos fragmentos de PDF.js sin salto de renglón no siempre viene un
  // espacio. Se agrega uno virtual si hay una separación visible o si el
  // segundo fragmento está en otro renglón.
  function needsGap(prev, next) {
    if (!prev.transform || !next.transform) return false;
    const h = itemHeight(prev) || itemHeight(next);
    if (!h) return false;
    if (Math.abs(next.transform[5] - prev.transform[5]) > 0.5 * h) return true;
    return next.transform[4] - (prev.transform[4] + (prev.width || 0)) > 0.15 * h;
  }

  // Separación vertical mayor que 1.8 renglones: cambio de párrafo. Termina la
  // oración aunque no haya punto (un cierre sin punto antes de las firmas) y
  // corta el recuadro.
  const PARAGRAPH_GAP = 1.8;

  function isParagraphBreak(prev, next) {
    if (!prev.transform || !next.transform) return false;
    const h = itemHeight(prev) || itemHeight(next);
    return h > 0 && prev.transform[5] - next.transform[5] > PARAGRAPH_GAP * h;
  }

  // Un punto termina la oración solo si lo que sigue empieza con mayúscula o es
  // el final: así «Chilpancingo, Gro., a los quince días» no se parte en dos.
  function endsSentence(units, u) {
    for (let j = u + 1; j < units.length; j++) {
      const c = units[j].ch;
      if (/\s/.test(c)) continue;
      return /\p{Lu}/u.test(c);
    }
    return true;
  }

  // Guion de corte: «fir-» al final del renglón seguido de «man». Devuelve el
  // índice de la primera letra del renglón siguiente, o -1 si no es corte.
  function hyphenBreak(units, u) {
    let j = u + 1;
    while (j < units.length && (units[j].ch === ' ' || units[j].ch === '\t')) j++;
    if (j >= units.length || (units[j].ch !== '\n' && units[j].ch !== '\r')) return -1;
    while (j < units.length && /\s/.test(units[j].ch)) j++;
    return j < units.length && /\p{Ll}/u.test(units[j].ch) ? j : -1;
  }

  // Construye el texto normalizado de una página (minúsculas, sin acentos, sin
  // signos, espacios unificados) y, para cada carácter, de qué fragmento y
  // posición original salió.
  function buildPageIndex(items) {
    items = items || [];
    const units = [];
    items.forEach((item, i) => {
      const s = String(item.str || '');
      if (i > 0 && isParagraphBreak(items[i - 1], item)) {
        units.push({ ch: '\n', item: i - 1, offset: String(items[i - 1].str || '').length, paragraph: true });
      } else if (i > 0 && !items[i - 1].hasEOL && needsGap(items[i - 1], item)) {
        units.push({ ch: ' ', item: i - 1, offset: String(items[i - 1].str || '').length });
      }
      for (let k = 0; k < s.length; k++) units.push({ ch: s[k], item: i, offset: k });
      if (item.hasEOL) units.push({ ch: '\n', item: i, offset: s.length });
    });

    let text = '';
    const pos = [];
    const sentenceEnds = [];
    let lastSpace = true;
    for (let u = 0; u < units.length; u++) {
      const { ch, item, offset, paragraph } = units[u];
      if (paragraph && text.trimEnd().length && sentenceEnds[sentenceEnds.length - 1] !== text.trimEnd().length) {
        sentenceEnds.push(text.trimEnd().length);
      }
      if (ch === '-' || ch === '­') {
        const next = hyphenBreak(units, u);
        if (next >= 0) {
          u = next - 1;
          continue;
        }
      }
      if (ch === '.' && endsSentence(units, u) && sentenceEnds[sentenceEnds.length - 1] !== text.length) {
        sentenceEnds.push(text.length);
      }
      const norm = ch.normalize('NFKD').replace(/[̀-ͯ]/g, '').toLowerCase();
      for (const c of norm) {
        if (isWordChar(c)) {
          text += c;
          pos.push({ item, offset });
          lastSpace = false;
        } else if (!lastSpace) {
          text += ' ';
          pos.push({ item, offset });
          lastSpace = true;
        }
      }
    }
    return { text, pos, sentenceEnds, items };
  }

  function normalizeForSearch(s) {
    return buildPageIndex([{ str: String(s || '') }]).text.trim();
  }

  function pageChars(items) {
    return (items || []).reduce((n, item) => n + String(item.str || '').replace(/\s/g, '').length, 0);
  }

  // GetTranscription devuelve el texto del PDF y, después, un bloque por cada
  // página transcrita que empieza con «[Página N]».
  function splitTranscription(texto) {
    const pages = new Map();
    const re = /^\[Página (\d+)\]\n?/gm;
    const s = String(texto || '');
    const marks = [...s.matchAll(re)];
    marks.forEach((m, i) => {
      const end = i + 1 < marks.length ? marks[i + 1].index : s.length;
      pages.set(Number(m[1]), s.slice(m.index + m[0].length, end).trim());
    });
    return pages;
  }

  // Texto original (con acentos y mayúsculas) que corresponde a text[start..end).
  function snippet(index, start, end) {
    if (!(end > start) || !index.pos[start] || !index.pos[end - 1]) return '';
    const a = index.pos[start];
    const b = index.pos[end - 1];
    const parts = [];
    for (let i = a.item; i <= b.item; i++) {
      const s = String(index.items[i].str || '');
      parts.push(s.slice(i === a.item ? a.offset : 0, i === b.item ? b.offset + 1 : s.length));
    }
    return parts.join(' ').replace(/\s+/g, ' ').trim();
  }

  // Apariciones de una frase normalizada que empiezan y terminan en límite de
  // palabra («marzo» no coincide dentro de «marzoxx»).
  function occurrences(text, phrase) {
    const out = [];
    if (!phrase) return out;
    for (let i = text.indexOf(phrase); i >= 0; i = text.indexOf(phrase, i + 1)) {
      const e = i + phrase.length;
      if ((i === 0 || text[i - 1] === ' ') && (e === text.length || text[e] === ' ')) out.push(i);
    }
    return out;
  }

  // Oración que contiene la posición `at`, sin espacios en los bordes.
  function sentenceBounds(index, at) {
    const text = index.text;
    let s = 0;
    let e = text.length;
    for (const b of index.sentenceEnds) {
      if (b <= at) s = b;
      else { e = b; break; }
    }
    while (s < e && text[s] === ' ') s++;
    while (e > s && text[e - 1] === ' ') e--;
    return [s, e];
  }

  /* ---------- Cierre ---------- */

  // Fórmulas con que suele empezar o cerrar la oración de firma, ya normalizadas.
  const CLOSING_FORMULAS = [
    'dias del mes de', 'dia del mes de', 'lo firman', 'la firman', 'firman por',
    'firman de conformidad', 'firmado en', 'firmada en', 'se firma', 'suscriben',
    'para constancia', 'firmas que anteceden'
  ];
  const DATE_PATTERNS = [
    /\b(?:\d{1,2} (?:de )?)?(?:enero|febrero|marzo|abril|mayo|junio|julio|agosto|septiembre|setiembre|octubre|noviembre|diciembre)\b/,
    /\b\d{1,2} \d{1,2} \d{2,4}\b/
  ];
  const MAX_CLOSING_WORDS = 80;

  // Oraciones de una página que tienen una fórmula de cierre y una fecha. Cada
  // candidato va desde la primera fórmula (o la fecha, si va antes) hasta el
  // final de la oración, con un máximo de 80 palabras.
  function findClosings(index) {
    const text = index.text;
    const bySentence = new Map();
    for (const f of CLOSING_FORMULAS) {
      for (const i of occurrences(text, f)) {
        const [s, e] = sentenceBounds(index, i);
        const cur = bySentence.get(s);
        if (!cur || i < cur.formula) bySentence.set(s, { s, e, formula: i });
      }
    }
    const out = [];
    for (const { s, e, formula } of [...bySentence.values()].sort((a, b) => a.s - b.s)) {
      const sentence = text.slice(s, e);
      const dates = DATE_PATTERNS.map(re => sentence.search(re)).filter(i => i >= 0);
      if (!dates.length) continue;
      const start = Math.min(formula, s + Math.min(...dates));
      const words = text.slice(start, e).split(' ');
      const end = words.length > MAX_CLOSING_WORDS ? start + words.slice(0, MAX_CLOSING_WORDS).join(' ').length : e;
      out.push({ start, end });
    }
    return out;
  }

  /* ---------- Cláusula de vigencia ---------- */

  // Largo mínimo del tramo de la cláusula que se acepta cuando no aparece
  // completa (la IA la recorta y puede cruzar de página).
  const MIN_CLAUSE_WORDS = 12;

  // Busca la cláusula guardada en las páginas. Si aparece completa, devuelve
  // todas sus apariciones. Si no, el tramo más largo de palabras seguidas (12 o
  // más) que aparezca exactamente una vez en todo el documento.
  function findClause(pages, clause) {
    const q = normalizeForSearch(clause);
    if (!q) return [];
    const full = [];
    for (const { page, index } of pages) {
      for (const start of occurrences(index.text, q)) full.push({ page, start, end: start + q.length, complete: true });
    }
    if (full.length) return full;
    const words = q.split(' ');
    for (let len = words.length - 1; len >= MIN_CLAUSE_WORDS; len--) {
      for (let i = 0; i + len <= words.length; i++) {
        const phrase = words.slice(i, i + len).join(' ');
        const hits = [];
        for (const { page, index } of pages) {
          for (const start of occurrences(index.text, phrase)) hits.push({ page, start, end: start + phrase.length, complete: false });
          if (hits.length > 1) break;
        }
        if (hits.length === 1) return hits;
      }
    }
    return [];
  }

  /* ---------- Recuadros ---------- */

  // Convierte text[start..end) en rectángulos (coordenadas del PDF, un
  // rectángulo por renglón). Devuelve null cuando la coincidencia no es un
  // tramo contiguo y legible en la página: texto girado, fragmentos sin
  // posición (transcripción), saltos entre columnas o texto que sube. En esos
  // casos la vista no dibuja recuadro: nunca uno aproximado.
  // `measure(item, s)` (opcional) da el ancho de un texto con la letra del
  // fragmento; sin ella se supone que todas las letras miden lo mismo.
  function matchRects(index, start, end, measure) {
    const widthOf = (item, s) => (measure ? measure(item, s) : s.length);
    if (!(end > start)) return null;
    const a = index.pos[start];
    const b = index.pos[end - 1];
    if (!a || !b) return null;
    const lines = [];
    let line = null;
    let prev = null;
    for (let i = a.item; i <= b.item; i++) {
      const item = index.items[i];
      const s = String(item.str || '');
      if (!s.trim()) continue;
      const t = item.transform;
      if (!t || t[1] !== 0 || t[2] !== 0) return null;
      const h = itemHeight(item);
      if (!h) return null;
      const x = t[4];
      const y = t[5];
      const w = item.width || 0;
      const from = i === a.item ? a.offset : 0;
      const to = i === b.item ? Math.min(b.offset + 1, s.length) : s.length;
      const full = widthOf(item, s) || s.length;
      const x0 = x + (w * widthOf(item, s.slice(0, from))) / full;
      const x1 = x + (w * widthOf(item, s.slice(0, to))) / full;
      if (!line) {
        line = { x0, x1, y, h, startX: x };
      } else {
        const dy = prev.y - y;
        const gap = x - prev.end;
        if (Math.abs(dy) <= 0.5 * h && gap >= -0.5 * h && gap <= 3 * h) {
          line.x1 = Math.max(line.x1, x1);
          line.h = Math.max(line.h, h);
        } else if (dy > 0.5 * h && dy <= PARAGRAPH_GAP * h && x <= line.startX + 3 * h) {
          lines.push(line);
          line = { x0, x1, y, h, startX: x };
        } else {
          return null;
        }
      }
      prev = { y, end: x + w };
    }
    if (!line) return null;
    lines.push(line);
    return lines.map(l => ({ x0: l.x0, y0: l.y - 0.25 * l.h, x1: l.x1, y1: l.y + l.h }));
  }

  const Revision = {
    MIN_PAGE_CHARS,
    MIN_CLAUSE_WORDS,
    matchRects,
    findClause,
    CLOSING_FORMULAS,
    findClosings,
    occurrences,
    buildPageIndex,
    normalizeForSearch,
    pageChars,
    splitTranscription,
    snippet
  };

  if (typeof module !== 'undefined' && module.exports) module.exports = Revision;
  else window.Revision = Revision;
})();
