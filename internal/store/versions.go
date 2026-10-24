package store

import (
	"database/sql"
	"errors"
	"sort"
)

// Un convenio renovado deja la versión anterior en su historial: la anterior
// guarda en reemplazado_por el id de la que la sustituye. La versión vigente
// (reemplazado_por NULL) es la que aparece en el listado; al borrarla, la
// anterior vuelve a quedar vigente (ON DELETE SET NULL).

// maxVersionDepth limita los recorridos del historial por si hubiera un ciclo.
const maxVersionDepth = 1000

// familyHead sigue reemplazado_por hasta la versión vigente.
func familyHead(id int64, replaced map[int64]int64) int64 {
	for i := 0; i < maxVersionDepth; i++ {
		next := replaced[id]
		if next == 0 {
			return id
		}
		id = next
	}
	return id
}

func (s *Store) replacedBy(id int64) (int64, error) {
	var next sql.NullInt64
	err := s.db.QueryRow("SELECT reemplazado_por FROM convenios WHERE id=?", id).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errors.New("el convenio no existe")
	}
	return next.Int64, err
}

func (s *Store) head(id int64) (int64, error) {
	for i := 0; i < maxVersionDepth; i++ {
		next, err := s.replacedBy(id)
		if err != nil || next == 0 {
			return id, err
		}
		id = next
	}
	return id, nil
}

// LinkVersion registra que newer renueva a older: older pasa al historial de
// newer junto con sus propias versiones anteriores.
func (s *Store) LinkVersion(older, newer int64) error {
	if older == newer {
		return errors.New("un convenio no puede ser versión de sí mismo")
	}
	if _, err := s.replacedBy(older); err != nil {
		return err
	}
	// Si older ya es posterior a newer en su historial, se formaría un ciclo.
	cur := newer
	for i := 0; cur != 0 && i < maxVersionDepth; i++ {
		if cur == older {
			return errors.New("ese convenio ya es una versión más reciente de este")
		}
		next, err := s.replacedBy(cur)
		if err != nil {
			return err
		}
		cur = next
	}
	p := orderedPair(older, newer)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE convenios SET reemplazado_por=?, actualizado_en=datetime('now') WHERE id=?", newer, older); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM convenio_duplicados WHERE convenio_a=? AND convenio_b=?", p.a, p.b); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.RefreshDuplicates()
}

// UnlinkVersion saca un convenio del historial y lo vuelve a mostrar como
// convenio vigente. Sus propias versiones anteriores lo siguen.
func (s *Store) UnlinkVersion(id int64) error {
	if _, err := s.db.Exec("UPDATE convenios SET reemplazado_por=NULL, actualizado_en=datetime('now') WHERE id=?", id); err != nil {
		return err
	}
	return s.RefreshDuplicates()
}

// versionCounts devuelve, por versión vigente, cuántas versiones anteriores tiene.
func (s *Store) versionCounts() (map[int64]int, error) {
	rows, err := s.db.Query("SELECT id, reemplazado_por FROM convenios WHERE reemplazado_por IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	replaced := map[int64]int64{}
	for rows.Next() {
		var id, next int64
		if err := rows.Scan(&id, &next); err != nil {
			return nil, err
		}
		replaced[id] = next
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	counts := map[int64]int{}
	for id := range replaced {
		counts[familyHead(id, replaced)]++
	}
	return counts, nil
}

// History devuelve todas las versiones del historial al que pertenece id, la
// vigente primero y luego de la más reciente a la más antigua. Devuelve una
// lista vacía si el convenio no tiene otras versiones.
func (s *Store) History(id int64) ([]VersionRef, int64, error) {
	head, err := s.head(id)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`
		WITH RECURSIVE fam(id, depth) AS (
			SELECT ?, 0
			UNION
			SELECT c.id, fam.depth + 1 FROM convenios c JOIN fam ON c.reemplazado_por = fam.id
			WHERE fam.depth < ?
		)
		SELECT c.id, coalesce(c.fecha_firma, ''), coalesce(c.fecha_vigencia, ''), c.vigencia_indefinida,
		       c.tipo_documento, coalesce(c.reemplazado_por, 0), c.dependencia, c.dependencia_corta,
		       c.archivo_nombre_original, c.facultad, c.facultad_ciudad
		FROM convenios c JOIN fam ON fam.id = c.id`, head, maxVersionDepth)
	if err != nil {
		return nil, 0, err
	}
	var refs []VersionRef
	var sums []ConventionSummary
	for rows.Next() {
		var v VersionRef
		var c ConventionSummary
		var indef int
		if err := rows.Scan(&v.ID, &v.FechaFirma, &v.FechaVigencia, &indef, &v.TipoDocumento, &v.ReemplazadoPor,
			&c.Dependencia, &c.DependenciaCorta, &c.ArchivoNombreOriginal, &c.Facultad, &c.FacultadCiudad); err != nil {
			rows.Close()
			return nil, 0, err
		}
		v.VigenciaIndefinida = indef != 0
		v.Actual = v.ID == head
		c.ID, c.FechaFirma, c.TipoDocumento = v.ID, v.FechaFirma, v.TipoDocumento
		refs = append(refs, v)
		sums = append(sums, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if len(refs) < 2 {
		return []VersionRef{}, head, nil
	}
	if err := s.fillFileNames(sums); err != nil {
		return nil, 0, err
	}
	for i := range refs {
		refs[i].ArchivoNombre = sums[i].ArchivoNombre
	}
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].Actual != refs[j].Actual {
			return refs[i].Actual
		}
		if refs[i].FechaFirma != refs[j].FechaFirma {
			return refs[i].FechaFirma > refs[j].FechaFirma
		}
		return refs[i].ID > refs[j].ID
	})
	return refs, head, nil
}
