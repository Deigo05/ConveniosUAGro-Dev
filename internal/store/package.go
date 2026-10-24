package store

import (
	"archive/zip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"convenios-uagro-desktop/internal/pdfutil"
)

// Formato del paquete de exportación (.zip):
//
//	manifest.json   formato, versión y fecha
//	convenios.json  datos, etiquetas y transcripción de cada convenio
//	pdfs/<32 hex>.pdf
//
// Importar un paquete agrega los convenios a la base actual sin reemplazarla;
// los que ya existen (mismo SHA-256) se omiten.
const (
	PackageFormat  = "convenios-uagro"
	packageVersion = 1
	maxImportErrs  = 50
)

var rePackagePDF = regexp.MustCompile(`^pdfs/[0-9a-f]{32}\.pdf$`)

// PackageTag es una etiqueta por nombre, para recrearla en otra base.
type PackageTag struct {
	Tipo   string `json:"tipo"`
	Nombre string `json:"nombre"`
	Color  string `json:"color"`
	// Estado de una ciudad, para no mezclarla con un estado que no le toca.
	Estado string `json:"estado,omitempty"`
}

// PackagePage es una página transcrita.
type PackagePage struct {
	Pagina int    `json:"pagina"`
	Texto  string `json:"texto"`
}

// PackageRecord es un convenio dentro del paquete.
type PackageRecord struct {
	Dependencia         string `json:"dependencia"`
	FirmanteDependencia string `json:"firmante_dependencia"`
	CargoDependencia    string `json:"cargo_dependencia"`
	DependenciaCorta    string `json:"dependencia_corta,omitempty"`
	TipoDocumento       string `json:"tipo_documento,omitempty"`
	// ReemplazadoPorSHA identifica la versión que reemplaza a esta; se usa el
	// SHA-256 porque los id cambian al importar en otra instalación.
	ReemplazadoPorSHA     string        `json:"reemplazado_por_sha256,omitempty"`
	FirmanteUAGro         string        `json:"firmante_uagro"`
	CargoUAGro            string        `json:"cargo_uagro"`
	Facultad              string        `json:"facultad"`
	FacultadCiudad        string        `json:"facultad_ciudad"`
	FechaFirma            string        `json:"fecha_firma"`
	FechaVigencia         string        `json:"fecha_vigencia"`
	VigenciaIndefinida    bool          `json:"vigencia_indefinida"`
	VigenciaCalculada     bool          `json:"vigencia_calculada"`
	VigenciaTexto         string        `json:"vigencia_texto"`
	Notas                 string        `json:"notas"`
	ArchivoNombreOriginal string        `json:"archivo_nombre_original"`
	Archivo               string        `json:"archivo"`
	ArchivoBytes          int64         `json:"archivo_bytes"`
	ArchivoSHA256         string        `json:"archivo_sha256"`
	Paginas               int           `json:"paginas"`
	EsEscaneado           bool          `json:"es_escaneado"`
	TextoPDF              string        `json:"texto_pdf"`
	IAEstado              string        `json:"ia_estado"`
	EtiquetasRevisadas    bool          `json:"etiquetas_revisadas"`
	TranscripcionEstado   string        `json:"transcripcion_estado"`
	PaginasPorTranscribir int           `json:"paginas_por_transcribir"`
	PaginasTranscritas    int           `json:"paginas_transcritas"`
	Transcripcion         []PackagePage `json:"transcripcion"`
	Etiquetas             []PackageTag  `json:"etiquetas"`
	CreadoEn              string        `json:"creado_en"`
	// NoEsConvenio: 1 si la IA cree que no es un convenio, 2 si el usuario
	// confirmó que sí lo es.
	NoEsConvenio int `json:"no_es_convenio,omitempty"`
}

type packageManifest struct {
	Formato     string `json:"formato"`
	Version     int    `json:"version"`
	AppVersion  string `json:"app_version"`
	ExportadoEn string `json:"exportado_en"`
	Total       int    `json:"total"`
}

// ImportSummary resume una importación.
type ImportSummary struct {
	Importados int      `json:"importados"`
	Duplicados int      `json:"duplicados"`
	Errores    []string `json:"errores"`
	IDs        []int64  `json:"ids"`
}

