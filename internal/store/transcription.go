package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// BeginTranscription reinicia el trabajo de transcripción para un convenio.
func (s *Store) BeginTranscription(id int64, total int) error {
	if total < 0 {
		return errors.New("el número de páginas no es válido")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM transcripcion_paginas WHERE convenio_id=?", id); err != nil {
		return err
	}
	estado := "en_proceso"
	if total == 0 {
		estado = "completa"
	}
	res, err := tx.Exec(`
		UPDATE convenios SET transcripcion_estado=?, transcripcion_error='',
			paginas_por_transcribir=?, paginas_transcritas=0, texto=texto_pdf,
			actualizado_en=datetime('now')
		WHERE id=?`, estado, total, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("el convenio no existe")
	}
	return tx.Commit()
}

// SaveTranscriptionPage guarda una página y reconstruye el texto buscable.
func (s *Store) SaveTranscriptionPage(id int64, page int, texto string) (TranscriptionStatus, error) {
	var status TranscriptionStatus
	if page < 1 {
		return status, errors.New("número de página inválido")
	}
	if strings.TrimSpace(texto) == "" {
		return status, errors.New("la transcripción de la página está vacía")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return status, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		INSERT INTO transcripcion_paginas (convenio_id, pagina, texto) VALUES (?, ?, ?)
		ON CONFLICT(convenio_id, pagina) DO UPDATE SET texto=excluded.texto`,
		id, page, texto); err != nil {
		return status, err
	}
	var por int
	if err := tx.QueryRow("SELECT paginas_por_transcribir FROM convenios WHERE id=?", id).Scan(&por); err != nil {
		return status, err
	}
	var hechas int
	if err := tx.QueryRow("SELECT count(*) FROM transcripcion_paginas WHERE convenio_id=?", id).Scan(&hechas); err != nil {
		return status, err
	}
	estado := "en_proceso"
	if por > 0 && hechas >= por {
		estado = "completa"
	}
	if err := rebuildText(tx, id); err != nil {
		return status, err
	}
	if _, err := tx.Exec(`
		UPDATE convenios SET paginas_transcritas=?, transcripcion_estado=?,
			transcripcion_error='', actualizado_en=datetime('now')
		WHERE id=?`, hechas, estado, id); err != nil {
		return status, err
	}
	if err := tx.Commit(); err != nil {
		return status, err
	}
	return s.GetTranscription(id)
}

// FailTranscription registra el error sin perder las páginas ya transcritas.
func (s *Store) FailTranscription(id int64, mensaje string) error {
	_, err := s.db.Exec(`
		UPDATE convenios SET transcripcion_estado='error', transcripcion_error=?,
			actualizado_en=datetime('now') WHERE id=?`, strings.TrimSpace(mensaje), id)
	return err
}

// FinishTranscription cierra el trabajo aunque no se haya alcanzado el total.
func (s *Store) FinishTranscription(id int64) (TranscriptionStatus, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return TranscriptionStatus{}, err
	}
	defer tx.Rollback()
	var hechas int
	if err := tx.QueryRow("SELECT count(*) FROM transcripcion_paginas WHERE convenio_id=?", id).Scan(&hechas); err != nil {
		return TranscriptionStatus{}, err
	}
	if err := rebuildText(tx, id); err != nil {
		return TranscriptionStatus{}, err
	}
	if _, err := tx.Exec(`
		UPDATE convenios SET paginas_transcritas=?, transcripcion_estado='completa',
			transcripcion_error='', actualizado_en=datetime('now') WHERE id=?`, hechas, id); err != nil {
		return TranscriptionStatus{}, err
	}
	if err := tx.Commit(); err != nil {
		return TranscriptionStatus{}, err
	}
	return s.GetTranscription(id)
}

// GetTranscription devuelve el estado y el texto del convenio.
func (s *Store) GetTranscription(id int64) (TranscriptionStatus, error) {
	var st TranscriptionStatus
	err := s.db.QueryRow(`
		SELECT transcripcion_estado, transcripcion_error, paginas_por_transcribir,
		       paginas_transcritas, texto
		FROM convenios WHERE id=?`, id).Scan(
		&st.Estado, &st.Error, &st.PaginasPorTranscribir, &st.PaginasTranscritas, &st.Texto)
	if errors.Is(err, sql.ErrNoRows) {
		return st, errors.New("el convenio no existe")
	}
	return st, err
}

func rebuildText(tx *sql.Tx, id int64) error {
	var base string
	if err := tx.QueryRow("SELECT texto_pdf FROM convenios WHERE id=?", id).Scan(&base); err != nil {
		return err
	}
	rows, err := tx.Query("SELECT pagina, texto FROM transcripcion_paginas WHERE convenio_id=? ORDER BY pagina", id)
	if err != nil {
		return err
	}
	defer rows.Close()
	parts := []string{}
	if strings.TrimSpace(base) != "" {
		parts = append(parts, strings.TrimSpace(base))
	}
	for rows.Next() {
		var page int
		var texto string
		if err := rows.Scan(&page, &texto); err != nil {
			return err
		}
		parts = append(parts, fmt.Sprintf("[Página %d]\n%s", page, strings.TrimSpace(texto)))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	texto := strings.Join(parts, "\n\n")
	_, err = tx.Exec("UPDATE convenios SET texto=?, huella=NULL WHERE id=?", texto, id)
	return err
}
