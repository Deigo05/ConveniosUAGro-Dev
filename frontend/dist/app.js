'use strict';

const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

const TYPE_ORDER = ['ambito', 'estado', 'ciudad', 'facultad', 'tema'];
const TYPE_LABEL = { ambito: 'Ámbito', estado: 'Estado', ciudad: 'Ciudad', facultad: 'Facultad', tema: 'Tema / Rubro' };
const PALETTE = [
  '#193060', '#2E5AAC', '#3A5A9C', '#5B7DB1', '#7A9CC6',
  '#2E7D32', '#43A047', '#6A994E', '#8D6E63', '#795548',
  '#9A5B00', '#D97706', '#C2410C', '#B42318', '#8E1B12',
  '#7B1FA2', '#5E35B1', '#455A64', '#6D6E71', '#3F4043'
];

const state = {
  view: 'convenios',
  tags: [],
  stats: {},
  config: {},
  app: {},
  filters: { query: '', tag_ids: [], vigencia: '', solo_pendientes: false, order: 'id', dir: 'desc', page: 1 },
  result: { items: [], total: 0, total_registrados: 0, page: 1, pages: 1 },
  selected: new Set(),
  current: null,
  pendingFiles: [],
  transcribing: null,
  autoQueue: [],
  autoRunning: false,
  autoTotal: 0,
  autoDone: 0,
  confirmResolve: null,
  searchTimer: null,
  tagTimer: null,
  loadingCount: 0
};

document.addEventListener('DOMContentLoaded', init);

async function init() {
  bindEvents();
  if (window.pdfjsLib) {
    window.pdfjsLib.GlobalWorkerOptions.workerSrc = 'vendor/pdf.worker.min.js';
  }
  renderPalette($('#tag-palette'), $('#tag-color'));
  renderPalette($('#tag-edit-palette'), $('#tag-edit-color'));
  showLoading(true);
  try {
    const b = await call('GetBootstrap');
    applyBootstrap(b);
    await loadList();
  } catch (err) {
    showToast('No se pudo iniciar la aplicación: ' + errorText(err), 'error');
    $('#list-area').innerHTML = errorPanel(errorText(err));
  } finally {
    showLoading(false);
  }
}

function api() {
  if (!window.go || !window.go.main || !window.go.main.App) {
    throw new Error('La interfaz no está conectada con la aplicación.');
  }
  return window.go.main.App;
}

async function call(name, ...args) {
  const fn = api()[name];
  if (typeof fn !== 'function') throw new Error('Método no disponible: ' + name);
  return await fn(...args);
}

function bindEvents() {
  $$('.nav__item').forEach(btn => btn.addEventListener('click', () => showView(btn.dataset.view)));
  $('#btn-settings-top').addEventListener('click', () => showView('ajustes'));
  $('#btn-import-top').addEventListener('click', openImport);
  $('#btn-import-confirm').addEventListener('click', confirmImport);

  $('#search').addEventListener('input', onSearch);
  $('#btn-search-clear').addEventListener('click', () => {
    $('#search').value = '';
    state.filters.query = '';
    state.filters.page = 1;
    updateSearchClear();
    scheduleLoad(0);
  });
  $('#filter-vigencia').addEventListener('change', e => {
    state.filters.vigencia = e.target.value;
    state.filters.page = 1;
    loadList();
  });
  $('#sort-select').addEventListener('change', e => {
    state.filters.order = e.target.value;
    state.filters.dir = e.target.value === 'dependencia' ? 'asc' : 'desc';
    state.filters.page = 1;
    loadList();
  });
  $('#btn-tag-filter').addEventListener('click', toggleTagFilter);
  $('#btn-pending').addEventListener('click', togglePending);
  $('#btn-clear-filters').addEventListener('click', clearFilters);

  $('#list-area').addEventListener('click', onListClick);
  $('#list-area').addEventListener('change', onListChange);
  $('#pagination').addEventListener('click', onPagination);
  $('#btn-bulk-clear').addEventListener('click', clearSelection);
  $('#btn-bulk-delete').addEventListener('click', bulkDelete);

  $('#detail-back').addEventListener('click', () => showView('convenios'));
  $('#form-edit').addEventListener('submit', saveConvenio);
  $('#btn-download').addEventListener('click', downloadCurrent);
  $('#btn-delete').addEventListener('click', deleteCurrent);
  $('#f-indefinida').addEventListener('change', syncIndefinite);
  $('#btn-save-tags').addEventListener('click', saveTags);
  $('#btn-no-tags').addEventListener('click', leaveWithoutTags);
  $('#btn-accept-suggestions').addEventListener('click', acceptSuggestions);
  $('#btn-discard-suggestions').addEventListener('click', discardSuggestions);
  $('#btn-ai').addEventListener('click', extractAI);
  $('#btn-transcribe').addEventListener('click', transcribeCurrent);
  $('#btn-download-transcription').addEventListener('click', downloadTranscription);
  $('#btn-new-tag-detail').addEventListener('click', () => {
    showView('etiquetas');
    setTimeout(() => $('#tag-name')?.focus(), 50);
  });
  $('#tags-selector').addEventListener('change', onTagSelectorChange);

  $('#tag-create-form').addEventListener('submit', createTag);
  $('#tag-color').addEventListener('input', e => {
    $('#tag-color-value').textContent = e.target.value.toUpperCase();
    markPalette($('#tag-palette'), e.target.value);
  });
  $('#tags-groups').addEventListener('click', onTagsGroupsClick);
  $('#btn-tag-save').addEventListener('click', saveTagEdit);
  $('#tag-edit-form').addEventListener('submit', e => e.preventDefault());
  $('#tag-edit-color').addEventListener('input', e => {
    $('#tag-edit-color-value').textContent = e.target.value.toUpperCase();
    markPalette($('#tag-edit-palette'), e.target.value);
  });

  $('#settings-form').addEventListener('submit', saveSettings);
  $('#btn-clear-key').addEventListener('click', clearKey);
  $('#btn-open-data').addEventListener('click', openDataFolder);
  $('#btn-backup').addEventListener('click', backupData);

  $('#confirm-cancel').addEventListener('click', () => closeConfirm(false));
  $('#confirm-ok').addEventListener('click', () => closeConfirm(true));
  $('#dialog-confirm').addEventListener('cancel', e => { e.preventDefault(); closeConfirm(false); });

  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      if ($('#dialog-confirm').open) closeConfirm(false);
      if ($('#dialog-import').open) $('#dialog-import').close();
      if ($('#dialog-tag').open) $('#dialog-tag').close();
    }
    const target = e.target;
    const typing = target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT');
    if (!typing && e.key === '/') {
      e.preventDefault();
      if (state.view !== 'convenios') showView('convenios');
      $('#search')?.focus();
    }
    if (!typing && (e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'n') {
      e.preventDefault();
      openImport();
    }
  });

  window.addEventListener('unhandledrejection', e => {
    console.error(e.reason);
    showToast(errorText(e.reason), 'error');
  });
}

function applyBootstrap(b) {
  state.tags = b.tags || [];
  state.stats = b.stats || {};
  state.config = b.config || {};
  state.app = b.app || {};
  renderStats();
  renderTagFilter();
  renderNav();
  renderSettings();
}

function renderNav() {
  const total = state.stats.total || 0;
  $('#nav-count').textContent = total;
  $('#nav-count').hidden = total === 0;
  document.title = 'Convenios UAGro';
}

function renderStats() {
  const s = state.stats || {};
  $('#stats-total').textContent = s.total || 0;
  $('#stats-vigentes').textContent = s.vigentes || 0;
  $('#stats-por-vencer').textContent = s.por_vencer || 0;
  $('#stats-vencidos').textContent = s.vencidos || 0;
  const pendientes = s.etiquetas_pendientes || 0;
  $('#pending-count').textContent = pendientes;
  $('#btn-pending').hidden = pendientes === 0 && !state.filters.solo_pendientes;
  $('#btn-pending').classList.toggle('is-active', Boolean(state.filters.solo_pendientes));
}

