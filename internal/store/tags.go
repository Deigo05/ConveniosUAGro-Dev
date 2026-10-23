package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// NormalizeKey removes accents and lowercases for comparisons.
func NormalizeKey(s string) string {
	s = norm.NFD.String(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// ListTags devuelve el catálogo con el número de convenios que usa cada etiqueta.
func (s *Store) ListTags() ([]Tag, error) {
	rows, err := s.db.Query(`
		SELECT e.id, e.nombre, e.tipo, e.color, e.protegida, count(ce.convenio_id)
		FROM etiquetas e
		LEFT JOIN convenio_etiquetas ce ON ce.etiqueta_id = e.id
		GROUP BY e.id
		ORDER BY CASE e.tipo WHEN 'ambito' THEN 0 WHEN 'estado' THEN 1
		                      WHEN 'ciudad' THEN 2 ELSE 3 END,
		         e.nombre COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Nombre, &t.Tipo, &t.Color, &t.Protegida, &t.Total); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// CreateTag crea una etiqueta de ciudad o tema. Las base no se crean desde la interfaz.
func (s *Store) CreateTag(in TagInput) (Tag, error) {
	tipo := strings.ToLower(strings.TrimSpace(in.Tipo))
	if tipo != "ciudad" && tipo != "tema" {
		return Tag{}, errors.New("solo se pueden crear etiquetas de ciudad o tema")
	}
	nombre, err := validateTagName(in.Nombre)
	if err != nil {
		return Tag{}, err
	}
	color := in.Color
	if strings.TrimSpace(color) == "" {
		color = colorForType(tipo)
	}
	color, err = validateColor(color)
	if err != nil {
		return Tag{}, err
	}
	var existe int
	if err := s.db.QueryRow("SELECT count(*) FROM etiquetas WHERE tipo=? AND nombre=?", tipo, nombre).Scan(&existe); err != nil {
		return Tag{}, err
	}
	if existe > 0 {
		return Tag{}, fmt.Errorf("ya existe la etiqueta «%s» en ese grupo", nombre)
	}
	res, err := s.db.Exec("INSERT INTO etiquetas (nombre, tipo, color, protegida) VALUES (?, ?, ?, 0)", nombre, tipo, color)
	if err != nil {
		return Tag{}, err
	}
	id, _ := res.LastInsertId()
	return Tag{ID: id, Nombre: nombre, Tipo: tipo, Color: color}, nil
}

// UpdateTag cambia nombre y color de una etiqueta no protegida.
func (s *Store) UpdateTag(in TagInput) error {
	nombre, err := validateTagName(in.Nombre)
	if err != nil {
		return err
	}
	color, err := validateColor(in.Color)
	if err != nil {
		return err
	}
	var tipo string
	var protegida bool
	if err := s.db.QueryRow("SELECT tipo, protegida FROM etiquetas WHERE id=?", in.ID).Scan(&tipo, &protegida); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("la etiqueta no existe")
		}
		return err
	}
	if protegida {
		return errors.New("las etiquetas base no se pueden modificar")
	}
	var otra int
	if err := s.db.QueryRow("SELECT count(*) FROM etiquetas WHERE tipo=? AND nombre=? AND id<>?", tipo, nombre, in.ID).Scan(&otra); err != nil {
		return err
	}
	if otra > 0 {
		return fmt.Errorf("ya existe la etiqueta «%s» en ese grupo", nombre)
	}
	_, err = s.db.Exec("UPDATE etiquetas SET nombre=?, color=? WHERE id=?", nombre, color, in.ID)
	return err
}

// DeleteTag elimina una etiqueta no protegida y sus asignaciones.
func (s *Store) DeleteTag(id int64) error {
	var protegida bool
	if err := s.db.QueryRow("SELECT protegida FROM etiquetas WHERE id=?", id).Scan(&protegida); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("la etiqueta no existe")
		}
		return err
	}
	if protegida {
		return errors.New("las etiquetas base no se pueden eliminar")
	}
	_, err := s.db.Exec("DELETE FROM etiquetas WHERE id=?", id)
	return err
}

// TagsForConvention devuelve las etiquetas asignadas a un convenio.
func (s *Store) TagsForConvention(id int64) ([]Tag, error) {
	rows, err := s.db.Query(`
		SELECT e.id, e.nombre, e.tipo, e.color, e.protegida
		FROM convenio_etiquetas ce JOIN etiquetas e ON e.id = ce.etiqueta_id
		WHERE ce.convenio_id = ?
		ORDER BY CASE e.tipo WHEN 'ambito' THEN 0 WHEN 'estado' THEN 1
		                      WHEN 'ciudad' THEN 2 ELSE 3 END,
		         e.nombre COLLATE NOCASE`, id)
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

// TagsForIDs agrupa las etiquetas de varios convenios para el listado.
func (s *Store) TagsForIDs(ids []int64) (map[int64][]Tag, error) {
	out := make(map[int64][]Tag, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	ph := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
		out[id] = []Tag{}
	}
	rows, err := s.db.Query(`
		SELECT ce.convenio_id, e.id, e.nombre, e.tipo, e.color, e.protegida
		FROM convenio_etiquetas ce JOIN etiquetas e ON e.id = ce.etiqueta_id
		WHERE ce.convenio_id IN (`+strings.Join(ph, ",")+`)
		ORDER BY CASE e.tipo WHEN 'ambito' THEN 0 WHEN 'estado' THEN 1
		                      WHEN 'ciudad' THEN 2 ELSE 3 END,
		         e.nombre COLLATE NOCASE`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int64
		var t Tag
		if err := rows.Scan(&cid, &t.ID, &t.Nombre, &t.Tipo, &t.Color, &t.Protegida); err != nil {
			return nil, err
		}
		out[cid] = append(out[cid], t)
	}
	return out, rows.Err()
}

func (s *Store) tagsByID(ids []int64) ([]Tag, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ph := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	rows, err := s.db.Query("SELECT id, nombre, tipo, color, protegida FROM etiquetas WHERE id IN ("+strings.Join(ph, ",")+")", args...)
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

// ReplaceTags deja exactamente las etiquetas indicadas. Solo permite una de ámbito
// y una de estado. Si markReviewed, marca la clasificación como revisada.
func (s *Store) ReplaceTags(convenioID int64, ids []int64, markReviewed bool) error {
	tags, err := s.tagsByID(ids)
	if err != nil {
		return err
	}
	seenType := map[string]bool{}
	seen := map[int64]bool{}
	var final []int64
	for _, t := range tags {
		if seen[t.ID] {
			continue
		}
		if (t.Tipo == "ambito" || t.Tipo == "estado") && seenType[t.Tipo] {
			continue
		}
		if t.Tipo == "ambito" || t.Tipo == "estado" {
			seenType[t.Tipo] = true
		}
		seen[t.ID] = true
		final = append(final, t.ID)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM convenio_etiquetas WHERE convenio_id=?", convenioID); err != nil {
		return err
	}
	for _, id := range final {
		if _, err := tx.Exec("INSERT OR IGNORE INTO convenio_etiquetas (convenio_id, etiqueta_id) VALUES (?, ?)", convenioID, id); err != nil {
			return err
		}
	}
	if markReviewed {
		if _, err := tx.Exec("UPDATE convenios SET etiquetas_revisadas=1, etiquetas_sugeridas='', actualizado_en=datetime('now') WHERE id=?", convenioID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec("UPDATE convenios SET actualizado_en=datetime('now') WHERE id=?", convenioID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetSuggestions guarda las sugerencias de la IA como JSON.
func (s *Store) SetSuggestions(id int64, suggestions []Suggestion) error {
	data := ""
	if len(suggestions) > 0 {
		b, err := json.Marshal(suggestions)
		if err != nil {
			return err
		}
		data = string(b)
	}
	_, err := s.db.Exec("UPDATE convenios SET etiquetas_sugeridas=?, etiquetas_revisadas=0, actualizado_en=datetime('now') WHERE id=?", data, id)
	return err
}

// ParseSuggestions convierte el JSON guardado en una lista segura.
func ParseSuggestions(raw string) []Suggestion {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var list []Suggestion
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil
	}
	out := make([]Suggestion, 0, len(list))
	for _, s := range list {
		s.Tipo = strings.ToLower(strings.TrimSpace(s.Tipo))
		s.Nombre = strings.TrimSpace(s.Nombre)
		if s.Nombre == "" || (s.Tipo != "ambito" && s.Tipo != "estado" && s.Tipo != "ciudad" && s.Tipo != "tema") {
			continue
		}
		if len([]rune(s.Nombre)) > 80 {
			s.Nombre = string([]rune(s.Nombre)[:80])
		}
		out = append(out, s)
	}
	return out
}

func (s *Store) findTag(tipo, nombre string) (Tag, bool, error) {
	rows, err := s.db.Query("SELECT id, nombre, tipo, color, protegida FROM etiquetas WHERE tipo=?", tipo)
	if err != nil {
		return Tag{}, false, err
	}
	defer rows.Close()
	key := NormalizeKey(nombre)
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Nombre, &t.Tipo, &t.Color, &t.Protegida); err != nil {
			return Tag{}, false, err
		}
		if NormalizeKey(t.Nombre) == key {
			return t, true, nil
		}
	}
	return Tag{}, false, rows.Err()
}

// AcceptSuggestions convierte las sugerencias en etiquetas reales. Las ciudades
// nuevas se crean; ámbito, estado y tema solo se aceptan si ya existen.
func (s *Store) AcceptSuggestions(convenioID int64) error {
	var raw string
	if err := s.db.QueryRow("SELECT etiquetas_sugeridas FROM convenios WHERE id=?", convenioID).Scan(&raw); err != nil {
		return err
	}
	var ids []int64
	seen := map[string]bool{}
	for _, sug := range ParseSuggestions(raw) {
		tipo := sug.Tipo
		nombre := strings.TrimSpace(sug.Nombre)
		if nombre == "" || seen[tipo+"|"+NormalizeKey(nombre)] {
			continue
		}
		seen[tipo+"|"+NormalizeKey(nombre)] = true
		tag, ok, err := s.findTag(tipo, nombre)
		if err != nil {
			return err
		}
		if !ok {
			if tipo != "ciudad" {
				continue // los temas y las etiquetas base no se inventan
			}
			tag, err = s.CreateTag(TagInput{Nombre: nombre, Tipo: "ciudad", Color: ColorCiudad})
			if err != nil {
				// Si otra instancia la creó, se busca de nuevo.
				tag, ok, err = s.findTag(tipo, nombre)
				if err != nil || !ok {
					continue
				}
			}
		}
		ids = append(ids, tag.ID)
	}
	return s.ReplaceTags(convenioID, ids, true)
}

// DiscardSuggestions marca la clasificación como revisada. Si remove, quita
// también las etiquetas actuales (equivale a «dejar sin etiquetas»).
func (s *Store) DiscardSuggestions(convenioID int64, remove bool) error {
	if remove {
		if err := s.ReplaceTags(convenioID, nil, true); err != nil {
			return err
		}
	}
	_, err := s.db.Exec("UPDATE convenios SET etiquetas_sugeridas='', etiquetas_revisadas=1, actualizado_en=datetime('now') WHERE id=?", convenioID)
	return err
}

// TagByID returns a tag by id.
func (s *Store) TagByID(id int64) (Tag, error) {
	var t Tag
	err := s.db.QueryRow("SELECT id, nombre, tipo, color, protegida FROM etiquetas WHERE id=?", id).
		Scan(&t.ID, &t.Nombre, &t.Tipo, &t.Color, &t.Protegida)
	return t, err
}
