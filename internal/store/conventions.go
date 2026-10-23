package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"convenios-uagro-desktop/internal/pdfutil"
)

const (
	pageSize      = 40
	maxBulkDelete = 200
)

var reSavedName = regexp.MustCompile(`^[0-9a-f]{32}\.pdf$`)

// Insert registra un PDF ya guardado. La clasificación queda pendiente.
func (s *Store) Insert(f pdfutil.SavedFile) (int64, error) {
	estado := "no_requerida"
	if f.PaginasSinTexto > 0 {
		estado = "pendiente"
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`
		INSERT INTO convenios (
			dependencia, responsable, archivo_nombre_original, archivo_guardado, archivo_bytes,
			archivo_sha256, paginas, es_escaneado, texto_pdf, texto, ia_estado,
			transcripcion_estado, paginas_por_transcribir, etiquetas_revisadas,
			creado_en, actualizado_en
		) VALUES ('', '', ?, ?, ?, ?, ?, ?, ?, ?, 'omitido', ?, ?, 0, datetime('now'), datetime('now'))`,
		f.NombreOriginal, f.NombreGuardado, f.Bytes, f.SHA256, f.Paginas, boolInt(f.Escaneado),
		f.Texto, f.Texto, estado, f.PaginasSinTexto)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, texto := range f.PaginasTexto {
		if strings.TrimSpace(texto) == "" {
			continue
		}
		if _, err := tx.Exec("INSERT OR REPLACE INTO pdf_paginas (convenio_id, pagina, texto) VALUES (?, ?, ?)", id, i+1, texto); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// DuplicateID devuelve el id de otro convenio con el mismo SHA-256, si existe.
func (s *Store) DuplicateID(sha string, exclude int64) (int64, bool, error) {
	if sha == "" {
		return 0, false, nil
	}
	var id int64
	err := s.db.QueryRow("SELECT id FROM convenios WHERE archivo_sha256=? AND id<>? ORDER BY id LIMIT 1", sha, exclude).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func ftsQuery(q string) string {
	re := regexp.MustCompile(`[\p{L}\p{N}]+`)
	terms := re.FindAllString(q, 11)
	if len(terms) > 10 {
		terms = terms[:10]
	}
	if len(terms) == 0 {
		return ""
	}
	for i, t := range terms {
		terms[i] = `"` + strings.ReplaceAll(t, `"`, "") + `"*`
	}
	return strings.Join(terms, " AND ")
}

func (s *Store) tagGroups(ids []int64) (map[string][]int64, error) {
	out := map[string][]int64{}
	if len(ids) == 0 {
		return out, nil
	}
	ph := strings.Repeat("?,", len(ids))
	ph = strings.TrimRight(ph, ",")
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.Query("SELECT id, tipo FROM etiquetas WHERE id IN ("+ph+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var tipo string
		if err := rows.Scan(&id, &tipo); err != nil {
			return nil, err
		}
		out[tipo] = append(out[tipo], id)
	}
	return out, rows.Err()
}

// List aplica búsqueda FTS, etiquetas, vigencia, orden y paginación.
func (s *Store) List(f ListFilter) (ListResult, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	conds := []string{"1=1"}
	var args []interface{}

	fts := ftsQuery(f.Query)
	from := "convenios c"
	if fts != "" {
		from = "convenios_fts JOIN convenios c ON c.id = convenios_fts.rowid"
		conds = append(conds, "convenios_fts MATCH ?")
		args = append(args, fts)
	}

	groups, err := s.tagGroups(f.TagIDs)
	if err != nil {
		return ListResult{}, err
	}
	for _, tipo := range []string{"ambito", "estado", "ciudad", "tema"} {
		ids := groups[tipo]
		if len(ids) == 0 {
			continue
		}
		ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
		conds = append(conds, "EXISTS (SELECT 1 FROM convenio_etiquetas ce JOIN etiquetas e ON e.id=ce.etiqueta_id WHERE ce.convenio_id=c.id AND e.tipo=? AND ce.etiqueta_id IN ("+ph+"))")
		args = append(args, tipo)
		for _, id := range ids {
			args = append(args, id)
		}
	}

	today := Today()
	switch f.Vigencia {
	case "vigentes":
		conds = append(conds, "(c.vigencia_indefinida=1 OR (c.fecha_vigencia IS NOT NULL AND c.fecha_vigencia<>'' AND c.fecha_vigencia>=?))")
		args = append(args, today)
	case "no_vigentes":
		conds = append(conds, "(c.vigencia_indefinida=0 AND c.fecha_vigencia IS NOT NULL AND c.fecha_vigencia<>'' AND c.fecha_vigencia<?)")
		args = append(args, today)
	case "sin_fecha":
		conds = append(conds, "(c.vigencia_indefinida=0 AND (c.fecha_vigencia IS NULL OR c.fecha_vigencia=''))")
	}
	if f.SoloPendientes {
		conds = append(conds, "c.etiquetas_revisadas=0")
	}

	where := " WHERE " + strings.Join(conds, " AND ")
	var total int
	if err := s.db.QueryRow("SELECT count(*) FROM "+from+where, args...).Scan(&total); err != nil {
		return ListResult{}, err
	}
	var totalRegistrados int
	if err := s.db.QueryRow("SELECT count(*) FROM convenios").Scan(&totalRegistrados); err != nil {
		return ListResult{}, err
	}
	pages := (total + pageSize - 1) / pageSize
	if pages < 1 {
		pages = 1
	}
	if f.Page > pages {
		f.Page = pages
	}

	order := orderSQL(f.Order, f.Dir)
	if fts != "" && (f.Order == "" || f.Order == "relevancia") {
		order = "bm25(convenios_fts), c.id DESC"
	}
	offset := (f.Page - 1) * pageSize
	query := `SELECT c.id, c.dependencia, coalesce(c.responsable,''), coalesce(c.fecha_firma,''), coalesce(c.fecha_vigencia,''),
		c.vigencia_indefinida, c.vigencia_calculada, c.archivo_nombre_original, c.paginas,
		c.es_escaneado, c.ia_estado, c.ia_error, c.etiquetas_revisadas,
		c.transcripcion_estado, c.transcripcion_error, c.paginas_por_transcribir,
		c.paginas_transcritas, c.creado_en, c.actualizado_en`
	if fts != "" {
		query += `, snippet(convenios_fts, 3, '', '', '…', 12)`
	} else {
		query += `, ''`
	}
	query += " FROM " + from + where + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	argsPage := append(append([]interface{}{}, args...), pageSize, offset)

	rows, err := s.db.Query(query, argsPage...)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()
	var items []ConventionSummary
	var ids []int64
	for rows.Next() {
		var c ConventionSummary
		var indef, calc, scanned, revisadas int
		if err := rows.Scan(&c.ID, &c.Dependencia, &c.Responsable, &c.FechaFirma, &c.FechaVigencia,
			&indef, &calc, &c.ArchivoNombreOriginal, &c.Paginas, &scanned,
			&c.IAEstado, &c.IAError, &revisadas, &c.TranscripcionEstado,
			&c.TranscripcionError, &c.PaginasPorTranscribir, &c.PaginasTranscritas,
			&c.CreadoEn, &c.ActualizadoEn, &c.Fragmento); err != nil {
			return ListResult{}, err
		}
		c.VigenciaIndefinida = indef != 0
		c.VigenciaCalculada = calc != 0
		c.EsEscaneado = scanned != 0
		c.EtiquetasRevisadas = revisadas != 0
		c.Estado = EstadoConvenio(c.FechaVigencia, c.VigenciaIndefinida)
		c.Etiquetas = []Tag{}
		items = append(items, c)
		ids = append(ids, c.ID)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, err
	}
	tagMap, err := s.TagsForIDs(ids)
	if err != nil {
		return ListResult{}, err
	}
	for i := range items {
		if tags, ok := tagMap[items[i].ID]; ok {
			items[i].Etiquetas = tags
		}
		items[i].Riesgo = EvaluateRisk(items[i])
	}
	return ListResult{Items: items, Total: total, TotalRegistrados: totalRegistrados, Page: f.Page, Pages: pages}, nil
}

func orderSQL(order, dir string) string {
	asc := strings.EqualFold(dir, "asc")
	sentido := "DESC"
	if asc {
		sentido = "ASC"
	}
	switch order {
	case "dependencia":
		return "(c.dependencia IS NULL OR c.dependencia='') ASC, c.dependencia COLLATE NOCASE " + sentido + ", c.id DESC"
	case "fecha_firma":
		return "(c.fecha_firma IS NULL OR c.fecha_firma='') ASC, c.fecha_firma " + sentido + ", c.id DESC"
	case "fecha_vigencia":
		return "(c.fecha_vigencia IS NULL OR c.fecha_vigencia='') ASC, c.fecha_vigencia " + sentido + ", c.id DESC"
	case "id":
		return "c.id " + sentido
	default:
		return "c.id DESC"
	}
}

// EstadoConvenio calcula la etiqueta de vigencia que ve el usuario.
func EstadoConvenio(fecha string, indefinida bool) string {
	if indefinida {
		return "indefinida"
	}
	if strings.TrimSpace(fecha) == "" {
		return "sin_fecha"
	}
	d, err := time.Parse("2006-01-02", fecha)
	if err != nil {
		return "sin_fecha"
	}
	hoy := time.Now()
	hoy = time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 0, 0, 0, 0, hoy.Location())
	if d.Before(hoy) {
		return "vencido"
	}
	if d.Sub(hoy).Hours()/24 <= 90 {
		return "por_vencer"
	}
	return "vigente"
}

// Get devuelve la ficha completa.
func (s *Store) Get(id int64) (ConventionDetail, error) {
	var c ConventionDetail
	var indef, calc, scanned, revisadas int
	var fechaFirma, fechaVigencia sql.NullString
	var sugerenciasRaw, textoPDF string
	err := s.db.QueryRow(`
		SELECT id, dependencia, responsable, fecha_firma, fecha_vigencia, vigencia_indefinida,
		       vigencia_texto, vigencia_calculada, notas, archivo_nombre_original,
		       archivo_guardado, archivo_bytes, archivo_sha256, paginas, es_escaneado,
		       texto_pdf, texto, ia_estado, ia_error, etiquetas_sugeridas, etiquetas_revisadas,
		       transcripcion_estado, transcripcion_error, paginas_por_transcribir,
		       paginas_transcritas, creado_en, actualizado_en
		FROM convenios WHERE id=?`, id).Scan(
		&c.ID, &c.Dependencia, &c.Responsable, &fechaFirma, &fechaVigencia, &indef,
		&c.VigenciaTexto, &calc, &c.Notas, &c.ArchivoNombreOriginal,
		&c.ArchivoGuardado, &c.ArchivoBytes, &c.ArchivoSHA256, &c.Paginas, &scanned,
		&textoPDF, &c.Texto, &c.IAEstado, &c.IAError, &sugerenciasRaw, &revisadas,
		&c.TranscripcionEstado, &c.TranscripcionError, &c.PaginasPorTranscribir,
		&c.PaginasTranscritas, &c.CreadoEn, &c.ActualizadoEn)
	if errors.Is(err, sql.ErrNoRows) {
		return c, errors.New("el convenio no existe")
	}
	if err != nil {
		return c, err
	}
	_ = fechaFirma
	_ = fechaVigencia
	_ = textoPDF
	c.FechaFirma = fechaFirma.String
	c.FechaVigencia = fechaVigencia.String
	c.VigenciaIndefinida = indef != 0
	c.VigenciaCalculada = calc != 0
	c.EsEscaneado = scanned != 0
	c.EtiquetasRevisadas = revisadas != 0
	c.EtiquetasSugeridas = ParseSuggestions(sugerenciasRaw)
	c.Estado = EstadoConvenio(c.FechaVigencia, c.VigenciaIndefinida)
	tags, err := s.TagsForConvention(id)
	if err != nil {
		return c, err
	}
	c.Etiquetas = tags
	c.Riesgo = EvaluateRisk(c.ConventionSummary)
	return c, nil
}

// Update guarda la captura manual.
func (s *Store) Update(in ConventionInput) error {
	if !IsDate(in.FechaFirma) || !IsDate(in.FechaVigencia) {
		return errors.New("las fechas deben tener el formato AAAA-MM-DD y un año entre 1950 y 2100")
	}
	_, err := s.db.Exec(`
		UPDATE convenios SET dependencia=?, responsable=?, fecha_firma=?, fecha_vigencia=?,
			vigencia_indefinida=?, vigencia_calculada=0, vigencia_texto=?, notas=?,
			actualizado_en=datetime('now')
		WHERE id=?`,
		strings.TrimSpace(in.Dependencia), strings.TrimSpace(in.Responsable), nullIfEmpty(in.FechaFirma), nullIfEmpty(in.FechaVigencia),
		boolInt(in.VigenciaIndefinida), strings.TrimSpace(in.VigenciaTexto), strings.TrimSpace(in.Notas), in.ID)
	return err
}

func nullIfEmpty(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.TrimSpace(s)
}

// SetIAStatus guarda el resultado o el error de una extracción.
func (s *Store) SetIAStatus(id int64, estado, mensaje string) error {
	_, err := s.db.Exec("UPDATE convenios SET ia_estado=?, ia_error=?, actualizado_en=datetime('now') WHERE id=?", estado, mensaje, id)
	return err
}

// SetAIResult guarda los campos extraídos y las sugerencias de etiquetas.
func (s *Store) SetAIResult(id int64, in ConventionInput, estado string, sugerencias []Suggestion, notas string) error {
	if !IsDate(in.FechaFirma) || !IsDate(in.FechaVigencia) {
		return errors.New("la IA devolvió una fecha inválida")
	}
	data := ""
	if len(sugerencias) > 0 {
		b, err := jsonMarshal(sugerencias)
		if err != nil {
			return err
		}
		data = b
	}
	_, err := s.db.Exec(`
		UPDATE convenios SET dependencia=?, fecha_firma=?, fecha_vigencia=?,
			vigencia_indefinida=?, vigencia_calculada=?, vigencia_texto=?, notas=?,
			ia_estado=?, ia_error='', etiquetas_sugeridas=?, etiquetas_revisadas=0,
			actualizado_en=datetime('now')
		WHERE id=?`,
		strings.TrimSpace(in.Dependencia), nullIfEmpty(in.FechaFirma), nullIfEmpty(in.FechaVigencia),
		boolInt(in.VigenciaIndefinida), boolInt(in.VigenciaCalculada), strings.TrimSpace(in.VigenciaTexto),
		strings.TrimSpace(in.Notas+" "+notas), estado, data, id)
	return err
}

// jsonMarshal is a small indirection to keep the import list tidy.
func jsonMarshal(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// DeleteMany elimina convenios y devuelve los nombres de archivo a borrar.
func (s *Store) DeleteMany(ids []int64) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	uniq := make([]int64, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if !seen[id] && len(uniq) < maxBulkDelete {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(uniq)), ",")
	args := make([]interface{}, len(uniq))
	for i, id := range uniq {
		args[i] = id
	}
	rows, err := s.db.Query("SELECT archivo_guardado FROM convenios WHERE id IN ("+ph+")", args...)
	if err != nil {
		return nil, err
	}
	var files []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		files = append(files, name)
	}
	rows.Close()
	_, err = s.db.Exec("DELETE FROM convenios WHERE id IN ("+ph+")", args...)
	return files, err
}

// PDF devuelve la ruta y el nombre original de un PDF.
func (s *Store) PDF(id int64) (path, original string, err error) {
	var saved, orig string
	if err := s.db.QueryRow("SELECT archivo_guardado, archivo_nombre_original FROM convenios WHERE id=?", id).Scan(&saved, &orig); err != nil {
		return "", "", err
	}
	if !reSavedName.MatchString(saved) {
		return "", "", errors.New("nombre de archivo inválido")
	}
	base, err := filepath.Abs(s.paths.PDFs)
	if err != nil {
		return "", "", err
	}
	path, err = filepath.Abs(filepath.Join(base, saved))
	if err != nil {
		return "", "", err
	}
	if filepath.Dir(path) != base {
		return "", "", errors.New("ruta de archivo inválida")
	}
	return path, orig, nil
}

// Stats calcula los contadores del listado.
func (s *Store) Stats() (Stats, error) {
	var st Stats
	today := Today()
	err := s.db.QueryRow(`
		SELECT count(*),
		       coalesce(sum(CASE WHEN vigencia_indefinida=1 OR (fecha_vigencia IS NOT NULL AND fecha_vigencia<>'' AND fecha_vigencia>=?) THEN 1 ELSE 0 END),0),
		       coalesce(sum(CASE WHEN vigencia_indefinida=0 AND fecha_vigencia>=? AND fecha_vigencia<=date(?, '+90 days') THEN 1 ELSE 0 END),0),
		       coalesce(sum(CASE WHEN vigencia_indefinida=0 AND fecha_vigencia IS NOT NULL AND fecha_vigencia<>'' AND fecha_vigencia<? THEN 1 ELSE 0 END),0),
		       coalesce(sum(CASE WHEN vigencia_indefinida=0 AND (fecha_vigencia IS NULL OR fecha_vigencia='') THEN 1 ELSE 0 END),0),
		       coalesce(sum(CASE WHEN vigencia_indefinida=1 THEN 1 ELSE 0 END),0),
		       coalesce(sum(CASE WHEN etiquetas_revisadas=0 THEN 1 ELSE 0 END),0)
		FROM convenios`, today, today, today, today).Scan(
		&st.Total, &st.Vigentes, &st.PorVencer, &st.Vencidos, &st.SinFecha,
		&st.Indefinidas, &st.EtiquetasPendientes)
	if err != nil {
		return st, err
	}
	if err := s.db.QueryRow("SELECT count(*) FROM etiquetas").Scan(&st.Etiquetas); err != nil {
		return st, err
	}
	return st, nil
}

// SetReviewed marca la clasificación sin cambiar etiquetas.
func (s *Store) SetReviewed(id int64, reviewed bool) error {
	_, err := s.db.Exec("UPDATE convenios SET etiquetas_revisadas=?, etiquetas_sugeridas=CASE WHEN ?=1 THEN etiquetas_sugeridas ELSE '' END, actualizado_en=datetime('now') WHERE id=?",
		boolInt(reviewed), boolInt(reviewed), id)
	return err
}

// TextForAI devuelve el texto extraído del PDF.
func (s *Store) TextForAI(id int64) (string, string, error) {
	var texto, nombre string
	err := s.db.QueryRow("SELECT texto, coalesce(dependencia, '') FROM convenios WHERE id=?", id).Scan(&texto, &nombre)
	return texto, nombre, err
}

// ExistingTags returns the catalog without counts (para el prompt de IA).
func (s *Store) ExistingTags() ([]Tag, error) {
	rows, err := s.db.Query("SELECT id, nombre, tipo, color, protegida FROM etiquetas ORDER BY tipo, nombre")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Nombre, &t.Tipo, &t.Color, &t.Protegida); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (s *Store) String() string {
	return fmt.Sprintf("Store(%s)", s.paths.DB)
}