async function loadList() {
  const area = $('#list-area');
  area.setAttribute('aria-busy', 'true');
  try {
    const res = await call('ListConvenios', state.filters);
    state.result = res;
    state.selected.clear();
    renderList();
    renderSummary();
    renderPagination();
    updateSearchClear();
    updateClearFilters();
    updateBulkBar();
  } catch (err) {
    area.innerHTML = errorPanel(errorText(err));
    showToast(errorText(err), 'error');
  } finally {
    area.removeAttribute('aria-busy');
  }
}

function scheduleLoad(ms = 240) {
  clearTimeout(state.searchTimer);
  state.searchTimer = setTimeout(loadList, ms);
}

function onSearch(e) {
  state.filters.query = e.target.value.trimStart();
  state.filters.page = 1;
  updateSearchClear();
  scheduleLoad();
}

function updateSearchClear() {
  $('#btn-search-clear').hidden = !state.filters.query;
}

function updateClearFilters() {
  const active = Boolean(state.filters.query || state.filters.vigencia || state.filters.solo_pendientes || state.filters.tag_ids.length);
  $('#btn-clear-filters').hidden = !active;
  $('#tag-filter-count').hidden = state.filters.tag_ids.length === 0;
  $('#tag-filter-count').textContent = state.filters.tag_ids.length;
  $('#btn-pending').hidden = (state.stats.etiquetas_pendientes || 0) === 0 && !state.filters.solo_pendientes;
  $('#btn-pending').classList.toggle('is-active', Boolean(state.filters.solo_pendientes));
}

function clearFilters() {
  state.filters = { query: '', tag_ids: [], vigencia: '', solo_pendientes: false, order: state.filters.order, dir: state.filters.dir, page: 1 };
  $('#search').value = '';
  $('#filter-vigencia').value = '';
  $('#tag-filter-panel').hidden = true;
  $('#btn-tag-filter').setAttribute('aria-expanded', 'false');
  renderTagFilter();
  updateClearFilters();
  updateSearchClear();
  loadList();
}

function togglePending() {
  state.filters.solo_pendientes = !state.filters.solo_pendientes;
  state.filters.page = 1;
  updateClearFilters();
  loadList();
}

function toggleTagFilter() {
  const panel = $('#tag-filter-panel');
  const open = panel.hidden;
  panel.hidden = !open;
  $('#btn-tag-filter').setAttribute('aria-expanded', String(open));
  if (open) renderTagFilter();
}

function renderTagFilter() {
  const groups = $('#tag-filter-groups');
  if (!groups) return;
  groups.innerHTML = TYPE_ORDER.map(tipo => {
    const tags = state.tags.filter(t => t.tipo === tipo);
    return `<fieldset>
      <legend>${TYPE_LABEL[tipo]}</legend>
      <div class="tag-filter__group">
        ${tags.length ? tags.map(tag => `
          <label class="filter-tag">
            <input type="checkbox" data-filter-tag="${tag.id}" ${state.filters.tag_ids.includes(tag.id) ? 'checked' : ''}>
            <span style="width:8px;height:8px;border-radius:50%;background:${safeColor(tag.color)};flex:none"></span>
            <span>${esc(tag.nombre)}</span>
            <span class="muted">${tag.total || 0}</span>
          </label>`).join('') : '<span class="tag-filter__empty">Sin etiquetas.</span>'}
      </div>
    </fieldset>`;
  }).join('');
  $$('[data-filter-tag]', groups).forEach(input => {
    input.addEventListener('change', () => {
      const id = Number(input.dataset.filterTag);
      if (input.checked) {
        if (!state.filters.tag_ids.includes(id)) state.filters.tag_ids.push(id);
      } else {
        state.filters.tag_ids = state.filters.tag_ids.filter(x => x !== id);
      }
      state.filters.page = 1;
      updateClearFilters();
      clearTimeout(state.tagTimer);
      state.tagTimer = setTimeout(loadList, 180);
    });
  });
}

function renderList() {
  const area = $('#list-area');
  const items = state.result.items || [];
  if (!items.length) {
    const filtered = state.filters.query || state.filters.tag_ids.length || state.filters.vigencia || state.filters.solo_pendientes;
    area.innerHTML = filtered ? `
      <div class="empty-state empty-filter">
        <h2>No hay convenios con esos filtros</h2>
        <p>Prueba con otras palabras o quita algunos filtros.</p>
        <button class="btn btn--outline" id="empty-clear">Limpiar filtros</button>
      </div>` : `
      <div class="empty-state">
        <div class="empty-state__art" aria-hidden="true">
          <svg viewBox="0 0 24 24"><path d="M6 3h8l4 4v14H6V3Z"/><path d="M14 3v5h5M9 13h6M9 17h6"/></svg>
        </div>
        <h2>Tu biblioteca de convenios está vacía</h2>
        <p>Importa el primer PDF para empezar. La aplicación guarda todo en este equipo y no incluye datos de ejemplo.</p>
        <button class="btn btn--primary" id="empty-import">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 16V4m0 0L8 8m4-4 4 4M5 20h14"/></svg>
          Importar mi primer PDF
        </button>
      </div>`;
    $('#empty-clear')?.addEventListener('click', clearFilters);
    $('#empty-import')?.addEventListener('click', openImport);
    return;
  }

  area.innerHTML = `<div class="table-wrap"><table class="data-table">
    <thead><tr>
      <th class="col-check"><input type="checkbox" class="checkbox" id="select-all" aria-label="Seleccionar todos"></th>
      <th data-sort="dependencia">Dependencia <span class="sort-mark">${sortMark('dependencia')}</span></th>
      <th data-sort="fecha_firma">Firma <span class="sort-mark">${sortMark('fecha_firma')}</span></th>
      <th data-sort="fecha_vigencia">Vigencia <span class="sort-mark">${sortMark('fecha_vigencia')}</span></th>
      <th>Estado</th>
      <th>Documento</th>
      <th></th>
    </tr></thead>
    <tbody>${items.map(rowHTML).join('')}</tbody>
  </table></div>`;
  $('#select-all')?.addEventListener('change', onSelectAll);
  $$('[data-sort]', area).forEach(th => th.addEventListener('click', () => {
    const field = th.dataset.sort;
    if (state.filters.order === field) {
      state.filters.dir = state.filters.dir === 'asc' ? 'desc' : 'asc';
    } else {
      state.filters.order = field;
      state.filters.dir = field === 'dependencia' ? 'asc' : 'desc';
    }
    $('#sort-select').value = ['id', 'dependencia', 'fecha_firma', 'fecha_vigencia'].includes(state.filters.order) ? state.filters.order : 'id';
    loadList();
  }));
  syncSelectionUI();
}

function sortMark(field) {
  if (state.filters.order !== field) return '↕';
  return state.filters.dir === 'asc' ? '↑' : '↓';
}

