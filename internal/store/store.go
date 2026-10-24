// Package store contiene la base SQLite local y las consultas de la aplicación.
package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql search.sql
var schemaFS embed.FS

const schemaVersion = 6

// busquedaExpr calcula la columna busqueda de una fila de convenios. Es la
// misma expresión que usan los disparadores de search.sql.
const busquedaExpr = `trim(
	dependencia_corta || ' ' || tipo_documento || ' ' ||
	firmante_dependencia || ' ' || cargo_dependencia || ' ' || firmante_uagro || ' ' ||
	cargo_uagro || ' ' || facultad || ' ' || facultad_ciudad || ' ' || vigencia_texto || ' ' ||
	coalesce((SELECT group_concat(e.nombre, ' ') FROM convenio_etiquetas ce
	          JOIN etiquetas e ON e.id = ce.etiqueta_id WHERE ce.convenio_id = convenios.id), ''))`

// Paths agrupa las rutas de datos de la aplicación.
type Paths struct {
	Base    string
	DB      string
	PDFs    string
	Backups string
	Config  string
}

// DefaultPaths usa %APPDATA%\ConveniosUAGro en Windows. La variable
// CONVENIOS_DATA_DIR permite usar una carpeta temporal en pruebas.
func DefaultPaths() (Paths, error) {
	base := strings.TrimSpace(os.Getenv("CONVENIOS_DATA_DIR"))
	if base == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return Paths{}, fmt.Errorf("no se pudo obtener la carpeta de configuración: %w", err)
		}
		base = filepath.Join(dir, "ConveniosUAGro")
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return Paths{}, err
	}
	paths := Paths{
		Base:    abs,
		DB:      filepath.Join(abs, "convenios.db"),
		PDFs:    filepath.Join(abs, "pdfs"),
		Backups: filepath.Join(abs, "respaldos"),
		Config:  filepath.Join(abs, "settings.json"),
	}
	for _, dir := range []string{paths.Base, paths.PDFs, paths.Backups} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Paths{}, fmt.Errorf("no se pudo crear %s: %w", dir, err)
		}
	}
	return paths, nil
}

// Store encapsula la conexión SQLite y las rutas de archivos.
type Store struct {
	db    *sql.DB
	paths Paths
	// dupMu evita que dos recálculos de duplicados se mezclen.
	dupMu sync.Mutex
}

// Open abre (o crea) la base de datos y aplica el esquema.
func Open(paths Paths) (*Store, error) {
	db, err := sql.Open("sqlite", paths.DB)
	if err != nil {
		return nil, err
	}
	// Una sola conexión evita bloqueos de escritura en una app de escritorio.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
	}

	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(string(schema)); err != nil {
		db.Close()
		return nil, fmt.Errorf("no se pudo crear el esquema: %w", err)
	}

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return nil, err
	}
	if version > schemaVersion {
		db.Close()
		return nil, fmt.Errorf("la base de datos es más reciente que esta aplicación (versión %d)", version)
	}
	switch {
	case version == 0:
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version=%d", schemaVersion)); err != nil {
			db.Close()
			return nil, err
		}
	case version == 1:
		if err := migrateV2(db); err != nil {
			db.Close()
			return nil, err
		}
		version = 2
	}
	if version == 2 {
		if err := migrateV3(db); err != nil {
			db.Close()
			return nil, err
		}
		version = 3
	}
	if version == 3 {
		if err := migrateV4(db); err != nil {
			db.Close()
			return nil, err
		}
		version = 4
	}
	if version == 4 {
		if err := migrateV5(db); err != nil {
			db.Close()
			return nil, err
		}
		version = 5
	}
	if version == 5 {
		if err := migrateV6(db); err != nil {
			db.Close()
			return nil, err
		}
	} else if version == 6 {
		if err := repairV6(db); err != nil {
			db.Close()
			return nil, err
		}
	}
	search, err := schemaFS.ReadFile("search.sql")
	if err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(string(search)); err != nil {
		db.Close()
		return nil, fmt.Errorf("no se pudo crear el índice de búsqueda: %w", err)
	}
	if err := ensureSearchIndex(db); err != nil {
		db.Close()
		return nil, err
	}

	st := &Store{db: db, paths: paths}
	if err := st.SeedTags(); err != nil {
		db.Close()
		return nil, err
	}
	// Una transcripción solo vive mientras la interfaz está enviando páginas.
	// Si la app se cerró, se deja lista para reintentar al abrir la ficha.
	if _, err := db.Exec("UPDATE convenios SET transcripcion_estado='pendiente' WHERE transcripcion_estado='en_proceso'"); err != nil {
		db.Close()
		return nil, err
	}
	return st, nil
}

