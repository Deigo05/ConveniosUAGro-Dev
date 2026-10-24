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
	res, err := s.db.Exec(`
		INSERT INTO convenios (
			dependencia, archivo_nombre_original, archivo_guardado, archivo_bytes,
			archivo_sha256, paginas, es_escaneado, texto_pdf, texto, ia_estado,
			transcripcion_estado, paginas_por_transcribir, etiquetas_revisadas,
			creado_en, actualizado_en
		) VALUES ('', ?, ?, ?, ?, ?, ?, ?, ?, 'omitido', ?, ?, 0, datetime('now'), datetime('now'))`,
		f.NombreOriginal, f.NombreGuardado, f.Bytes, f.SHA256, f.Paginas, boolInt(f.Escaneado),
		f.Texto, f.Texto, estado, f.PaginasSinTexto)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
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

var (
	reSearchPart = regexp.MustCompile(`"([^"]*)"|[\p{L}\p{N}]+`)
	reSearchWord = regexp.MustCompile(`[\p{L}\p{N}]+`)
	// Palabras tan comunes que, como prefijo, coincidirían con casi todo.
	stopwords = map[string]bool{
		"a": true, "al": true, "con": true, "de": true, "del": true, "el": true, "en": true,
		"la": true, "las": true, "lo": true, "los": true, "o": true, "para": true, "por": true,
		"se": true, "un": true, "una": true, "y": true,
	}
)

// searchTerms convierte el texto del buscador en términos FTS5: las palabras
// sueltas se buscan como prefijo y lo que va entre comillas, como frase exacta.
func searchTerms(q string) []string {
	var terms, common []string
	for _, m := range reSearchPart.FindAllStringSubmatch(q, 20) {
		if strings.HasPrefix(m[0], `"`) {
			words := reSearchWord.FindAllString(m[1], 12)
			if len(words) > 0 {
				terms = append(terms, `"`+strings.Join(words, " ")+`"`)
			}
			continue
		}
		term := `"` + m[0] + `"*`
		if stopwords[NormalizeKey(m[0])] {
			common = append(common, term)
			continue
		}
		terms = append(terms, term)
	}
	if len(terms) == 0 {
		terms = common // la búsqueda solo tenía palabras comunes
	}
	if len(terms) > 10 {
		terms = terms[:10]
	}
	return terms
}

// ftsQuery exige todos los términos.
func ftsQuery(q string) string { return strings.Join(searchTerms(q), " AND ") }

// ftsQueryAny acepta cualquiera de los términos; se usa como respaldo cuando
// ningún convenio contiene todas las palabras.
func ftsQueryAny(q string) string { return strings.Join(searchTerms(q), " OR ") }