function rowHTML(c) {
  const tags = (c.etiquetas || []).map(tag => chipHTML(tag)).join('');
  const badges = [];
  if (state.config.ai_enabled && c.ia_estado === 'pendiente') badges.push('<span class="badge">Analizando…</span>');
  if (state.config.ai_enabled && (c.ia_estado === 'error' || c.ia_estado === 'omitido') && !c.etiquetas_revisadas) {
    // "omitido" es lo normal en captura manual; no lo mostramos como error.
  }
  if (!c.etiquetas_revisadas) badges.push('<span class="badge badge--amber">Etiquetas pendientes</span>');
  if (c.transcripcion_estado === 'en_proceso') {
    badges.push('<span class="badge badge--blue">Transcribiendo…</span>');
  } else if (c.transcripcion_estado === 'completa') {
    badges.push('<span class="badge badge--green">Texto transcrito</span>');
  } else if (c.transcripcion_estado === 'error') {
    badges.push('<span class="badge badge--red">Transcripción incompleta</span>');
  } else if (c.transcripcion_estado === 'pendiente' && c.paginas_por_transcribir > 0) {
    badges.push('<span class="badge" title="Las páginas escaneadas aún no se pueden buscar">Transcripción pendiente</span>');
  } else if (c.es_escaneado) {
    badges.push('<span class="badge" title="El PDF no tiene texto seleccionable">Sin texto</span>');
  }
  if (c.vigencia_calculada) badges.push('<span class="badge badge--blue">Calculada</span>');
  const selected = state.selected.has(c.id);
  return `<tr data-open="${c.id}" class="${selected ? 'is-selected' : ''}">
    <td class="col-check"><input type="checkbox" class="checkbox row-check" data-select="${c.id}" ${selected ? 'checked' : ''} aria-label="Seleccionar ${esc(c.dependencia || c.archivo_nombre_original)}"></td>
    <td class="cell-main">
      <div class="dep-name">${esc(c.dependencia || 'Sin capturar')}</div>
      <div class="dep-file" title="${esc(c.archivo_nombre_original)}">${esc(c.archivo_nombre_original)}</div>
      <div class="row-badges">${badges.join('')}</div>
      <div class="chips chip-list">${tags || '<span class="empty-chips">Sin etiquetas</span>'}</div>
    </td>
    <td class="cell-date">${formatDate(c.fecha_firma) || '—'}</td>
    <td class="cell-date">${c.vigencia_indefinida ? '<span class="muted">Indefinida</span>' : (formatDate(c.fecha_vigencia) || '—')}</td>
    <td>${stateBadge(c.estado)}</td>
    <td><button class="btn btn--text btn--small" data-download="${c.id}" title="Descargar PDF"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 4v11m0 0 4-4m-4 4-4-4M5 20h14"/></svg> PDF</button></td>
    <td class="cell-actions"><button class="btn btn--outline btn--small" data-open-btn="${c.id}">Abrir</button></td>
  </tr>`;
}

function stateBadge(estado) {
  const map = {
    vigente: ['Vigente', 'badge--green'],
    por_vencer: ['Por vencer', 'badge--amber'],
    vencido: ['Vencido', 'badge--red'],
    indefinida: ['Indefinida', 'badge--blue'],
    sin_fecha: ['Sin fecha', '']
  };
  const [label, cls] = map[estado] || ['Sin fecha', ''];
  return `<span class="badge ${cls}">${label}</span>`;
}

function renderSummary() {
  const r = state.result;
  const filtered = Boolean(state.filters.query || state.filters.tag_ids.length || state.filters.vigencia || state.filters.solo_pendientes);
  $('#list-summary').textContent = filtered
    ? `${r.total} resultado${r.total === 1 ? '' : 's'} con los filtros actuales · ${r.total_registrados} en total`
    : `${r.total_registrados} convenio${r.total_registrados === 1 ? '' : 's'} registrado${r.total_registrados === 1 ? '' : 's'}`;
}

function renderPagination() {
  const r = state.result;
  const nav = $('#pagination');
  if (!r.total || r.pages <= 1) { nav.innerHTML = ''; return; }
  nav.innerHTML = `<button class="btn btn--outline btn--small" data-page="${r.page - 1}" ${r.page <= 1 ? 'disabled' : ''}>← Anterior</button>
    <span>Página ${r.page} de ${r.pages}</span>
    <button class="btn btn--outline btn--small" data-page="${r.page + 1}" ${r.page >= r.pages ? 'disabled' : ''}>Siguiente →</button>`;
}

function onPagination(e) {
  const btn = e.target.closest('[data-page]');
  if (!btn || btn.disabled) return;
  state.filters.page = Number(btn.dataset.page);
  loadList().then(() => window.scrollTo({ top: 0, behavior: 'smooth' }));
}

function onListClick(e) {
  const download = e.target.closest('[data-download]');
  if (download) {
    e.stopPropagation();
    downloadPDF(Number(download.dataset.download));
    return;
  }
  const openBtn = e.target.closest('[data-open-btn]');
  if (openBtn) {
    e.stopPropagation();
    openDetail(Number(openBtn.dataset.openBtn));
    return;
  }
  const row = e.target.closest('[data-open]');
  if (row && !e.target.closest('input')) openDetail(Number(row.dataset.open));
}

function onListChange(e) {
  const input = e.target.closest('[data-select]');
  if (input) {
    const id = Number(input.dataset.select);
    if (input.checked) state.selected.add(id); else state.selected.delete(id);
    input.closest('tr')?.classList.toggle('is-selected', input.checked);
    syncSelectionUI();
  }
}

function onSelectAll(e) {
  if (e.target.checked) {
    (state.result.items || []).forEach(c => state.selected.add(c.id));
  } else {
    state.selected.clear();
  }
  $$('.row-check').forEach(input => {
    input.checked = e.target.checked;
    input.closest('tr')?.classList.toggle('is-selected', e.target.checked);
  });
  syncSelectionUI();
}

function syncSelectionUI() {
  const count = state.selected.size;
  const all = state.result.items || [];
  const selectAll = $('#select-all');
  if (selectAll) {
    selectAll.checked = count > 0 && count === all.length;
    selectAll.indeterminate = count > 0 && count < all.length;
  }
  const bar = $('#bulk-bar');
  bar.hidden = count === 0;
  $('#bulk-count').textContent = `${count} convenio${count === 1 ? '' : 's'} seleccionado${count === 1 ? '' : 's'}`;
}

function updateBulkBar() { syncSelectionUI(); }

function clearSelection() {
  state.selected.clear();
  $$('.row-check').forEach(i => { i.checked = false; i.closest('tr')?.classList.remove('is-selected'); });
  syncSelectionUI();
}

async function bulkDelete() {
  const ids = Array.from(state.selected);
  if (!ids.length) return;
  const ok = await confirmDialog('Eliminar convenios', `¿Eliminar ${ids.length} convenio${ids.length === 1 ? '' : 's'}? También se borrarán sus PDF. Esta acción no se puede deshacer.`, 'Eliminar');
  if (!ok) return;
  showLoading(true);
  try {
    await call('DeleteConvenios', ids);
    state.selected.clear();
    await refreshData();
    await loadList();
    showToast('Convenios eliminados.', 'success');
  } catch (err) {
    showToast(errorText(err), 'error');
  } finally {
    showLoading(false);
  }
}

async function openDetail(id) {
  showLoading(true);
  try {
    const c = await call('GetConvenio', id);
    state.current = c;
    renderDetail();
    showView('detalle');
  } catch (err) {
    showToast(errorText(err), 'error');
  } finally {
    showLoading(false);
  }
}

function renderDetail() {
  const c = state.current;
  if (!c) return;
  const title = c.dependencia || 'Convenio sin capturar';
  $('#detail-title').textContent = title;
  $('#detail-state-badge').outerHTML = stateBadge(c.estado).replace('<span ', '<span id="detail-state-badge" ');
  $('#detail-subtitle').textContent = `${c.archivo_nombre_original} · ${c.paginas || '?'} página${c.paginas === 1 ? '' : 's'}${c.es_escaneado ? ' · PDF escaneado (sin texto seleccionable)' : ''}`;
  $('#pdf-caption').textContent = c.archivo_nombre_original;
  $('#pdf-badge').outerHTML = (c.es_escaneado ? '<span class="badge badge--amber" id="pdf-badge">Sin texto</span>' : '<span class="badge badge--green" id="pdf-badge">Texto indexado</span>');
  $('#pdf-frame').src = `/pdf/${c.id}?v=${Date.now()}`;

  $('#f-dependencia').value = c.dependencia || '';
  $('#f-firmante-dependencia').value = c.firmante_dependencia || '';
  $('#f-cargo-dependencia').value = c.cargo_dependencia || '';
  $('#f-firmante-uagro').value = c.firmante_uagro || '';
  $('#f-cargo-uagro').value = c.cargo_uagro || '';
  $('#f-facultad').value = c.facultad || '';
  $('#f-facultad-ciudad').value = c.facultad_ciudad || '';
  $('#f-firma').value = c.fecha_firma || '';
  $('#f-vigencia').value = c.fecha_vigencia || '';
  $('#f-vigencia-texto').value = c.vigencia_texto || '';
  $('#f-notas').value = c.notas || '';
  $('#f-indefinida').checked = Boolean(c.vigencia_indefinida);
  $('#f-calculada').hidden = !c.vigencia_calculada;
  syncIndefinite(false);
  $('#detail-classification').outerHTML = c.etiquetas_revisadas
    ? '<span class="badge badge--green" id="detail-classification">Clasificado</span>'
    : '<span class="badge badge--amber" id="detail-classification">Clasificación pendiente</span>';

  renderCurrentTags();
  renderSuggestions();
  renderTagSelector();
  renderAIBanner();
  renderAICard();
  renderTranscription();
  renderDetailMeta();
}

