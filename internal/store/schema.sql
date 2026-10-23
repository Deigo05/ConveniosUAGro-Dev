PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS convenios (
  id                      INTEGER PRIMARY KEY AUTOINCREMENT,
  dependencia             TEXT NOT NULL DEFAULT '',
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
  creada_en TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_etiquetas_tipo_nombre ON etiquetas(tipo, nombre);

CREATE TABLE IF NOT EXISTS convenio_etiquetas (
  convenio_id INTEGER NOT NULL REFERENCES convenios(id) ON DELETE CASCADE,
  etiqueta_id INTEGER NOT NULL REFERENCES etiquetas(id) ON DELETE CASCADE,
  PRIMARY KEY (convenio_id, etiqueta_id)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS ix_convenio_etiquetas_etiqueta ON convenio_etiquetas(etiqueta_id);

-- Texto transcrito por página. Se conserva si el proceso se interrumpe.
CREATE TABLE IF NOT EXISTS transcripcion_paginas (
  convenio_id INTEGER NOT NULL REFERENCES convenios(id) ON DELETE CASCADE,
  pagina      INTEGER NOT NULL,
  texto       TEXT NOT NULL,
  PRIMARY KEY (convenio_id, pagina)
) WITHOUT ROWID;

-- Índice de búsqueda de texto completo. Se mantiene con los disparadores de abajo.
CREATE VIRTUAL TABLE IF NOT EXISTS convenios_fts USING fts5(
  dependencia,
  archivo_nombre_original,
  notas,
  texto,
  content='convenios',
  content_rowid='id',
  tokenize='unicode61 remove_diacritics 2'
);

CREATE TRIGGER IF NOT EXISTS convenios_ai AFTER INSERT ON convenios BEGIN
  INSERT INTO convenios_fts(rowid, dependencia, archivo_nombre_original, notas, texto)
  VALUES (new.id, new.dependencia, new.archivo_nombre_original, new.notas, new.texto);
END;

CREATE TRIGGER IF NOT EXISTS convenios_ad AFTER DELETE ON convenios BEGIN
  INSERT INTO convenios_fts(convenios_fts, rowid, dependencia, archivo_nombre_original, notas, texto)
  VALUES ('delete', old.id, old.dependencia, old.archivo_nombre_original, old.notas, old.texto);
END;

CREATE TRIGGER IF NOT EXISTS convenios_au
AFTER UPDATE OF dependencia, archivo_nombre_original, notas, texto ON convenios BEGIN
  INSERT INTO convenios_fts(convenios_fts, rowid, dependencia, archivo_nombre_original, notas, texto)
  VALUES ('delete', old.id, old.dependencia, old.archivo_nombre_original, old.notas, old.texto);
  INSERT INTO convenios_fts(rowid, dependencia, archivo_nombre_original, notas, texto)
  VALUES (new.id, new.dependencia, new.archivo_nombre_original, new.notas, new.texto);
END;