// migrateV2 agrega el texto original y el estado de transcripción a bases creadas
// por la primera versión de la aplicación de escritorio.
func migrateV2(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		"ALTER TABLE convenios ADD COLUMN texto_pdf TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE convenios ADD COLUMN transcripcion_estado TEXT NOT NULL DEFAULT 'no_requerida'",
		"ALTER TABLE convenios ADD COLUMN transcripcion_error TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE convenios ADD COLUMN paginas_por_transcribir INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE convenios ADD COLUMN paginas_transcritas INTEGER NOT NULL DEFAULT 0",
		`CREATE TABLE IF NOT EXISTS transcripcion_paginas (
			convenio_id INTEGER NOT NULL REFERENCES convenios(id) ON DELETE CASCADE,
			pagina INTEGER NOT NULL,
			texto TEXT NOT NULL,
			PRIMARY KEY (convenio_id, pagina)
		) WITHOUT ROWID`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`
		UPDATE convenios SET
			texto_pdf = texto,
			paginas_por_transcribir = CASE WHEN es_escaneado = 1 THEN paginas ELSE 0 END,
			transcripcion_estado = CASE WHEN es_escaneado = 1 THEN 'pendiente' ELSE 'no_requerida' END
	`); err != nil {
		return err
	}
	if _, err := tx.Exec("PRAGMA user_version=2"); err != nil {
		return err
	}
	return tx.Commit()
}

// migrateV3 agrega firmantes, facultad y datos de calidad del expediente.
func migrateV3(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		"ALTER TABLE convenios ADD COLUMN firmante_dependencia TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE convenios ADD COLUMN cargo_dependencia TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE convenios ADD COLUMN firmante_uagro TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE convenios ADD COLUMN cargo_uagro TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE convenios ADD COLUMN facultad TEXT NOT NULL DEFAULT ''",
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	if _, err := tx.Exec("PRAGMA user_version=3"); err != nil {
		return err
	}
	return tx.Commit()
}