// packageRecords lee los convenios indicados (todos si ids está vacío).
func (s *Store) packageRecords(ids []int64) ([]PackageRecord, map[string]string, error) {
	where, args := "", []interface{}{}
	if len(ids) > 0 {
		where = " WHERE id IN (" + strings.TrimRight(strings.Repeat("?,", len(ids)), ",") + ")"
		for _, id := range ids {
			args = append(args, id)
		}
	}
	rows, err := s.db.Query(`SELECT id, dependencia, firmante_dependencia, cargo_dependencia, firmante_uagro,
		cargo_uagro, facultad, facultad_ciudad, coalesce(fecha_firma,''), coalesce(fecha_vigencia,''),
		vigencia_indefinida, vigencia_calculada, vigencia_texto, notas, archivo_nombre_original,
		archivo_guardado, archivo_bytes, archivo_sha256, paginas, es_escaneado, texto_pdf, ia_estado,
		etiquetas_revisadas, transcripcion_estado, paginas_por_transcribir, paginas_transcritas, creado_en,
		dependencia_corta, tipo_documento,
		coalesce((SELECT n.archivo_sha256 FROM convenios n WHERE n.id = convenios.reemplazado_por), ''),
		no_es_convenio
		FROM convenios`+where+" ORDER BY id", args...)
	if err != nil {
		return nil, nil, err
	}
	var recs []PackageRecord
	var recIDs []int64
	files := map[string]string{} // ruta en el paquete → nombre guardado
	for rows.Next() {
		var r PackageRecord
		var id int64
		var saved string
		if err := rows.Scan(&id, &r.Dependencia, &r.FirmanteDependencia, &r.CargoDependencia, &r.FirmanteUAGro,
			&r.CargoUAGro, &r.Facultad, &r.FacultadCiudad, &r.FechaFirma, &r.FechaVigencia,
			&r.VigenciaIndefinida, &r.VigenciaCalculada, &r.VigenciaTexto, &r.Notas, &r.ArchivoNombreOriginal,
			&saved, &r.ArchivoBytes, &r.ArchivoSHA256, &r.Paginas, &r.EsEscaneado, &r.TextoPDF, &r.IAEstado,
			&r.EtiquetasRevisadas, &r.TranscripcionEstado, &r.PaginasPorTranscribir, &r.PaginasTranscritas, &r.CreadoEn,
			&r.DependenciaCorta, &r.TipoDocumento, &r.ReemplazadoPorSHA, &r.NoEsConvenio); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if reSavedName.MatchString(saved) {
			r.Archivo = "pdfs/" + saved
			files[r.Archivo] = saved
		}
		recs = append(recs, r)
		recIDs = append(recIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	tagMap, err := s.TagsForIDs(recIDs)
	if err != nil {
		return nil, nil, err
	}
	for i, id := range recIDs {
		for _, t := range tagMap[id] {
			recs[i].Etiquetas = append(recs[i].Etiquetas, PackageTag{Tipo: t.Tipo, Nombre: t.Nombre, Color: t.Color, Estado: t.Estado})
		}
		pages, err := s.db.Query("SELECT pagina, texto FROM transcripcion_paginas WHERE convenio_id=? ORDER BY pagina", id)
		if err != nil {
			return nil, nil, err
		}
		for pages.Next() {
			var p PackagePage
			if err := pages.Scan(&p.Pagina, &p.Texto); err != nil {
				pages.Close()
				return nil, nil, err
			}
			recs[i].Transcripcion = append(recs[i].Transcripcion, p)
		}
		pages.Close()
	}
	return recs, files, nil
}

// ExportPackage guarda en dst un paquete con los convenios indicados (todos si
// ids está vacío). Devuelve cuántos se exportaron.
func (s *Store) ExportPackage(dst string, ids []int64, appVersion string) (int, error) {
	recs, files, err := s.packageRecords(ids)
	if err != nil {
		return 0, err
	}
	if len(recs) == 0 {
		return 0, errors.New("no hay convenios para exportar")
	}
	tmp := dst + ".tmp"
	_ = os.Remove(tmp)
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	ok := false
	zw := zip.NewWriter(f)
	defer func() {
		zw.Close()
		f.Close()
		if !ok {
			os.Remove(tmp)
		}
	}()
	writeJSON := func(name string, v interface{}) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	if err := writeJSON("manifest.json", packageManifest{
		Formato: PackageFormat, Version: packageVersion, AppVersion: appVersion,
		ExportadoEn: time.Now().Format(time.RFC3339), Total: len(recs),
	}); err != nil {
		return 0, err
	}
	if err := writeJSON("convenios.json", recs); err != nil {
		return 0, err
	}
	for zipPath, saved := range files {
		src := filepath.Join(s.paths.PDFs, saved)
		if _, err := os.Stat(src); err != nil {
			continue // el registro se exporta aunque falte su PDF; al importar se avisará
		}
		if err := addFileToZip(zw, src, zipPath); err != nil {
			return 0, err
		}
	}
	if err := zw.Close(); err != nil {
		return 0, err
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return 0, err
	}
	ok = true
	return len(recs), nil
}

// ImportPackage agrega a la base los convenios de un paquete exportado, de un
// respaldo ZIP de la aplicación o de un archivo convenios.db de otra instalación
// (en ese caso los PDF se buscan en la carpeta pdfs\ junto a él).
func (s *Store) ImportPackage(src string) (ImportSummary, error) {
	switch strings.ToLower(filepath.Ext(src)) {
	case ".db":
		return s.importFromDB(src, func(name string) (io.ReadCloser, error) {
			return os.Open(filepath.Join(filepath.Dir(src), "pdfs", strings.TrimPrefix(name, "pdfs/")))
		})
	case ".zip":
	default:
		return ImportSummary{}, errors.New("seleccione un paquete .zip, un respaldo .zip o un archivo convenios.db")
	}
	zr, err := zip.OpenReader(src)
	if err != nil {
		return ImportSummary{}, errors.New("el archivo no es un ZIP válido")
	}
	defer zr.Close()
	entries := map[string]*zip.File{}
	for _, f := range zr.File {
		entries[f.Name] = f
	}
	open := func(name string) (io.ReadCloser, error) {
		f, ok := entries[name]
		if !ok {
			return nil, errors.New("el PDF no está en el paquete")
		}
		return f.Open()
	}
	if f, ok := entries["convenios.json"]; ok {
		rc, err := f.Open()
		if err != nil {
			return ImportSummary{}, err
		}
		var recs []PackageRecord
		err = json.NewDecoder(io.LimitReader(rc, 512<<20)).Decode(&recs)
		rc.Close()
		if err != nil {
			return ImportSummary{}, errors.New("convenios.json no tiene un formato válido")
		}
		return s.importRecords(recs, open), nil
	}
	if f, ok := entries["convenios.db"]; ok {
		// Respaldo ZIP: se extrae solo la base; los PDF se leen del ZIP.
		dir, err := os.MkdirTemp("", "convenios-import-*")
		if err != nil {
			return ImportSummary{}, err
		}
		defer os.RemoveAll(dir)
		dbPath := filepath.Join(dir, "convenios.db")
		if err := extractZipFile(f, dbPath); err != nil {
			return ImportSummary{}, err
		}
		return s.importFromDB(dbPath, open)
	}
	return ImportSummary{}, errors.New("el ZIP no es un paquete ni un respaldo de Convenios UAGro")
}

// importFromDB abre una copia temporal de otra base (se migra al esquema
// actual sin tocar el original) y agrega sus convenios.
func (s *Store) importFromDB(dbPath string, open func(string) (io.ReadCloser, error)) (ImportSummary, error) {
	if same, _ := sameFile(dbPath, s.paths.DB); same {
		return ImportSummary{}, errors.New("ese archivo es la base que la aplicación ya está usando")
	}
	dir, err := os.MkdirTemp("", "convenios-db-*")
	if err != nil {
		return ImportSummary{}, err
	}
	defer os.RemoveAll(dir)
	copyDB := filepath.Join(dir, "convenios.db")
	if err := copyFile(dbPath, copyDB); err != nil {
		return ImportSummary{}, fmt.Errorf("no se pudo leer la base: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(dbPath + suffix); err == nil {
			_ = copyFile(dbPath+suffix, copyDB+suffix)
		}
	}
	other, err := Open(Paths{Base: dir, DB: copyDB, PDFs: filepath.Join(dir, "pdfs"), Backups: dir, Config: filepath.Join(dir, "settings.json")})
	if err != nil {
		return ImportSummary{}, fmt.Errorf("no se pudo abrir la base a importar: %w", err)
	}
	recs, _, err := other.packageRecords(nil)
	other.Close()
	if err != nil {
		return ImportSummary{}, err
	}
	return s.importRecords(recs, open), nil
}

func (s *Store) importRecords(recs []PackageRecord, open func(string) (io.ReadCloser, error)) ImportSummary {
	sum := ImportSummary{Errores: []string{}, IDs: []int64{}}
	fail := func(r PackageRecord, msg string) {
		if len(sum.Errores) < maxImportErrs {
			name := r.ArchivoNombreOriginal
			if name == "" {
				name = r.Dependencia
			}
			sum.Errores = append(sum.Errores, name+": "+msg)
		}
	}
	// idBySHA relaciona el SHA-256 declarado en el paquete con el convenio
	// que le corresponde aquí, para rehacer el historial de versiones.
	idBySHA := map[string]int64{}
	for _, r := range recs {
		declared := r.ArchivoSHA256
		if declared != "" {
			if id, dup, _ := s.DuplicateID(declared, 0); dup {
				idBySHA[declared] = id
				sum.Duplicados++
				continue
			}
		}
		if !rePackagePDF.MatchString(r.Archivo) {
			fail(r, "falta el PDF en el paquete")
			continue
		}
		saved, size, sha, err := s.copyImportedPDF(r.Archivo, open)
		if err != nil {
			fail(r, err.Error())
			continue
		}
		if id, dup, _ := s.DuplicateID(sha, 0); dup {
			idBySHA[declared] = id
			os.Remove(filepath.Join(s.paths.PDFs, saved))
			sum.Duplicados++
			continue
		}
		r.ArchivoBytes, r.ArchivoSHA256 = size, sha
		id, err := s.insertRecord(r, saved)
		if err != nil {
			os.Remove(filepath.Join(s.paths.PDFs, saved))
			fail(r, err.Error())
			continue
		}
		if declared != "" {
			idBySHA[declared] = id
		}
		sum.Importados++
		sum.IDs = append(sum.IDs, id)
	}
	s.importHistory(recs, idBySHA)
	return sum
}

// importHistory vuelve a unir las versiones del paquete. Solo une versiones
// que aún no tienen otra más reciente en esta base.
func (s *Store) importHistory(recs []PackageRecord, idBySHA map[string]int64) {
	for _, r := range recs {
		older, newer := idBySHA[r.ArchivoSHA256], idBySHA[r.ReemplazadoPorSHA]
		if r.ReemplazadoPorSHA == "" || older == 0 || newer == 0 {
			continue
		}
		if next, err := s.replacedBy(older); err == nil && next == 0 {
			_ = s.LinkVersion(older, newer)
		}
	}
}

// copyImportedPDF copia el PDF con un nombre nuevo y valida que lo sea.
func (s *Store) copyImportedPDF(name string, open func(string) (io.ReadCloser, error)) (saved string, size int64, sha string, err error) {
	rc, err := open(name)
	if err != nil {
		return "", 0, "", errors.New("no se encontró el PDF")
	}
	defer rc.Close()
	tmp, err := os.CreateTemp(s.paths.PDFs, ".import-*.pdf")
	if err != nil {
		return "", 0, "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(rc, pdfutil.MaxBytes+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", 0, "", err
	}
	if n == 0 || n > pdfutil.MaxBytes {
		return "", 0, "", errors.New("el PDF está vacío o supera el tamaño permitido")
	}
	head := make([]byte, 5)
	if f, err := os.Open(tmpName); err == nil {
		_, _ = io.ReadFull(f, head)
		f.Close()
	}
	if string(head) != "%PDF-" {
		return "", 0, "", errors.New("el archivo del paquete no es un PDF válido")
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", 0, "", err
	}
	saved = hex.EncodeToString(b) + ".pdf"
	if err := os.Rename(tmpName, filepath.Join(s.paths.PDFs, saved)); err != nil {
		return "", 0, "", err
	}
	return saved, n, hex.EncodeToString(hash.Sum(nil)), nil
}

var (
	validIA    = map[string]bool{"omitido": true, "pendiente": true, "ok": true, "parcial": true, "error": true}
	validTrans = map[string]bool{"no_requerida": true, "pendiente": true, "completa": true, "error": true}
	validTipo  = map[string]bool{"ambito": true, "estado": true, "ciudad": true, "tema": true, "facultad": true}
)

func (s *Store) insertRecord(r PackageRecord, saved string) (int64, error) {
	if !IsDate(r.FechaFirma) {
		r.FechaFirma = ""
	}
	if !IsDate(r.FechaVigencia) {
		r.FechaVigencia = ""
	}
	if !validIA[r.IAEstado] || r.IAEstado == "pendiente" {
		r.IAEstado = "omitido"
	}
	if r.TranscripcionEstado == "en_proceso" {
		r.TranscripcionEstado = "pendiente"
	}
	if !validTrans[r.TranscripcionEstado] {
		r.TranscripcionEstado = "no_requerida"
	}
	if r.CreadoEn == "" {
		r.CreadoEn = time.Now().UTC().Format("2006-01-02 15:04:05")
	}
	// Las etiquetas se resuelven antes de abrir la transacción: la base usa una
	// sola conexión y una consulta fuera de la transacción la bloquearía.
	var tagIDs []int64
	seenType := map[string]bool{}
	for _, t := range r.Etiquetas {
		if !validTipo[t.Tipo] {
			continue
		}
		if t.Tipo != "tema" && seenType[t.Tipo] {
			continue
		}
		tag, ok, err := s.findTag(t.Tipo, t.Nombre)
		if err != nil {
			return 0, err
		}
		if !ok {
			if t.Tipo == "ambito" || t.Tipo == "estado" {
				continue // las etiquetas base no se inventan
			}
			color := t.Color
			if _, err := validateColor(color); err != nil {
				color = colorForType(t.Tipo)
			}
			tag, err = s.CreateTag(TagInput{Nombre: t.Nombre, Tipo: t.Tipo, Color: color, Estado: t.Estado})
			if err != nil {
				continue
			}
		} else if t.Tipo == "ciudad" && tag.Estado == "" && t.Estado != "" {
			if e, ok := estadoCanonico(t.Estado); ok {
				if _, err := s.db.Exec("UPDATE etiquetas SET estado=? WHERE id=?", e, tag.ID); err != nil {
					return 0, err
				}
			}
		}
		seenType[t.Tipo] = true
		tagIDs = append(tagIDs, tag.ID)
	}

	texto := r.TextoPDF
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO convenios (
			dependencia, firmante_dependencia, cargo_dependencia, firmante_uagro, cargo_uagro,
			facultad, facultad_ciudad, fecha_firma, fecha_vigencia, vigencia_indefinida, vigencia_texto,
			vigencia_calculada, notas, archivo_nombre_original, archivo_guardado, archivo_bytes,
			archivo_sha256, paginas, es_escaneado, texto_pdf, texto, ia_estado, etiquetas_revisadas,
			transcripcion_estado, paginas_por_transcribir, paginas_transcritas, creado_en, actualizado_en,
			dependencia_corta, tipo_documento, no_es_convenio)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,datetime('now'),?,?,?)`,
		r.Dependencia, r.FirmanteDependencia, r.CargoDependencia, r.FirmanteUAGro, r.CargoUAGro,
		r.Facultad, r.FacultadCiudad, nullIfEmpty(r.FechaFirma), nullIfEmpty(r.FechaVigencia),
		boolInt(r.VigenciaIndefinida), r.VigenciaTexto, boolInt(r.VigenciaCalculada), r.Notas,
		pdfutilName(r.ArchivoNombreOriginal), saved, r.ArchivoBytes, r.ArchivoSHA256, r.Paginas,
		boolInt(r.EsEscaneado), r.TextoPDF, texto, r.IAEstado, boolInt(r.EtiquetasRevisadas),
		r.TranscripcionEstado, r.PaginasPorTranscribir, r.PaginasTranscritas, r.CreadoEn,
		cutRunes(cleanFileName(r.DependenciaCorta), 60), NormalizeTipoDocumento(r.TipoDocumento), noConvenioValue(r.NoEsConvenio))
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, p := range r.Transcripcion {
		if p.Pagina < 1 || strings.TrimSpace(p.Texto) == "" {
			continue
		}
		if _, err := tx.Exec("INSERT OR REPLACE INTO transcripcion_paginas (convenio_id, pagina, texto) VALUES (?, ?, ?)", id, p.Pagina, p.Texto); err != nil {
			return 0, err
		}
	}
	if err := rebuildText(tx, id); err != nil {
		return 0, err
	}
	for _, tid := range tagIDs {
		if _, err := tx.Exec("INSERT OR IGNORE INTO convenio_etiquetas (convenio_id, etiqueta_id) VALUES (?, ?)", id, tid); err != nil {
			return 0, err
		}
	}
	if _, err := tx.Exec("UPDATE convenios SET busqueda = "+busquedaExpr+" WHERE id=?", id); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func pdfutilName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "convenio.pdf"
	}
	return name
}

func extractZipFile(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(rc, 4<<30)); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func sameFile(a, b string) (bool, error) {
	sa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(sa, sb), nil
}

func noConvenioValue(v int) int {
	if v == convenioDudoso || v == convenioConfirmado {
		return v
	}
	return convenioNormal
}
