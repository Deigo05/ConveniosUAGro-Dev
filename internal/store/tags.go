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

// tagColumns son las columnas de etiquetas que leen todas las consultas.
const tagColumns = "e.id, e.nombre, e.tipo, e.color, e.protegida, e.estado"

const tagOrder = `ORDER BY CASE e.tipo WHEN 'ambito' THEN 0 WHEN 'estado' THEN 1
		                      WHEN 'ciudad' THEN 2 WHEN 'facultad' THEN 3 ELSE 4 END,
		         e.nombre COLLATE NOCASE`

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanTag(r scanner, extra ...interface{}) (Tag, error) {
	var t Tag
	err := r.Scan(append([]interface{}{&t.ID, &t.Nombre, &t.Tipo, &t.Color, &t.Protegida, &t.Estado}, extra...)...)
	return t, err
}

func (s *Store) queryTags(query string, args ...interface{}) ([]Tag, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []Tag
	for rows.Next() {
		t, err := scanTag(rows)
		if err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// ListTags devuelve el catálogo con el número de convenios que usa cada etiqueta.
func (s *Store) ListTags() ([]Tag, error) {
	rows, err := s.db.Query(`
		SELECT ` + tagColumns + `, count(ce.convenio_id)
		FROM etiquetas e
		LEFT JOIN convenio_etiquetas ce ON ce.etiqueta_id = e.id
		GROUP BY e.id
		` + tagOrder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []Tag
	for rows.Next() {
		var total int
		t, err := scanTag(rows, &total)
		if err != nil {
			return nil, err
		}
		t.Total = total
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// estadoCanonico devuelve el nombre base de un estado de México ("Guerrero").
func estadoCanonico(nombre string) (string, bool) {
	k := NormalizeKey(nombre)
	if k == "" {
		return "", false
	}
	for _, e := range estadosMx {
		if NormalizeKey(e) == k {
			return e, true
		}
	}
	return "", false
}

// CreateTag crea una etiqueta de ciudad, tema o facultad. Las base no se crean desde la interfaz.
func (s *Store) CreateTag(in TagInput) (Tag, error) {
	tipo := strings.ToLower(strings.TrimSpace(in.Tipo))
	if tipo != "ciudad" && tipo != "tema" && tipo != "facultad" {
		return Tag{}, errors.New("solo se pueden crear etiquetas de ciudad, tema o facultad")
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
	estado := ""
	if tipo == "ciudad" && strings.TrimSpace(in.Estado) != "" {
		var ok bool
		if estado, ok = estadoCanonico(in.Estado); !ok {
			return Tag{}, errors.New("el estado de la ciudad no es válido")
		}
	}
	var existe int
	if err := s.db.QueryRow("SELECT count(*) FROM etiquetas WHERE tipo=? AND nombre=?", tipo, nombre).Scan(&existe); err != nil {
		return Tag{}, err
	}
	if existe > 0 {
		return Tag{}, fmt.Errorf("ya existe la etiqueta «%s» en ese grupo", nombre)
	}
	res, err := s.db.Exec("INSERT INTO etiquetas (nombre, tipo, color, protegida, estado) VALUES (?, ?, ?, 0, ?)", nombre, tipo, color, estado)
	if err != nil {
		return Tag{}, err
	}
	id, _ := res.LastInsertId()
	return Tag{ID: id, Nombre: nombre, Tipo: tipo, Color: color, Estado: estado}, nil
}

// EnsureTag busca la etiqueta (sin distinguir acentos) o la crea con el color
// de su tipo.
func (s *Store) EnsureTag(tipo, nombre string) (Tag, error) {
	return s.ensureTag(tipo, nombre, "")
}

// ensureTag busca la etiqueta (sin distinguir acentos) o la crea. Tolera que
// dos análisis simultáneos la creen a la vez.
func (s *Store) ensureTag(tipo, nombre, estado string) (Tag, error) {
	if tag, ok, err := s.findTag(tipo, nombre); err != nil || ok {
		return tag, err
	}
	tag, err := s.CreateTag(TagInput{Nombre: nombre, Tipo: tipo, Estado: estado})
	if err == nil {
		return tag, nil
	}
	if tag, ok, ferr := s.findTag(tipo, nombre); ferr == nil && ok {
		return tag, nil
	}
	return Tag{}, err
}

// UpdateTag cambia nombre y color de una etiqueta no protegida; en una
// ciudad también su estado, que pasa a los convenios que la usan.
func (s *Store) UpdateTag(in TagInput) error {
	nombre, err := validateTagName(in.Nombre)
	if err != nil {
		return err
	}
	color, err := validateColor(in.Color)
	if err != nil {
		return err
	}
	var tipo, estadoActual string
	var protegida bool
	if err := s.db.QueryRow("SELECT tipo, protegida, estado FROM etiquetas WHERE id=?", in.ID).Scan(&tipo, &protegida, &estadoActual); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("la etiqueta no existe")
		}
		return err
	}
	if protegida {
		return errors.New("las etiquetas base no se pueden modificar")
	}
	estado := estadoActual
	if tipo == "ciudad" {
		estado = ""
		if strings.TrimSpace(in.Estado) != "" {
			var ok bool
			if estado, ok = estadoCanonico(in.Estado); !ok {
				return errors.New("el estado de la ciudad no es válido")
			}
		}
	}
	var otra int
	if err := s.db.QueryRow("SELECT count(*) FROM etiquetas WHERE tipo=? AND nombre=? AND id<>?", tipo, nombre, in.ID).Scan(&otra); err != nil {
		return err
	}
	if otra > 0 {
		return fmt.Errorf("ya existe la etiqueta «%s» en ese grupo", nombre)
	}
	if _, err := s.db.Exec("UPDATE etiquetas SET nombre=?, color=?, estado=? WHERE id=?", nombre, color, estado, in.ID); err != nil {
		return err
	}
	if tipo == "ciudad" && estado != "" && estado != estadoActual {
		return s.applyCityState(in.ID, estado)
	}
	return nil
}

// applyCityState deja el estado de la ciudad (y "Nacional") en los convenios
// que la usan.
func (s *Store) applyCityState(cityID int64, estado string) error {
	ids, err := s.conventionsWithTag(cityID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		tags, err := s.TagsForConvention(id)
		if err != nil {
			return err
		}
		keep := make([]int64, 0, len(tags))
		for _, t := range tags {
			if t.Tipo != "estado" {
				keep = append(keep, t.ID)
			}
		}
		if err := s.ReplaceTagsAuto(id, keep); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) conventionsWithTag(tagID int64) ([]int64, error) {
	rows, err := s.db.Query("SELECT convenio_id FROM convenio_etiquetas WHERE etiqueta_id=?", tagID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteTagIfUnused elimina una etiqueta no protegida si ya no está asignada.
func (s *Store) DeleteTagIfUnused(id int64) error {
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM convenio_etiquetas WHERE etiqueta_id=?", id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := s.db.Exec("DELETE FROM etiquetas WHERE id=? AND protegida=0", id)
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
	return s.queryTags(`
		SELECT `+tagColumns+`
		FROM convenio_etiquetas ce JOIN etiquetas e ON e.id = ce.etiqueta_id
		WHERE ce.convenio_id = ?
		`+tagOrder, id)
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
		SELECT ce.convenio_id, `+tagColumns+`
		FROM convenio_etiquetas ce JOIN etiquetas e ON e.id = ce.etiqueta_id
		WHERE ce.convenio_id IN (`+strings.Join(ph, ",")+`)
		`+tagOrder, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int64
		var t Tag
		if err := rows.Scan(&cid, &t.ID, &t.Nombre, &t.Tipo, &t.Color, &t.Protegida, &t.Estado); err != nil {
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
	tags, err := s.queryTags("SELECT "+tagColumns+" FROM etiquetas e WHERE e.id IN ("+strings.Join(ph, ",")+")", args...)
	if err != nil {
		return nil, err
	}
	// Respeta el orden pedido: con varias etiquetas del mismo tipo gana la primera.
	pos := make(map[int64]int, len(ids))
	for i, id := range ids {
		if _, ok := pos[id]; !ok {
			pos[id] = i
		}
	}
	ordered := make([]Tag, len(tags))
	copy(ordered, tags)
	for i := 1; i < len(ordered); i++ {
		for j := i; j > 0 && pos[ordered[j].ID] < pos[ordered[j-1].ID]; j-- {
			ordered[j], ordered[j-1] = ordered[j-1], ordered[j]
		}
	}
	return ordered, nil
}

// TagConflictError explica por qué una combinación de etiquetas no es válida.
type TagConflictError struct{ msg string }

func (e TagConflictError) Error() string { return e.msg }

// ReplaceTags deja exactamente las etiquetas indicadas, con una sola de
// ámbito, estado, ciudad y facultad. Aplica las reglas de ubicación: la
// ciudad agrega su estado y un estado de México agrega «Nacional». Si una
// ciudad no corresponde al estado o el estado va con «Internacional», devuelve
// TagConflictError. Si markReviewed, marca la clasificación como revisada.
func (s *Store) ReplaceTags(convenioID int64, ids []int64, markReviewed bool) error {
	final, err := s.consistentTags(ids, true)
	if err != nil {
		return err
	}
	return s.writeTags(convenioID, final, markReviewed)
}

// ReplaceTagsAuto es ReplaceTags para los cambios automáticos (facultad,
// reparaciones): corrige los conflictos de ubicación en vez de rechazarlos.
func (s *Store) ReplaceTagsAuto(convenioID int64, ids []int64) error {
	final, err := s.consistentTags(ids, false)
	if err != nil {
		return err
	}
	return s.writeTags(convenioID, final, false)
}

// consistentTags valida las etiquetas y agrega las que implican la ciudad y el
// estado. strict=false corrige los conflictos en vez de rechazarlos: el estado
// de la ciudad y «Nacional» ganan (se usa con las sugerencias de la IA).
func (s *Store) consistentTags(ids []int64, strict bool) ([]int64, error) {
	tags, err := s.tagsByID(ids)
	if err != nil {
		return nil, err
	}
	exclusive := map[string]bool{"ambito": true, "estado": true, "ciudad": true, "facultad": true}
	byType := map[string]Tag{}
	var rest []Tag
	seen := map[int64]bool{}
	for _, t := range tags {
		if seen[t.ID] {
			continue
		}
		seen[t.ID] = true
		if exclusive[t.Tipo] {
			if _, dup := byType[t.Tipo]; !dup {
				byType[t.Tipo] = t
			}
			continue
		}
		rest = append(rest, t)
	}
	if city, ok := byType["ciudad"]; ok && city.Estado == "" {
		// Una ciudad sin estado (de una versión anterior o creada sin él)
		// aprende el estado con el que se usa por primera vez.
		if est, hasEst := byType["estado"]; hasEst {
			if _, err := s.db.Exec("UPDATE etiquetas SET estado=? WHERE id=? AND estado=''", est.Nombre, city.ID); err != nil {
				return nil, err
			}
		}
	} else if ok {
		est, hasEst := byType["estado"]
		if hasEst && NormalizeKey(est.Nombre) != NormalizeKey(city.Estado) {
			if strict {
				return nil, TagConflictError{fmt.Sprintf("la ciudad «%s» pertenece a %s; no puede ir con el estado «%s»", city.Nombre, city.Estado, est.Nombre)}
			}
			hasEst = false
		}
		if !hasEst {
			tag, ok, err := s.findTag("estado", city.Estado)
			if err != nil {
				return nil, err
			}
			if ok {
				byType["estado"] = tag
			}
		}
	}
	if _, ok := byType["estado"]; ok {
		amb, hasAmb := byType["ambito"]
		if hasAmb && NormalizeKey(amb.Nombre) != NormalizeKey(TagNacional) {
			if strict {
				return nil, TagConflictError{fmt.Sprintf("un estado de México va con «%s», no con «%s»", TagNacional, amb.Nombre)}
			}
			hasAmb = false
		}
		if !hasAmb {
			tag, ok, err := s.findTag("ambito", TagNacional)
			if err != nil {
				return nil, err
			}
			if ok {
				byType["ambito"] = tag
			}
		}
	}
	final := make([]int64, 0, len(byType)+len(rest))
	for _, tipo := range []string{"ambito", "estado", "ciudad", "facultad"} {
		if t, ok := byType[tipo]; ok {
			final = append(final, t.ID)
		}
	}
	for _, t := range rest {
		final = append(final, t.ID)
	}
	return final, nil
}

func (s *Store) writeTags(convenioID int64, final []int64, markReviewed bool) error {
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
		if s.Nombre == "" || (s.Tipo != "ambito" && s.Tipo != "estado" && s.Tipo != "ciudad" && s.Tipo != "tema" && s.Tipo != "facultad") {
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
	tags, err := s.queryTags("SELECT "+tagColumns+" FROM etiquetas e WHERE e.tipo=?", tipo)
	if err != nil {
		return Tag{}, false, err
	}
	key := NormalizeKey(nombre)
	for _, t := range tags {
		if NormalizeKey(t.Nombre) == key {
			return t, true, nil
		}
	}
	return Tag{}, false, nil
}

// findCity busca la etiqueta de una ciudad sugerida por la IA. Prefiere la
// variante con el mismo estado ("Venustiano Carranza (Chiapas)"); una ciudad
// sin estado lo adopta. Si no existe, la crea con ese estado.
func (s *Store) findCity(nombre, estado string) (Tag, error) {
	tags, err := s.queryTags("SELECT " + tagColumns + " FROM etiquetas e WHERE e.tipo='ciudad'")
	if err != nil {
		return Tag{}, err
	}
	key, estKey := NormalizeKey(nombre), NormalizeKey(estado)
	var same, stateless, other *Tag
	for i := range tags {
		t := &tags[i]
		k := NormalizeKey(t.Nombre)
		if estado != "" && k == NormalizeKey(nombre+" ("+estado+")") {
			return *t, nil
		}
		if k != key {
			continue
		}
		switch {
		case estado != "" && NormalizeKey(t.Estado) == estKey:
			same = t
		case t.Estado == "":
			stateless = t
		default:
			other = t
		}
	}
	switch {
	case same != nil:
		return *same, nil
	case stateless != nil:
		if estado != "" {
			if _, err := s.db.Exec("UPDATE etiquetas SET estado=? WHERE id=?", estado, stateless.ID); err != nil {
				return Tag{}, err
			}
			stateless.Estado = estado
		}
		return *stateless, nil
	case other != nil:
		// Mismo nombre en otro estado: gana la ciudad ya registrada y
		// consistentTags corrige el estado.
		return *other, nil
	}
	return s.ensureTag("ciudad", nombre, estado)
}

// AcceptSuggestions convierte las sugerencias en etiquetas reales. Las ciudades
// nuevas se crean con el estado sugerido; ámbito, estado y tema solo se
// aceptan si ya existen. Los conflictos de ubicación se corrigen solos.
func (s *Store) AcceptSuggestions(convenioID int64) error {
	var raw string
	if err := s.db.QueryRow("SELECT etiquetas_sugeridas FROM convenios WHERE id=?", convenioID).Scan(&raw); err != nil {
		return err
	}
	sugs := ParseSuggestions(raw)
	estado := ""
	for _, sug := range sugs {
		if sug.Tipo == "estado" {
			if e, ok := estadoCanonico(sug.Nombre); ok {
				estado = e
				break
			}
		}
	}
	var ids []int64
	seen := map[string]bool{}
	for _, sug := range sugs {
		tipo := sug.Tipo
		nombre := strings.TrimSpace(sug.Nombre)
		if nombre == "" || seen[tipo+"|"+NormalizeKey(nombre)] {
			continue
		}
		seen[tipo+"|"+NormalizeKey(nombre)] = true
		var tag Tag
		var err error
		switch tipo {
		case "ciudad":
			tag, err = s.findCity(nombre, estado)
		case "facultad":
			tag, err = s.ensureTag(tipo, nombre, "")
		default:
			var ok bool
			tag, ok, err = s.findTag(tipo, nombre)
			if err == nil && !ok {
				continue // los temas y las etiquetas base no se inventan
			}
		}
		if err != nil {
			return err
		}
		ids = append(ids, tag.ID)
	}
	final, err := s.consistentTags(ids, false)
	if err != nil {
		return err
	}
	return s.writeTags(convenioID, final, true)
}

// MarkTagsReviewed marca la clasificación como revisada sin cambiar etiquetas.
func (s *Store) MarkTagsReviewed(convenioID int64) error {
	_, err := s.db.Exec("UPDATE convenios SET etiquetas_revisadas=1, etiquetas_sugeridas='', actualizado_en=datetime('now') WHERE id=?", convenioID)
	return err
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
	return scanTag(s.db.QueryRow("SELECT "+tagColumns+" FROM etiquetas e WHERE e.id=?", id))
}
