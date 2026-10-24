package store

import (
	"database/sql"
	"errors"
)

// openAgreementTag devuelve la etiqueta base "Convenio Abierto".
func (s *Store) openAgreementTag() (Tag, error) {
	var t Tag
	err := s.db.QueryRow("SELECT id, nombre, tipo, color, protegida FROM etiquetas WHERE tipo='facultad' AND nombre=?", TagConvenioAbierto).
		Scan(&t.ID, &t.Nombre, &t.Tipo, &t.Color, &t.Protegida)
	if errors.Is(err, sql.ErrNoRows) {
		return t, errors.New("falta la etiqueta base «Convenio Abierto»")
	}
	return t, err
}

// HasFacultyTag indica si el convenio ya tiene una etiqueta de facultad.
func (s *Store) HasFacultyTag(id int64) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM convenio_etiquetas ce JOIN etiquetas e ON e.id=ce.etiqueta_id
		WHERE ce.convenio_id=? AND e.tipo='facultad'`, id).Scan(&n)
	return n > 0, err
}

// MarkOpenAgreement asigna "Convenio Abierto" si el convenio no tiene ninguna
// etiqueta de facultad. No toca una facultad elegida a mano.
func (s *Store) MarkOpenAgreement(id int64) error {
	has, err := s.HasFacultyTag(id)
	if err != nil || has {
		return err
	}
	tag, err := s.openAgreementTag()
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT OR IGNORE INTO convenio_etiquetas (convenio_id, etiqueta_id) VALUES (?, ?)", id, tag.ID)
	return err
}

// UnmarkOpenAgreement quita "Convenio Abierto" (por ejemplo, al capturar una
// facultad que todavía no está en el catálogo).
func (s *Store) UnmarkOpenAgreement(id int64) error {
	tag, err := s.openAgreementTag()
	if err != nil {
		return err
	}
	_, err = s.db.Exec("DELETE FROM convenio_etiquetas WHERE convenio_id=? AND etiqueta_id=?", id, tag.ID)
	return err
}

// OpenAgreementCandidates devuelve los convenios ya analizados o clasificados
// que no nombran facultad y aún no tienen etiqueta de facultad.
func (s *Store) OpenAgreementCandidates() ([]int64, error) {
	rows, err := s.db.Query(`
		SELECT c.id FROM convenios c
		WHERE trim(c.facultad) = '' AND c.no_es_convenio <> 1
		  AND (c.ia_estado IN ('ok','parcial') OR c.etiquetas_revisadas = 1)
		  AND NOT EXISTS (SELECT 1 FROM convenio_etiquetas ce JOIN etiquetas e ON e.id=ce.etiqueta_id
		                  WHERE ce.convenio_id=c.id AND e.tipo='facultad')`)
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
