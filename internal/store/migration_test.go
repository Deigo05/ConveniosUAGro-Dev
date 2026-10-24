package store

import (
	"database/sql"
	"testing"
)

func TestMigrationV1ToLatest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONVENIOS_DATA_DIR", dir)
	paths, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", paths.DB)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE convenios (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			dependencia TEXT NOT NULL DEFAULT '',
			fecha_firma TEXT,
			fecha_vigencia TEXT,
			vigencia_indefinida INTEGER NOT NULL DEFAULT 0,
			vigencia_texto TEXT NOT NULL DEFAULT '',
			vigencia_calculada INTEGER NOT NULL DEFAULT 0,
			notas TEXT NOT NULL DEFAULT '',
			archivo_nombre_original TEXT NOT NULL,
			archivo_guardado TEXT NOT NULL UNIQUE,
			archivo_bytes INTEGER NOT NULL DEFAULT 0,
			archivo_sha256 TEXT NOT NULL DEFAULT '',
			paginas INTEGER NOT NULL DEFAULT 0,
			es_escaneado INTEGER NOT NULL DEFAULT 0,
			texto TEXT NOT NULL DEFAULT '',
			ia_estado TEXT NOT NULL DEFAULT 'omitido',
			ia_error TEXT NOT NULL DEFAULT '',
			etiquetas_sugeridas TEXT NOT NULL DEFAULT '',
			etiquetas_revisadas INTEGER NOT NULL DEFAULT 0,
			creado_en TEXT NOT NULL DEFAULT (datetime('now')),
			actualizado_en TEXT NOT NULL DEFAULT (datetime('now'))
		);
		INSERT INTO convenios (archivo_nombre_original, archivo_guardado, paginas, es_escaneado, texto)
		VALUES ('viejo.pdf', 'dddddddddddddddddddddddddddddddd.pdf', 2, 1, 'texto viejo');
		PRAGMA user_version=1;
	`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	st, err := Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var version int
	if err := st.DB().QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Fatalf("user_version = %d", version)
	}
	d, err := st.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	if d.Texto != "texto viejo" || d.TranscripcionEstado != "pendiente" || d.PaginasPorTranscribir != 2 {
		t.Fatalf("migración inesperada: %+v", d)
	}
	res, err := st.List(ListFilter{Query: "viejo"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 {
		t.Fatalf("la búsqueda tras migrar devolvió %d resultados", res.Total)
	}
}

// TestMigrationV5ToV6 parte de una base con el índice FTS anterior (contenido
// externo) y comprueba que el nuevo índice busca también firmantes y etiquetas.
func TestMigrationV5ToV6(t *testing.T) {
	st, paths := testStore(t)
	id := insertSample(t, st, "Convenio con texto original")
	if err := st.Update(ConventionInput{ID: id, Dependencia: "Ayuntamiento", FirmanteUAGro: "Juan Ramón García"}); err != nil {
		t.Fatal(err)
	}
	// Regresa la base al esquema 5 con el índice antiguo.
	for _, stmt := range []string{
		"DROP TRIGGER convenios_fts_ai", "DROP TRIGGER convenios_fts_ad", "DROP TRIGGER convenios_fts_au",
		"DROP TRIGGER convenios_busqueda_au", "DROP TRIGGER convenio_etiquetas_busqueda_ai",
		"DROP TRIGGER convenio_etiquetas_busqueda_ad", "DROP TRIGGER etiquetas_busqueda_au", "DROP TABLE convenios_fts",
		"ALTER TABLE convenios DROP COLUMN busqueda",
		"ALTER TABLE convenios DROP COLUMN dependencia_corta",
		"ALTER TABLE convenios DROP COLUMN tipo_documento",
		"ALTER TABLE convenios DROP COLUMN huella",
		`CREATE VIRTUAL TABLE convenios_fts USING fts5(dependencia, archivo_nombre_original, notas, texto,
			content='convenios', content_rowid='id', tokenize='unicode61 remove_diacritics 2')`,
		"INSERT INTO convenios_fts(convenios_fts) VALUES('rebuild')",
		`CREATE TRIGGER convenios_au AFTER UPDATE OF dependencia, archivo_nombre_original, notas, texto ON convenios BEGIN
			INSERT INTO convenios_fts(convenios_fts, rowid, dependencia, archivo_nombre_original, notas, texto)
			VALUES ('delete', old.id, old.dependencia, old.archivo_nombre_original, old.notas, old.texto);
			INSERT INTO convenios_fts(rowid, dependencia, archivo_nombre_original, notas, texto)
			VALUES (new.id, new.dependencia, new.archivo_nombre_original, new.notas, new.texto);
		END`,
		"PRAGMA user_version=5",
	} {
		if _, err := st.DB().Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	st.Close()
	st2, err := Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	for _, q := range []string{"original", "Garcia", "Juan Ramón"} {
		res, err := st2.List(ListFilter{Query: q})
		if err != nil {
			t.Fatal(err)
		}
		if res.Total != 1 {
			t.Fatalf("búsqueda %q tras migrar: %d resultados", q, res.Total)
		}
	}
	// Las columnas nuevas existen y alimentan la búsqueda.
	if err := st2.Update(ConventionInput{ID: id, Dependencia: "Ayuntamiento", DependenciaCorta: "AYTO", TipoDocumento: "Convenio Marco"}); err != nil {
		t.Fatal(err)
	}
	if res, err := st2.List(ListFilter{Query: "AYTO"}); err != nil || res.Total != 1 {
		t.Fatalf("búsqueda por siglas tras migrar: %+v, %v", res, err)
	}
}

// TestRepairIncompleteV6 abre una base que una compilación de prueba dejó en
// la versión 6 sin las últimas columnas.
func TestRepairIncompleteV6(t *testing.T) {
	st, paths := testStore(t)
	id := insertSample(t, st, "texto de prueba")
	for _, stmt := range []string{
		"DROP TRIGGER convenios_busqueda_au", "DROP TRIGGER convenio_etiquetas_busqueda_ai",
		"DROP TRIGGER convenio_etiquetas_busqueda_ad", "DROP TRIGGER etiquetas_busqueda_au",
		"DROP INDEX ix_convenios_reemplazado",
		"ALTER TABLE convenios DROP COLUMN dependencia_corta",
		"ALTER TABLE convenios DROP COLUMN tipo_documento",
		"ALTER TABLE convenios DROP COLUMN huella",
		// Disparador del índice de la versión 5 que quedó en la base.
		`CREATE TRIGGER convenios_ai AFTER INSERT ON convenios BEGIN
			INSERT INTO convenios_fts(rowid, dependencia, archivo_nombre_original, notas, texto)
			VALUES (new.id, new.dependencia, new.archivo_nombre_original, new.notas, new.texto);
		END`,
	} {
		if _, err := st.DB().Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	st.Close()
	st2, err := Open(paths)
	if err != nil {
		t.Fatalf("no abre la base incompleta: %v", err)
	}
	defer st2.Close()
	// Importar debe funcionar otra vez.
	insertSample(t, st2, "otro convenio")
	if err := st2.Update(ConventionInput{ID: id, Dependencia: "Ayuntamiento", DependenciaCorta: "AYTO"}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"AYTO", "prueba"} {
		if res, err := st2.List(ListFilter{Query: q}); err != nil || res.Total != 1 {
			t.Fatalf("búsqueda %q tras reparar: %+v %v", q, res.Total, err)
		}
	}
}
