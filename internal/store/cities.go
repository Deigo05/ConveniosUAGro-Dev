package store

import (
	"convenios-uagro-desktop/internal/municipios"
)

// officialCity devuelve el nombre oficial del municipio («Acapulco» →
// «Acapulco de Juárez») y, si estado viene vacío, su estado. Si el municipio
// no está en el catálogo de nombres, devuelve lo mismo que recibió.
func officialCity(nombre, estado string) (string, string) {
	oficial, est, ok := municipios.Oficial(nombre, estado)
	if !ok {
		return nombre, estado
	}
	if estado == "" {
		estado = est
	}
	return oficial, estado
}

// UnifyCityNames deja con su nombre oficial las etiquetas de ciudad escritas
// con el nombre corto del municipio. Si ya existe la etiqueta oficial, los
// convenios de la corta pasan a ella y la corta se elimina; si no, la corta se
// renombra. Es idempotente y se ejecuta en cada arranque. Devuelve cuántas
// etiquetas cambió.
func (s *Store) UnifyCityNames() (int, error) {
	tags, err := s.queryTags("SELECT " + tagColumns + " FROM etiquetas e WHERE e.tipo='ciudad' ORDER BY e.id")
	if err != nil {
		return 0, err
	}
	gone := map[int64]bool{}
	changed := 0
	var firstErr error
	for i := range tags {
		t := tags[i]
		if gone[t.ID] {
			continue
		}
		oficial, estado, ok := municipios.Oficial(t.Nombre, t.Estado)
		if !ok || (t.Nombre == oficial && t.Estado == estado) {
			continue
		}
		// Otra etiqueta con el nombre oficial (o una variante sin acentos)
		// del mismo estado o sin estado.
		var target *Tag
		for j := range tags {
			o := &tags[j]
			if o.ID == t.ID || gone[o.ID] || NormalizeKey(o.Nombre) != NormalizeKey(oficial) {
				continue
			}
			if o.Estado == "" || NormalizeKey(o.Estado) == NormalizeKey(estado) {
				target = o
				break
			}
		}
		if err := s.unifyCity(t, target, oficial, estado); err != nil {
			// Una etiqueta que no se pudo unificar no detiene las demás.
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if target != nil {
			gone[t.ID] = true
			target.Nombre, target.Estado = oficial, estado
		}
		changed++
	}
	return changed, firstErr
}

func (s *Store) unifyCity(t Tag, target *Tag, oficial, estado string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if target == nil {
		if _, err := tx.Exec("UPDATE etiquetas SET nombre=?, estado=? WHERE id=?", oficial, estado, t.ID); err != nil {
			return err
		}
		return tx.Commit()
	}
	for _, stmt := range []struct {
		q    string
		args []interface{}
	}{
		{`INSERT OR IGNORE INTO convenio_etiquetas (convenio_id, etiqueta_id)
		  SELECT convenio_id, ? FROM convenio_etiquetas WHERE etiqueta_id=?`, []interface{}{target.ID, t.ID}},
		{"DELETE FROM convenio_etiquetas WHERE etiqueta_id=?", []interface{}{t.ID}},
		{"DELETE FROM etiquetas WHERE id=?", []interface{}{t.ID}},
		{"UPDATE etiquetas SET nombre=?, estado=? WHERE id=?", []interface{}{oficial, estado, target.ID}},
	} {
		if _, err := tx.Exec(stmt.q, stmt.args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}
