package store

import (
	"errors"
	"strings"
)

// maxBulkArchive limita cuántos convenios se archivan o restauran de una vez.
const maxBulkArchive = 500

// SetArchived archiva (archived=true) o devuelve a la lista principal
// (archived=false) los convenios indicados. Archivar no borra nada: solo los
// saca de la lista principal y los deja en la sección de archivados, con su
// propio buscador. Devuelve cuántos cambiaron.
func (s *Store) SetArchived(ids []int64, archived bool) (int, error) {
	uniq := make([]interface{}, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	if len(uniq) == 0 {
		return 0, errors.New("no hay convenios seleccionados")
	}
	if len(uniq) > maxBulkArchive {
		return 0, errors.New("se pueden archivar hasta 500 convenios a la vez")
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(uniq)), ",")
	q := "UPDATE convenios SET archivado_en=datetime('now','localtime'), actualizado_en=datetime('now') WHERE archivado_en IS NULL AND id IN (" + ph + ")"
	if !archived {
		q = "UPDATE convenios SET archivado_en=NULL, actualizado_en=datetime('now') WHERE archivado_en IS NOT NULL AND id IN (" + ph + ")"
	}
	res, err := s.db.Exec(q, uniq...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
