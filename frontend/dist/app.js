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
  filters: { query: '', tag_ids: [], vigencia: '', solo_pendientes: false, solo_no_convenios: false, incluir_anteriores: false, order: 'id', dir: 'desc', page: 1 },
  result: { items: [], total: 0, total_registrados: 0, page: 1, pages: 1 },
  selected: new Set(),
  current: null,
  pendingFiles: [],
  transcribing: new Set(),
  activity: new Map(),
  autoQueue: [],
  autoRunning: false,
  autoTotal: 0,
  autoDone: 0,
  confirmResolve: null,
  searchTimer: null,
  tagTimer: null,
  loadingCount: 0,
  listLoading: false,
  pollTimer: null,
  update: null,
  updateError: '',
  updateChecking: false,
  dirty: false,
  formId: null,
  tagFilterQuery: '',
  tagsQuery: '',
  dragDepth: 0,
  dups: [],
  dupsFilter: '',
  dupCompare: '',
  importChecks: [],
  versionDir: 'older'
};

const UPDATE_INTERVAL_MS = 6 * 60 * 60 * 1000;
// Archivos que la cola automática procesa a la vez y páginas escaneadas que se
// envían al mismo tiempo entre todos ellos. El costo por documento no cambia:
// solo se reparten las esperas del servicio de IA.
const AUTO_FILES_AT_ONCE = 3;
const PAGES_AT_ONCE = 10;

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
    setupRuntime();
    if (state.config.update_check !== false) {
      setTimeout(() => checkForUpdates(false), 4000);
      setInterval(() => checkForUpdates(false), UPDATE_INTERVAL_MS);
    }
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
  $$('.nav__item').forEach(btn => btn.addEventListener('click', () => navigate(btn.dataset.view)));
  $('#btn-settings-top').addEventListener('click', () => navigate('ajustes'));
  $('#btn-import-top').addEventListener('click', openImport);
  $$('[data-stat-filter]').forEach(btn => btn.addEventListener('click', () => setVigencia(btn.dataset.statFilter)));
  $('#btn-import-confirm').addEventListener('click', confirmImport);

  $('#search').addEventListener('input', onSearch);
  $('#btn-search-clear').addEventListener('click', () => {
    $('#search').value = '';
    state.filters.query = '';
    state.filters.page = 1;
    syncRelevanceOrder();
    updateSearchClear();
    scheduleLoad(0);
  });
  $('#filter-vigencia').addEventListener('change', e => setVigencia(e.target.value));
  $('#sort-select').addEventListener('change', e => {
    state.filters.order = e.target.value;
    state.filters.dir = e.target.value === 'dependencia' ? 'asc' : 'desc';
    state.filters.page = 1;
    loadList();
  });
  $('#tag-filter-search').addEventListener('input', e => {
    state.tagFilterQuery = e.target.value;
    renderTagFilter();
  });
  $('#btn-tag-filter').addEventListener('click', toggleTagFilter);
  $('#btn-pending').addEventListener('click', togglePending);
  $('#btn-noconv').addEventListener('click', toggleNoConvenios);
  $('#btn-mark-noconv').addEventListener('click', () => setNoConvenio(true));
  $('#btn-clear-filters').addEventListener('click', clearFilters);

  $('#list-area').addEventListener('click', onListClick);
  $('#list-area').addEventListener('change', onListChange);
  $('#pagination').addEventListener('click', onPagination);
  $('#btn-bulk-clear').addEventListener('click', clearSelection);
  $('#btn-bulk-delete').addEventListener('click', bulkDelete);
  $('#btn-bulk-csv').addEventListener('click', () => exportCSV(Array.from(state.selected)));
  $('#btn-bulk-export').addEventListener('click', () => exportPackage(Array.from(state.selected)));
  $('#btn-bulk-pdf').addEventListener('click', () => downloadPDFs(Array.from(state.selected)));
  $('#btn-export-results').addEventListener('click', () => exportCSV([], state.filters));

  $('#detail-back').addEventListener('click', () => navigate('convenios'));
  $('#form-edit').addEventListener('submit', saveConvenio);
  $('#form-edit').addEventListener('input', markDirty);
  $('#form-edit').addEventListener('change', markDirty);
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
  $('#btn-new-tag-detail').addEventListener('click', async () => {
    if (!await navigate('etiquetas')) return;
    setTimeout(() => $('#tag-name')?.focus(), 50);
  });
  $('#tags-selector').addEventListener('change', onTagSelectorChange);

  $('#tag-create-form').addEventListener('submit', createTag);
  $('#tag-type').addEventListener('change', syncTagStateField);
  $('#tag-color').addEventListener('input', e => {
    $('#tag-color-value').textContent = e.target.value.toUpperCase();
    markPalette($('#tag-palette'), e.target.value);
  });
  $('#tags-groups').addEventListener('click', onTagsGroupsClick);
  bindDuplicateEvents();
  $('#tags-search').addEventListener('input', e => {
    state.tagsQuery = e.target.value;
    renderTagsView();
  });
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
  $('#btn-export-all').addEventListener('click', () => exportPackage([]));
  $('#btn-export-csv-all').addEventListener('click', () => exportCSV([], emptyFilter()));
  $('#btn-import-package').addEventListener('click', () => importPackage(''));

  $('#settings-update-check').addEventListener('change', saveUpdatePreference);
  $('#btn-check-update').addEventListener('click', () => checkForUpdates(true));
  $('#btn-update-settings').addEventListener('click', applyUpdate);
  $('#btn-update-now').addEventListener('click', applyUpdate);
  $('#btn-update-notes').addEventListener('click', showUpdateNotes);
  $('#btn-release-page').addEventListener('click', () => call('OpenReleasePage', state.update?.url || '').catch(err => showToast(errorText(err), 'error')));
  $('#btn-update-dismiss').addEventListener('click', dismissUpdate);
  $('#update-dialog-close').addEventListener('click', () => $('#dialog-update').close());
  $('#update-dialog-cancel').addEventListener('click', () => $('#dialog-update').close());
  $('#update-dialog-apply').addEventListener('click', () => { $('#dialog-update').close(); applyUpdate(); });

  $('#confirm-cancel').addEventListener('click', () => closeConfirm(false));
  $('#confirm-ok').addEventListener('click', () => closeConfirm(true));
  $('#dialog-confirm').addEventListener('cancel', e => { e.preventDefault(); closeConfirm(false); });

  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      if ($('#dialog-confirm').open) closeConfirm(false);
      if ($('#dialog-import').open) $('#dialog-import').close();
      if ($('#dialog-tag').open) $('#dialog-tag').close();
      if ($('#dialog-update').open) $('#dialog-update').close();
    }
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's' && state.view === 'detalle') {
      e.preventDefault();
      $('#form-edit').requestSubmit();
      return;
    }
    const target = e.target;
    const typing = target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT');
    if (!typing && e.key === '/') {
      e.preventDefault();
      navigate('convenios').then(ok => { if (ok) $('#search')?.focus(); });
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
  $('#sidebar-version').textContent = state.app.version ? `Versión ${state.app.version}` : '';
  document.title = 'Convenios UAGro';
}

function renderStats() {
  const s = state.stats || {};
  $('#stats-total').textContent = s.total || 0;
  const anteriores = s.versiones_anteriores || 0;
  $('#stats-total-hint').textContent = anteriores ? `vigentes · ${anteriores} ${anteriores === 1 ? 'versión anterior' : 'versiones anteriores'} en historial` : 'registrados en total';
  $('#toggle-old-wrap').hidden = !anteriores && !state.filters.incluir_anteriores;
  const porRevisar = (s.duplicados || 0) + (s.renovaciones || 0);
  $('#nav-dups-count').textContent = porRevisar;
  $('#nav-dups-count').hidden = porRevisar === 0;
  $('#stats-vigentes').textContent = s.vigentes || 0;
  $('#stats-por-vencer').textContent = s.por_vencer || 0;
  $('#stats-vencidos').textContent = s.vencidos || 0;
  $('#pending-count').textContent = s.etiquetas_pendientes || 0;
  $('#noconv-count').textContent = s.no_convenios || 0;
  syncReviewButtons();
  $$('[data-stat-filter]').forEach(card => card.classList.toggle('is-active', card.dataset.statFilter !== '' && card.dataset.statFilter === state.filters.vigencia));
}

function setVigencia(value) {
  state.filters.vigencia = value || '';
  state.filters.page = 1;
  $('#filter-vigencia').value = state.filters.vigencia;
  renderStats();
  if (state.view !== 'convenios') showView('convenios');
  loadList();
}

function emptyFilter() {
  return { query: '', tag_ids: [], vigencia: '', solo_pendientes: false, solo_no_convenios: false, incluir_anteriores: false, order: 'id', dir: 'desc', page: 1 };
}

// syncReviewButtons muestra los atajos de revisión solo si hay algo que revisar
// (o si su filtro está activo, para poder quitarlo).
function syncReviewButtons() {
  const s = state.stats || {};
  $('#btn-pending').hidden = (s.etiquetas_pendientes || 0) === 0 && !state.filters.solo_pendientes;
  $('#btn-pending').classList.toggle('is-active', Boolean(state.filters.solo_pendientes));
  $('#btn-noconv').hidden = (s.no_convenios || 0) === 0 && !state.filters.solo_no_convenios;
  $('#btn-noconv').classList.toggle('is-active', Boolean(state.filters.solo_no_convenios));
}

// Con una búsqueda activa, el orden "Más recientes" se cambia por relevancia;
// al borrar la búsqueda se regresa al orden por fecha de registro.
function syncRelevanceOrder() {
  const hasQuery = Boolean(state.filters.query.trim());
  if (hasQuery && state.filters.order === 'id') {
    state.filters.order = 'relevancia';
    state.filters.dir = 'desc';
  } else if (!hasQuery && state.filters.order === 'relevancia') {
    state.filters.order = 'id';
    state.filters.dir = 'desc';
  }
  $('#sort-select').value = state.filters.order;
}

