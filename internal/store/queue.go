package store

// cola_procesamiento guarda los convenios que esperan procesarse con IA. Es el
// trabajo pendiente de esta instalación: sobrevive a cerrar la aplicación y no
// viaja en los paquetes exportados.

// EnqueueForAI agrega convenios al final de la cola. Un convenio que ya está
// en la cola no se duplica; reanalisis marca el trabajo pedido a mano sobre un
// convenio ya analizado, que la limpieza al arrancar no debe borrar.
func (s *Store) EnqueueForAI(ids []int64, reanalisis bool) error {
	v := 0
	if reanalisis {
		v = 1
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.Exec(`INSERT INTO cola_procesamiento (convenio_id, reanalisis)
			SELECT id, ? FROM convenios WHERE id=?
			ON CONFLICT(convenio_id) DO UPDATE SET reanalisis=max(reanalisis, excluded.reanalisis)`, v, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Dequeue quita un convenio de la cola (terminó o se abandonó).
func (s *Store) Dequeue(id int64) error {
	_, err := s.db.Exec("DELETE FROM cola_procesamiento WHERE convenio_id=?", id)
	return err
}

// QueuedIDs devuelve los convenios en cola, en el orden en que se encolaron.
func (s *Store) QueuedIDs() ([]int64, error) {
	rows, err := s.db.Query("SELECT convenio_id FROM cola_procesamiento ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// QueueCount es cuántos convenios esperan en la cola.
func (s *Store) QueueCount() (int, error) {
	var n int
	err := s.db.QueryRow("SELECT count(*) FROM cola_procesamiento").Scan(&n)
	return n, err
}

// PruneQueue quita de la cola los convenios que ya se resolvieron (analizados
// antes de que la aplicación se cerrara) y los archivados, además de las filas
// huérfanas. No borra un reanálisis pedido a mano, una transcripción a medias
// ni un convenio con error: eso es trabajo que el usuario quiere terminar.
func (s *Store) PruneQueue() error {
	_, err := s.db.Exec(`DELETE FROM cola_procesamiento WHERE
		convenio_id NOT IN (SELECT id FROM convenios)
		OR convenio_id IN (SELECT id FROM convenios WHERE archivado_en IS NOT NULL)
		OR (reanalisis=0 AND convenio_id IN (
			SELECT id FROM convenios
			WHERE ia_estado IN ('ok','parcial') AND transcripcion_estado NOT IN ('pendiente','en_proceso')))`)
	return err
}

// ClearQueue vacía la cola sin tocar los convenios.
func (s *Store) ClearQueue() error {
	_, err := s.db.Exec("DELETE FROM cola_procesamiento")
	return err
}
