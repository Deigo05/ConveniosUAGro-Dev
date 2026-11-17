package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// MsgAnalisisInterrumpido es el error que queda en un análisis que se cortó
// porque la aplicación se cerró.
const MsgAnalisisInterrumpido = "El análisis se interrumpió al cerrar la aplicación; se reintentará."

// MaxRetryRounds es cuántas veces, como máximo, se reintenta un convenio con
// error al abrir la aplicación. Evita gastar sin fin en un documento que
// siempre falla; la cuenta se reinicia cuando el análisis sale bien.
const MaxRetryRounds = 3

// permanentErrors son los errores que no se arreglan reintentando: falta
// configurar la IA o el PDF no tiene texto que analizar.
var permanentErrors = []string{
	"no tiene texto seleccionable",
	"la clave de ia es inválida",
	"no hay una clave de ia",
	"configure una clave",
	"configure y active",
	"active el análisis",
	"active la ia",
	"activa la ia",
	"el servicio rechazó la solicitud",
	"no se pudo cargar el motor de pdf",
}

// RetryableError indica si un error de IA o de transcripción puede
// desaparecer al reintentar (servicio saturado, tiempo agotado, red, una
// respuesta mal formada o cortada).
func RetryableError(msg string) bool {
	m := strings.ToLower(strings.TrimSpace(msg))
	for _, p := range permanentErrors {
		if strings.Contains(m, p) {
			return false
		}
	}
	return true
}

func retryKey(id int64) string { return fmt.Sprintf("reintento:%d", id) }

// ClearRetries reinicia la cuenta de reintentos automáticos de un convenio.
func (s *Store) ClearRetries(id int64) error {
	_, err := s.db.Exec("DELETE FROM meta WHERE clave=?", retryKey(id))
	return err
}

func (s *Store) retryRounds(id int64) (int, error) {
	var v string
	err := s.db.QueryRow("SELECT valor FROM meta WHERE clave=?", retryKey(id)).Scan(&v)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	var n int
	fmt.Sscanf(v, "%d", &n)
	return n, nil
}

// RetryCandidates devuelve los convenios que quedaron con «IA con error» o
// con la transcripción incompleta (con error o interrumpida a la mitad) por
// un fallo que puede resolverse reintentando, y suma una ronda a cada uno.
// Los que ya agotaron MaxRetryRounds no se devuelven ni los archivados.
func (s *Store) RetryCandidates() ([]int64, error) {
	rows, err := s.db.Query(`SELECT id, ia_estado, ia_error, transcripcion_estado, transcripcion_error
		FROM convenios
		WHERE archivado_en IS NULL AND (ia_estado = 'error' OR transcripcion_estado = 'error'
		   OR (transcripcion_estado = 'pendiente' AND paginas_transcritas > 0))
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		var ia, iaErr, trans, transErr string
		if err := rows.Scan(&id, &ia, &iaErr, &trans, &transErr); err != nil {
			rows.Close()
			return nil, err
		}
		if trans == "error" && !RetryableError(transErr) {
			continue
		}
		if ia == "error" && trans != "error" && !RetryableError(iaErr) {
			continue
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		n, err := s.retryRounds(id)
		if err != nil {
			return nil, err
		}
		if n >= MaxRetryRounds {
			continue
		}
		if _, err := s.db.Exec("INSERT OR REPLACE INTO meta (clave, valor) VALUES (?, ?)", retryKey(id), fmt.Sprint(n+1)); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// ResumeTranscription retoma una transcripción incompleta sin borrar las
// páginas ya transcritas y devuelve cuáles son, para enviar solo las que
// faltan.
func (s *Store) ResumeTranscription(id int64, total int) ([]int, error) {
	if total < 0 {
		return nil, errors.New("el número de páginas no es válido")
	}
	rows, err := s.db.Query("SELECT pagina FROM transcripcion_paginas WHERE convenio_id=? ORDER BY pagina", id)
	if err != nil {
		return nil, err
	}
	done := []int{}
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return nil, err
		}
		done = append(done, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	res, err := s.db.Exec(`UPDATE convenios SET transcripcion_estado='en_proceso', transcripcion_error='',
		paginas_por_transcribir=?, paginas_transcritas=?, actualizado_en=datetime('now') WHERE id=?`,
		total, len(done), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, errors.New("el convenio no existe")
	}
	return done, nil
}