async function loadList(options = {}) {
  if (state.listLoading) return;
  state.listLoading = true;
  const selected = options.preserveSelection ? new Set(state.selected) : null;
  const area = $('#list-area');
  area.setAttribute('aria-busy', 'true');
  try {
    const res = await call('ListConvenios', state.filters);
    state.result = res;
    if (selected) {
      const visible = new Set(res.items.map(c => c.id));
      state.selected = new Set([...selected].filter(id => visible.has(id)));
    } else {
      state.selected.clear();
    }
    renderList();
    renderSummary();
    renderPagination();
    renderStats();
    renderProcessingBanner();
    syncProcessingPoll();
    updateSearchClear();
    updateClearFilters();
    updateBulkBar();
  } catch (err) {
    area.innerHTML = errorPanel(errorText(err));
    showToast(errorText(err), 'error');
  } finally {
    state.listLoading = false;
    area.removeAttribute('aria-busy');
  }
}

function isProcessing(c) {
  return c.transcripcion_estado === 'en_proceso' ||
    c.ia_estado === 'pendiente' ||
    (c.transcripcion_estado === 'pendiente' && Number(c.paginas_por_transcribir || 0) > 0);
}

function processingItems() {
  return (state.result.items || []).filter(isProcessing);
}

function renderProcessingBanner() {
  const banner = $('#processing-banner');
  if (!banner) return;
  const items = processingItems();
  const active = items.length > 0 || state.autoRunning;
  banner.hidden = !active;
  if (!active) return;
  const transcripcion = items.filter(c => c.transcripcion_estado === 'en_proceso').length;
  const analisis = items.filter(c => c.ia_estado === 'pendiente').length;
  const enCola = items.filter(c => c.transcripcion_estado === 'pendiente' && Number(c.paginas_por_transcribir || 0) > 0).length;
  const partes = [];
  if (transcripcion) partes.push(`${transcripcion} en transcripción`);
  if (analisis) partes.push(`${analisis} en análisis`);
  if (enCola) partes.push(`${enCola} en cola`);
  if (!partes.length) partes.push('preparando el procesamiento');
  const n = Math.max(items.length, state.autoRunning ? state.autoTotal - state.autoDone : 0, 1);
  $('#processing-text').textContent = `Procesando ${n} convenio${n === 1 ? '' : 's'} con IA (${partes.join(' y ')}). Puedes seguir usando el buscador; los datos se actualizarán solos.`;
}

function syncProcessingPoll() {
  clearTimeout(state.pollTimer);
  state.pollTimer = null;
  if (state.view !== 'convenios') return;
  if (!state.autoRunning && processingItems().length === 0) return;
  state.pollTimer = setTimeout(() => {
    if (state.view === 'convenios') loadList({ preserveSelection: true });
  }, 3000);
}

function scheduleLoad(ms = 240) {
  clearTimeout(state.searchTimer);
  state.searchTimer = setTimeout(loadList, ms);
}

function onSearch(e) {
  state.filters.query = e.target.value.trimStart();
  state.filters.page = 1;
  syncRelevanceOrder();
  updateSearchClear();
  scheduleLoad();
}

function updateSearchClear() {
  $('#btn-search-clear').hidden = !state.filters.query;
}

function updateClearFilters() {
  const active = Boolean(state.filters.query || state.filters.vigencia || state.filters.solo_pendientes || state.filters.solo_no_convenios || state.filters.tag_ids.length);
  $('#btn-clear-filters').hidden = !active;
  $('#tag-filter-count').hidden = state.filters.tag_ids.length === 0;
  $('#tag-filter-count').textContent = state.filters.tag_ids.length;
  syncReviewButtons();
}

