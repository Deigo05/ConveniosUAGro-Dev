package store

import (
	"database/sql"
	"errors"
	"strings"

	"convenios-uagro-desktop/internal/fechas"
)

// dateCheckKey resume, en SQL, los datos de los que depende la comprobación
// de fechas: si cambia, el convenio se vuelve a comprobar. El texto lo cubren
// actualizado_en, su largo y la invalidación de rebuildText.
const dateCheckKey = `c.actualizado_en || '|' || coalesce(c.fecha_firma, '') || '|' || coalesce(c.fecha_vigencia, '') || '|' ||
	c.vigencia_indefinida || c.vigencia_calculada || '|' || length(c.vigencia_texto) || '|' || length(c.texto)`

// RefreshDateChecks compara las fechas de los convenios que cambiaron desde
// la última vez con su texto (ver internal/fechas) y guarda las que conviene
// confirmar. No llama a la IA. Una fecha confirmada por el usuario sigue
// confirmada mientras no cambie.
func (s *Store) RefreshDateChecks() error {
	rows, err := s.db.Query(`SELECT c.id FROM convenios c
		LEFT JOIN revision_fechas_hechas h ON h.convenio_id = c.id
		WHERE h.clave IS NULL OR h.clave <> ` + dateCheckKey)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.checkDates(id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) checkDates(id int64) error {
	var in fechas.Input
	var firma, vigencia sql.NullString
	var indef, calc int
	var clave string
	err := s.db.QueryRow(`SELECT c.texto, c.fecha_firma, c.fecha_vigencia, c.vigencia_texto,
		c.vigencia_indefinida, c.vigencia_calculada, `+dateCheckKey+` FROM convenios c WHERE c.id=?`, id).Scan(
		&in.Texto, &firma, &vigencia, &in.VigenciaTexto, &indef, &calc, &clave)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	in.FechaFirma, in.FechaVigencia = firma.String, vigencia.String
	in.VigenciaIndefinida, in.VigenciaCalculada = indef != 0, calc != 0
	valor := map[string]string{fechas.Firma: in.FechaFirma, fechas.Vigencia: in.FechaVigencia}
	results := map[string]fechas.Result{}
	for _, r := range fechas.Check(in) {
		results[r.Campo] = r
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, campo := range []string{fechas.Firma, fechas.Vigencia} {
		r, ok := results[campo]
		if !ok {
			if _, err := tx.Exec("DELETE FROM revision_fechas WHERE convenio_id=? AND campo=?", id, campo); err != nil {
				return err
			}
			continue
		}
		// La confirmación del usuario se conserva solo si es de esta misma fecha.
		if _, err := tx.Exec(`INSERT INTO revision_fechas (convenio_id, campo, valor, motivo, propuesta, cita)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(convenio_id, campo) DO UPDATE SET
				confirmada = CASE WHEN revision_fechas.confirmada = excluded.valor THEN revision_fechas.confirmada END,
				valor = excluded.valor, motivo = excluded.motivo, propuesta = excluded.propuesta, cita = excluded.cita`,
			id, campo, valor[campo], r.Motivo, r.Propuesta, r.Cita); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO revision_fechas_hechas (convenio_id, clave) VALUES (?, ?)
		ON CONFLICT(convenio_id) DO UPDATE SET clave = excluded.clave`, id, clave); err != nil {
		return err
	}
	return tx.Commit()
}

// pendingDateFilter es la condición SQL «tiene alguna fecha por confirmar».
const pendingDateFilter = `EXISTS (SELECT 1 FROM revision_fechas rf WHERE rf.convenio_id = c.id
	AND (rf.confirmada IS NULL OR rf.confirmada <> rf.valor))`

// DateReviews devuelve las fechas por confirmar de un convenio.
func (s *Store) DateReviews(id int64) ([]fechas.Result, error) {
	rows, err := s.db.Query(`SELECT campo, motivo, propuesta, cita FROM revision_fechas
		WHERE convenio_id=? AND (confirmada IS NULL OR confirmada <> valor) ORDER BY campo`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []fechas.Result{}
	for rows.Next() {
		var r fechas.Result
		if err := rows.Scan(&r.Campo, &r.Motivo, &r.Propuesta, &r.Cita); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// HasPendingFirma indica si la fecha de firma del convenio está por confirmar.
func (s *Store) HasPendingFirma(id int64) bool {
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM revision_fechas WHERE convenio_id=? AND campo=?
		AND (confirmada IS NULL OR confirmada <> valor)`, id, fechas.Firma).Scan(&n)
	return n > 0
}

// ConfirmDate registra que el usuario revisó la fecha y es correcta. La marca
// vuelve si la fecha cambia después.
func (s *Store) ConfirmDate(id int64, campo string) error {
	if campo != fechas.Firma && campo != fechas.Vigencia {
		return errors.New("campo de fecha inválido")
	}
	_, err := s.db.Exec("UPDATE revision_fechas SET confirmada=valor WHERE convenio_id=? AND campo=?", id, campo)
	return err
}

// attachDateReviews marca en los resúmenes qué fechas están por confirmar.
func (s *Store) attachDateReviews(items []ConventionSummary) error {
	if len(items) == 0 {
		return nil
	}
	index := make(map[int64]int, len(items))
	args := make([]interface{}, 0, len(items))
	for i, c := range items {
		index[c.ID] = i
		args = append(args, c.ID)
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(items)), ",")
	rows, err := s.db.Query(`SELECT convenio_id, campo FROM revision_fechas
		WHERE (confirmada IS NULL OR confirmada <> valor) AND convenio_id IN (`+ph+`) ORDER BY campo`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var campo string
		if err := rows.Scan(&id, &campo); err != nil {
			return err
		}
		if i, ok := index[id]; ok {
			items[i].FechasPorConfirmar = append(items[i].FechasPorConfirmar, campo)
		}
	}
	return rows.Err()
}
