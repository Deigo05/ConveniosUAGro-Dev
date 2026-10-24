package store

import (
	"regexp"
	"strings"
	"unicode"
)

// NameCorrection unifica un nombre mal leído por el modelo de visión con la
// variante correcta que aparece en otra parte del mismo documento.
type NameCorrection struct {
	Original string `json:"original"`
	Correcto string `json:"correcto"`
	Cambios  int    `json:"cambios"`
}

// HasTranscription indica si el convenio tiene páginas transcritas por visión.
func (s *Store) HasTranscription(id int64) (bool, error) {
	var n int
	err := s.db.QueryRow("SELECT count(*) FROM transcripcion_paginas WHERE convenio_id=?", id).Scan(&n)
	return n > 0, err
}

// minNameSimilarity evita que una "corrección" cambie un nombre por otro
// distinto: solo se aceptan variantes que difieren en pocas letras.
const minNameSimilarity = 0.6

// ApplyNameCorrections reemplaza en las páginas transcritas las variantes que
// el modelo marcó como errores de lectura. Por seguridad solo acepta nombres de
// dos o más palabras, parecidos entre sí, y cuya forma correcta ya aparece en
// el documento; nunca introduce un nombre nuevo. Devuelve lo que se cambió.
func (s *Store) ApplyNameCorrections(id int64, corrections []NameCorrection) ([]NameCorrection, error) {
	if len(corrections) == 0 {
		return nil, nil
	}
	var full string
	if err := s.db.QueryRow("SELECT texto FROM convenios WHERE id=?", id).Scan(&full); err != nil {
		return nil, err
	}
	rows, err := s.db.Query("SELECT pagina, texto FROM transcripcion_paginas WHERE convenio_id=? ORDER BY pagina", id)
	if err != nil {
		return nil, err
	}
	pages := map[int]string{}
	var order []int
	for rows.Next() {
		var p int
		var t string
		if err := rows.Scan(&p, &t); err != nil {
			rows.Close()
			return nil, err
		}
		pages[p] = t
		order = append(order, p)
	}
	rows.Close()
	if len(order) == 0 {
		return nil, nil
	}

	var applied []NameCorrection
	changed := map[int]bool{}
	for _, c := range corrections {
		orig := cleanName(c.Original)
		good := cleanName(c.Correcto)
		if orig == "" || good == "" || orig == good || len(strings.Fields(orig)) < 2 || len(strings.Fields(good)) < 2 {
			continue
		}
		if similarity(NormalizeKey(orig), NormalizeKey(good)) < minNameSimilarity {
			continue
		}
		goodRe := namePattern(good)
		if !goodRe.MatchString(full) {
			continue // la variante "correcta" no está en el documento
		}
		origRe := namePattern(orig)
		total := 0
		for _, p := range order {
			out, n := replaceName(pages[p], origRe, good)
			if n > 0 {
				pages[p] = out
				changed[p] = true
				total += n
			}
		}
		if total > 0 {
			applied = append(applied, NameCorrection{Original: orig, Correcto: good, Cambios: total})
		}
	}
	if len(applied) == 0 {
		return nil, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for p := range changed {
		if _, err := tx.Exec("UPDATE transcripcion_paginas SET texto=? WHERE convenio_id=? AND pagina=?", pages[p], id, p); err != nil {
			return nil, err
		}
	}
	if err := rebuildText(tx, id); err != nil {
		return nil, err
	}
	return applied, tx.Commit()
}

// TranscriptionPage devuelve el texto transcrito de una página.
func (s *Store) TranscriptionPage(id int64, page int) (string, error) {
	var t string
	err := s.db.QueryRow("SELECT texto FROM transcripcion_paginas WHERE convenio_id=? AND pagina=?", id, page).Scan(&t)
	return t, err
}

// ApplyPageCorrections aplica en una página las correcciones que el modelo
// confirmó al mirar su imagen. A diferencia de ApplyNameCorrections no exige
// que las variantes se parezcan (un nombre tapado por una firma puede leerse
// muy distinto), pero el fragmento original debe estar en esa página y ser
// corto, para que nunca se reescriba un párrafo.
func (s *Store) ApplyPageCorrections(id int64, page int, corrections []NameCorrection) ([]NameCorrection, error) {
	text, err := s.TranscriptionPage(id, page)
	if err != nil {
		return nil, err
	}
	var applied []NameCorrection
	for _, c := range corrections {
		orig := cleanName(c.Original)
		good := cleanName(c.Correcto)
		ow, gw := len(strings.Fields(orig)), len(strings.Fields(good))
		if orig == "" || good == "" || strings.EqualFold(orig, good) || ow > 12 || gw > ow+4 || len([]rune(good)) > 160 {
			continue
		}
		out, n := replaceName(text, namePattern(orig), good)
		if n == 0 {
			continue
		}
		text = out
		applied = append(applied, NameCorrection{Original: orig, Correcto: good, Cambios: n})
	}
	if len(applied) == 0 {
		return nil, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE transcripcion_paginas SET texto=? WHERE convenio_id=? AND pagina=?", text, id, page); err != nil {
		return nil, err
	}
	if err := rebuildText(tx, id); err != nil {
		return nil, err
	}
	return applied, tx.Commit()
}

func cleanName(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "[?]", " ")), " ")
}

