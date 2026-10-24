package store

import (
	"database/sql"
	"errors"
	"regexp"
)

// Valores de convenios.no_es_convenio.
const (
	convenioNormal     = 0
	convenioDudoso     = 1 // la IA cree que el archivo no es un convenio
	convenioConfirmado = 2 // el usuario dijo que sí lo es
)

// Flag indica si ya se aplicó una reparación de una sola vez.
func (s *Store) Flag(name string) (bool, error) {
	var v string
	err := s.db.QueryRow("SELECT valor FROM meta WHERE clave=?", name).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// SetFlag registra que la reparación ya se aplicó.
func (s *Store) SetFlag(name string) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO meta (clave, valor) VALUES (?, datetime('now'))", name)
	return err
}

// SetNoConvenio lo marca el usuario: noEs=true lo deja «por revisar» y
// noEs=false confirma que sí es un convenio (la IA ya no lo vuelve a marcar).
func (s *Store) SetNoConvenio(id int64, noEs bool) error {
	v := convenioConfirmado
	if noEs {
		v = convenioDudoso
	}
	_, err := s.db.Exec("UPDATE convenios SET no_es_convenio=?, actualizado_en=datetime('now') WHERE id=?", v, id)
	return err
}

// SetAIConvenioFlag guarda lo que la IA opinó del tipo de archivo sin pasar
// por encima de lo que confirmó el usuario.
func (s *Store) SetAIConvenioFlag(id int64, esConvenio bool) error {
	if esConvenio {
		_, err := s.db.Exec("UPDATE convenios SET no_es_convenio=? WHERE id=? AND no_es_convenio=?", convenioNormal, id, convenioDudoso)
		return err
	}
	_, err := s.db.Exec("UPDATE convenios SET no_es_convenio=? WHERE id=? AND no_es_convenio=?", convenioDudoso, id, convenioNormal)
	return err
}

// IsFlaggedNoConvenio indica si el archivo está marcado como «no es convenio».
func (s *Store) IsFlaggedNoConvenio(id int64) (bool, error) {
	var v int
	err := s.db.QueryRow("SELECT no_es_convenio FROM convenios WHERE id=?", id).Scan(&v)
	return v == convenioDudoso, err
}

var reNoConvenioNote = regexp.MustCompile(`(?i)(no (es|constituye|corresponde a|se trata de) (un|una) (convenio|acuerdo|contrato)|, no un convenio|no es un instrumento de colaboraci)`)

// FlagNoConveniosFromNotes marca los archivos que la IA ya había descrito en
// las notas como "no es un convenio" antes de que existiera la marca.
func (s *Store) FlagNoConveniosFromNotes() (int, error) {
	rows, err := s.db.Query("SELECT id, notas FROM convenios WHERE no_es_convenio=0 AND notas<>''")
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		var notas string
		if err := rows.Scan(&id, &notas); err != nil {
			rows.Close()
			return 0, err
		}
		if reNoConvenioNote.MatchString(notas) {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, id := range ids {
		if _, err := s.db.Exec("UPDATE convenios SET no_es_convenio=? WHERE id=?", convenioDudoso, id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// BackfillCityStates asigna su estado a las ciudades que no lo tienen: el
// estado más frecuente entre los convenios que usan esa ciudad.
func (s *Store) BackfillCityStates() error {
	rows, err := s.db.Query(`
		SELECT c.id, e.nombre, count(*) AS n
		FROM etiquetas c
		JOIN convenio_etiquetas cc ON cc.etiqueta_id = c.id
		JOIN convenio_etiquetas ce ON ce.convenio_id = cc.convenio_id
		JOIN etiquetas e ON e.id = ce.etiqueta_id AND e.tipo = 'estado'
		WHERE c.tipo = 'ciudad' AND c.estado = ''
		GROUP BY c.id, e.id
		ORDER BY c.id, n DESC, e.nombre`)
	if err != nil {
		return err
	}
	best := map[int64]string{}
	var order []int64
	for rows.Next() {
		var id int64
		var estado string
		var n int
		if err := rows.Scan(&id, &estado, &n); err != nil {
			rows.Close()
			return err
		}
		if _, ok := best[id]; !ok {
			best[id] = estado
			order = append(order, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range order {
		if _, err := s.db.Exec("UPDATE etiquetas SET estado=? WHERE id=? AND estado=''", best[id], id); err != nil {
			return err
		}
	}
	return nil
}

// AnalyzedConventions devuelve los convenios procesados por la IA que no
// están marcados como «no es convenio».
func (s *Store) AnalyzedConventions() ([]int64, error) {
	rows, err := s.db.Query("SELECT id FROM convenios WHERE ia_estado IN ('ok','parcial') AND no_es_convenio<>1 ORDER BY id")
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

// EnsureAmbito agrega «Nacional» al convenio que no tiene ámbito. Un estado
// ya lo implica (ver consistentTags); esto cubre los que no tienen ubicación.
func (s *Store) EnsureAmbito(id int64) error {
	tags, err := s.TagsForConvention(id)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(tags)+1)
	for _, t := range tags {
		if t.Tipo == "ambito" {
			return nil
		}
		ids = append(ids, t.ID)
	}
	nac, ok, err := s.findTag("ambito", TagNacional)
	if err != nil || !ok {
		return err
	}
	return s.ReplaceTagsAuto(id, append(ids, nac.ID))
}
