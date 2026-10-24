-- Índice de búsqueda de texto completo. Guarda su propia copia del texto para
-- que cada disparador pueda borrar por rowid y volver a leer la fila actual:
-- así el índice queda correcto sin importar el orden en que SQLite ejecute los
-- disparadores encadenados.
CREATE VIRTUAL TABLE IF NOT EXISTS convenios_fts USING fts5(
  dependencia,
  archivo_nombre_original,
  notas,
  texto,
  busqueda,
  tokenize='unicode61 remove_diacritics 2'
);

CREATE TRIGGER IF NOT EXISTS convenios_fts_ai AFTER INSERT ON convenios BEGIN
  DELETE FROM convenios_fts WHERE rowid = new.id;
  INSERT INTO convenios_fts(rowid, dependencia, archivo_nombre_original, notas, texto, busqueda)
  SELECT id, dependencia, archivo_nombre_original, notas, texto, busqueda FROM convenios WHERE id = new.id;
END;

CREATE TRIGGER IF NOT EXISTS convenios_fts_ad AFTER DELETE ON convenios BEGIN
  DELETE FROM convenios_fts WHERE rowid = old.id;
END;

CREATE TRIGGER IF NOT EXISTS convenios_fts_au
AFTER UPDATE OF dependencia, archivo_nombre_original, notas, texto, busqueda ON convenios BEGIN
  DELETE FROM convenios_fts WHERE rowid = new.id;
  INSERT INTO convenios_fts(rowid, dependencia, archivo_nombre_original, notas, texto, busqueda)
  SELECT id, dependencia, archivo_nombre_original, notas, texto, busqueda FROM convenios WHERE id = new.id;
END;

-- "busqueda" junta siglas, tipo de documento, firmantes, cargos, facultad, cláusula de vigencia y los
-- nombres de las etiquetas, para encontrar un convenio desde el buscador
-- principal por cualquiera de esos datos.
CREATE TRIGGER IF NOT EXISTS convenios_busqueda_au
AFTER UPDATE OF dependencia_corta, tipo_documento, firmante_dependencia, cargo_dependencia,
  firmante_uagro, cargo_uagro, facultad, facultad_ciudad, vigencia_texto ON convenios BEGIN
  UPDATE convenios SET busqueda = trim(
    dependencia_corta || ' ' || tipo_documento || ' ' ||
    firmante_dependencia || ' ' || cargo_dependencia || ' ' || firmante_uagro || ' ' ||
    cargo_uagro || ' ' || facultad || ' ' || facultad_ciudad || ' ' || vigencia_texto || ' ' ||
    coalesce((SELECT group_concat(e.nombre, ' ') FROM convenio_etiquetas ce
              JOIN etiquetas e ON e.id = ce.etiqueta_id WHERE ce.convenio_id = convenios.id), ''))
  WHERE id = new.id;
END;

CREATE TRIGGER IF NOT EXISTS convenio_etiquetas_busqueda_ai AFTER INSERT ON convenio_etiquetas BEGIN
  UPDATE convenios SET busqueda = trim(
    dependencia_corta || ' ' || tipo_documento || ' ' ||
    firmante_dependencia || ' ' || cargo_dependencia || ' ' || firmante_uagro || ' ' ||
    cargo_uagro || ' ' || facultad || ' ' || facultad_ciudad || ' ' || vigencia_texto || ' ' ||
    coalesce((SELECT group_concat(e.nombre, ' ') FROM convenio_etiquetas ce
              JOIN etiquetas e ON e.id = ce.etiqueta_id WHERE ce.convenio_id = convenios.id), ''))
  WHERE id = new.convenio_id;
END;

CREATE TRIGGER IF NOT EXISTS convenio_etiquetas_busqueda_ad AFTER DELETE ON convenio_etiquetas BEGIN
  UPDATE convenios SET busqueda = trim(
    dependencia_corta || ' ' || tipo_documento || ' ' ||
    firmante_dependencia || ' ' || cargo_dependencia || ' ' || firmante_uagro || ' ' ||
    cargo_uagro || ' ' || facultad || ' ' || facultad_ciudad || ' ' || vigencia_texto || ' ' ||
    coalesce((SELECT group_concat(e.nombre, ' ') FROM convenio_etiquetas ce
              JOIN etiquetas e ON e.id = ce.etiqueta_id WHERE ce.convenio_id = convenios.id), ''))
  WHERE id = old.convenio_id;
END;

CREATE TRIGGER IF NOT EXISTS etiquetas_busqueda_au AFTER UPDATE OF nombre ON etiquetas BEGIN
  UPDATE convenios SET busqueda = trim(
    dependencia_corta || ' ' || tipo_documento || ' ' ||
    firmante_dependencia || ' ' || cargo_dependencia || ' ' || firmante_uagro || ' ' ||
    cargo_uagro || ' ' || facultad || ' ' || facultad_ciudad || ' ' || vigencia_texto || ' ' ||
    coalesce((SELECT group_concat(e.nombre, ' ') FROM convenio_etiquetas ce
              JOIN etiquetas e ON e.id = ce.etiqueta_id WHERE ce.convenio_id = convenios.id), ''))
  WHERE id IN (SELECT convenio_id FROM convenio_etiquetas WHERE etiqueta_id = new.id);
END;

-- Índice sobre una columna que agregan las migraciones (por eso va aquí y no
-- en schema.sql, que se ejecuta antes de migrar).
CREATE INDEX IF NOT EXISTS ix_convenios_reemplazado ON convenios(reemplazado_por);