// namePattern busca el nombre como palabras completas, sin distinguir
// mayúsculas, y tolera marcas [?] y saltos de línea entre las palabras.
func namePattern(name string) *regexp.Regexp {
	words := strings.Fields(name)
	for i, w := range words {
		words[i] = regexp.QuoteMeta(w)
	}
	sep := `(?:\s*\[\?\])?\s+`
	return regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}])(` + strings.Join(words, sep) + `(?:\s*\[\?\])?)($|[^\p{L}\p{N}])`)
}

// replaceName sustituye cada aparición conservando el uso de mayúsculas del
// documento (encabezados en mayúsculas siguen en mayúsculas).
func replaceName(text string, re *regexp.Regexp, good string) (string, int) {
	var b strings.Builder
	last, n := 0, 0
	for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
		start, end := m[4], m[5] // grupo 2: el nombre
		found := text[start:end]
		repl := good
		if isUpper(found) {
			repl = strings.ToUpper(good)
		}
		b.WriteString(text[last:start])
		b.WriteString(repl)
		last = end
		n++
	}
	if n == 0 {
		return text, 0
	}
	b.WriteString(text[last:])
	return b.String(), n
}

func isUpper(s string) bool {
	letters := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if !unicode.IsUpper(r) {
				return false
			}
		}
	}
	return letters > 0
}

// similarity es 1 - distancia de Levenshtein / longitud mayor.
func similarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 && len(rb) == 0 {
		return 1
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	longest := max(len(ra), len(rb))
	return 1 - float64(prev[len(rb)])/float64(longest)
}

// CloneAnalysis copia los datos, etiquetas y transcripción de un convenio ya
// procesado a un duplicado recién importado (mismo SHA-256), para no volver a
// pagar la IA por el mismo documento. Devuelve false si el original aún no
// tiene nada que copiar.
func (s *Store) CloneAnalysis(src, dst int64) (bool, error) {
	var ia, trans string
	if err := s.db.QueryRow("SELECT ia_estado, transcripcion_estado FROM convenios WHERE id=?", src).Scan(&ia, &trans); err != nil {
		return false, err
	}
	if ia != "ok" && ia != "parcial" && trans != "completa" {
		return false, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		UPDATE convenios SET (dependencia, dependencia_corta, tipo_documento, firmante_dependencia, cargo_dependencia, firmante_uagro,
			cargo_uagro, facultad, facultad_ciudad, fecha_firma, fecha_vigencia, vigencia_indefinida,
			vigencia_texto, vigencia_calculada, notas, texto, ia_estado, etiquetas_revisadas,
			transcripcion_estado, paginas_por_transcribir, paginas_transcritas, no_es_convenio) =
		(SELECT dependencia, dependencia_corta, tipo_documento, firmante_dependencia, cargo_dependencia, firmante_uagro,
			cargo_uagro, facultad, facultad_ciudad, fecha_firma, fecha_vigencia, vigencia_indefinida,
			vigencia_texto, vigencia_calculada, notas, texto, ia_estado, etiquetas_revisadas,
			transcripcion_estado, paginas_por_transcribir, paginas_transcritas, no_es_convenio
		 FROM convenios WHERE id=?),
		huella=NULL, actualizado_en=datetime('now')
		WHERE id=?`, src, dst); err != nil {
		return false, err
	}
	if _, err := tx.Exec("DELETE FROM transcripcion_paginas WHERE convenio_id=?", dst); err != nil {
		return false, err
	}
	if _, err := tx.Exec("INSERT INTO transcripcion_paginas (convenio_id, pagina, texto) SELECT ?, pagina, texto FROM transcripcion_paginas WHERE convenio_id=?", dst, src); err != nil {
		return false, err
	}
	if _, err := tx.Exec("DELETE FROM convenio_etiquetas WHERE convenio_id=?", dst); err != nil {
		return false, err
	}
	if _, err := tx.Exec("INSERT INTO convenio_etiquetas (convenio_id, etiqueta_id) SELECT ?, etiqueta_id FROM convenio_etiquetas WHERE convenio_id=?", dst, src); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
