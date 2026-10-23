package store

import (
	"sort"
	"strings"
	"time"
)

// allSummaries carga todos los convenios con sus etiquetas y riesgo.
// Es una app local; el volumen no justifica una consulta más compleja.
func (s *Store) allSummaries() ([]ConventionSummary, error) {
	rows, err := s.db.Query(`
		SELECT c.id, c.dependencia, coalesce(c.responsable,''), coalesce(c.fecha_firma,''),
		       coalesce(c.fecha_vigencia,''), c.vigencia_indefinida, c.vigencia_calculada,
		       c.archivo_nombre_original, c.paginas, c.es_escaneado, c.ia_estado,
		       c.ia_error, c.etiquetas_revisadas, c.transcripcion_estado,
		       c.transcripcion_error, c.paginas_por_transcribir, c.paginas_transcritas,
		       c.creado_en, c.actualizado_en
		FROM convenios c ORDER BY c.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ConventionSummary
	var ids []int64
	for rows.Next() {
		var c ConventionSummary
		var indef, calc, scanned, revisadas int
		if err := rows.Scan(&c.ID, &c.Dependencia, &c.Responsable, &c.FechaFirma, &c.FechaVigencia,
			&indef, &calc, &c.ArchivoNombreOriginal, &c.Paginas, &scanned,
			&c.IAEstado, &c.IAError, &revisadas, &c.TranscripcionEstado,
			&c.TranscripcionError, &c.PaginasPorTranscribir, &c.PaginasTranscritas,
			&c.CreadoEn, &c.ActualizadoEn); err != nil {
			return nil, err
		}
		c.VigenciaIndefinida = indef != 0
		c.VigenciaCalculada = calc != 0
		c.EsEscaneado = scanned != 0
		c.EtiquetasRevisadas = revisadas != 0
		c.Estado = EstadoConvenio(c.FechaVigencia, c.VigenciaIndefinida)
		items = append(items, c)
		ids = append(ids, c.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	tags, err := s.TagsForIDs(ids)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Etiquetas = tags[items[i].ID]
		items[i].Riesgo = EvaluateRisk(items[i])
	}
	return items, nil
}

// Radar devuelve el tablero de riesgo y las contrapartes repetidas.
func (s *Store) Radar() (RadarResult, error) {
	items, err := s.allSummaries()
	if err != nil {
		return RadarResult{}, err
	}
	var out RadarResult
	out.Total = len(items)
	out.Items = items
	for _, c := range items {
		switch c.Riesgo.Nivel {
		case "alto":
			out.Alto++
		case "medio":
			out.Medio++
		default:
			out.Bajo++
		}
		if strings.TrimSpace(c.Responsable) == "" {
			out.SinResponsable++
		}
		if c.EsEscaneado && c.TranscripcionEstado != "completa" {
			out.SinTranscribir++
		}
		if c.Estado == "vencido" {
			out.Vencidos++
		}
		if c.Estado == "por_vencer" {
			out.PorVencer++
		}
	}
	sort.SliceStable(out.Items, func(i, j int) bool {
		if out.Items[i].Riesgo.Puntaje != out.Items[j].Riesgo.Puntaje {
			return out.Items[i].Riesgo.Puntaje > out.Items[j].Riesgo.Puntaje
		}
		return out.Items[i].ID > out.Items[j].ID
	})
	if len(out.Items) > 40 {
		out.Items = out.Items[:40]
	}

	groups := map[string]*CounterpartyGroup{}
	for _, c := range items {
		key := NormalizeKey(c.Dependencia)
		if key == "" {
			continue
		}
		g := groups[key]
		if g == nil {
			g = &CounterpartyGroup{Dependencia: c.Dependencia}
			groups[key] = g
		}
		g.Total++
		g.IDs = append(g.IDs, c.ID)
		if c.Estado == "vencido" {
			g.Vencidos++
		}
	}
	for _, g := range groups {
		if g.Total > 1 {
			out.Contrapartes = append(out.Contrapartes, *g)
		}
	}
	sort.SliceStable(out.Contrapartes, func(i, j int) bool {
		if out.Contrapartes[i].Total != out.Contrapartes[j].Total {
			return out.Contrapartes[i].Total > out.Contrapartes[j].Total
		}
		return out.Contrapartes[i].Vencidos > out.Contrapartes[j].Vencidos
	})
	if len(out.Contrapartes) > 12 {
		out.Contrapartes = out.Contrapartes[:12]
	}
	return out, nil
}

// Related calcula convenios hermanos de forma determinista: contraparte,
// etiquetas compartidas, traslape de fechas y archivos idénticos.
func (s *Store) Related(id int64) ([]RelatedConvention, error) {
	target, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	items, err := s.allSummaries()
	if err != nil {
		return nil, err
	}
	targetKey := NormalizeKey(target.Dependencia)
	var out []RelatedConvention
	for _, c := range items {
		if c.ID == id {
			continue
		}
		score := 0
		motivos := []string{}
		key := NormalizeKey(c.Dependencia)
		if targetKey != "" && key == targetKey {
			score += 60
			motivos = append(motivos, "Misma contraparte")
		} else if targetKey != "" && key != "" && tokenOverlap(targetKey, key) >= 0.6 {
			score += 30
			motivos = append(motivos, "Contraparte con nombre muy parecido")
		}
		shared := map[string]bool{}
		for _, a := range target.Etiquetas {
			for _, b := range c.Etiquetas {
				if a.ID != b.ID || shared[a.Tipo+":"+NormalizeKey(a.Nombre)] {
					continue
				}
				switch a.Tipo {
				case "estado":
					score += 15
				case "ciudad":
					score += 10
				case "tema":
					score += 5
				default:
					score += 3
				}
				shared[a.Tipo+":"+NormalizeKey(a.Nombre)] = true
				if len(motivos) < 4 {
					motivos = append(motivos, "Comparte "+TipoTitulo(a.Tipo)+" «"+a.Nombre+"»")
				}
			}
		}
		if datesOverlap(target.ConventionSummary, c) {
			score += 15
			motivos = append(motivos, "Vigencias traslapadas")
		}
		if score < 20 {
			continue
		}
		out = append(out, RelatedConvention{ConventionSummary: c, Score: score, Motivos: motivos})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID > out[j].ID
	})
	if len(out) > 8 {
		out = out[:8]
	}
	return out, nil
}

// TipoTitulo devuelve el nombre visible de un tipo de etiqueta.
func TipoTitulo(tipo string) string {
	switch tipo {
	case "ambito":
		return "Ámbito"
	case "estado":
		return "Estado"
	case "ciudad":
		return "Ciudad"
	case "tema":
		return "Tema"
	default:
		return "Etiqueta"
	}
}

func tokenOverlap(a, b string) float64 {
	stop := map[string]bool{"de": true, "del": true, "la": true, "el": true, "los": true, "las": true, "y": true, "con": true, "para": true, "por": true, "en": true, "un": true, "una": true, "al": true, "a": true}
	sa := map[string]bool{}
	sb := map[string]bool{}
	for _, p := range strings.Fields(a) {
		if len([]rune(p)) > 2 && !stop[p] {
			sa[p] = true
		}
	}
	for _, p := range strings.Fields(b) {
		if len([]rune(p)) > 2 && !stop[p] {
			sb[p] = true
		}
	}
	if len(sa) == 0 || len(sb) == 0 {
		return 0
	}
	inter := 0
	for p := range sa {
		if sb[p] {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func datesOverlap(a, b ConventionSummary) bool {
	as, ae, aok := interval(a)
	bs, be, bok := interval(b)
	if !aok || !bok {
		return false
	}
	return !as.After(be) && !bs.After(ae)
}

func interval(c ConventionSummary) (time.Time, time.Time, bool) {
	start, err := time.Parse("2006-01-02", c.FechaFirma)
	if err != nil {
		start = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	end := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	if !c.VigenciaIndefinida && c.FechaVigencia != "" {
		t, err := time.Parse("2006-01-02", c.FechaVigencia)
		if err != nil {
			return time.Time{}, time.Time{}, false
		}
		end = t
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, false
	}
	return start, end, true
}