function renderDetailMeta() {
  const c = state.current;
  const rows = [
    ['Archivo', c.archivo_nombre_original],
    ['Páginas', String(c.paginas || 'No detectadas')],
    ['Tamaño', formatBytes(c.archivo_bytes)],
    ['Registrado', formatDateTime(c.creado_en)],
    ['Última edición', formatDateTime(c.actualizado_en)]
  ];
  $('#detail-meta').innerHTML = rows.map(([k, v]) => `<dt>${esc(k)}</dt><dd>${esc(v || '—')}</dd>`).join('');
}

function syncIndefinite(clear = true) {
  const checked = $('#f-indefinida').checked;
  $('#f-vigencia').disabled = checked;
  if (checked && clear) $('#f-vigencia').value = '';
}

function renderCurrentTags() {
  const c = state.current;
  const tags = c?.etiquetas || [];
  $('#tags-current').innerHTML = tags.length
    ? tags.map(t => chipHTML(t, { clickable: true })).join('')
    : '<span class="empty-chips">Este convenio no tiene etiquetas.</span>';
  $$('#tags-current .chip--button').forEach(chip => chip.addEventListener('click', () => {
    showView('convenios');
    const id = Number(chip.dataset.tagId);
    state.filters.tag_ids = [id];
    state.filters.page = 1;
    renderTagFilter();
    updateClearFilters();
    loadList();
  }));
}

function renderSuggestions() {
  const c = state.current;
  const sugs = c?.etiquetas_sugeridas || [];
  const box = $('#tags-suggestions');
  if (sugs.length) {
    box.hidden = false;
    $('#suggestions-chips').innerHTML = sugs.map(s => {
      const tag = state.tags.find(t => t.tipo === s.tipo && normalize(t.nombre) === normalize(s.nombre));
      return chipHTML({ nombre: s.nombre, tipo: s.tipo, color: tag?.color || colorForType(s.tipo) }, { new: s.nueva });
    }).join('');
  } else {
    box.hidden = true;
    $('#suggestions-chips').innerHTML = '';
  }
}

function renderTagSelector() {
  const c = state.current;
  const selected = new Set((c?.etiquetas || []).map(t => t.id));
  const html = TYPE_ORDER.map(tipo => {
    const tags = state.tags.filter(t => t.tipo === tipo);
    return `<fieldset>
      <legend>${TYPE_LABEL[tipo]}${tipo === 'ciudad' || tipo === 'facultad' ? ' · una sola' : ''}</legend>
      <div class="tag-selector__list">
        ${tags.length ? tags.map(t => `
          <label class="tag-selector__item">
            <input type="checkbox" data-select-tag="${t.id}" data-type="${t.tipo}" ${selected.has(t.id) ? 'checked' : ''}>
            <span style="width:9px;height:9px;border-radius:50%;background:${safeColor(t.color)};flex:none"></span>
            <span>${esc(t.nombre)}</span>
          </label>`).join('') : '<span class="tag-selector__empty">Sin etiquetas.</span>'}
      </div>
    </fieldset>`;
  }).join('');
  $('#tags-selector').innerHTML = `<h3>Seleccionar etiquetas</h3><div class="tag-selector__groups">${html}</div>`;
}

function onTagSelectorChange(e) {
  const input = e.target.closest('[data-select-tag]');
  if (!input || !input.checked) return;
  const type = input.dataset.type;
  if (type !== 'ambito' && type !== 'estado' && type !== 'ciudad' && type !== 'facultad') return;
  $$(`[data-select-tag][data-type="${type}"]`, $('#tags-selector')).forEach(other => {
    if (other !== input) other.checked = false;
  });
}

async function saveConvenio(e) {
  e.preventDefault();
  const c = state.current;
  if (!c) return;
  const in_ = {
    id: c.id,
    dependencia: $('#f-dependencia').value.trim(),
    firmante_dependencia: $('#f-firmante-dependencia').value.trim(),
    cargo_dependencia: $('#f-cargo-dependencia').value.trim(),
    firmante_uagro: $('#f-firmante-uagro').value.trim(),
    cargo_uagro: $('#f-cargo-uagro').value.trim(),
    facultad: $('#f-facultad').value.trim(),
    facultad_ciudad: $('#f-facultad-ciudad').value.trim(),
    fecha_firma: $('#f-firma').value,
    fecha_vigencia: $('#f-indefinida').checked ? '' : $('#f-vigencia').value,
    vigencia_indefinida: $('#f-indefinida').checked,
    vigencia_calculada: false,
    vigencia_texto: $('#f-vigencia-texto').value,
    notas: $('#f-notas').value
  };
  setButtonBusy($('#btn-save'), true, 'Guardando…');
  try {
    state.current = await call('SaveConvenio', in_);
    renderDetail();
    await refreshData();
    await loadList();
    showToast('Cambios guardados.', 'success');
  } catch (err) {
    showToast(errorText(err), 'error');
  } finally {
    setButtonBusy($('#btn-save'), false, 'Guardar cambios');
  }
}

async function saveTags() {
  const c = state.current;
  if (!c) return;
  const ids = $$('#tags-selector [data-select-tag]:checked').map(i => Number(i.dataset.selectTag));
  setButtonBusy($('#btn-save-tags'), true, 'Guardando…');
  try {
    state.current = await call('SaveConvenioTags', c.id, ids);
    renderDetail();
    await refreshData();
    await loadList();
    showToast('Etiquetas actualizadas.', 'success');
  } catch (err) {
    showToast(errorText(err), 'error');
  } finally {
    setButtonBusy($('#btn-save-tags'), false, 'Guardar etiquetas');
  }
}

async function leaveWithoutTags() {
  const c = state.current;
  if (!c) return;
  const ok = await confirmDialog('Dejar sin etiquetas', 'Se quitarán todas las etiquetas de este convenio y se marcará como clasificado. ¿Continuar?', 'Dejar sin etiquetas');
  if (!ok) return;
  try {
    state.current = await call('DiscardSuggestions', c.id, true);
    renderDetail();
    await refreshData();
    await loadList();
    showToast('El convenio quedó sin etiquetas.', 'success');
  } catch (err) { showToast(errorText(err), 'error'); }
}

async function acceptSuggestions() {
  const c = state.current;
  if (!c) return;
  try {
    state.current = await call('AcceptSuggestions', c.id);
    renderDetail();
    await refreshData();
    await loadList();
    showToast('Sugerencias aplicadas.', 'success');
  } catch (err) { showToast(errorText(err), 'error'); }
}

async function discardSuggestions() {
  const c = state.current;
  if (!c) return;
  try {
    state.current = await call('DiscardSuggestions', c.id, false);
    renderDetail();
    await refreshData();
    await loadList();
    showToast('Sugerencias descartadas.', 'success');
  } catch (err) { showToast(errorText(err), 'error'); }
}

