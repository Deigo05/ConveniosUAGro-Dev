package store

import (
	"hash/fnv"
	"math/bits"
	"sort"
	"strings"
	"unicode"
)

// Motivos de un par de convenios relacionados.
const (
	MotivoArchivo    = "archivo"    // el mismo PDF (mismo SHA-256)
	MotivoTexto      = "texto"      // contenido casi igual, p. ej. otro escaneo
	MotivoDatos      = "datos"      // misma contraparte, fecha, tipo y facultad
	MotivoRenovacion = "renovacion" // misma contraparte con otra fecha de firma
)

const (
	// fingerprintWords es el mínimo de palabras para calcular la huella.
	fingerprintWords = 80
	// fingerprintMaxDist es cuántos bits pueden diferir dos huellas para
	// considerar que el texto es el mismo. Dos lecturas del mismo documento
	// quedan a pocos bits; textos distintos, a unos 32.
	fingerprintMaxDist = 8
)

// fingerprint calcula un SimHash del texto con grupos de tres palabras, que
// tolera los errores de lectura de una transcripción. Devuelve 0 si el texto
// es demasiado corto para compararlo.
func fingerprint(text string) uint64 {
	words := strings.FieldsFunc(NormalizeKey(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	kept := words[:0]
	for _, w := range words {
		if len(w) >= 3 && w != "pagina" {
			kept = append(kept, w)
		}
	}
	if len(kept) < fingerprintWords {
		return 0
	}
	var v [64]int
	h := fnv.New64a()
	for i := 0; i+2 < len(kept); i++ {
		h.Reset()
		h.Write([]byte(kept[i] + " " + kept[i+1] + " " + kept[i+2]))
		x := h.Sum64()
		for b := 0; b < 64; b++ {
			if x&(1<<b) != 0 {
				v[b]++
			} else {
				v[b]--
			}
		}
	}
	var out uint64
	for b, n := range v {
		if n > 0 {
			out |= 1 << b
		}
	}
	if out == 0 {
		out = 1
	}
	return out
}

// fillFingerprints calcula la huella de los convenios cuyo texto cambió.
func (s *Store) fillFingerprints() error {
	rows, err := s.db.Query("SELECT id, texto FROM convenios WHERE huella IS NULL")
	if err != nil {
		return err
	}
	pending := map[int64]uint64{}
	for rows.Next() {
		var id int64
		var texto string
		if err := rows.Scan(&id, &texto); err != nil {
			rows.Close()
			return err
		}
		pending[id] = fingerprint(texto)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for id, h := range pending {
		if _, err := s.db.Exec("UPDATE convenios SET huella=? WHERE id=?", int64(h), id); err != nil {
			return err
		}
	}
	return nil
}

// dupInfo son los datos de un convenio que se comparan.
type dupInfo struct {
	id, replacedBy          int64
	sha, dep, siglas, fecha string
	tipo, unidad            string
	huella                  uint64
}

func (s *Store) dupInfos() ([]dupInfo, error) {
	rows, err := s.db.Query(`SELECT c.id, coalesce(c.reemplazado_por, 0), c.archivo_sha256, c.dependencia,
		c.dependencia_corta, coalesce(c.fecha_firma, ''), c.tipo_documento, coalesce(c.huella, 0),
		c.facultad, c.facultad_ciudad,
		coalesce((SELECT e.nombre FROM convenio_etiquetas ce JOIN etiquetas e ON e.id = ce.etiqueta_id
		          WHERE ce.convenio_id = c.id AND e.tipo = 'facultad' LIMIT 1), '')
		FROM convenios c ORDER BY c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dupInfo
	for rows.Next() {
		var d dupInfo
		var dep, siglas, facultad, ciudad, tag string
		var huella int64
		if err := rows.Scan(&d.id, &d.replacedBy, &d.sha, &dep, &siglas, &d.fecha, &d.tipo, &huella,
			&facultad, &ciudad, &tag); err != nil {
			return nil, err
		}
		d.huella = uint64(huella)
		d.dep = counterpartKey(dep)
		d.siglas = compactKey(siglas)
		unidad := tag
		if unidad == "" {
			unidad = facultad + " " + ciudad
		}
		if d.unidad = compactKey(unidad); d.unidad == compactKey(TagConvenioAbierto) {
			d.unidad = ""
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// counterpartKey compara contrapartes sin acentos, signos ni mayúsculas, y
// solo con la primera cuando hay varias.
func counterpartKey(dep string) string {
	if i := strings.Index(dep, ";"); i >= 0 {
		dep = dep[:i]
	}
	return compactKey(dep)
}

func compactKey(s string) string {
	var b strings.Builder
	for _, r := range NormalizeKey(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func compatible(a, b string) bool { return a == "" || b == "" || a == b }

// classify decide si dos convenios parecen el mismo documento o una
// renovación. Devuelve "" si no se relacionan.
func classify(x, y *dupInfo) string {
	if x.sha != "" && x.sha == y.sha {
		return MotivoArchivo
	}
	similar := x.huella != 0 && y.huella != 0 && bits.OnesCount64(x.huella^y.huella) <= fingerprintMaxDist
	sameDep := (x.dep != "" && x.dep == y.dep) || (x.siglas != "" && x.siglas == y.siglas)
	if sameDep {
		// Otro tipo de documento u otra facultad con la misma contraparte es
		// otro convenio, no un duplicado.
		if !compatible(x.tipo, y.tipo) || !compatible(x.unidad, y.unidad) {
			return ""
		}
		switch {
		case x.fecha != "" && y.fecha != "" && x.fecha != y.fecha:
			return MotivoRenovacion
		case x.fecha != "" && x.fecha == y.fecha:
			return MotivoDatos
		case similar:
			return MotivoTexto
		}
		return ""
	}
	// Con contrapartes distintas, un texto parecido es solo el mismo formato
	// de convenio. Si falta la contraparte (aún sin analizar) basta el texto.
	if similar && (x.dep == "" || y.dep == "") && compatible(x.fecha, y.fecha) {
		return MotivoTexto
	}
	return ""
}

type idPair struct{ a, b int64 }

func orderedPair(x, y int64) idPair {
	if x > y {
		x, y = y, x
	}
	return idPair{x, y}
}

// RefreshDuplicates recalcula todos los pares de posibles duplicados y
// renovaciones. Es rápido (comparaciones en memoria) y se llama después de
// cada cambio que afecta los datos comparados.
func (s *Store) RefreshDuplicates() error {
	s.dupMu.Lock()
	defer s.dupMu.Unlock()
	if err := s.fillFingerprints(); err != nil {
		return err
	}
	infos, err := s.dupInfos()
	if err != nil {
		return err
	}
	dismissed, err := s.dismissedPairs()
	if err != nil {
		return err
	}
	replaced := make(map[int64]int64, len(infos))
	byID := make(map[int64]*dupInfo, len(infos))
	for i := range infos {
		replaced[infos[i].id] = infos[i].replacedBy
		byID[infos[i].id] = &infos[i]
	}
	heads := make(map[int64]int64, len(infos))
	for _, d := range infos {
		heads[d.id] = familyHead(d.id, replaced)
	}

	pairs := map[idPair]string{}
	// Para no sugerir cada combinación de una contraparte con muchos
	// convenios, cada uno se propone solo como renovación del anterior más
	// reciente.
	bestOlder := map[int64]int64{}
	for i := range infos {
		for j := i + 1; j < len(infos); j++ {
			x, y := &infos[i], &infos[j]
			if heads[x.id] == heads[y.id] || dismissed[idPair{x.id, y.id}] {
				continue
			}
			m := classify(x, y)
			if m == "" {
				continue
			}
			if m != MotivoRenovacion {
				pairs[idPair{x.id, y.id}] = m
				continue
			}
			if x.replacedBy != 0 || y.replacedBy != 0 {
				continue // solo se proponen entre versiones vigentes
			}
			older, newer := x, y
			if y.fecha < x.fecha {
				older, newer = y, x
			}
			cur, ok := bestOlder[newer.id]
			if !ok || byID[cur].fecha < older.fecha || (byID[cur].fecha == older.fecha && cur < older.id) {
				bestOlder[newer.id] = older.id
			}
		}
	}
	for newer, older := range bestOlder {
		if p := orderedPair(newer, older); pairs[p] == "" {
			pairs[p] = MotivoRenovacion
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM convenio_duplicados"); err != nil {
		return err
	}
	for p, m := range pairs {
		if _, err := tx.Exec("INSERT INTO convenio_duplicados (convenio_a, convenio_b, motivo) VALUES (?, ?, ?)", p.a, p.b, m); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) dismissedPairs() (map[idPair]bool, error) {
	rows, err := s.db.Query("SELECT convenio_a, convenio_b FROM duplicados_descartados")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[idPair]bool{}
	for rows.Next() {
		var p idPair
		if err := rows.Scan(&p.a, &p.b); err != nil {
			return nil, err
		}
		out[p] = true
	}
	return out, rows.Err()
}

// DismissDuplicate marca dos convenios como distintos para no sugerirlos otra vez.
func (s *Store) DismissDuplicate(a, b int64) error {
	p := orderedPair(a, b)
	if p.a == p.b {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT OR IGNORE INTO duplicados_descartados (convenio_a, convenio_b) VALUES (?, ?)", p.a, p.b); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM convenio_duplicados WHERE convenio_a=? AND convenio_b=?", p.a, p.b); err != nil {
		return err
	}
	return tx.Commit()
}

var motivoOrder = map[string]int{MotivoArchivo: 0, MotivoTexto: 1, MotivoDatos: 2, MotivoRenovacion: 3}

// ListDuplicates devuelve los pares pendientes de revisar, primero los más
// seguros (mismo archivo) y al final las posibles renovaciones.
func (s *Store) ListDuplicates() ([]DuplicatePair, error) {
	rows, err := s.db.Query("SELECT convenio_a, convenio_b, motivo FROM convenio_duplicados")
	if err != nil {
		return nil, err
	}
	type raw struct {
		p idPair
		m string
	}
	var list []raw
	var ids []int64
	seen := map[int64]bool{}
	for rows.Next() {
		var r raw
		if err := rows.Scan(&r.p.a, &r.p.b, &r.m); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, r)
		for _, id := range []int64{r.p.a, r.p.b} {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []DuplicatePair{}
	if len(ids) == 0 {
		return out, nil
	}
	res, err := s.list(ListFilter{ids: ids}, "")
	if err != nil {
		return nil, err
	}
	sum := make(map[int64]ConventionSummary, len(res.Items))
	for _, c := range res.Items {
		sum[c.ID] = c
	}
	for _, r := range list {
		a, okA := sum[r.p.a]
		b, okB := sum[r.p.b]
		if okA && okB {
			out = append(out, DuplicatePair{Motivo: r.m, A: a, B: b})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if motivoOrder[out[i].Motivo] != motivoOrder[out[j].Motivo] {
			return motivoOrder[out[i].Motivo] < motivoOrder[out[j].Motivo]
		}
		return out[i].B.ID > out[j].B.ID
	})
	return out, nil
}

// relatedFor lista los convenios relacionados con id que faltan por revisar.
func (s *Store) relatedFor(id int64) ([]RelatedRef, error) {
	rows, err := s.db.Query(`SELECT c.id, d.motivo, c.dependencia, c.dependencia_corta, c.tipo_documento,
		coalesce(c.fecha_firma, ''), c.archivo_nombre_original, c.facultad, c.facultad_ciudad
		FROM convenio_duplicados d
		JOIN convenios c ON c.id = CASE WHEN d.convenio_a = ? THEN d.convenio_b ELSE d.convenio_a END
		WHERE d.convenio_a = ? OR d.convenio_b = ?
		ORDER BY c.id`, id, id, id)
	if err != nil {
		return nil, err
	}
	var refs []RelatedRef
	var sums []ConventionSummary
	for rows.Next() {
		var r RelatedRef
		var c ConventionSummary
		if err := rows.Scan(&r.ID, &r.Motivo, &c.Dependencia, &c.DependenciaCorta, &c.TipoDocumento,
			&r.FechaFirma, &c.ArchivoNombreOriginal, &c.Facultad, &c.FacultadCiudad); err != nil {
			rows.Close()
			return nil, err
		}
		c.ID, c.FechaFirma = r.ID, r.FechaFirma
		r.Dependencia = c.Dependencia
		refs = append(refs, r)
		sums = append(sums, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.fillFileNames(sums); err != nil {
		return nil, err
	}
	for i := range refs {
		refs[i].ArchivoNombre = sums[i].ArchivoNombre
	}
	sort.SliceStable(refs, func(i, j int) bool { return motivoOrder[refs[i].Motivo] < motivoOrder[refs[j].Motivo] })
	return refs, nil
}

// fillFileNames calcula el nombre de descarga de resúmenes parciales.
func (s *Store) fillFileNames(items []ConventionSummary) error {
	ids := make([]int64, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	tags, err := s.TagsForIDs(ids)
	if err != nil {
		return err
	}
	for i := range items {
		items[i].Etiquetas = tags[items[i].ID]
		items[i].ArchivoNombre = FileName(items[i])
	}
	return nil
}

// attachMarks agrega a los resúmenes las marcas de duplicado, renovación y
// versiones.
func (s *Store) attachMarks(items []ConventionSummary) error {
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
	rows, err := s.db.Query("SELECT convenio_a, convenio_b, motivo FROM convenio_duplicados WHERE convenio_a IN ("+ph+") OR convenio_b IN ("+ph+")", append(args, args...)...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a, b int64
		var m string
		if err := rows.Scan(&a, &b, &m); err != nil {
			rows.Close()
			return err
		}
		for _, id := range []int64{a, b} {
			if i, ok := index[id]; ok {
				if m == MotivoRenovacion {
					items[i].PosibleRenovacion = true
				} else {
					items[i].Duplicados++
				}
			}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	counts, err := s.versionCounts()
	if err != nil {
		return err
	}
	for i := range items {
		items[i].Versiones = counts[items[i].ID]
	}
	return nil
}
