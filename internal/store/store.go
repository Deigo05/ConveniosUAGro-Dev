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

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

const schemaVersion = 3

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
	ColorAmbito = "#193060"
	ColorEstado = "#6D6E71"
	ColorCiudad = "#2E7D32"
	ColorTema   = "#3A5A9C"
)

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
	case "tema":
		return ColorTema
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