function renderAIBanner() {
  const c = state.current;
  const el = $('#detail-ai-banner');
  el.hidden = true;
  el.className = 'banner banner--info';
  if (!state.config.ai_enabled) return;
  if (c.ia_estado === 'error' && c.ia_error) {
    el.hidden = false;
    el.className = 'banner banner--warn';
    el.textContent = 'No se pudo extraer la información automáticamente: ' + c.ia_error;
  } else if (c.ia_estado === 'pendiente') {
    el.hidden = false;
    el.textContent = 'El análisis con IA está en curso.';
  } else if (c.ia_estado === 'parcial') {
    el.hidden = false;
    el.className = 'banner banner--warn';
    el.textContent = 'La IA encontró solo parte de la información. Revisa y completa los campos.';
  } else if (c.ia_estado === 'ok' && !c.etiquetas_revisadas) {
    el.hidden = false;
    el.textContent = 'Datos extraídos automáticamente. Revísalos contra el documento y guarda.';
  }
}

function renderAICard() {
  const c = state.current;
  const btn = $('#btn-ai');
  const help = $('#ai-help');
  const badge = $('#ai-badge');
  const status = $('#ai-status');
  badge.className = 'badge';
  if (!state.config.ai_enabled) {
    badge.textContent = 'Desactivada';
    badge.classList.add('badge');
    help.textContent = 'Activa el análisis con IA y configura una clave en Ajustes.';
    btn.disabled = true;
    status.textContent = '';
  } else if (!state.config.has_api_key) {
    badge.textContent = 'Falta la clave';
    badge.classList.add('badge--amber');
    help.textContent = 'Configura la clave del servicio de IA en Ajustes.';
    btn.disabled = true;
    status.textContent = '';
  } else {
    badge.textContent = 'Disponible';
    badge.classList.add('badge--blue');
    help.textContent = 'Se enviará el texto del PDF al servicio configurado. No se envían los datos si la IA está desactivada.';
    btn.disabled = c.es_escaneado && !c.texto;
    status.textContent = c.ia_estado === 'error' ? 'La última extracción falló.' : '';
  }
  if (c.es_escaneado && !c.texto && c.transcripcion_estado !== 'completa') {
    help.textContent = 'Este PDF no tiene texto seleccionable. La extracción automática no puede leerlo; captura los datos manualmente.';
  }
}

function renderTranscription() {
  const c = state.current;
  const card = $('#transcription-card');
  if (!c) { card.hidden = true; return; }
  const estado = c.transcripcion_estado || 'no_requerida';
  const por = Number(c.paginas_por_transcribir || 0);
  const hechas = Number(c.paginas_transcritas || 0);
  const mostrar = Boolean(c.es_escaneado || por > 0 || estado !== 'no_requerida');
  card.hidden = !mostrar;
  if (!mostrar) return;

  const badge = $('#transcription-badge');
  badge.className = 'badge';
  const helps = {
    no_requerida: 'Este PDF ya tiene texto seleccionable.',
    pendiente: 'Hay páginas escaneadas que todavía no se pueden buscar.',
    en_proceso: 'La transcripción está en curso. Puedes seguir usando la aplicación.',
    completa: 'El texto transcrito ya forma parte de la búsqueda inteligente.',
    error: 'La transcripción no terminó; puedes reintentarla.'
  };
  const badges = {
    no_requerida: ['Sin transcribir', ''],
    pendiente: ['Pendiente', 'badge--amber'],
    en_proceso: ['Transcribiendo…', 'badge--blue'],
    completa: ['Completa', 'badge--green'],
    error: ['Incompleta', 'badge--red']
  };
  const [label, cls] = badges[estado] || badges.pendiente;
  badge.textContent = label;
  if (cls) badge.classList.add(cls);
  $('#transcription-help').textContent = helps[estado] || '';
  $('#transcription-status').textContent = por > 0 ? `${hechas} de ${por} páginas` : '';

  const wrap = $('#transcription-progress-wrap');
  wrap.hidden = !(estado === 'en_proceso' || (por > 0 && hechas > 0));
  const pct = por > 0 ? Math.min(100, Math.round((hechas / por) * 100)) : 0;
  $('#transcription-progress').value = pct;
  $('#transcription-progress-text').textContent = `${hechas} / ${por} páginas (${pct}%)`;
  const detail = $('#transcription-detail');
  detail.hidden = !c.texto;
  $('#transcription-text').textContent = c.texto || '';
  $('#btn-download-transcription').hidden = !c.texto;

  const btn = $('#btn-transcribe');
  const aiReady = Boolean(state.config.ai_enabled && state.config.has_api_key);
  const busy = state.transcribing === c.id;
  btn.disabled = !aiReady || busy;
  btn.textContent = estado === 'completa' ? 'Volver a transcribir' : (estado === 'error' ? 'Reintentar transcripción' : 'Transcribir páginas escaneadas');
  if (!state.config.ai_enabled) {
    $('#transcription-help').textContent = 'Activa el análisis con IA en Ajustes para transcribir páginas escaneadas.';
  } else if (!state.config.has_api_key) {
    $('#transcription-help').textContent = 'Configura la clave de IA en Ajustes para transcribir.';
  } else if (c.es_escaneado && c.texto && estado === 'no_requerida') {
    $('#transcription-help').textContent = 'El PDF ya tiene texto; no hay páginas escaneadas pendientes.';
  }
}

async function transcribeCurrent() {
  const c = state.current;
  if (!c) return;
  try {
    await transcribeConvention(c.id, { confirm: true });
    showToast('Transcripción completada. El texto ya se puede buscar.', 'success');
  } catch (err) {
    showToast(errorText(err), 'error');
  }
}

function startAutoAI(ids) {
  if (!ids || !ids.length || !state.config.ai_enabled || !state.config.has_api_key) return;
  state.autoQueue.push(...ids.filter(Boolean));
  if (!state.autoRunning) runAutoAIQueue();
}

async function runAutoAIQueue() {
  state.autoRunning = true;
  state.autoTotal = state.autoQueue.length;
  state.autoDone = 0;
  try {
    while (state.autoQueue.length) {
      const id = state.autoQueue.shift();
      state.autoDone++;
      setAIActivity(`IA: archivo ${state.autoDone}/${state.autoTotal}`);
      try {
        await processAutoFile(id);
      } catch (err) {
        console.error(err);
        showToast('La IA no pudo completar un archivo: ' + errorText(err), 'error');
      }
    }
  } finally {
    state.autoRunning = false;
    setAIActivity('');
    await refreshData();
    await loadList();
  }
}

async function processAutoFile(id) {
  let c = await call('GetConvenio', id);
  if (!state.config.ai_enabled) return;
  const necesitaTranscripcion = (c.es_escaneado && c.transcripcion_estado !== 'completa') ||
    Number(c.paginas_por_transcribir || 0) > 0;
  let transcrito = false;
  if (necesitaTranscripcion) {
    await transcribeConvention(id, { auto: true });
    c = await call('GetConvenio', id);
    transcrito = true;
  }
  if (!transcrito && c.texto && c.texto.trim()) {
    setAIActivity('IA: extrayendo datos…');
    await call('ExtractAI', id);
  }
  if (state.current?.id === id) {
    state.current = await call('GetConvenio', id);
    renderDetail();
  }
  await refreshData();
  await loadList();
}

