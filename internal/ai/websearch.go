package ai

import (
	"context"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	reResult  = regexp.MustCompile(`(?is)<a[^>]*class="result__a"[^>]*>(.*?)</a>`)
	reSnippet = regexp.MustCompile(`(?is)<a[^>]*class="result__snippet"[^>]*>(.*?)</a>`)
	reTags    = regexp.MustCompile(`(?is)<[^>]+>`)
	reDoubt   = regexp.MustCompile(`\[\?\]|\[ilegible\]`)
)

// DuckDuckGo responde con un reto anti-robots si recibe muchas búsquedas
// seguidas. Por eso las búsquedas van de una en una, con una pausa mínima, se
// guardan en caché y, si aparece el reto, se dejan de intentar un rato.
const (
	searchGap     = 2500 * time.Millisecond
	searchBackoff = 15 * time.Minute
)

var search struct {
	sync.Mutex
	last         time.Time
	blockedUntil time.Time
	cache        map[string]string
}

// searchWeb consulta DuckDuckGo HTML y devuelve títulos y fragmentos. No requiere clave.
func (c *Client) searchWeb(ctx context.Context, query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	search.Lock()
	defer search.Unlock()
	if r, ok := search.cache[query]; ok {
		return r
	}
	now := time.Now()
	if now.Before(search.blockedUntil) {
		return ""
	}
	if wait := searchGap - now.Sub(search.last); wait > 0 {
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(wait):
		}
	}
	search.last = time.Now()
	body, ok := fetchSearch(ctx, query)
	if !ok {
		return ""
	}
	if strings.Contains(body, "anomaly-modal") || strings.Contains(body, "bots use DuckDuckGo") {
		log.Printf("la búsqueda web pidió verificación anti-robots; se pausa %s", searchBackoff)
		search.blockedUntil = time.Now().Add(searchBackoff)
		return ""
	}
	results := parseResults(body)
	if search.cache == nil {
		search.cache = map[string]string{}
	}
	search.cache[query] = results
	return results
}

func fetchSearch(ctx context.Context, query string) (string, bool) {
	u := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) ConveniosUAGro/1.0")
	req.Header.Set("Accept-Language", "es-MX,es;q=0.9")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", false
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return string(data), true
}

func parseResults(body string) string {
	titles := reResult.FindAllStringSubmatch(body, 5)
	snippets := reSnippet.FindAllStringSubmatch(body, 5)
	var b strings.Builder
	for i := 0; i < len(titles) && i < 5; i++ {
		title := cleanHTML(titles[i][1])
		snippet := ""
		if i < len(snippets) {
			snippet = cleanHTML(snippets[i][1])
		}
		if title == "" && snippet == "" {
			continue
		}
		b.WriteString("- " + title)
		if snippet != "" {
			b.WriteString(": " + snippet)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// locationCatalog envía solo ámbitos, estados y ciudades: los temas no hacen
// falta para ubicar a la contraparte y ahorran tokens.
func locationCatalog(tags []Tag) string {
	var loc []Tag
	for _, t := range tags {
		if t.Tipo == "ambito" || t.Tipo == "estado" || t.Tipo == "ciudad" {
			loc = append(loc, t)
		}
	}
	return catalogText(loc)
}

func cleanHTML(s string) string {
	s = reTags.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

// locationQuery arma la búsqueda con el nombre (las siglas si el nombre es
// muy largo) y el domicilio del documento: "C.B.T.i.s. No. 86 Huauchinango".
func locationQuery(ext *Extraction) string {
	name := strings.TrimSpace(strings.Split(ext.Dependencia, ";")[0])
	if short := strings.TrimSpace(ext.DependenciaCorta); short != "" && len([]rune(name)) > 70 {
		name = short
	}
	if name == "" {
		return ""
	}
	dom := strings.Join(strings.Fields(reDoubt.ReplaceAllString(ext.Domicilio, " ")), " ")
	if dom != "" {
		return truncate(name+" "+dom, 220)
	}
	return name + " dirección ciudad estado México"
}

// enrichLocation busca en la web si falta la ciudad o el estado, si la ciudad
// sería nueva o si el domicilio se leyó con dudas, y pide al modelo que
// confirme la ubicación con los resultados. Nunca inventa si no hay evidencia.
func (c *Client) enrichLocation(ctx context.Context, ext *Extraction, catalogo []Tag) {
	if !ext.EsConvenio {
		return
	}
	hasState, hasCity, uncertainCity := false, false, false
	for _, s := range ext.Sugerencias {
		switch s.Tipo {
		case "estado":
			hasState = true
		case "ciudad":
			hasCity = true
			if s.Nueva {
				uncertainCity = true
			}
		}
	}
	if hasState && hasCity && !uncertainCity && !reDoubt.MatchString(ext.Domicilio) {
		return
	}
	query := locationQuery(ext)
	if query == "" {
		return
	}
	results := c.searchWeb(ctx, query)
	if results == "" {
		return
	}
	domicilio := ext.Domicilio
	if domicilio == "" {
		domicilio = "(no consta)"
	}
	prompt := fmt.Sprintf(`Contraparte: %s
Domicilio en el documento (puede traer errores de lectura): %s
Resultados web:
%s

Devuelve {"etiquetas":[{"tipo":"ambito"|"estado"|"ciudad","nombre":s}]} con la ubicación de la contraparte (nunca la de la UAGro): máximo una de cada tipo, nombres exactos del catálogo. La ciudad es el municipio con su nombre oficial y acentos, debe pertenecer al estado y puede ser nueva. Si los resultados y el domicilio no la confirman, devuelve {"etiquetas":[]}.
%s`, ext.Dependencia, domicilio, results, locationCatalog(catalogo))
	body := map[string]interface{}{
		"model":            c.Model,
		"temperature":      0.0,
		"max_tokens":       4000,
		"reasoning_effort": c.apiEffort(EffortLow),
		"response_format":  map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": "Ubicas geográficamente a la contraparte de un convenio. Responde solo JSON."},
			{"role": "user", "content": prompt},
		},
	}
	content, finish, err := c.chat(ctx, body)
	if err != nil || finish == "length" {
		return
	}
	raw, err := parseJSON(content)
	if err != nil {
		return
	}
	refined := validateSuggestions(raw["etiquetas"], catalogo)
	if len(refined) == 0 {
		return
	}
	// Conserva etiquetas no ubicacionales y reemplaza las de ubicación por las refinadas.
	var merged []Suggestion
	for _, s := range ext.Sugerencias {
		if s.Tipo != "ambito" && s.Tipo != "estado" && s.Tipo != "ciudad" {
			merged = append(merged, s)
		}
	}
	seen := map[string]bool{}
	for _, s := range refined {
		if s.Tipo != "ambito" && s.Tipo != "estado" && s.Tipo != "ciudad" {
			continue
		}
		k := s.Tipo + "|" + strings.ToLower(s.Nombre)
		if seen[k] {
			continue
		}
		seen[k] = true
		merged = append(merged, s)
	}
	// Si la web solo confirmó parte de la ubicación, conserva lo demás.
	for _, s := range ext.Sugerencias {
		if (s.Tipo == "ambito" || s.Tipo == "estado") && !hasType(merged, s.Tipo) {
			merged = append(merged, s)
		}
	}
	ext.Sugerencias = merged
}

func hasType(list []Suggestion, tipo string) bool {
	for _, s := range list {
		if s.Tipo == tipo {
			return true
		}
	}
	return false
}