// migrateV4 agrega el tipo de etiqueta "facultad" reconstruyendo el catálogo.
func migrateV4(db *sql.DB) error {
	if _, err := db.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		db.Exec("PRAGMA foreign_keys=ON")
		return err
	}
	defer func() {
		tx.Rollback()
		db.Exec("PRAGMA foreign_keys=ON")
	}()
	for _, stmt := range []string{
		`CREATE TABLE etiquetas_new (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			nombre    TEXT NOT NULL COLLATE NOCASE,
			tipo      TEXT NOT NULL CHECK (tipo IN ('ambito','estado','ciudad','tema','facultad')),
			color     TEXT NOT NULL DEFAULT '#3A5A9C',
			protegida INTEGER NOT NULL DEFAULT 0,
			creada_en TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`INSERT INTO etiquetas_new (id, nombre, tipo, color, protegida, creada_en)
		 SELECT id, nombre, tipo, color, protegida, creada_en FROM etiquetas`,
		"DROP TABLE etiquetas",
		"ALTER TABLE etiquetas_new RENAME TO etiquetas",
		"CREATE UNIQUE INDEX IF NOT EXISTS ux_etiquetas_tipo_nombre ON etiquetas(tipo, nombre)",
		`DELETE FROM convenio_etiquetas
		 WHERE etiqueta_id IN (SELECT id FROM etiquetas WHERE tipo='ciudad')
		   AND etiqueta_id NOT IN (
		     SELECT MIN(ce.etiqueta_id) FROM convenio_etiquetas ce
		     JOIN etiquetas e ON e.id=ce.etiqueta_id
		     WHERE e.tipo='ciudad' GROUP BY ce.convenio_id
		   )`,
		`DELETE FROM convenio_etiquetas
		 WHERE etiqueta_id IN (SELECT id FROM etiquetas WHERE tipo='facultad')
		   AND etiqueta_id NOT IN (
		     SELECT MIN(ce.etiqueta_id) FROM convenio_etiquetas ce
		     JOIN etiquetas e ON e.id=ce.etiqueta_id
		     WHERE e.tipo='facultad' GROUP BY ce.convenio_id
		   )`,
		"PRAGMA user_version=4",
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return err
	}
	return nil
}

// migrateV5 separa la ciudad de la facultad de la ciudad de la dependencia.
func migrateV5(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("ALTER TABLE convenios ADD COLUMN facultad_ciudad TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if _, err := tx.Exec("PRAGMA user_version=5"); err != nil {
		return err
	}
	return tx.Commit()
}

// migrateV6 agrega las siglas y el tipo de documento (para el nombre del
// archivo), la huella del texto y el historial de versiones, y reemplaza el índice FTS por uno que también busca en firmantes,
// facultad y etiquetas. search.sql lo vuelve a crear y ensureSearchIndex lo llena.
func migrateV6(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := addV6Columns(tx); err != nil {
		return err
	}
	for _, stmt := range []string{
		"DROP TRIGGER IF EXISTS convenios_ai",
		"DROP TRIGGER IF EXISTS convenios_ad",
		"DROP TRIGGER IF EXISTS convenios_au",
		"DROP TABLE IF EXISTS convenios_fts",
		"PRAGMA user_version=6",
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// v6Columns son las columnas que la versión 6 agregó a tablas existentes.
var v6Columns = []struct{ table, name, def string }{
	{"convenios", "busqueda", "TEXT NOT NULL DEFAULT ''"},
	{"convenios", "dependencia_corta", "TEXT NOT NULL DEFAULT ''"},
	{"convenios", "tipo_documento", "TEXT NOT NULL DEFAULT ''"},
	{"convenios", "huella", "INTEGER"},
	{"convenios", "reemplazado_por", "INTEGER REFERENCES convenios(id) ON DELETE SET NULL"},
	{"convenios", "no_es_convenio", "INTEGER NOT NULL DEFAULT 0"},
	{"etiquetas", "estado", "TEXT NOT NULL DEFAULT ''"},
}

// addV6Columns agrega las columnas de la versión 6 que falten y devuelve
// cuáles agregó, como "tabla.columna".
func addV6Columns(tx *sql.Tx) (map[string]bool, error) {
	added := map[string]bool{}
	existing := map[string]map[string]bool{}
	for _, col := range v6Columns {
		if existing[col.table] == nil {
			cols, err := tableColumns(tx, col.table)
			if err != nil {
				return nil, err
			}
			existing[col.table] = cols
		}
		if existing[col.table][col.name] {
			continue
		}
		if _, err := tx.Exec("ALTER TABLE " + col.table + " ADD COLUMN " + col.name + " " + col.def); err != nil {
			return nil, err
		}
		added[col.table+"."+col.name] = true
	}
	return added, nil
}

// repairV6 completa una base marcada como versión 6 por una compilación de
// prueba: quita los disparadores del índice antiguo y agrega las columnas que
// falten. Si faltaba alguna, también rehace el índice de búsqueda.
func repairV6(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Disparadores del índice de la versión 5: con el índice actual cada
	// alta fallaría (inserta dos veces la misma fila en convenios_fts).
	for _, stmt := range []string{
		"DROP TRIGGER IF EXISTS convenios_ai",
		"DROP TRIGGER IF EXISTS convenios_ad",
		"DROP TRIGGER IF EXISTS convenios_au",
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	added, err := addV6Columns(tx)
	if err != nil {
		return err
	}
	if !added["convenios.busqueda"] && !added["convenios.dependencia_corta"] && !added["convenios.tipo_documento"] {
		// El índice de búsqueda no depende de las demás columnas.
		return tx.Commit()
	}
	for _, stmt := range []string{
		"DROP TRIGGER IF EXISTS convenios_busqueda_au",
		"DROP TRIGGER IF EXISTS convenio_etiquetas_busqueda_ai",
		"DROP TRIGGER IF EXISTS convenio_etiquetas_busqueda_ad",
		"DROP TRIGGER IF EXISTS etiquetas_busqueda_au",
		"DROP TRIGGER IF EXISTS convenios_fts_ai",
		"DROP TRIGGER IF EXISTS convenios_fts_ad",
		"DROP TRIGGER IF EXISTS convenios_fts_au",
		"DROP TABLE IF EXISTS convenios_fts",
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func tableColumns(tx *sql.Tx, table string) (map[string]bool, error) {
	rows, err := tx.Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		cols[name] = true
	}
	return cols, rows.Err()
}

// ensureSearchIndex rellena el índice si no coincide con la tabla (después de
// migrar o si se interrumpió una escritura).
func ensureSearchIndex(db *sql.DB) error {
	var convenios, indexados int
	if err := db.QueryRow("SELECT count(*) FROM convenios").Scan(&convenios); err != nil {
		return err
	}
	if err := db.QueryRow("SELECT count(*) FROM convenios_fts").Scan(&indexados); err != nil {
		return err
	}
	if convenios == indexados {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM convenios_fts"); err != nil {
		return err
	}
	// Actualizar busqueda dispara convenios_fts_au, que vuelve a indexar la fila.
	if _, err := tx.Exec("UPDATE convenios SET busqueda = " + busquedaExpr); err != nil {
		return fmt.Errorf("no se pudo reconstruir el índice de búsqueda: %w", err)
	}
	return tx.Commit()
}

// Close cierra la base.
func (s *Store) Close() error { return s.db.Close() }

// Paths devuelve una copia de las rutas.
func (s *Store) Paths() Paths { return s.paths }

// DB expone la conexión para pruebas y respaldos internos.
func (s *Store) DB() *sql.DB { return s.db }

var (
	reColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	reName  = regexp.MustCompile(`^[^\x00-\x1f\x7f]{1,80}$`)
)

const (
	ColorAmbito   = "#193060"
	ColorEstado   = "#6D6E71"
	ColorCiudad   = "#2E7D32"
	ColorFacultad = "#6A1B9A"
	ColorTema     = "#3A5A9C"
	ColorAbierto  = "#00796B"
)

// TagConvenioAbierto es la etiqueta de facultad para los convenios que no
// nombran una facultad o unidad académica de la UAGro.
const TagConvenioAbierto = "Convenio Abierto"

// TagNacional es el ámbito de toda contraparte de México.
const TagNacional = "Nacional"

var estadosMx = []string{
	"Aguascalientes", "Baja California", "Baja California Sur", "Campeche", "Chiapas",
	"Chihuahua", "Ciudad de México", "Coahuila", "Colima", "Durango", "Estado de México",
	"Guanajuato", "Guerrero", "Hidalgo", "Jalisco", "Michoacán", "Morelos", "Nayarit",
	"Nuevo León", "Oaxaca", "Puebla", "Querétaro", "Quintana Roo", "San Luis Potosí",
	"Sinaloa", "Sonora", "Tabasco", "Tamaulipas", "Tlaxcala", "Veracruz", "Yucatán",
	"Zacatecas",
}

// SeedTags garantiza las etiquetas base. Se ejecuta en cada arranque y es idempotente.
func (s *Store) SeedTags() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range []struct{ nombre, color string }{
		{"Nacional", ColorAmbito}, {"Internacional", ColorAmbito},
	} {
		if _, err := tx.Exec(
			"INSERT OR IGNORE INTO etiquetas (nombre, tipo, color, protegida) VALUES (?, 'ambito', ?, 1)",
			item.nombre, item.color,
		); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(
		"INSERT OR IGNORE INTO etiquetas (nombre, tipo, color, protegida) VALUES (?, 'facultad', ?, 1)",
		TagConvenioAbierto, ColorAbierto,
	); err != nil {
		return err
	}
	for _, estado := range estadosMx {
		if _, err := tx.Exec(
			"INSERT OR IGNORE INTO etiquetas (nombre, tipo, color, protegida) VALUES (?, 'estado', ?, 1)",
			estado, ColorEstado,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func validateTagName(nombre string) (string, error) {
	nombre = strings.TrimSpace(nombre)
	if !reName.MatchString(nombre) {
		return "", errors.New("el nombre debe tener entre 1 y 80 caracteres")
	}
	return nombre, nil
}

func validateColor(color string) (string, error) {
	color = strings.ToUpper(strings.TrimSpace(color))
	if !reColor.MatchString(color) {
		return "", errors.New("el color debe tener el formato #RRGGBB")
	}
	return color, nil
}

func colorForType(tipo string) string {
	switch tipo {
	case "ciudad":
		return ColorCiudad
	case "facultad":
		return ColorFacultad
	default:
		return ColorTema
	}
}

// Acronym returns the initials used by the UI when a PDF has no dependency.
func Acronym(text string) string {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "PDF"
	}
	var b strings.Builder
	for _, p := range parts {
		if len([]rune(p)) > 0 {
			b.WriteRune([]rune(p)[0])
		}
		if b.Len() >= 3 {
			break
		}
	}
	return strings.ToUpper(b.String())
}