async function transcribeConvention(id, options = {}) {
  const auto = Boolean(options.auto);
  if (state.transcribing) {
    if (!auto) showToast('Ya hay una transcripción en curso.', 'info');
    return;
  }
  if (!window.pdfjsLib) throw new Error('No se pudo cargar el motor de PDF para transcribir.');
  if (!state.config.ai_enabled || !state.config.has_api_key) {
    throw new Error('Activa la IA y configura la clave en Ajustes.');
  }
  let c = state.current?.id === id ? state.current : await call('GetConvenio', id);
  let pdf;
  let paginas = [];
  try {
    pdf = await window.pdfjsLib.getDocument({ url: `/pdf/${id}` }).promise;
    for (let i = 1; i <= pdf.numPages; i++) {
      const page = await pdf.getPage(i);
      const content = await page.getTextContent();
      const chars = content.items.reduce((n, item) => n + String(item.str || '').replace(/\s/g, '').length, 0);
      if (chars < 30) paginas.push(i);
    }
    if (!paginas.length) {
      if (!auto) showToast('No se encontraron páginas escaneadas para transcribir.', 'info');
      return;
    }
    if (options.confirm) {
      const ok = await confirmDialog(
        'Transcribir páginas escaneadas',
        `Se enviarán ${paginas.length} página${paginas.length === 1 ? '' : 's'} al servicio externo de IA. Puede tardar y generar costo en su proveedor. El texto se agregará a la búsqueda. ¿Continuar?`,
        'Transcribir',
        false
      );
      if (!ok) return;
    }
    await call('BeginTranscription', id, paginas.length);
    state.transcribing = id;
    if (state.current?.id === id) {
      state.current.transcripcion_estado = 'en_proceso';
      state.current.paginas_por_transcribir = paginas.length;
      state.current.paginas_transcritas = 0;
      renderTranscription();
    }
    setAIActivity(`IA: transcribiendo 0/${paginas.length}`);
    const cola = paginas.slice();
    let failed = null;
    const concurrency = Math.min(4, cola.length);
    const worker = async () => {
      while (cola.length && !failed) {
        const pageNumber = cola.shift();
        try {
          const page = await pdf.getPage(pageNumber);
          const viewportBase = page.getViewport({ scale: 1 });
          const scale = Math.min(1.45, 1600 / Math.max(viewportBase.width, viewportBase.height));
          const viewport = page.getViewport({ scale: Math.max(scale, 0.8) });
          const canvas = document.createElement('canvas');
          canvas.width = Math.ceil(viewport.width);
          canvas.height = Math.ceil(viewport.height);
          const ctx = canvas.getContext('2d', { alpha: false });
          ctx.fillStyle = '#ffffff';
          ctx.fillRect(0, 0, canvas.width, canvas.height);
          await page.render({ canvasContext: ctx, viewport }).promise;
          const base64 = canvas.toDataURL('image/jpeg', 0.82).split(',')[1];
          const status = await call('TranscribePage', id, pageNumber, base64);
          if (state.current?.id === id) {
            state.current.paginas_transcritas = status.paginas_transcritas;
            state.current.paginas_por_transcribir = status.paginas_por_transcribir;
            state.current.transcripcion_estado = status.estado;
            if (status.texto) state.current.texto = status.texto;
            renderTranscription();
            renderAICard();
          }
          setAIActivity(`IA: transcribiendo ${status.paginas_transcritas}/${status.paginas_por_transcribir}`);
        } catch (err) {
          failed = err;
        }
      }
    };
    await Promise.all(Array.from({ length: concurrency }, worker));
    if (failed) throw failed;
    await call('FinishTranscription', id);
    if (state.current?.id === id) {
      state.current = await call('GetConvenio', id);
      renderDetail();
    }
    await refreshData();
    await loadList();
    if (state.config.ai_enabled && state.config.has_api_key) {
      setAIActivity('IA: extrayendo datos…');
      try {
        await call('ExtractAI', id);
        if (state.current?.id === id) {
          state.current = await call('GetConvenio', id);
          renderDetail();
        }
        await refreshData();
        await loadList();
        showToast('Transcripción completa. Datos extraídos automáticamente.', 'success');
      } catch (err) {
        showToast('Transcripción completa, pero la extracción automática falló: ' + errorText(err), 'error');
      }
    }
  } catch (err) {
    try { await call('FailTranscription', id, errorText(err)); } catch (_) {}
    if (state.current?.id === id) {
      try { state.current = await call('GetConvenio', id); renderDetail(); } catch (_) {}
    }
    throw err;
  } finally {
    state.transcribing = null;
    setAIActivity('');
    if (state.current?.id === id) renderTranscription();
  }
}

async function downloadTranscription() {
  if (!state.current) return;
  try {
    const dst = await call('DownloadTranscription', state.current.id);
    if (dst) showToast('Transcripción guardada en ' + dst, 'success');
  } catch (err) {
    showToast(errorText(err), 'error');
  }
}

async function extractAI() {
  const c = state.current;
  if (!c) return;
  setButtonBusy($('#btn-ai'), true, 'Analizando…');
  $('#ai-status').textContent = 'Consultando el servicio…';
  try {
    state.current = await call('ExtractAI', c.id);
    renderDetail();
    await refreshData();
    await loadList();
    showToast('Análisis completado. Revisa los datos y las sugerencias.', 'success');
  } catch (err) {
    $('#ai-status').textContent = '';
    showToast(errorText(err), 'error');
    try { state.current = await call('GetConvenio', c.id); renderDetail(); } catch (_) {}
  } finally {
    setButtonBusy($('#btn-ai'), false, 'Extraer datos con IA');
  }
}

async function downloadCurrent() {
  if (state.current) await downloadPDF(state.current.id);
}

async function downloadPDF(id) {
  try {
    const dst = await call('DownloadPDF', id);
    if (dst) showToast('PDF guardado en ' + dst, 'success');
  } catch (err) {
    showToast(errorText(err), 'error');
  }
}

async function deleteCurrent() {
  const c = state.current;
  if (!c) return;
  const ok = await confirmDialog('Eliminar convenio', `¿Eliminar «${c.dependencia || c.archivo_nombre_original}»? También se borrará su PDF. Esta acción no se puede deshacer.`, 'Eliminar');
  if (!ok) return;
  showLoading(true);
  try {
    await call('DeleteConvenios', [c.id]);
    state.current = null;
    await refreshData();
    await loadList();
    showView('convenios');
    showToast('Convenio eliminado.', 'success');
  } catch (err) {
    showToast(errorText(err), 'error');
  } finally {
    showLoading(false);
  }
}

async function openImport() {
  if (state.view !== 'convenios') showView('convenios');
  try {
    const paths = await call('ChoosePDFs');
    if (!paths || !paths.length) return;
    state.pendingFiles = paths;
    $('#import-list').innerHTML = paths.map(p => `<div class="import-item"><svg viewBox="0 0 24 24"><path d="M6 3h8l4 4v14H6V3Z"/><path d="M14 3v5h5"/></svg><span>${esc(baseName(p))}</span></div>`).join('');
    $('#import-summary').hidden = true;
    $('#import-summary').textContent = '';
    $('#btn-import-confirm').hidden = false;
    $('#btn-import-confirm').disabled = false;
    $('#btn-import-confirm').textContent = `Importar ${paths.length} PDF${paths.length === 1 ? '' : 's'}`;
    $('#btn-import-cancel').textContent = 'Cancelar';
    $('#dialog-import').showModal();
  } catch (err) {
    showToast(errorText(err), 'error');
  }
}

