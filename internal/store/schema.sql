PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS convenios (
  id                      INTEGER PRIMARY KEY AUTOINCREMENT,
  dependencia             TEXT NOT NULL DEFAULT '',
  dependencia_corta       TEXT NOT NULL DEFAULT '',
  tipo_documento          TEXT NOT NULL DEFAULT '',
  huella                  INTEGER,
  reemplazado_por         INTEGER REFERENCES convenios(id) ON DELETE SET NULL,
  -- 0: normal; 1: la IA cree que el archivo no es un convenio; 2: el usuario
  -- confirmó que sí lo es (la IA ya no lo vuelve a marcar).
  no_es_convenio          INTEGER NOT NULL DEFAULT 0,
  firmante_dependencia    TEXT NOT NULL DEFAULT '',
  cargo_dependencia       TEXT NOT NULL DEFAULT '',
  firmante_uagro          TEXT NOT NULL DEFAULT '',
  cargo_uagro             TEXT NOT NULL DEFAULT '',
  facultad                TEXT NOT NULL DEFAULT '',
  facultad_ciudad         TEXT NOT NULL DEFAULT '',
  fecha_firma             TEXT,
  fecha_vigencia          TEXT,
  vigencia_indefinida     INTEGER NOT NULL DEFAULT 0,
  vigencia_texto          TEXT NOT NULL DEFAULT '',
  vigencia_calculada      INTEGER NOT NULL DEFAULT 0,
  notas                   TEXT NOT NULL DEFAULT '',
  archivo_nombre_original TEXT NOT NULL,
  archivo_guardado        TEXT NOT NULL UNIQUE,
  archivo_bytes           INTEGER NOT NULL DEFAULT 0,
  archivo_sha256          TEXT NOT NULL DEFAULT '',
  paginas                 INTEGER NOT NULL DEFAULT 0,
  es_escaneado            INTEGER NOT NULL DEFAULT 0,
  texto_pdf               TEXT NOT NULL DEFAULT '',
  texto                   TEXT NOT NULL DEFAULT '',
  busqueda                TEXT NOT NULL DEFAULT '',
  ia_estado               TEXT NOT NULL DEFAULT 'omitido'
                          CHECK (ia_estado IN ('omitido','pendiente','ok','parcial','error')),
  ia_error                TEXT NOT NULL DEFAULT '',
  etiquetas_sugeridas     TEXT NOT NULL DEFAULT '',
  etiquetas_revisadas     INTEGER NOT NULL DEFAULT 0,
  transcripcion_estado    TEXT NOT NULL DEFAULT 'no_requerida'
                          CHECK (transcripcion_estado IN ('no_requerida','pendiente','en_proceso','completa','error')),
  transcripcion_error     TEXT NOT NULL DEFAULT '',
  paginas_por_transcribir INTEGER NOT NULL DEFAULT 0,
  paginas_transcritas     INTEGER NOT NULL DEFAULT 0,
  creado_en               TEXT NOT NULL DEFAULT (datetime('now')),
  actualizado_en          TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS ix_convenios_sha ON convenios(archivo_sha256);
CREATE INDEX IF NOT EXISTS ix_convenios_fechas ON convenios(fecha_vigencia, fecha_firma);

CREATE TABLE IF NOT EXISTS etiquetas (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  nombre    TEXT NOT NULL COLLATE NOCASE,
  tipo      TEXT NOT NULL CHECK (tipo IN ('ambito','estado','ciudad','tema','facultad')),
  color     TEXT NOT NULL DEFAULT '#3A5A9C',
  protegida INTEGER NOT NULL DEFAULT 0,
  -- Estado al que pertenece una etiqueta de ciudad; evita combinar, por
  -- ejemplo, "Acapulco de Juárez" con "Veracruz".
  estado    TEXT NOT NULL DEFAULT '',
  creada_en TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_etiquetas_tipo_nombre ON etiquetas(tipo, nombre);

CREATE TABLE IF NOT EXISTS convenio_etiquetas (
  convenio_id INTEGER NOT NULL REFERENCES convenios(id) ON DELETE CASCADE,
  etiqueta_id INTEGER NOT NULL REFERENCES etiquetas(id) ON DELETE CASCADE,
  PRIMARY KEY (convenio_id, etiqueta_id)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS ix_convenio_etiquetas_etiqueta ON convenio_etiquetas(etiqueta_id);

-- Pares de convenios que parecen duplicados o renovaciones. Se recalculan con
-- RefreshDuplicates; al borrar un convenio sus pares desaparecen solos.
CREATE TABLE IF NOT EXISTS convenio_duplicados (
  convenio_a INTEGER NOT NULL REFERENCES convenios(id) ON DELETE CASCADE,
  convenio_b INTEGER NOT NULL REFERENCES convenios(id) ON DELETE CASCADE,
  motivo     TEXT NOT NULL CHECK (motivo IN ('archivo', 'texto', 'datos', 'renovacion')),
  PRIMARY KEY (convenio_a, convenio_b),
  CHECK (convenio_a < convenio_b)
);
CREATE INDEX IF NOT EXISTS ix_convenio_duplicados_b ON convenio_duplicados(convenio_b);

-- Pares que el usuario revisó y marcó como distintos: no se vuelven a sugerir.
CREATE TABLE IF NOT EXISTS duplicados_descartados (
  convenio_a INTEGER NOT NULL REFERENCES convenios(id) ON DELETE CASCADE,
  convenio_b INTEGER NOT NULL REFERENCES convenios(id) ON DELETE CASCADE,
  PRIMARY KEY (convenio_a, convenio_b),
  CHECK (convenio_a < convenio_b)
);

-- Marcas de mantenimiento ya aplicadas (reparaciones que solo corren una vez).
CREATE TABLE IF NOT EXISTS meta (
  clave TEXT PRIMARY KEY,
  valor TEXT NOT NULL DEFAULT ''
) WITHOUT ROWID;

-- Texto transcrito por página. Se conserva si el proceso se interrumpe.
CREATE TABLE IF NOT EXISTS transcripcion_paginas (
  convenio_id INTEGER NOT NULL REFERENCES convenios(id) ON DELETE CASCADE,
  pagina      INTEGER NOT NULL,
  texto       TEXT NOT NULL,
  PRIMARY KEY (convenio_id, pagina)
) WITHOUT ROWID;

-- El índice de búsqueda (convenios_fts) y sus disparadores están en search.sql;
-- se crean después de las migraciones porque dependen de la columna busqueda.