function clearFilters() {
  state.filters = { ...emptyFilter(), order: state.filters.order, dir: state.filters.dir };
  $('#toggle-old').checked = false;
  syncRelevanceOrder();
  $('#search').value = '';
  $('#filter-vigencia').value = '';
  state.tagFilterQuery = '';
  $('#tag-filter-search').value = '';
  renderStats();
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

function toggleNoConvenios() {
  state.filters.solo_no_convenios = !state.filters.solo_no_convenios;
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
  const q = normalize(state.tagFilterQuery.trim());
  groups.innerHTML = TYPE_ORDER.map(tipo => {
    const tags = state.tags.filter(t => t.tipo === tipo &&
      (!q || normalize(t.nombre).includes(q) || state.filters.tag_ids.includes(t.id)));
    return `<fieldset>
      <legend>${TYPE_LABEL[tipo]}</legend>
      <div class="tag-filter__group">
        ${tags.length ? tags.map(tag => `
          <label class="filter-tag">
            <input type="checkbox" data-filter-tag="${tag.id}" ${state.filters.tag_ids.includes(tag.id) ? 'checked' : ''}>
            <span style="width:8px;height:8px;border-radius:50%;background:${safeColor(tag.color)};flex:none"></span>
            <span>${esc(tag.nombre)}</span>
            <span class="muted">${tag.total || 0}</span>
          </label>`).join('') : `<span class="tag-filter__empty">${q ? 'Sin coincidencias.' : 'Sin etiquetas.'}</span>`}
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
  $('#approx-banner').hidden = !(state.result.aproximado && items.length);
  $('#btn-export-results').hidden = !items.length;
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
    $('#sort-select').value = ['id', 'relevancia', 'dependencia', 'fecha_firma', 'fecha_vigencia'].includes(state.filters.order) ? state.filters.order : 'id';
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
  if (state.config.ai_enabled && c.ia_estado === 'pendiente') {
    badges.push('<span class="badge badge--blue">Analizando…</span>');
  } else if (state.config.ai_enabled && c.ia_estado === 'error') {
    badges.push('<span class="badge badge--red" title="' + esc(c.ia_error || '') + '">IA con error</span>');
  }
  if (!c.etiquetas_revisadas && !isProcessing(c) && !state.autoRunning) badges.push('<span class="badge badge--amber">Etiquetas pendientes</span>');
  if (c.transcripcion_estado === 'en_proceso') {
    const total = Number(c.paginas_transcritas || 0) + Number(c.paginas_por_transcribir || 0);
    const done = Number(c.paginas_transcritas || 0);
    badges.push(`<span class="badge badge--blue">Transcribiendo${total ? ` ${done}/${total}` : ''}…</span>`);
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
  if (c.no_es_convenio) badges.unshift('<span class="badge badge--violet" title="La IA cree que este archivo no es un convenio. Ábrelo para confirmarlo o eliminarlo.">¿No es convenio?</span>');
  if (c.reemplazado_por) badges.unshift('<span class="badge" title="Una versión renovada la reemplazó; se ve en su historial">Versión anterior</span>');
  if (c.duplicados) badges.unshift(`<button type="button" class="badge badge--red badge--link" data-goto-dups title="Revisar en Duplicados">Posible duplicado${c.duplicados > 1 ? ` (${c.duplicados})` : ''}</button>`);
  if (c.posible_renovacion) badges.push('<button type="button" class="badge badge--blue badge--link" data-goto-dups title="Revisar en Duplicados">¿Renovación?</button>');
  if (c.versiones) badges.push(`<span class="badge badge--green" title="Tiene versiones anteriores en su historial">${c.versiones + 1} versiones</span>`);
  const selected = state.selected.has(c.id);
  const firmantes = [c.firmante_dependencia, c.firmante_uagro].filter(Boolean).map(esc).join(' · ');
  const rowClass = [selected ? 'is-selected' : '', c.no_es_convenio ? 'is-noconv' : ''].filter(Boolean).join(' ');
  return `<tr data-open="${c.id}" class="${rowClass}">
    <td class="col-check"><input type="checkbox" class="checkbox row-check" data-select="${c.id}" ${selected ? 'checked' : ''} aria-label="Seleccionar ${esc(c.dependencia || c.archivo_nombre_original)}"></td>
    <td class="cell-main">
      <div class="dep-name">${esc(c.dependencia || 'Sin capturar')}</div>
      ${firmantes ? `<div class="dep-meta" title="Firmantes">${firmantes}</div>` : ''}
      <div class="dep-file" title="Archivo original: ${esc(c.archivo_nombre_original)}">${esc(c.archivo_nombre || c.archivo_nombre_original)}</div>
      <div class="row-badges">${badges.join('')}</div>
      <div class="chips chip-list">${tags || '<span class="empty-chips">Sin etiquetas</span>'}</div>
      ${snippetHTML(c.fragmento)}
    </td>
    <td class="cell-date">${formatDate(c.fecha_firma) || '—'}</td>
    <td class="cell-date">${c.vigencia_indefinida ? '<span class="muted">Indefinida</span>' : (formatDate(c.fecha_vigencia) || '—')}${relativeVigencia(c)}</td>
    <td>${stateBadge(c.estado)}</td>
    <td><button class="btn btn--text btn--small" data-download="${c.id}" title="Descargar PDF"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 4v11m0 0 4-4m-4 4-4-4M5 20h14"/></svg> PDF</button></td>
    <td class="cell-actions"><button class="btn btn--outline btn--small" data-open-btn="${c.id}">Abrir</button></td>
  </tr>`;
}

// snippetHTML muestra el fragmento del texto donde aparece la búsqueda; el
// servidor marca la coincidencia con los caracteres de control 2 y 3.
function snippetHTML(text) {
  if (!text || !state.filters.query.trim()) return '';
  const html = esc(text).replace(/\u0002/g, '<mark>').replace(/\u0003/g, '</mark>');
  return `<div class="snippet">${html}</div>`;
}

function localDate(iso) {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso || '');
  return m ? new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3])) : null;
}

function addMonths(date, n) {
  const last = new Date(date.getFullYear(), date.getMonth() + n + 1, 0).getDate();
  return new Date(date.getFullYear(), date.getMonth() + n, Math.min(date.getDate(), last));
}

// spanText cuenta años, meses y días de calendario entre dos fechas
// (from <= to): «1 año y 7 meses», «2 meses y 5 días», «12 días».
function spanText(from, to) {
  let months = (to.getFullYear() - from.getFullYear()) * 12 + to.getMonth() - from.getMonth();
  if (addMonths(from, months) > to) months--;
  const days = Math.round((to - addMonths(from, months)) / 86400000);
  const unit = (n, one, many) => `${n} ${n === 1 ? one : many}`;
  const years = Math.floor(months / 12);
  const rest = months % 12;
  if (years) return unit(years, 'año', 'años') + (rest ? ' y ' + unit(rest, 'mes', 'meses') : '');
  if (months) return unit(months, 'mes', 'meses') + (months < 3 && days ? ' y ' + unit(days, 'día', 'días') : '');
  return unit(days, 'día', 'días');
}

function relativeVigencia(c) {
  if (c.vigencia_indefinida) return '';
  const target = localDate(c.fecha_vigencia);
  if (!target) return '';
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const days = Math.round((target - today) / 86400000);
  if (days === 0) return '<small class="is-warn">Vence hoy</small>';
  if (days === 1) return '<small class="is-warn">Vence mañana</small>';
  if (days === -1) return '<small class="is-bad">Venció ayer</small>';
  if (days < 0) return `<small class="is-bad">Venció hace ${spanText(target, today)}</small>`;
  return `<small class="${days <= 90 ? 'is-warn' : ''}">Vence en ${spanText(today, target)}</small>`;
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
  const filtered = Boolean(state.filters.query || state.filters.tag_ids.length || state.filters.vigencia || state.filters.solo_pendientes || state.filters.solo_no_convenios);
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
  if (e.target.closest('[data-goto-dups]')) {
    e.stopPropagation();
    navigate('duplicados');
    return;
  }
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
    clearDirty();
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
  $('#detail-subtitle').textContent = `${c.tipo_documento && c.tipo_documento !== 'Otro' ? c.tipo_documento + ' · ' : ''}${c.paginas || '?'} página${c.paginas === 1 ? '' : 's'}${c.es_escaneado ? ' · PDF escaneado (sin texto seleccionable)' : ''}`;
  const fileName = c.archivo_nombre || c.archivo_nombre_original;
  $('#pdf-caption').textContent = fileName;
  $('#pdf-caption').title = fileName === c.archivo_nombre_original ? '' : `Archivo original: ${c.archivo_nombre_original}`;
  $('#pdf-badge').outerHTML = (c.es_escaneado ? '<span class="badge badge--amber" id="pdf-badge">Sin texto</span>' : '<span class="badge badge--green" id="pdf-badge">Texto indexado</span>');
  const pdfSrc = `/pdf/${c.id}`;
  if (state.formId !== c.id || !$('#pdf-frame').src.includes(pdfSrc)) {
    $('#pdf-frame').src = `${pdfSrc}?v=${Date.now()}`;
  }

  // Si el usuario está editando, una actualización automática (IA en curso)
  // no debe sobrescribir lo que escribió.
  if (!(state.dirty && state.formId === c.id)) fillForm(c);
  state.formId = c.id;
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
  renderRelations();
  renderNoConvenio();
}

function renderNoConvenio() {
  const c = state.current;
  const el = $('#detail-noconv-banner');
  $('#btn-mark-noconv').hidden = Boolean(c.no_es_convenio);
  el.hidden = !c.no_es_convenio;
  if (!c.no_es_convenio) return;
  el.innerHTML = `<span><strong>Este archivo está marcado como «no es un convenio».</strong> Puede ser un acta, un oficio, una constancia u otro documento. Mientras tenga la marca no se le asigna facultad ni «Convenio Abierto».</span>
    <span class="banner__buttons">
      <button class="btn btn--outline btn--small" data-no-convenio="0">Sí es un convenio</button>
      <button class="btn btn--danger-text btn--small" data-delete-current>Eliminar archivo</button>
    </span>`;
}

async function setNoConvenio(noEs) {
  const c = state.current;
  if (!c) return;
  try {
    state.current = await call('SetNoConvenio', c.id, noEs);
    renderDetail();
    await refreshData();
    await loadList({ preserveSelection: true });
    showToast(noEs
      ? 'Marcado como «no es un convenio». Lo encontrarás con el filtro «No son convenios».'
      : 'Confirmado como convenio. Se le agregaron sus etiquetas obligatorias.', 'success');
  } catch (err) {
    showToast(errorText(err), 'error');
  }
}

function fillForm(c) {
  $('#f-dependencia').value = c.dependencia || '';
  $('#f-dependencia-corta').value = c.dependencia_corta || '';
  $('#f-tipo-documento').value = c.tipo_documento || '';
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
}

function markDirty() {
  if (state.view !== 'detalle') return;
  state.dirty = true;
  $('#unsaved-hint').hidden = false;
}

function clearDirty() {
  state.dirty = false;
  $('#unsaved-hint').hidden = true;
}

// navigate cambia de vista, pero antes pregunta si hay cambios sin guardar.
async function navigate(view) {
  if (state.view === 'detalle' && view !== 'detalle' && state.dirty) {
    const ok = await confirmDialog('Cambios sin guardar', 'Hay cambios en los datos del convenio que no se han guardado. ¿Salir sin guardarlos?', 'Salir sin guardar');
    if (!ok) return false;
    clearDirty();
  }
  showView(view);
  return true;
}

function renderDetailMeta() {
  const c = state.current;
  const rows = [
    ['Se descarga como', c.archivo_nombre || c.archivo_nombre_original],
    ['Archivo original', c.archivo_nombre_original],
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
  $$('#tags-current .chip--button').forEach(chip => chip.addEventListener('click', async () => {
    if (!await navigate('convenios')) return;
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
            <input type="checkbox" data-select-tag="${t.id}" data-type="${t.tipo}" data-name="${esc(t.nombre)}" data-estado="${esc(t.estado || '')}" ${selected.has(t.id) ? 'checked' : ''}>
            <span style="width:9px;height:9px;border-radius:50%;background:${safeColor(t.color)};flex:none"></span>
            <span>${esc(t.nombre)}</span>
            ${t.tipo === 'ciudad' && t.estado ? `<small class="tag-selector__state">${esc(t.estado)}</small>` : ''}
          </label>`).join('') : '<span class="tag-selector__empty">Sin etiquetas.</span>'}
      </div>
    </fieldset>`;
  }).join('');
  $('#tags-selector').innerHTML = `<h3>Seleccionar etiquetas</h3>
    <p class="muted tag-selector__help">Al elegir una ciudad se marcan su estado y «Nacional»; las ciudades de otro estado aparecen atenuadas.</p>
    <div class="tag-selector__groups">${html}</div>`;
  markOtherStateCities();
}

const EXCLUSIVE_TYPES = new Set(['ambito', 'estado', 'ciudad', 'facultad']);

function onTagSelectorChange(e) {
  const input = e.target.closest('[data-select-tag]');
  if (!input) return;
  const box = $('#tags-selector');
  if (input.checked && EXCLUSIVE_TYPES.has(input.dataset.type)) {
    $$(`[data-select-tag][data-type="${input.dataset.type}"]`, box).forEach(other => {
      if (other !== input) other.checked = false;
    });
  }
  syncLocation(input);
  markOtherStateCities();
}

// syncLocation evita combinaciones imposibles: una ciudad trae su estado, un
// estado de México va con «Nacional» y deja fuera las ciudades de otro estado.
// Al quitar un estado (o «Nacional») se quita lo que depende de él.
function syncLocation(input) {
  const box = $('#tags-selector');
  const inputs = tipo => $$(`[data-select-tag][data-type="${tipo}"]`, box);
  const only = (tipo, nombre) => {
    const target = inputs(tipo).find(i => i.dataset.name === nombre);
    if (target) inputs(tipo).forEach(i => { i.checked = i === target; });
  };
  const dropCities = keep => inputs('ciudad').forEach(i => {
    if (i.checked && i.dataset.estado && !keep(i.dataset.estado)) i.checked = false;
  });
  const { type, name, estado } = input.dataset;
  if (input.checked) {
    if (type === 'ciudad' && estado) {
      only('estado', estado);
      only('ambito', 'Nacional');
    } else if (type === 'estado') {
      dropCities(e => e === name);
      only('ambito', 'Nacional');
    } else if (type === 'ambito' && name !== 'Nacional') {
      inputs('estado').forEach(i => { i.checked = false; });
      dropCities(() => false);
    }
  } else if (type === 'estado') {
    dropCities(e => e !== name);
  } else if (type === 'ambito' && name === 'Nacional') {
    inputs('estado').forEach(i => { i.checked = false; });
    dropCities(() => false);
  }
}

function markOtherStateCities() {
  const box = $('#tags-selector');
  const estado = $('[data-select-tag][data-type="estado"]:checked', box)?.dataset.name || '';
  $$('[data-select-tag][data-type="ciudad"]', box).forEach(i => {
    i.closest('.tag-selector__item').classList.toggle('is-other', Boolean(estado && i.dataset.estado && i.dataset.estado !== estado));
  });
}

async function saveConvenio(e) {
  e.preventDefault();
  const c = state.current;
  if (!c) return;
  const in_ = {
    id: c.id,
    dependencia: $('#f-dependencia').value.trim(),
    dependencia_corta: $('#f-dependencia-corta').value.trim(),
    tipo_documento: $('#f-tipo-documento').value,
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
    clearDirty();
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
  const busy = state.transcribing.has(c.id);
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
    // transcribeConvention avisa por su cuenta cuando termina.
    await transcribeConvention(c.id, { confirm: true });
  } catch (err) {
    showToast(errorText(err), 'error');
  }
}

function startAutoAI(ids) {
  if (!ids || !ids.length || !state.config.ai_enabled || !state.config.has_api_key) return;
  const nuevos = ids.filter(Boolean);
  state.autoQueue.push(...nuevos);
  if (state.autoRunning) {
    state.autoTotal += nuevos.length;
    renderAIActivity();
  } else {
    runAutoAIQueue();
  }
  renderProcessingBanner();
  syncProcessingPoll();
}

// runAutoAIQueue procesa la cola con varios archivos a la vez. Mientras uno
// espera al servicio de IA, los demás avanzan.
async function runAutoAIQueue() {
  state.autoRunning = true;
  state.autoTotal = state.autoQueue.length;
  state.autoDone = 0;
  let failed = 0;
  renderAIActivity();
  renderProcessingBanner();
  syncProcessingPoll();
  const worker = async () => {
    while (state.autoQueue.length) {
      const id = state.autoQueue.shift();
      try {
        await processAutoFile(id);
      } catch (err) {
        console.error(err);
        failed++;
        showToast('La IA no pudo completar un archivo: ' + errorText(err), 'error');
      } finally {
        state.autoDone++;
        renderAIActivity();
      }
    }
  };
  const total = state.autoTotal;
  try {
    await Promise.all(Array.from({ length: AUTO_FILES_AT_ONCE }, worker));
  } finally {
    state.autoRunning = false;
    renderAIActivity();
    await refreshData();
    await loadList({ preserveSelection: true });
    renderProcessingBanner();
    syncProcessingPoll();
    if (total > 1 && failed < total) {
      const ok = total - failed;
      showToast(`Listo: ${ok} archivo${ok === 1 ? '' : 's'} procesado${ok === 1 ? '' : 's'} con IA.`, 'success');
    }
    // Archivos agregados mientras se cerraba la cola.
    if (state.autoQueue.length && !state.autoRunning) runAutoAIQueue();
  }
}

// La actividad de IA se guarda por documento y el indicador muestra un resumen.
function setDocActivity(id, text) {
  if (text) state.activity.set(id, text);
  else state.activity.delete(id);
  renderAIActivity();
}

function renderAIActivity() {
  const items = [...state.activity.values()];
  let text = '';
  if (state.autoRunning && state.autoTotal > 1) {
    text = `IA: ${state.autoDone} de ${state.autoTotal} archivos listos`;
    if (items.length) text += ` · ${items.length} en proceso`;
  } else if (items.length === 1) {
    text = items[0];
  } else if (items.length > 1) {
    text = `IA: ${items.length} documentos en proceso`;
  }
  setAIActivity(text);
}

// Las páginas escaneadas de todos los documentos comparten un mismo cupo de
// envíos simultáneos.
const pageSlots = { free: PAGES_AT_ONCE, waiting: [] };

function acquirePageSlot() {
  if (pageSlots.free > 0) {
    pageSlots.free--;
    return Promise.resolve();
  }
  return new Promise(resolve => pageSlots.waiting.push(resolve));
}

function releasePageSlot() {
  const next = pageSlots.waiting.shift();
  if (next) next();
  else pageSlots.free++;
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
    setDocActivity(id, 'IA: extrayendo datos…');
    try {
      await call('ExtractAI', id);
    } finally {
      setDocActivity(id, '');
    }
  }
  const fresh = await call('GetConvenio', id);
  notifyRelations(fresh);
  if (state.current?.id === id) {
    state.current = fresh;
    renderDetail();
  }
  await refreshData();
  await loadList();
}

// renderPageJPEG dibuja una página del PDF y la devuelve como JPEG en base64.
// maxSide limita el lado mayor en píxeles y maxScale el aumento.
async function renderPageJPEG(pdf, pageNumber, maxSide, maxScale, quality) {
  const page = await pdf.getPage(pageNumber);
  const base = page.getViewport({ scale: 1 });
  const scale = Math.min(maxScale, maxSide / Math.max(base.width, base.height));
  const viewport = page.getViewport({ scale: Math.max(scale, 0.8) });
  const canvas = document.createElement('canvas');
  canvas.width = Math.ceil(viewport.width);
  canvas.height = Math.ceil(viewport.height);
  const ctx = canvas.getContext('2d', { alpha: false });
  ctx.fillStyle = '#ffffff';
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  await page.render({ canvasContext: ctx, viewport }).promise;
  const base64 = canvas.toDataURL('image/jpeg', quality).split(',')[1];
  canvas.width = canvas.height = 0;
  return base64;
}

// verifyDoubts vuelve a mirar, con más resolución, las páginas donde la
// extracción encontró lecturas contradictorias (un nombre distinto en el
// proemio y bajo la firma, dos fechas de firma). Si la imagen confirma la
// lectura correcta, el servidor corrige la transcripción y vuelve a extraer
// los datos. Devuelve la ficha actualizada o null si no había dudas.
async function verifyDoubts(id, pdf = null) {
  const pages = await call('PagesToVerify', id);
  if (!pages || !pages.length || !window.pdfjsLib) return null;
  setDocActivity(id, 'IA: revisando dudas de lectura con la imagen…');
  let doc = pdf;
  try {
    doc = doc || await window.pdfjsLib.getDocument({ url: `/pdf/${id}` }).promise;
    const images = [];
    for (const n of pages) {
      if (n >= 1 && n <= doc.numPages) images.push({ pagina: n, imagen: await renderPageJPEG(doc, n, 2400, 2.2, 0.9) });
    }
    return images.length ? await call('VerifyAI', id, images) : null;
  } catch (err) {
    console.error(err);
    showToast('No se pudieron revisar las dudas de lectura con la imagen: ' + errorText(err), 'error');
    return null;
  } finally {
    if (doc && !pdf) doc.destroy().catch(() => {});
  }
}

async function transcribeConvention(id, options = {}) {
  const auto = Boolean(options.auto);
  if (state.transcribing.has(id)) {
    if (!auto) showToast('Este documento ya se está transcribiendo.', 'info');
    return;
  }
  if (!window.pdfjsLib) throw new Error('No se pudo cargar el motor de PDF para transcribir.');
  if (!state.config.ai_enabled || !state.config.has_api_key) {
    throw new Error('Activa la IA y configura la clave en Ajustes.');
  }
  state.transcribing.add(id);
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
    if (state.current?.id === id) {
      state.current.transcripcion_estado = 'en_proceso';
      state.current.paginas_por_transcribir = paginas.length;
      state.current.paginas_transcritas = 0;
      renderTranscription();
    }
    setDocActivity(id, `IA: transcribiendo 0/${paginas.length}`);
    let failed = null;
    const transcribePage = async pageNumber => {
      await acquirePageSlot();
      try {
        if (failed) return;
        const base64 = await renderPageJPEG(pdf, pageNumber, 1600, 1.45, 0.82);
        const status = await call('TranscribePage', id, pageNumber, base64);
        if (state.current?.id === id) {
          state.current.paginas_transcritas = status.paginas_transcritas;
          state.current.paginas_por_transcribir = status.paginas_por_transcribir;
          state.current.transcripcion_estado = status.estado;
          if (status.texto) state.current.texto = status.texto;
          renderTranscription();
          renderAICard();
        }
        setDocActivity(id, `IA: transcribiendo ${status.paginas_transcritas}/${status.paginas_por_transcribir}`);
      } catch (err) {
        failed = failed || err;
      } finally {
        releasePageSlot();
      }
    };
    await Promise.all(paginas.map(transcribePage));
    if (failed) throw failed;
    await call('FinishTranscription', id);
    if (state.current?.id === id) {
      state.current = await call('GetConvenio', id);
      renderDetail();
    }
    await refreshData();
    await loadList();
    if (state.config.ai_enabled && state.config.has_api_key) {
      setDocActivity(id, 'IA: extrayendo datos…');
      try {
        await call('ExtractAI', id);
        await verifyDoubts(id, pdf);
        if (state.current?.id === id) {
          state.current = await call('GetConvenio', id);
          renderDetail();
        }
        await refreshData();
        await loadList({ preserveSelection: true });
        if (!auto) showToast('Transcripción completa. Datos extraídos automáticamente.', 'success');
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
    if (pdf) pdf.destroy().catch(() => {});
    state.transcribing.delete(id);
    setDocActivity(id, '');
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
    $('#ai-status').textContent = 'Revisando dudas de lectura…';
    const verified = await verifyDoubts(c.id);
    setDocActivity(c.id, '');
    $('#ai-status').textContent = '';
    if (verified && state.current?.id === c.id) {
      state.current = verified;
      renderDetail();
    }
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

async function downloadPDFs(ids) {
  if (!ids.length) return;
  try {
    const r = await call('DownloadPDFs', ids);
    if (r && r.path) showToast(`${r.total} PDF guardado${r.total === 1 ? '' : 's'} en ${r.path}`, 'success');
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
  try {
    const paths = await call('ChoosePDFs');
    if (!paths || !paths.length) return;
    await openImportWith(paths);
  } catch (err) {
    showToast(errorText(err), 'error');
  }
}

const MAX_IMPORT = 60;

async function openImportWith(paths) {
  if (state.view !== 'convenios' && !await navigate('convenios')) return;
  if (paths.length > MAX_IMPORT) {
    showToast(`Se importan hasta ${MAX_IMPORT} PDF a la vez; se tomarán los primeros ${MAX_IMPORT}.`, 'info');
    paths = paths.slice(0, MAX_IMPORT);
  }
  try {
    state.pendingFiles = paths;
    try {
      state.importChecks = await call('CheckImport', paths);
    } catch (_) {
      state.importChecks = paths.map(p => ({ path: p, name: baseName(p) }));
    }
    $('#btn-import-confirm').hidden = false;
    $('#btn-import-cancel').textContent = 'Cancelar';
    renderImportChecks();
    $('#dialog-import').showModal();
  } catch (err) {
    showToast(errorText(err), 'error');
  }
}

async function confirmImport() {
  if (!state.pendingFiles.length) return;
  const paths = selectedImportPaths();
  if (!paths.length) return;
  const omitted = state.importChecks.filter(c => !c.error).length - paths.length;
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
      <span class="import-item__status">${r.error ? 'Error' : (r.cloned ? 'Duplicado · datos copiados' : (r.duplicate_id ? 'Duplicado' : 'Importado'))}</span>
    </div>${r.error ? `<div class="import-item import-item--error"><span>${esc(r.error)}</span></div>` : ''}`).join('');
    $('#import-summary').className = 'banner ' + (errors.length ? 'banner--warn' : 'banner--info');
    $('#import-summary').textContent = `${ok} importado${ok === 1 ? '' : 's'}${errors.length ? `, ${errors.length} con error` : ''}${omitted > 0 ? `, ${omitted} omitido${omitted === 1 ? '' : 's'} por estar repetido${omitted === 1 ? '' : 's'}` : ''}.`;
    btn.hidden = true;
    $('#btn-import-cancel').textContent = 'Cerrar';
    await refreshData();
    await loadList();
    if (ok > 0) showToast(`${ok} PDF importado${ok === 1 ? '' : 's'}.`, 'success');
    if (errors.length) showToast(`${errors.length} archivo${errors.length === 1 ? '' : 's'} no se pudo importar.`, 'error');
    if (ok > 0 && state.config.ai_enabled && state.config.has_api_key) {
      showToast('La IA procesará automáticamente los archivos importados.', 'info');
      startAutoAI(results.filter(r => !r.error && !r.cloned).map(r => r.id));
    }
    if (ok > 0 && !errors.length) {
      $('#dialog-import').close();
      showView('convenios');
      await loadList();
      renderProcessingBanner();
      syncProcessingPoll();
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
  const map = { convenios: 'view-convenios', detalle: 'view-detail', duplicados: 'view-dups', etiquetas: 'view-tags', ajustes: 'view-settings' };
  $('#' + (map[view] || 'view-convenios')).hidden = false;
  $$('.nav__item').forEach(b => b.classList.toggle('is-active', b.dataset.view === view || (view === 'detalle' && b.dataset.view === 'convenios')));
  const titles = {
    convenios: ['Convenios', 'Administra y consulta los convenios de la UAGro.'],
    detalle: ['Ficha del convenio', 'Revisa el documento, los datos y las etiquetas.'],
    duplicados: ['Duplicados y versiones', 'Encuentra documentos repetidos y convenios renovados.'],
    etiquetas: ['Etiquetas', 'Catálogo local para clasificar los convenios.'],
    ajustes: ['Ajustes', 'Configuración local de la aplicación.']
  };
  const [title, sub] = titles[view] || titles.convenios;
  $('#page-title').textContent = title;
  $('#page-subtitle').textContent = sub;
  window.scrollTo({ top: 0, behavior: 'smooth' });
  if (view !== 'detalle') clearDirty();
  if (view === 'etiquetas') renderTagsView();
  if (view === 'ajustes') renderSettings();
  if (view === 'duplicados') loadDuplicates();
  if (view === 'convenios') {
    renderProcessingBanner();
    syncProcessingPoll();
  } else {
    clearTimeout(state.pollTimer);
    state.pollTimer = null;
  }
}

function renderTagsView() {
  const groups = $('#tags-groups');
  const total = state.tags.length;
  $('#tag-count').textContent = `${total} etiqueta${total === 1 ? '' : 's'}`;
  const q = normalize(state.tagsQuery.trim());
  groups.innerHTML = TYPE_ORDER.map(tipo => {
    const tags = state.tags.filter(t => t.tipo === tipo && (!q || normalize(t.nombre).includes(q)));
    return `<section class="tag-group ${q && !tags.length ? 'is-empty' : ''}">
      <div class="tag-group__head"><h2>${TYPE_LABEL[tipo]}</h2><span class="badge">${tags.length}</span><span class="muted">${tipo === 'ambito' || tipo === 'estado' ? 'Etiquetas base protegidas' : (tipo === 'facultad' ? 'Se crean solas a partir de la facultad del convenio' : 'Se pueden editar')}</span></div>
      <div class="tag-group__list">
        ${tags.length ? tags.map(tag => `<span class="tag-item">
          ${chipHTML(tag)}
          ${tipo === 'ciudad' ? `<span class="tag-item__state ${tag.estado ? '' : 'is-missing'}" title="Estado de la ciudad">${esc(tag.estado || 'Sin estado')}</span>` : ''}
          <span class="tag-item__count">${tag.total || 0}</span>
          ${tag.protegida ? '' : `<span class="tag-item__actions">
            <button type="button" data-edit-tag="${tag.id}" title="Editar ${esc(tag.nombre)}" aria-label="Editar ${esc(tag.nombre)}">✎</button>
            <button type="button" data-delete-tag="${tag.id}" title="Eliminar ${esc(tag.nombre)}" aria-label="Eliminar ${esc(tag.nombre)}">×</button>
          </span>`}
        </span>`).join('') : '<span class="muted">Sin etiquetas todavía.</span>'}
      </div>
    </section>`;
  }).join('');
  fillStateOptions($('#tag-state'));
  syncTagStateField();
}

// fillStateOptions llena una lista con los estados de México (las etiquetas de
// tipo estado) y conserva la opción elegida.
function fillStateOptions(select, value = select.value) {
  const estados = state.tags.filter(t => t.tipo === 'estado').map(t => t.nombre).sort((a, b) => a.localeCompare(b, 'es'));
  select.innerHTML = '<option value="">Sin estado</option>' + estados.map(e => `<option value="${esc(e)}">${esc(e)}</option>`).join('');
  select.value = estados.includes(value) ? value : '';
}

function syncTagStateField() {
  const isCity = $('#tag-type').value === 'ciudad';
  $('#tag-state').disabled = !isCity;
  $('#tag-state-wrap').classList.toggle('is-disabled', !isCity);
}

async function createTag(e) {
  e.preventDefault();
  const nombre = $('#tag-name').value.trim();
  if (!nombre) return;
  const tipo = $('#tag-type').value;
  const btn = e.target.querySelector('button[type="submit"]');
  setButtonBusy(btn, true, 'Creando…');
  try {
    await call('CreateTag', { id: 0, nombre, tipo, color: $('#tag-color').value, estado: tipo === 'ciudad' ? $('#tag-state').value : '' });
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
  $('#tag-edit-state-wrap').hidden = tag.tipo !== 'ciudad';
  fillStateOptions($('#tag-edit-state'), tag.estado || '');
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
    const tipo = $('#tag-edit-type').value;
    await call('UpdateTag', { id, nombre, tipo, color: $('#tag-edit-color').value, estado: tipo === 'ciudad' ? $('#tag-edit-state').value : '' });
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
  $('#settings-analysis-level').value = c.analysis_level || 'low';
  $('#settings-update-check').checked = c.update_check !== false;
  $('#settings-data-dir').textContent = state.app.data_dir || '—';
  $('#settings-exe').textContent = state.app.executable || '—';
  $('#settings-version').textContent = state.app.version || '—';
  $('#settings-build').textContent = buildLabel();
  renderUpdate();
}

function buildLabel() {
  const a = state.app || {};
  if (!a.commit) return 'Compilación local';
  const date = a.build_date ? ' · ' + formatLocalDateTime(a.build_date) : '';
  return a.commit.slice(0, 7) + date;
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
      vision_model: $('#settings-vision-model').value.trim(),
      analysis_level: $('#settings-analysis-level').value
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

function confirmDialog(title, message, okLabel = 'Confirmar', danger = true, options = {}) {
  return new Promise(resolve => {
    if (state.confirmResolve) state.confirmResolve(false);
    state.confirmResolve = resolve;
    $('#confirm-title').textContent = title;
    $('#confirm-message').textContent = message;
    $('#confirm-ok').textContent = okLabel;
    $('#confirm-ok').className = danger ? 'btn btn--danger' : 'btn btn--primary';
    $('#confirm-cancel').hidden = Boolean(options.info);
    if (!$('#dialog-confirm').open) $('#dialog-confirm').showModal();
  });
}

function infoDialog(title, message) {
  return confirmDialog(title, message, 'Entendido', false, { info: true });
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

function showLoading(show, text = '') {
  state.loadingCount = Math.max(0, state.loadingCount + (show ? 1 : -1));
  $('#loading').hidden = state.loadingCount === 0;
  $('#loading-text').textContent = (show && text) ? text : 'Cargando…';
}

function showToast(message, type = 'info') {
  const el = document.createElement('div');
  el.className = 'toast toast--' + type;
  el.setAttribute('role', type === 'error' ? 'alert' : 'status');
  el.innerHTML = `<span>${esc(message)}</span><button type="button" class="toast__close" aria-label="Cerrar aviso">×</button>`;
  const close = () => {
    el.style.opacity = '0';
    el.style.transform = 'translateY(6px)';
    setTimeout(() => el.remove(), 320);
  };
  el.querySelector('.toast__close').addEventListener('click', close);
  $('#toasts').appendChild(el);
  // Los errores se quedan más tiempo para poder leerlos.
  setTimeout(close, type === 'error' ? 9000 : 4600);
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
  return { ambito: '#193060', estado: '#6D6E71', ciudad: '#2E7D32', facultad: '#6A1B9A', tema: '#3A5A9C' }[tipo] || '#3A5A9C';
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

function formatLocalDateTime(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return formatDateTime(iso);
  return d.toLocaleString('es-MX', { dateStyle: 'medium', timeStyle: 'short' });
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

/* ---------- Exportar e importar ---------- */

async function exportPackage(ids) {
  try {
    const r = await call('ExportConvenios', ids || []);
    if (r && r.path) showToast(`${r.total} convenio${r.total === 1 ? '' : 's'} exportado${r.total === 1 ? '' : 's'} en ${r.path}`, 'success');
  } catch (err) {
    showToast(errorText(err), 'error');
  }
}

async function exportCSV(ids, filter) {
  try {
    const f = Object.assign(emptyFilter(), filter || {}, { page: 1 });
    const r = await call('ExportConveniosCSV', ids || [], f);
    if (r && r.path) showToast(`Lista de ${r.total} convenio${r.total === 1 ? '' : 's'} guardada en ${r.path}`, 'success');
  } catch (err) {
    showToast(errorText(err), 'error');
  }
}

async function importPackage(path) {
  try {
    if (!path) {
      path = await call('ChooseImportFile');
      if (!path) return;
    }
    const ok = await confirmDialog(
      'Importar convenios',
      `Se agregarán a esta base los convenios de «${baseName(path)}».\n\nNo se borra nada: los convenios que ya existen (mismo PDF) se omiten.`,
      'Importar', false);
    if (!ok) return;
    showLoading(true, 'Importando convenios…');
    let sum;
    try {
      sum = await call('ImportConvenios', path);
    } finally {
      showLoading(false);
    }
    await refreshData();
    await loadList();
    const partes = [`${sum.importados} importado${sum.importados === 1 ? '' : 's'}`];
    if (sum.duplicados) partes.push(`${sum.duplicados} ya existía${sum.duplicados === 1 ? '' : 'n'}`);
    if (sum.errores && sum.errores.length) partes.push(`${sum.errores.length} con error`);
    const resumen = partes.join(', ') + '.';
    if (sum.errores && sum.errores.length) {
      await infoDialog('Importación terminada con avisos', resumen + '\n\n' + sum.errores.join('\n'));
    } else {
      showToast(resumen, sum.importados ? 'success' : 'info');
    }
  } catch (err) {
    showToast(errorText(err), 'error');
  }
}

/* ---------- Arrastrar y soltar ---------- */

function setupRuntime() {
  const rt = window.runtime;
  if (rt && typeof rt.OnFileDrop === 'function') {
    rt.OnFileDrop((x, y, paths) => {
      hideDropOverlay();
      handleDroppedFiles(paths || []);
    }, false);
  }
  if (rt && typeof rt.EventsOn === 'function') {
    rt.EventsOn('update:progress', onUpdateProgress);
    rt.EventsOn('duplicates:refreshed', async () => {
      await refreshData();
      if (state.view === 'convenios') await loadList();
      if (state.view === 'duplicados') await loadDuplicates();
    });
  }
  const hasFiles = e => Array.from(e.dataTransfer?.types || []).includes('Files');
  document.addEventListener('dragenter', e => {
    if (!hasFiles(e) || document.querySelector('dialog[open]')) return;
    state.dragDepth++;
    $('#drop-overlay').hidden = false;
  });
  document.addEventListener('dragleave', e => {
    if (!hasFiles(e)) return;
    state.dragDepth = Math.max(0, state.dragDepth - 1);
    if (!state.dragDepth) hideDropOverlay();
  });
  document.addEventListener('drop', hideDropOverlay);
}

function hideDropOverlay() {
  state.dragDepth = 0;
  $('#drop-overlay').hidden = true;
}

async function handleDroppedFiles(paths) {
  if (document.querySelector('dialog[open]')) return;
  const pdfs = paths.filter(p => /\.pdf$/i.test(p));
  const packages = paths.filter(p => /\.(zip|db)$/i.test(p));
  if (!pdfs.length && !packages.length) {
    showToast('Solo se pueden importar PDF, paquetes o respaldos .zip y archivos convenios.db.', 'error');
    return;
  }
  for (const p of packages) await importPackage(p);
  if (pdfs.length) await openImportWith(pdfs);
}

/* ---------- Actualizaciones ---------- */

function storageGet(key) {
  try { return localStorage.getItem(key) || ''; } catch (_) { return ''; }
}

function storageSet(key, value) {
  try { localStorage.setItem(key, value); } catch (_) {}
}

function updateKey(u) {
  return u ? `${u.version}|${u.commit || ''}` : '';
}

async function checkForUpdates(manual) {
  if (state.updateChecking) return;
  state.updateChecking = true;
  const btn = $('#btn-check-update');
  if (manual) setButtonBusy(btn, true, 'Buscando…');
  try {
    state.update = await call('CheckUpdate');
    state.updateError = '';
    renderUpdate();
    if (manual) {
      const u = state.update;
      if (u.available) showToast(`Hay una versión nueva: ${u.version}.`, 'info');
      else if (u.pending) showToast('Hay cambios nuevos en GitHub; la versión instalable se está preparando.', 'info');
      else showToast('Ya tienes la versión más reciente.', 'success');
    }
  } catch (err) {
    state.updateError = errorText(err);
    renderUpdate();
    if (manual) showToast(state.updateError, 'error');
  } finally {
    state.updateChecking = false;
    if (manual) setButtonBusy(btn, false, 'Buscar ahora');
  }
}

function renderUpdate() {
  const u = state.update;
  const available = Boolean(u && u.available);
  const dismissed = available && storageGet('update-dismissed') === updateKey(u);
  $('#update-bar').hidden = !available || dismissed;
  $('#nav-update-dot').hidden = !available;
  if (available) {
    $('#update-text').textContent = `Nueva versión disponible: ${u.version}${u.commit ? ` (${u.commit.slice(0, 7)})` : ''}. Tus convenios se conservan al actualizar.`;
  }
  const badge = $('#update-badge');
  if (!badge) return;
  let label = 'Sin revisar';
  let cls = '';
  if (state.updateError) { label = 'Sin conexión'; cls = 'badge--amber'; }
  else if (available) { label = 'Actualización disponible'; cls = 'badge--blue'; }
  else if (u && u.pending) { label = 'Compilando versión nueva'; cls = 'badge--amber'; }
  else if (u) { label = 'Al día'; cls = 'badge--green'; }
  badge.className = 'badge ' + cls;
  badge.textContent = label;
  badge.title = state.updateError || '';
  $('#settings-remote').textContent = u && u.version
    ? `${u.version}${u.commit ? ' · ' + u.commit.slice(0, 7) : ''}${u.published ? ' · ' + formatLocalDateTime(u.published) : ''}`
    : (state.updateError ? state.updateError : '—');
  $('#btn-update-settings').hidden = !available;
}

function showUpdateNotes() {
  const u = state.update;
  if (!u) return;
  $('#update-dialog-title').textContent = `Novedades de la versión ${u.version}`;
  $('#update-notes').textContent = u.notes || 'El release no incluye notas.';
  $('#update-dialog-apply').hidden = !u.available;
  $('#dialog-update').showModal();
}

function dismissUpdate() {
  storageSet('update-dismissed', updateKey(state.update));
  renderUpdate();
}

async function saveUpdatePreference(e) {
  try {
    state.config = await call('SaveSettings', { update_check: e.target.checked });
    showToast(e.target.checked ? 'Se buscarán actualizaciones al iniciar.' : 'Búsqueda automática desactivada.', 'success');
  } catch (err) {
    e.target.checked = !e.target.checked;
    showToast(errorText(err), 'error');
  }
}

function onUpdateProgress(pct) {
  const value = Math.max(0, Math.min(100, Number(pct) || 0));
  $('#update-progress-wrap').hidden = false;
  $('#update-progress').value = value;
  $('#update-progress-text').textContent = `Descargando… ${value}%`;
  $('#update-text').textContent = `Descargando la actualización… ${value}%`;
}

async function applyUpdate() {
  const u = state.update;
  if (!u || !u.available) return;
  const busy = state.autoRunning || state.transcribing.size > 0;
  const ok = await confirmDialog(
    'Actualizar la aplicación',
    `Se descargará la versión ${u.version}, se guardará una copia de la base y la aplicación se reiniciará.\n\nTus convenios, PDF y ajustes no se modifican.` +
      (busy ? '\n\nHay procesamiento con IA en curso: se detendrá; las páginas ya transcritas se conservan y podrás reintentarlo.' : '') +
      (state.dirty ? '\n\nTienes cambios sin guardar en la ficha abierta: se perderán.' : ''),
    'Actualizar', false);
  if (!ok) return;
  $('#update-bar').hidden = false;
  $('#update-text').textContent = 'Descargando la actualización…';
  $('#btn-update-now').disabled = true;
  $('#btn-update-settings').disabled = true;
  try {
    await call('ApplyUpdate');
    $('#update-text').textContent = 'Actualización instalada. Reiniciando…';
    showLoading(true, 'Reiniciando con la versión nueva…');
  } catch (err) {
    $('#update-progress-wrap').hidden = true;
    renderUpdate();
    await infoDialog('No se pudo actualizar', errorText(err));
  } finally {
    $('#btn-update-now').disabled = false;
    $('#btn-update-settings').disabled = false;
  }
}

/* ---------- Duplicados y versiones ---------- */

const MOTIVO = {
  archivo: { label: 'Mismo archivo', help: 'Los dos PDF son idénticos.', cls: 'badge--red' },
  texto: { label: 'Mismo contenido', help: 'El texto es casi igual: puede ser otro escaneo del mismo documento.', cls: 'badge--amber' },
  datos: { label: 'Mismos datos', help: 'Misma contraparte, fecha de firma, tipo de documento y facultad.', cls: 'badge--amber' },
  renovacion: { label: 'Posible renovación', help: 'Misma contraparte, tipo y facultad con otra fecha de firma: uno puede ser la versión renovada del otro.', cls: 'badge--blue' }
};

function bindDuplicateEvents() {
  $('#dups-filter').addEventListener('click', e => {
    const btn = e.target.closest('[data-dups-filter]');
    if (!btn) return;
    state.dupsFilter = btn.dataset.dupsFilter;
    renderDuplicates();
  });
  $('#dups-list').addEventListener('click', onDuplicatesClick);
  $('#view-detail').addEventListener('click', onRelationsClick);
  $('#btn-add-version').addEventListener('click', openVersionDialog);
  $('#version-close').addEventListener('click', () => $('#dialog-version').close());
  $('#version-dir').addEventListener('click', e => {
    const btn = e.target.closest('[data-dir]');
    if (!btn) return;
    state.versionDir = btn.dataset.dir;
    $$('#version-dir [data-dir]').forEach(b => b.classList.toggle('is-active', b === btn));
  });
  let timer = null;
  $('#version-search').addEventListener('input', () => {
    clearTimeout(timer);
    timer = setTimeout(searchVersions, 250);
  });
  $('#version-results').addEventListener('click', e => {
    const btn = e.target.closest('[data-pick]');
    if (btn) pickVersion(Number(btn.dataset.pick), btn.dataset.pickName);
  });
  $('#toggle-old').addEventListener('change', e => {
    state.filters.incluir_anteriores = e.target.checked;
    state.filters.page = 1;
    loadList();
  });
  $('#import-list').addEventListener('change', e => {
    if (e.target.matches('[data-force]')) updateImportButton();
  });
}

function relationName(c) {
  return c.dependencia || c.archivo_nombre || c.archivo_nombre_original || `Convenio #${c.id}`;
}

// olderNewer ordena dos convenios por fecha de firma; sin fechas, el que se
// registró después se toma como el más reciente.
function olderNewer(a, b) {
  if (a.fecha_firma && b.fecha_firma && a.fecha_firma !== b.fecha_firma) {
    return a.fecha_firma < b.fecha_firma ? [a, b] : [b, a];
  }
  return a.id < b.id ? [a, b] : [b, a];
}

/* Importación: aviso antes de subir archivos repetidos */

function importWarning(c) {
  if (c.duplicate_id) return `Ya está registrado como #${c.duplicate_id}${c.duplicate_name ? ` «${c.duplicate_name}»` : ''}.`;
  if (c.repetido) return `Es el mismo archivo que «${c.repetido}» de esta selección.`;
  return '';
}

function renderImportChecks() {
  const checks = state.importChecks;
  $('#import-list').innerHTML = checks.map((c, i) => {
    const warn = importWarning(c);
    const cls = c.error ? 'import-item--error' : (warn ? 'import-item--warn' : '');
    const icon = c.error || warn
      ? '<path d="M12 4 3 20h18L12 4Z"/><path d="M12 9v5m0 3h.01"/>'
      : '<path d="M6 3h8l4 4v14H6V3Z"/><path d="M14 3v5h5"/>';
    return `<div class="import-item ${cls}">
      <svg viewBox="0 0 24 24">${icon}</svg>
      <span>${esc(c.name || baseName(c.path))}${c.error || warn ? `<span class="import-item__note">${esc(c.error || warn)}</span>` : ''}</span>
      ${warn && !c.error ? `<label class="import-item__force"><input type="checkbox" data-force="${i}"> Importar de todos modos</label>` : ''}
    </div>`;
  }).join('');
  const repeated = checks.filter(c => !c.error && importWarning(c)).length;
  const summary = $('#import-summary');
  if (repeated) {
    summary.hidden = false;
    summary.className = 'banner banner--warn';
    summary.textContent = repeated === 1
      ? 'Un archivo ya está registrado o se eligió dos veces. No se importará, salvo que marques «Importar de todos modos».'
      : `${repeated} archivos ya están registrados o se eligieron dos veces. No se importarán, salvo que marques «Importar de todos modos».`;
  } else {
    summary.hidden = true;
    summary.textContent = '';
  }
  updateImportButton();
}

function selectedImportPaths() {
  return state.importChecks
    .filter((c, i) => !c.error && (!importWarning(c) || $(`[data-force="${i}"]`)?.checked))
    .map(c => c.path);
}

function updateImportButton() {
  const n = selectedImportPaths().length;
  const btn = $('#btn-import-confirm');
  btn.disabled = n === 0;
  btn.textContent = n ? `Importar ${n} PDF${n === 1 ? '' : 's'}` : 'Nada que importar';
}

/* Avisos tras procesar con IA */

function notifyRelations(c) {
  const name = relationName(c);
  const rel = c.relacionados || [];
  if (c.versiones > 0) {
    showToast(`«${name}» se registró como versión renovada; la anterior pasó a su historial.`, 'success');
  }
  if (rel.some(r => r.motivo !== 'renovacion')) {
    showToast(`«${name}» parece un duplicado. Revísalo en Duplicados.`, 'info');
  } else if (rel.length) {
    showToast(`«${name}» podría ser la renovación de otro convenio. Revísalo en Duplicados.`, 'info');
  }
}

/* Ficha: avisos e historial */

function vigenciaLabel(v) {
  if (v.vigencia_indefinida) return 'Vigencia indefinida';
  return v.fecha_vigencia ? `Vigencia hasta ${formatDate(v.fecha_vigencia)}` : 'Sin fecha de vigencia';
}

function renderRelations() {
  const c = state.current;
  const hist = c.historial || [];
  const vb = $('#detail-version-banner');
  if (c.version_actual) {
    const cur = hist.find(v => v.actual);
    vb.innerHTML = `<span>Esta es una <strong>versión anterior</strong>. La versión vigente es «${esc(cur?.archivo_nombre || '#' + c.version_actual)}».</span>
      <button class="btn btn--outline btn--small" data-open-version="${c.version_actual}">Abrir la versión vigente</button>`;
    vb.hidden = false;
  } else {
    vb.hidden = true;
  }

  const rel = c.relacionados || [];
  const db = $('#detail-dup-banner');
  if (rel.length) {
    const dups = rel.filter(r => r.motivo !== 'renovacion').length;
    const items = rel.map(r => `<button type="button" class="link" data-open-version="${r.id}">${esc(MOTIVO[r.motivo]?.label || r.motivo)}: ${esc(r.dependencia || r.archivo_nombre)}${r.fecha_firma ? ` (${formatDate(r.fecha_firma)})` : ''}</button>`).join(', ');
    db.innerHTML = `<span>${dups ? 'Este documento parece estar duplicado' : 'Este convenio podría ser una renovación'} · ${items}</span>
      <button class="btn btn--outline btn--small" data-goto-dups>Revisar en Duplicados</button>`;
    db.className = 'banner banner--actions ' + (dups ? 'banner--warn' : 'banner--info');
    db.hidden = false;
  } else {
    db.hidden = true;
  }

  $('#history-hint').textContent = hist.length
    ? `${hist.length} versiones. En la lista principal solo aparece la vigente.`
    : 'Si este convenio renueva a otro (o fue renovado), agrégalo para ver todas sus versiones juntas.';
  $('#history-list').innerHTML = hist.length ? `<ol class="history-list">${hist.map(v => `
    <li class="history-item ${v.id === c.id ? 'is-current' : ''}">
      <div class="history-item__main">
        <strong>${v.actual ? 'Vigente' : 'Anterior'} · firmado ${formatDate(v.fecha_firma) || 'sin fecha'} ${v.id === c.id ? '<span class="badge badge--tiny">Abierta</span>' : ''}</strong>
        <span class="muted">${esc(v.archivo_nombre)}</span>
        <span class="muted">${vigenciaLabel(v)}</span>
      </div>
      <div class="history-item__actions">
        ${v.id !== c.id ? `<button class="btn btn--outline btn--small" data-open-version="${v.id}">Abrir</button>` : ''}
        <button class="btn btn--text btn--small" data-download-version="${v.id}">PDF</button>
        ${v.actual ? '' : `<button class="btn btn--text btn--small" data-unlink-version="${v.id}" title="Vuelve a mostrarlo como convenio independiente">Quitar del historial</button>`}
      </div>
    </li>`).join('')}</ol>` : '';
}

async function openOther(id) {
  if (state.dirty && state.view === 'detalle') {
    const ok = await confirmDialog('Cambios sin guardar', 'Hay cambios en los datos del convenio que no se han guardado. ¿Abrir el otro convenio sin guardarlos?', 'Abrir sin guardar');
    if (!ok) return;
    clearDirty();
  }
  if ($('#dialog-version').open) $('#dialog-version').close();
  openDetail(id);
}

async function onRelationsClick(e) {
  const noConv = e.target.closest('[data-no-convenio]');
  if (noConv) return setNoConvenio(noConv.dataset.noConvenio === '1');
  if (e.target.closest('[data-delete-current]')) return deleteCurrent();
  const open = e.target.closest('[data-open-version]');
  if (open) return openOther(Number(open.dataset.openVersion));
  const dl = e.target.closest('[data-download-version]');
  if (dl) return downloadPDF(Number(dl.dataset.downloadVersion));
  if (e.target.closest('[data-goto-dups]')) return navigate('duplicados');
  const un = e.target.closest('[data-unlink-version]');
  if (un) {
    const ok = await confirmDialog('Quitar del historial', 'Este convenio volverá a aparecer en la lista como un convenio independiente. No se borra nada.', 'Quitar', false);
    if (!ok) return;
    try {
      await call('UnlinkVersion', Number(un.dataset.unlinkVersion));
      await reloadAfterRelationChange();
      showToast('Se quitó del historial.', 'success');
    } catch (err) {
      showToast(errorText(err), 'error');
    }
  }
}

async function reloadAfterRelationChange() {
  if (state.view === 'detalle' && state.current) {
    state.current = await call('GetConvenio', state.current.id);
    renderDetail();
  }
  if (state.view === 'duplicados') await loadDuplicates();
  await refreshData();
  await loadList();
}

function openVersionDialog() {
  const c = state.current;
  if (!c) return;
  state.versionDir = 'older';
  $$('#version-dir [data-dir]').forEach(b => b.classList.toggle('is-active', b.dataset.dir === 'older'));
  $('#version-intro').textContent = `Elige el convenio que pertenece al historial de «${relationName(c)}». La versión más reciente es la que se mostrará en la lista.`;
  $('#version-search').value = c.dependencia_corta || c.dependencia || '';
  $('#version-results').innerHTML = '';
  $('#dialog-version').showModal();
  searchVersions();
}

async function searchVersions() {
  const c = state.current;
  if (!c) return;
  const q = $('#version-search').value.trim();
  try {
    const res = await call('ListConvenios', Object.assign(emptyFilter(), {
      query: q, incluir_anteriores: true, order: q ? 'relevancia' : 'fecha_firma', dir: 'desc'
    }));
    const family = new Set((c.historial || []).map(v => v.id));
    family.add(c.id);
    const items = (res.items || []).filter(i => !family.has(i.id));
    $('#version-results').innerHTML = items.length ? items.map(i => `
      <button type="button" class="version-result" data-pick="${i.id}" data-pick-name="${esc(relationName(i))}">
        <strong>${esc(relationName(i))}${i.reemplazado_por ? ' <span class="badge badge--tiny">Versión anterior</span>' : ''}</strong>
        <span class="muted">${esc(i.archivo_nombre || i.archivo_nombre_original)}</span>
        <span class="muted">Firma: ${formatDate(i.fecha_firma) || '—'} · ${i.vigencia_indefinida ? 'Vigencia indefinida' : `Vigencia: ${formatDate(i.fecha_vigencia) || '—'}`}</span>
      </button>`).join('') : '<div class="version-empty">No se encontraron otros convenios.</div>';
  } catch (err) {
    $('#version-results').innerHTML = `<div class="version-empty">${esc(errorText(err))}</div>`;
  }
}

async function pickVersion(id, name) {
  const c = state.current;
  const older = state.versionDir === 'older' ? id : c.id;
  const newer = state.versionDir === 'older' ? c.id : id;
  const olderName = older === c.id ? relationName(c) : name;
  const newerName = newer === c.id ? relationName(c) : name;
  $('#dialog-version').close();
  await linkVersions(older, newer, olderName, newerName);
}

async function linkVersions(older, newer, olderName, newerName) {
  const ok = await confirmDialog('Unir en el historial',
    `«${olderName}» pasará a ser una versión anterior de «${newerName}».\n\nEn la lista principal solo se verá la versión más reciente; la anterior queda en su historial y se puede abrir o descargar cuando quieras.`,
    'Unir', false);
  if (!ok) return;
  try {
    await call('LinkVersion', older, newer);
    await reloadAfterRelationChange();
    showToast('Historial actualizado.', 'success');
  } catch (err) {
    showToast(errorText(err), 'error');
  }
}

/* Herramienta de revisión */

async function loadDuplicates() {
  const list = $('#dups-list');
  if (!state.dups.length) list.innerHTML = '<div class="card version-empty">Buscando duplicados…</div>';
  try {
    state.dups = await call('ListDuplicates') || [];
  } catch (err) {
    showToast(errorText(err), 'error');
    state.dups = [];
  }
  renderDuplicates();
}

function renderDuplicates() {
  const all = state.dups;
  const nDup = all.filter(p => p.motivo !== 'renovacion').length;
  $('#dups-n-all').textContent = all.length;
  $('#dups-n-dup').textContent = nDup;
  $('#dups-n-ren').textContent = all.length - nDup;
  $$('#dups-filter [data-dups-filter]').forEach(b => b.classList.toggle('is-active', b.dataset.dupsFilter === state.dupsFilter));
  const f = state.dupsFilter;
  const list = all.filter(p => !f || (f === 'renovacion' ? p.motivo === 'renovacion' : p.motivo !== 'renovacion'));
  if (!list.length) {
    $('#dups-list').innerHTML = `<div class="card empty-state empty-filter">
      <h2>${all.length ? 'Nada en este filtro' : 'No hay duplicados por revisar'}</h2>
      <p>${all.length ? 'Cambia el filtro para ver los demás pares.' : 'Cuando se registre un documento repetido o una posible renovación, aparecerá aquí.'}</p>
    </div>`;
    return;
  }
  $('#dups-list').innerHTML = list.map(dupCard).join('');
}

function dupSide(c, tag) {
  return `<div class="dup-side">
    <div class="dup-side__head"><strong>${esc(c.dependencia || 'Sin dependencia capturada')}</strong>${tag ? `<span class="badge badge--tiny">${tag}</span>` : ''}</div>
    <div class="dep-file">${esc(c.archivo_nombre || c.archivo_nombre_original)}</div>
    <dl class="dup-side__meta">
      <dt>Firma</dt><dd>${formatDate(c.fecha_firma) || '—'}</dd>
      <dt>Vigencia</dt><dd>${c.vigencia_indefinida ? 'Indefinida' : (formatDate(c.fecha_vigencia) || '—')}</dd>
      <dt>Páginas</dt><dd>${c.paginas || '—'}</dd>
      <dt>Registrado</dt><dd>${formatDateTime(c.creado_en)}</dd>
      <dt>Archivo original</dt><dd>${esc(c.archivo_nombre_original)}</dd>
    </dl>
    <div class="dup-side__actions">
      <button class="btn btn--outline btn--small" data-open-version="${c.id}">Abrir</button>
      <button class="btn btn--text btn--small" data-download-version="${c.id}">PDF</button>
      <button class="btn btn--danger-text btn--small" data-dup-delete="${c.id}">Eliminar</button>
    </div>
  </div>`;
}

function dupLabels(p) {
  const [older, newer] = olderNewer(p.a, p.b);
  const isRen = p.motivo === 'renovacion';
  return { older, newer, isRen, tagOld: isRen ? 'Anterior' : 'Registrado primero', tagNew: isRen ? 'Más reciente' : '' };
}

function dupCompareHTML(p) {
  const { older, newer, tagOld, tagNew } = dupLabels(p);
  const pane = (c, tag) => `<figure class="dup-compare__pane">
      <figcaption><strong>${esc(tag)}</strong><span title="${esc(c.archivo_nombre_original)}">Archivo original: ${esc(c.archivo_nombre_original)}</span></figcaption>
      <iframe src="/pdf/${c.id}" title="PDF de ${esc(c.dependencia || c.archivo_nombre_original)}"></iframe>
    </figure>`;
  return pane(older, tagOld) + pane(newer, tagNew || 'Registrado después');
}

function dupCard(p) {
  const m = MOTIVO[p.motivo] || { label: p.motivo, help: '', cls: '' };
  const { older, newer, isRen, tagOld, tagNew } = dupLabels(p);
  const key = `${older.id}:${newer.id}`;
  const open = state.dupCompare === key;
  const link = `data-dup-link="${older.id}:${newer.id}"`;
  const dismiss = `data-dup-dismiss="${p.a.id}:${p.b.id}"`;
  return `<article class="card dup-card">
    <header class="dup-card__head"><span class="badge ${m.cls}">${esc(m.label)}</span><p class="muted">${esc(m.help)}</p></header>
    <div class="dup-card__cols">${dupSide(older, tagOld)}${dupSide(newer, tagNew)}</div>
    <div class="dup-compare" data-compare-panel="${key}" ${open ? '' : 'hidden'}>${open ? dupCompareHTML(p) : ''}</div>
    <footer class="dup-card__actions">
      <button class="btn btn--outline btn--small dup-compare-toggle" data-dup-compare="${key}" aria-expanded="${open}">
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg>
        <span>${open ? 'Ocultar PDFs' : 'Comparar PDFs'}</span>
      </button>
      ${isRen
        ? `<button class="btn btn--outline btn--small" ${dismiss}>No están relacionados</button>
           <button class="btn btn--primary btn--small" ${link}>Unir en el historial</button>`
        : `<button class="btn btn--text btn--small" ${link} title="El más reciente quedará como vigente">Es una renovación</button>
           <button class="btn btn--outline btn--small" ${dismiss}>No son duplicados</button>`}
    </footer>
  </article>`;
}

function findDupConvenio(id) {
  for (const p of state.dups) {
    if (p.a.id === id) return p.a;
    if (p.b.id === id) return p.b;
  }
  return { id };
}

// toggleDupCompare abre los dos PDF del par uno junto al otro. Solo queda una
// comparación abierta porque cada visor PDF consume memoria.
function toggleDupCompare(btn) {
  const key = btn.dataset.dupCompare;
  const opening = state.dupCompare !== key;
  $$('#dups-list [data-compare-panel]').forEach(panel => {
    panel.hidden = true;
    panel.innerHTML = '';
  });
  $$('#dups-list [data-dup-compare]').forEach(b => {
    b.setAttribute('aria-expanded', 'false');
    b.querySelector('span').textContent = 'Comparar PDFs';
  });
  state.dupCompare = opening ? key : '';
  if (!opening) return;
  const p = state.dups.find(d => dupLabels(d).older.id + ':' + dupLabels(d).newer.id === key);
  const panel = $(`#dups-list [data-compare-panel="${key}"]`);
  if (!p || !panel) return;
  panel.innerHTML = dupCompareHTML(p);
  panel.hidden = false;
  btn.setAttribute('aria-expanded', 'true');
  btn.querySelector('span').textContent = 'Ocultar PDFs';
  panel.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

async function onDuplicatesClick(e) {
  const cmp = e.target.closest('[data-dup-compare]');
  if (cmp) return toggleDupCompare(cmp);
  const open = e.target.closest('[data-open-version]');
  if (open) return openDetail(Number(open.dataset.openVersion));
  const dl = e.target.closest('[data-download-version]');
  if (dl) return downloadPDF(Number(dl.dataset.downloadVersion));
  const del = e.target.closest('[data-dup-delete]');
  if (del) {
    const c = findDupConvenio(Number(del.dataset.dupDelete));
    const ok = await confirmDialog('Eliminar convenio', `¿Eliminar «${relationName(c)}»? También se borrará su PDF. Esta acción no se puede deshacer.`, 'Eliminar');
    if (!ok) return;
    try {
      await call('DeleteConvenios', [c.id]);
      state.selected.delete(c.id);
      await reloadAfterRelationChange();
      showToast('Convenio eliminado.', 'success');
    } catch (err) {
      showToast(errorText(err), 'error');
    }
    return;
  }
  const dismiss = e.target.closest('[data-dup-dismiss]');
  if (dismiss) {
    const [a, b] = dismiss.dataset.dupDismiss.split(':').map(Number);
    try {
      await call('DismissDuplicate', a, b);
      await reloadAfterRelationChange();
      showToast('No se volverán a sugerir juntos.', 'success');
    } catch (err) {
      showToast(errorText(err), 'error');
    }
    return;
  }
  const link = e.target.closest('[data-dup-link]');
  if (link) {
    const [older, newer] = link.dataset.dupLink.split(':').map(Number);
    await linkVersions(older, newer, relationName(findDupConvenio(older)), relationName(findDupConvenio(newer)));
  }
}