async function confirmImport() {
  if (!state.pendingFiles.length) return;
  const paths = state.pendingFiles.slice();
  const btn = $('#btn-import-confirm');
  btn.disabled = true;
  btn.textContent = 'Importando…';
  $('#import-summary').hidden = false;
  $('#import-summary').className = 'banner banner--info';
  $('#import-summary').textContent = 'Copiando los PDF y preparando la búsqueda de texto…';
  try {
    const results = await call('ImportPDFs', paths);
    state.pendingFiles = [];
    const ok = results.filter(r => !r.error).length;
    const errors = results.filter(r => r.error);
    $('#import-list').innerHTML = results.map(r => `<div class="import-item ${r.error ? 'import-item--error' : 'import-item--ok'}">
      <svg viewBox="0 0 24 24">${r.error ? '<path d="M12 4 3 20h18L12 4Z"/><path d="M12 9v5m0 3h.01"/>' : '<path d="M20 6 9 17l-5-5"/>'}</svg>
      <span>${esc(r.name || baseName(r.path))}</span>
      <span class="import-item__status">${r.error ? 'Error' : (r.duplicate_id ? 'Duplicado' : 'Importado')}</span>
    </div>${r.error ? `<div class="import-item import-item--error"><span>${esc(r.error)}</span></div>` : ''}`).join('');
    $('#import-summary').className = 'banner ' + (errors.length ? 'banner--warn' : 'banner--info');
    $('#import-summary').textContent = `${ok} importado${ok === 1 ? '' : 's'}${errors.length ? `, ${errors.length} con error` : ''}.`;
    btn.hidden = true;
    $('#btn-import-cancel').textContent = 'Cerrar';
    await refreshData();
    await loadList();
    if (ok > 0) showToast(`${ok} PDF importado${ok === 1 ? '' : 's'}. Completa los datos.`, 'success');
    if (errors.length) showToast(`${errors.length} archivo${errors.length === 1 ? '' : 's'} no se pudo importar.`, 'error');
    if (ok > 0 && state.config.ai_enabled && state.config.has_api_key) {
      showToast('La IA procesará automáticamente los archivos importados.', 'info');
      startAutoAI(results.filter(r => !r.error).map(r => r.id));
    }
    if (results.length === 1 && !errors.length) {
      $('#dialog-import').close();
      await openDetail(results[0].id);
    }
  } catch (err) {
    $('#import-summary').className = 'banner banner--error';
    $('#import-summary').textContent = errorText(err);
    btn.disabled = false;
    btn.textContent = 'Reintentar';
  }
}

function showView(view) {
  state.view = view;
  $$('.view').forEach(v => v.hidden = true);
  const map = { convenios: 'view-convenios', detalle: 'view-detail', etiquetas: 'view-tags', ajustes: 'view-settings' };
  $('#' + (map[view] || 'view-convenios')).hidden = false;
  $$('.nav__item').forEach(b => b.classList.toggle('is-active', b.dataset.view === view || (view === 'detalle' && b.dataset.view === 'convenios')));
  const titles = {
    convenios: ['Convenios', 'Administra y consulta los convenios de la UAGro.'],
    detalle: ['Ficha del convenio', 'Revisa el documento, los datos y las etiquetas.'],
    etiquetas: ['Etiquetas', 'Catálogo local para clasificar los convenios.'],
    ajustes: ['Ajustes', 'Configuración local de la aplicación.']
  };
  const [title, sub] = titles[view] || titles.convenios;
  $('#page-title').textContent = title;
  $('#page-subtitle').textContent = sub;
  window.scrollTo({ top: 0, behavior: 'smooth' });
  if (view === 'etiquetas') renderTagsView();
  if (view === 'ajustes') renderSettings();
}

function renderTagsView() {
  const groups = $('#tags-groups');
  const total = state.tags.length;
  $('#tag-count').textContent = `${total} etiqueta${total === 1 ? '' : 's'}`;
  groups.innerHTML = TYPE_ORDER.map(tipo => {
    const tags = state.tags.filter(t => t.tipo === tipo);
    return `<section class="tag-group">
      <div class="tag-group__head"><h2>${TYPE_LABEL[tipo]}</h2><span class="badge">${tags.length}</span><span class="muted">${tipo === 'ambito' || tipo === 'estado' ? 'Etiquetas base protegidas' : 'Se pueden editar'}</span></div>
      <div class="tag-group__list">
        ${tags.length ? tags.map(tag => `<span class="tag-item">
          ${chipHTML(tag)}
          <span class="tag-item__count">${tag.total || 0}</span>
          ${tag.protegida ? '' : `<span class="tag-item__actions">
            <button type="button" data-edit-tag="${tag.id}" title="Editar ${esc(tag.nombre)}" aria-label="Editar ${esc(tag.nombre)}">✎</button>
            <button type="button" data-delete-tag="${tag.id}" title="Eliminar ${esc(tag.nombre)}" aria-label="Eliminar ${esc(tag.nombre)}">×</button>
          </span>`}
        </span>`).join('') : '<span class="muted">Sin etiquetas todavía.</span>'}
      </div>
    </section>`;
  }).join('');
}

async function createTag(e) {
  e.preventDefault();
  const nombre = $('#tag-name').value.trim();
  if (!nombre) return;
  const btn = e.target.querySelector('button[type="submit"]');
  setButtonBusy(btn, true, 'Creando…');
  try {
    await call('CreateTag', { id: 0, nombre, tipo: $('#tag-type').value, color: $('#tag-color').value });
    $('#tag-name').value = '';
    await refreshData();
    renderTagsView();
    showToast('Etiqueta creada.', 'success');
  } catch (err) {
    showToast(errorText(err), 'error');
  } finally {
    setButtonBusy(btn, false, 'Crear etiqueta');
  }
}

function onTagsGroupsClick(e) {
  const edit = e.target.closest('[data-edit-tag]');
  if (edit) {
    const tag = state.tags.find(t => t.id === Number(edit.dataset.editTag));
    if (tag) openTagDialog(tag);
    return;
  }
  const del = e.target.closest('[data-delete-tag]');
  if (del) {
    const tag = state.tags.find(t => t.id === Number(del.dataset.deleteTag));
    if (tag) deleteTag(tag);
  }
}

function openTagDialog(tag) {
  $('#tag-dialog-title').textContent = 'Editar etiqueta';
  $('#tag-edit-id').value = tag.id;
  $('#tag-edit-name').value = tag.nombre;
  $('#tag-edit-type').value = tag.tipo;
  $('#tag-edit-type').disabled = true;
  $('#tag-edit-color').value = tag.color;
  $('#tag-edit-color-value').textContent = tag.color.toUpperCase();
  markPalette($('#tag-edit-palette'), tag.color);
  $('#dialog-tag').showModal();
}

async function saveTagEdit() {
  const id = Number($('#tag-edit-id').value);
  const nombre = $('#tag-edit-name').value.trim();
  if (!id || !nombre) return;
  const btn = $('#btn-tag-save');
  setButtonBusy(btn, true, 'Guardando…');
  try {
    await call('UpdateTag', { id, nombre, tipo: $('#tag-edit-type').value, color: $('#tag-edit-color').value });
    await refreshData();
    renderTagsView();
    $('#dialog-tag').close();
    showToast('Etiqueta actualizada.', 'success');
  } catch (err) {
    showToast(errorText(err), 'error');
  } finally {
    setButtonBusy(btn, false, 'Guardar');
  }
}

async function deleteTag(tag) {
  const ok = await confirmDialog('Eliminar etiqueta', `¿Eliminar «${tag.nombre}»? Se quitará de todos los convenios que la tengan.`, 'Eliminar');
  if (!ok) return;
  try {
    await call('DeleteTag', tag.id);
    await refreshData();
    renderTagsView();
    if (state.current) {
      try { state.current = await call('GetConvenio', state.current.id); renderDetail(); } catch (_) {}
    }
    showToast('Etiqueta eliminada.', 'success');
  } catch (err) { showToast(errorText(err), 'error'); }
}

function renderSettings() {
  const c = state.config || {};
  $('#settings-ai-enabled').checked = Boolean(c.ai_enabled);
  $('#settings-web-search').checked = c.web_search_enabled !== false;
  $('#settings-api-key').value = '';
  $('#settings-api-key').placeholder = c.has_api_key ? 'Clave guardada (deja vacío para conservarla)' : 'Pega aquí tu clave';
  $('#settings-key-hint').textContent = c.has_api_key ? `Guardada: ${c.api_key_hint || '••••'}` : 'Sin clave configurada';
  $('#settings-base-url').value = c.base_url || 'https://api.deepseek.com';
  $('#settings-model').value = c.model || 'deepseek-flash';
  $('#settings-vision-model').value = c.vision_model || c.model || 'deepseek-flash';
  $('#settings-data-dir').textContent = state.app.data_dir || '—';
  $('#settings-version').textContent = state.app.version || '1.0.0';
}

