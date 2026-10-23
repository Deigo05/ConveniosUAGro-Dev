package store

import (
	"database/sql"
	"testing"
)

func TestMigrationV1ToV3(t *testing.T) {
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
	if version != 3 {
		t.Fatalf("user_version = %d", version)
	}
	d, err := st.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	if d.Texto != "texto viejo" || d.TranscripcionEstado != "pendiente" || d.PaginasPorTranscribir != 2 {
		t.Fatalf("migración inesperada: %+v", d)
	}
}