// relevanceTiers devuelve, de mayor a menor peso, consultas FTS que suben un
// resultado sobre los demás: lo buscado como frase en el nombre de la
// contraparte (la última palabra puede estar a medio escribir), la frase
// exacta en cualquier campo, la frase con la última palabra como prefijo y
// todas las palabras completas. Sin ellas, buscar "GRUPO PAR DE 3" ordenaba
// casi al azar, porque "par" y "3" también son el inicio de "para" y "39070",
// que aparecen en casi todos.
func relevanceTiers(q string) []string {
	words := reSearchWord.FindAllString(q, 12)
	if len(words) == 0 {
		return nil
	}
	phrase := `"` + strings.Join(words, " ") + `"`
	var exact []string
	for _, term := range searchTerms(q) {
		exact = append(exact, strings.TrimSuffix(term, "*"))
	}
	var tiers []string
	seen := map[string]bool{}
	for _, tier := range []string{"dependencia : " + phrase + "*", phrase, phrase + "*", strings.Join(exact, " AND ")} {
		if !seen[tier] {
			seen[tier] = true
			tiers = append(tiers, tier)
		}
	}
	return tiers
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

// List aplica búsqueda FTS, etiquetas, vigencia, orden y paginación. Si
// ningún convenio contiene todas las palabras buscadas, repite la búsqueda con
// cualquiera de ellas y marca el resultado como aproximado.
func (s *Store) List(f ListFilter) (ListResult, error) {
	res, err := s.list(f, ftsQuery(f.Query))
	if err != nil || res.Total > 0 {
		return res, err
	}
	all, any := ftsQuery(f.Query), ftsQueryAny(f.Query)
	if any == all {
		return res, nil
	}
	approx, err := s.list(f, any)
	if err != nil || approx.Total == 0 {
		return res, err
	}
	approx.Aproximado = true
	return approx, nil
}

func (s *Store) list(f ListFilter, fts string) (ListResult, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	conds := []string{"1=1"}
	var args []interface{}

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
	for _, tipo := range []string{"ambito", "estado", "ciudad", "facultad", "tema"} {
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
	case "por_vencer":
		conds = append(conds, "(c.vigencia_indefinida=0 AND c.fecha_vigencia>=? AND c.fecha_vigencia<=date(?, '+90 days'))")
		args = append(args, today, today)
	case "no_vigentes":
		conds = append(conds, "(c.vigencia_indefinida=0 AND c.fecha_vigencia IS NOT NULL AND c.fecha_vigencia<>'' AND c.fecha_vigencia<?)")
		args = append(args, today)
	case "sin_fecha":
		conds = append(conds, "(c.vigencia_indefinida=0 AND (c.fecha_vigencia IS NULL OR c.fecha_vigencia=''))")
	}
	if f.SoloPendientes {
		conds = append(conds, "c.etiquetas_revisadas=0")
	}
	if f.SoloNoConvenios {
		conds = append(conds, "c.no_es_convenio=1")
	}
	limit := pageSize
	if len(f.ids) > 0 {
		ph := strings.TrimRight(strings.Repeat("?,", len(f.ids)), ",")
		conds = append(conds, "c.id IN ("+ph+")")
		for _, id := range f.ids {
			args = append(args, id)
		}
		limit, f.Page = len(f.ids), 1
	} else if !f.IncluirAnteriores {
		// Las versiones anteriores solo se ven en el historial de la vigente.
		conds = append(conds, "c.reemplazado_por IS NULL")
	}

	where := " WHERE " + strings.Join(conds, " AND ")
	var total int
	if err := s.db.QueryRow("SELECT count(*) FROM "+from+where, args...).Scan(&total); err != nil {
		return ListResult{}, err
	}
	var totalRegistrados int
	if err := s.db.QueryRow("SELECT count(*) FROM convenios WHERE reemplazado_por IS NULL").Scan(&totalRegistrados); err != nil {
		return ListResult{}, err
	}
	pages := (total + limit - 1) / limit
	if pages < 1 {
		pages = 1
	}
	if f.Page > pages {
		f.Page = pages
	}

	order := orderSQL(f.Order, f.Dir)
	var orderArgs []interface{}
	if fts != "" && (f.Order == "" || f.Order == "relevancia") {
		var keys []string
		for _, tier := range relevanceTiers(f.Query) {
			if tier != fts {
				keys = append(keys, "c.id IN (SELECT rowid FROM convenios_fts WHERE convenios_fts MATCH ?) DESC")
				orderArgs = append(orderArgs, tier)
			}
		}
		// Dentro de cada grupo, una coincidencia en el nombre pesa más que en el texto.
		order = strings.Join(append(keys, "bm25(convenios_fts, 10.0), c.id DESC"), ", ")
	}
	offset := (f.Page - 1) * limit
	query := `SELECT c.id, c.dependencia, coalesce(c.firmante_dependencia,''), coalesce(c.cargo_dependencia,''),
		coalesce(c.firmante_uagro,''), coalesce(c.cargo_uagro,''), coalesce(c.facultad,''), coalesce(c.facultad_ciudad,''),
		coalesce(c.fecha_firma,''), coalesce(c.fecha_vigencia,''),
		c.vigencia_indefinida, c.vigencia_calculada, c.archivo_nombre_original, c.paginas,
		c.es_escaneado, c.ia_estado, c.ia_error, c.etiquetas_revisadas,
		c.transcripcion_estado, c.transcripcion_error, c.paginas_por_transcribir,
		c.paginas_transcritas, c.creado_en, c.actualizado_en, c.dependencia_corta, c.tipo_documento,
		coalesce(c.reemplazado_por, 0), c.no_es_convenio=1`
	if fts != "" {
		// char(2) y char(3) marcan la coincidencia; la interfaz los resalta.
		query += `, snippet(convenios_fts, -1, char(2), char(3), '…', 14)`
	} else {
		query += `, ''`
	}
	query += " FROM " + from + where + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	argsPage := append(append(append([]interface{}{}, args...), orderArgs...), limit, offset)

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
		if err := rows.Scan(&c.ID, &c.Dependencia, &c.FirmanteDependencia, &c.CargoDependencia,
			&c.FirmanteUAGro, &c.CargoUAGro, &c.Facultad, &c.FacultadCiudad,
			&c.FechaFirma, &c.FechaVigencia,
			&indef, &calc, &c.ArchivoNombreOriginal, &c.Paginas, &scanned,
			&c.IAEstado, &c.IAError, &revisadas, &c.TranscripcionEstado,
			&c.TranscripcionError, &c.PaginasPorTranscribir, &c.PaginasTranscritas,
			&c.CreadoEn, &c.ActualizadoEn, &c.DependenciaCorta, &c.TipoDocumento, &c.ReemplazadoPor, &c.NoEsConvenio, &c.Fragmento); err != nil {
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
		items[i].ArchivoNombre = FileName(items[i])
	}
	if err := s.attachMarks(items); err != nil {
		return ListResult{}, err
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
		SELECT id, dependencia, firmante_dependencia, cargo_dependencia, firmante_uagro,
		       cargo_uagro, facultad, facultad_ciudad, fecha_firma, fecha_vigencia, vigencia_indefinida,
		       vigencia_texto, vigencia_calculada, notas, archivo_nombre_original,
		       archivo_guardado, archivo_bytes, archivo_sha256, paginas, es_escaneado,
		       texto_pdf, texto, ia_estado, ia_error, etiquetas_sugeridas, etiquetas_revisadas,
		       transcripcion_estado, transcripcion_error, paginas_por_transcribir,
		       paginas_transcritas, creado_en, actualizado_en, dependencia_corta, tipo_documento,
		       coalesce(reemplazado_por, 0), no_es_convenio=1
		FROM convenios WHERE id=?`, id).Scan(
		&c.ID, &c.Dependencia, &c.FirmanteDependencia, &c.CargoDependencia, &c.FirmanteUAGro,
		&c.CargoUAGro, &c.Facultad, &c.FacultadCiudad, &fechaFirma, &fechaVigencia, &indef,
		&c.VigenciaTexto, &calc, &c.Notas, &c.ArchivoNombreOriginal,
		&c.ArchivoGuardado, &c.ArchivoBytes, &c.ArchivoSHA256, &c.Paginas, &scanned,
		&textoPDF, &c.Texto, &c.IAEstado, &c.IAError, &sugerenciasRaw, &revisadas,
		&c.TranscripcionEstado, &c.TranscripcionError, &c.PaginasPorTranscribir,
		&c.PaginasTranscritas, &c.CreadoEn, &c.ActualizadoEn, &c.DependenciaCorta, &c.TipoDocumento, &c.ReemplazadoPor, &c.NoEsConvenio)
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
	c.ArchivoNombre = FileName(c.ConventionSummary)
	if c.Relacionados, err = s.relatedFor(id); err != nil {
		return c, err
	}
	if c.Relacionados == nil {
		c.Relacionados = []RelatedRef{}
	}
	for _, r := range c.Relacionados {
		if r.Motivo == MotivoRenovacion {
			c.PosibleRenovacion = true
		} else {
			c.Duplicados++
		}
	}
	head := int64(0)
	if c.Historial, head, err = s.History(id); err != nil {
		return c, err
	}
	if head != id {
		c.VersionActual = head
	}
	if len(c.Historial) > 0 && head == id {
		c.Versiones = len(c.Historial) - 1
	}
	return c, nil
}

// Update guarda la captura manual.
func (s *Store) Update(in ConventionInput) error {
	if !IsDate(in.FechaFirma) || !IsDate(in.FechaVigencia) {
		return errors.New("las fechas deben tener el formato AAAA-MM-DD y un año entre 1950 y 2100")
	}
	tipo := NormalizeTipoDocumento(in.TipoDocumento)
	if strings.TrimSpace(in.TipoDocumento) != "" && tipo == "" {
		return errors.New("el tipo de documento no es válido")
	}
	_, err := s.db.Exec(`
		UPDATE convenios SET dependencia=?, dependencia_corta=?, tipo_documento=?,
			firmante_dependencia=?, cargo_dependencia=?,
			firmante_uagro=?, cargo_uagro=?, facultad=?, facultad_ciudad=?, fecha_firma=?, fecha_vigencia=?,
			vigencia_indefinida=?, vigencia_calculada=0, vigencia_texto=?, notas=?,
			actualizado_en=datetime('now')
		WHERE id=?`,
		strings.TrimSpace(in.Dependencia), cutRunes(cleanFileName(in.DependenciaCorta), 60), tipo,
		strings.TrimSpace(in.FirmanteDependencia),
		strings.TrimSpace(in.CargoDependencia), strings.TrimSpace(in.FirmanteUAGro),
		strings.TrimSpace(in.CargoUAGro), strings.TrimSpace(in.Facultad), strings.TrimSpace(in.FacultadCiudad),
		nullIfEmpty(in.FechaFirma), nullIfEmpty(in.FechaVigencia),
		boolInt(in.VigenciaIndefinida), strings.TrimSpace(in.VigenciaTexto), strings.TrimSpace(in.Notas), in.ID)
	return err
}

func nullIfEmpty(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.TrimSpace(s)
}

// SetFacultyCity guarda la sede de la facultad cuando se resuelve por catálogo.
func (s *Store) SetFacultyCity(id int64, city string) error {
	_, err := s.db.Exec("UPDATE convenios SET facultad_ciudad=?, actualizado_en=datetime('now') WHERE id=?", strings.TrimSpace(city), id)
	return err
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
		UPDATE convenios SET dependencia=?, dependencia_corta=?, tipo_documento=?,
			firmante_dependencia=?, cargo_dependencia=?,
			firmante_uagro=?, cargo_uagro=?, facultad=?, facultad_ciudad=?, fecha_firma=?, fecha_vigencia=?,
			vigencia_indefinida=?, vigencia_calculada=?, vigencia_texto=?, notas=?,
			ia_estado=?, ia_error='', etiquetas_sugeridas=?, etiquetas_revisadas=0,
			actualizado_en=datetime('now')
		WHERE id=?`,
		strings.TrimSpace(in.Dependencia), cutRunes(cleanFileName(in.DependenciaCorta), 60),
		NormalizeTipoDocumento(in.TipoDocumento), strings.TrimSpace(in.FirmanteDependencia),
		strings.TrimSpace(in.CargoDependencia), strings.TrimSpace(in.FirmanteUAGro),
		strings.TrimSpace(in.CargoUAGro), strings.TrimSpace(in.Facultad), strings.TrimSpace(in.FacultadCiudad),
		nullIfEmpty(in.FechaFirma), nullIfEmpty(in.FechaVigencia),
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
		FROM convenios WHERE reemplazado_por IS NULL`, today, today, today, today).Scan(
		&st.Total, &st.Vigentes, &st.PorVencer, &st.Vencidos, &st.SinFecha,
		&st.Indefinidas, &st.EtiquetasPendientes)
	if err != nil {
		return st, err
	}
	if err := s.db.QueryRow("SELECT count(*) FROM etiquetas").Scan(&st.Etiquetas); err != nil {
		return st, err
	}
	if err := s.db.QueryRow(`SELECT (SELECT count(*) FROM convenios WHERE reemplazado_por IS NOT NULL),
		(SELECT count(*) FROM convenio_duplicados WHERE motivo <> 'renovacion'),
		(SELECT count(*) FROM convenio_duplicados WHERE motivo = 'renovacion'),
		(SELECT count(*) FROM convenios WHERE no_es_convenio = 1)`).Scan(
		&st.VersionesAnteriores, &st.Duplicados, &st.Renovaciones, &st.NoConvenios); err != nil {
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
	return s.queryTags("SELECT " + tagColumns + " FROM etiquetas e ORDER BY e.tipo, e.nombre")
}

func (s *Store) String() string {
	return fmt.Sprintf("Store(%s)", s.paths.DB)
}