async function saveSettings(e) {
  e.preventDefault();
  const btn = $('#settings-form button[type="submit"]');
  setButtonBusy(btn, true, 'Guardando…');
  try {
    const view = await call('SaveSettings', {
      ai_enabled: $('#settings-ai-enabled').checked,
      web_search_enabled: $('#settings-web-search').checked,
      api_key: $('#settings-api-key').value.trim(),
      clear_api_key: false,
      base_url: $('#settings-base-url').value.trim(),
      model: $('#settings-model').value.trim(),
      vision_model: $('#settings-vision-model').value.trim()
    });
    state.config = view;
    await refreshData();
    renderSettings();
    showToast('Ajustes guardados.', 'success');
  } catch (err) {
    showToast(errorText(err), 'error');
  } finally {
    setButtonBusy(btn, false, 'Guardar ajustes de IA');
  }
}

async function clearKey() {
  const ok = await confirmDialog('Borrar clave de IA', 'Se borrará la clave guardada y se desactivará el análisis con IA. ¿Continuar?', 'Borrar clave');
  if (!ok) return;
  try {
    state.config = await call('SaveSettings', { ai_enabled: false, clear_api_key: true, base_url: $('#settings-base-url').value.trim(), model: $('#settings-model').value.trim(), vision_model: $('#settings-vision-model').value.trim() });
    await refreshData();
    renderSettings();
    showToast('Clave eliminada.', 'success');
  } catch (err) { showToast(errorText(err), 'error'); }
}

async function openDataFolder() {
  try { await call('OpenDataFolder'); } catch (err) { showToast(errorText(err), 'error'); }
}

async function backupData() {
  const btn = $('#btn-backup');
  setButtonBusy(btn, true, 'Creando…');
  try {
    const dst = await call('BackupData');
    if (dst) showToast('Respaldo guardado en ' + dst, 'success');
  } catch (err) {
    showToast(errorText(err), 'error');
  } finally {
    setButtonBusy(btn, false, 'Crear respaldo ZIP');
  }
}

async function refreshData() {
  try {
    const b = await call('GetBootstrap');
    applyBootstrap(b);
  } catch (err) {
    console.error(err);
  }
}

function confirmDialog(title, message, okLabel = 'Confirmar', danger = true) {
  return new Promise(resolve => {
    state.confirmResolve = resolve;
    $('#confirm-title').textContent = title;
    $('#confirm-message').textContent = message;
    $('#confirm-ok').textContent = okLabel;
    $('#confirm-ok').className = danger ? 'btn btn--danger' : 'btn btn--primary';
    $('#dialog-confirm').showModal();
  });
}

function closeConfirm(value) {
  if ($('#dialog-confirm').open) $('#dialog-confirm').close();
  if (state.confirmResolve) {
    state.confirmResolve(Boolean(value));
    state.confirmResolve = null;
  }
}

function setAIActivity(text) {
  const el = $('#ai-activity');
  if (!el) return;
  el.hidden = !text;
  $('#ai-activity-text').textContent = text || '';
}

function showLoading(show) {
  state.loadingCount = Math.max(0, state.loadingCount + (show ? 1 : -1));
  $('#loading').hidden = state.loadingCount === 0;
}

function showToast(message, type = 'info') {
  const el = document.createElement('div');
  el.className = 'toast toast--' + type;
  el.innerHTML = `<span>${esc(message)}</span>`;
  $('#toasts').appendChild(el);
  setTimeout(() => { el.style.opacity = '0'; el.style.transform = 'translateY(6px)'; }, 4200);
  setTimeout(() => el.remove(), 4600);
}

function setButtonBusy(btn, busy, text) {
  if (!btn) return;
  if (busy) {
    btn.dataset.original = btn.innerHTML;
    btn.disabled = true;
    btn.textContent = text;
  } else {
    btn.disabled = false;
    if (btn.dataset.original) { btn.innerHTML = btn.dataset.original; delete btn.dataset.original; }
    else btn.textContent = text;
  }
}

function chipHTML(tag, opts = {}) {
  const color = safeColor(tag.color || colorForType(tag.tipo));
  const fg = contrast(color);
  const isNew = opts.new ? '<span class="chip__new">nueva</span>' : '';
  const attrs = opts.clickable ? `data-tag-id="${tag.id}" role="button" tabindex="0"` : '';
  return `<span class="chip ${opts.clickable ? 'chip--button' : ''}" ${attrs} style="background:${color};color:${fg};border-color:${shade(color, -.12)}" title="${esc(TYPE_LABEL[tag.tipo] || '')}: ${esc(tag.nombre)}">${esc(tag.nombre)}${isNew}</span>`;
}

function colorForType(tipo) {
  return { ambito: '#193060', estado: '#6D6E71', ciudad: '#2E7D32', tema: '#3A5A9C' }[tipo] || '#3A5A9C';
}

function safeColor(color) {
  return /^#[0-9a-f]{6}$/i.test(color || '') ? color : '#3A5A9C';
}

function contrast(hex) {
  const c = hex.replace('#', '');
  const r = parseInt(c.slice(0, 2), 16) / 255;
  const g = parseInt(c.slice(2, 4), 16) / 255;
  const b = parseInt(c.slice(4, 6), 16) / 255;
  const lum = 0.2126 * (r <= .03928 ? r / 12.92 : Math.pow((r + .055) / 1.055, 2.4))
    + 0.7152 * (g <= .03928 ? g / 12.92 : Math.pow((g + .055) / 1.055, 2.4))
    + 0.0722 * (b <= .03928 ? b / 12.92 : Math.pow((b + .055) / 1.055, 2.4));
  return lum > .45 ? '#1b2430' : '#ffffff';
}

function shade(hex, amount) {
  const c = hex.replace('#', '');
  const n = parseInt(c, 16);
  let r = (n >> 16) & 255, g = (n >> 8) & 255, b = n & 255;
  r = Math.max(0, Math.min(255, Math.round(r + 255 * amount)));
  g = Math.max(0, Math.min(255, Math.round(g + 255 * amount)));
  b = Math.max(0, Math.min(255, Math.round(b + 255 * amount)));
  return '#' + [r, g, b].map(x => x.toString(16).padStart(2, '0')).join('');
}

function renderPalette(container, input) {
  if (!container) return;
  container.innerHTML = PALETTE.map(c => `<button type="button" class="palette__swatch" data-color="${c}" style="background:${c}" title="${c}" aria-label="Color ${c}"></button>`).join('');
  markPalette(container, input.value);
  $$('.palette__swatch', container).forEach(btn => btn.addEventListener('click', () => {
    input.value = btn.dataset.color;
    input.dispatchEvent(new Event('input', { bubbles: true }));
    markPalette(container, btn.dataset.color);
  }));
}

function markPalette(container, color) {
  $$('.palette__swatch', container).forEach(b => b.classList.toggle('is-selected', b.dataset.color.toUpperCase() === String(color).toUpperCase()));
}

function formatDate(iso) {
  if (!iso) return '';
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso);
  return m ? `${m[3]}/${m[2]}/${m[1]}` : iso;
}

function formatDateTime(iso) {
  if (!iso) return '';
  const s = String(iso).replace('T', ' ');
  return s.length >= 16 ? s.slice(0, 16) : s;
}

function formatBytes(n) {
  n = Number(n || 0);
  if (!n) return '—';
  if (n < 1024) return n + ' B';
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
  return (n / (1024 * 1024)).toFixed(1) + ' MB';
}

function baseName(path) {
  return String(path || '').replaceAll('\\', '/').split('/').pop();
}

function normalize(s) {
  return String(s || '').normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase();
}

function esc(value) {
  return String(value ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

function errorText(err) {
  if (!err) return 'Error desconocido.';
  if (typeof err === 'string') return err;
  return err.message || String(err);
}

function errorPanel(message) {
  return `<div class="empty-state"><div class="empty-state__art" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="M12 4 3 20h18L12 4Z"/><path d="M12 9v5m0 3h.01"/></svg></div><h2>No se pudo cargar la información</h2><p>${esc(message)}</p></div>`;
}
