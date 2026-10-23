package store

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var wordRe = regexp.MustCompile(`[\p{L}\p{N}]+`)

// PrepareAsk recupera fragmentos relevantes y arma el contexto para el asistente.
func (s *Store) PrepareAsk(question string, limit int) (string, []AskSource, error) {
	if limit < 1 || limit > 12 {
		limit = 8
	}
	terms := questionTerms(question)
	ids, err := s.askCandidates(terms, limit)
	if err != nil {
		return "", nil, err
	}
	var fuentes []AskSource
	var ctx strings.Builder
	for _, id := range ids {
		c, err := s.Get(id)
		if err != nil {
			continue
		}
		pages, err := s.askPages(id)
		if err != nil {
			return "", nil, err
		}
		if len(pages) == 0 {
			texto := strings.TrimSpace(c.Texto)
			if texto == "" {
				continue
			}
			pages = []pageText{{Pagina: 0, Texto: texto, Tipo: "pdf"}}
		}
		sort.SliceStable(pages, func(i, j int) bool {
			return pageScore(pages[i].Texto, terms) > pageScore(pages[j].Texto, terms)
		})
		used := 0
		header := fmt.Sprintf("[Convenio #%d | %s | Firma: %s | Vigencia: %s | Responsable: %s]",
			c.ID, fallback(c.Dependencia, "Sin contraparte"), emptyDash(c.FechaFirma),
			vigenciaTexto(c), fallback(c.Responsable, "Sin responsable"))
		for _, p := range pages {
			if used >= 2 || strings.TrimSpace(p.Texto) == "" {
				break
			}
			frag := pageFragment(p.Texto, terms, 1400)
			if frag == "" {
				continue
			}
			ctx.WriteString(header)
			ctx.WriteString(fmt.Sprintf(" [Página %d | %s]\n%s\n\n", p.Pagina, p.Tipo, frag))
			fuentes = append(fuentes, AskSource{
				ConvenioID: id, Dependencia: c.Dependencia, Pagina: p.Pagina,
				Fragmento: frag, Tipo: p.Tipo,
			})
			used++
		}
	}
	if len(fuentes) == 0 {
		return "", nil, nil
	}
	return ctx.String(), fuentes, nil
}

type pageText struct {
	Pagina int
	Texto  string
	Tipo   string
}

func (s *Store) askCandidates(terms []string, limit int) ([]int64, error) {
	if len(terms) > 0 {
		q := make([]string, 0, min(len(terms), 8))
		for _, t := range terms {
			if len(q) >= 8 {
				break
			}
			q = append(q, `"`+strings.ReplaceAll(t, `"`, "")+`"*`)
		}
		rows, err := s.db.Query(`SELECT c.id FROM convenios_fts
			JOIN convenios c ON c.id=convenios_fts.rowid
			WHERE convenios_fts MATCH ? ORDER BY bm25(convenios_fts) LIMIT ?`,
			strings.Join(q, " AND "), limit)
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
		if len(ids) > 0 {
			return ids, rows.Err()
		}
	}
	rows, err := s.db.Query("SELECT id FROM convenios ORDER BY id DESC LIMIT ?", limit)
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

func (s *Store) askPages(id int64) ([]pageText, error) {
	byPage := map[int]pageText{}
	rows, err := s.db.Query("SELECT pagina, texto FROM pdf_paginas WHERE convenio_id=? ORDER BY pagina", id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p int
		var t string
		if err := rows.Scan(&p, &t); err != nil {
			rows.Close()
			return nil, err
		}
		byPage[p] = pageText{Pagina: p, Texto: t, Tipo: "pdf"}
	}
	rows.Close()
	rows, err = s.db.Query("SELECT pagina, texto FROM transcripcion_paginas WHERE convenio_id=? ORDER BY pagina", id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p int
		var t string
		if err := rows.Scan(&p, &t); err != nil {
			rows.Close()
			return nil, err
		}
		if strings.TrimSpace(t) != "" {
			byPage[p] = pageText{Pagina: p, Texto: t, Tipo: "transcripción"}
		}
	}
	rows.Close()
	out := make([]pageText, 0, len(byPage))
	for _, p := range byPage {
		out = append(out, p)
	}
	return out, nil
}

func questionTerms(q string) []string {
	stop := map[string]bool{"que": true, "cual": true, "cuales": true, "como": true, "cuando": true, "donde": true, "quien": true, "quienes": true, "los": true, "las": true, "del": true, "con": true, "por": true, "para": true, "una": true, "unos": true, "unas": true, "este": true, "esta": true, "estos": true, "estas": true, "hay": true, "tiene": true, "tienen": true, "sobre": true, "desde": true, "hasta": true, "entre": true, "mas": true, "pero": true, "sus": true, "mis": true}
	var out []string
	seen := map[string]bool{}
	for _, w := range wordRe.FindAllString(q, -1) {
		k := NormalizeKey(w)
		if len([]rune(k)) < 3 || stop[k] || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
		if len(out) >= 10 {
			break
		}
	}
	return out
}

func pageScore(text string, terms []string) int {
	n := NormalizeKey(text)
	score := 0
	for _, t := range terms {
		score += strings.Count(n, t) * 2
	}
	return score
}

func pageFragment(text string, terms []string, max int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	start := 0
	low := strings.ToLower(text)
	for _, t := range terms {
		if i := strings.Index(low, t); i >= 0 {
			start = i - max/4
			if start < 0 {
				start = 0
			}
			break
		}
	}
	// Convertir posición de bytes a runas de forma conservadora.
	runeStart := len([]rune(text[:start]))
	if runeStart+max > len(runes) {
		runeStart = len(runes) - max
	}
	if runeStart < 0 {
		runeStart = 0
	}
	return strings.TrimSpace(string(runes[runeStart : runeStart+max]))
}

func fallback(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func vigenciaTexto(c ConventionDetail) string {
	if c.VigenciaIndefinida {
		return "indefinida"
	}
	if c.FechaVigencia == "" {
		return "sin fecha"
	}
	return c.FechaVigencia
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
