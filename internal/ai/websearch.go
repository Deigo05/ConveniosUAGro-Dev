package ai

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	reResult  = regexp.MustCompile(`(?is)<a[^>]*class="result__a"[^>]*>(.*?)</a>`)
	reSnippet = regexp.MustCompile(`(?is)<a[^>]*class="result__snippet"[^>]*>(.*?)</a>`)
	reTags    = regexp.MustCompile(`(?is)<[^>]+>`)
)

// searchWeb consulta DuckDuckGo HTML y devuelve títulos y fragmentos. No requiere clave.
func (c *Client) searchWeb(ctx context.Context, query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	u := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) ConveniosUAGro/1.0")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	body := string(data)
	titles := reResult.FindAllStringSubmatch(body, 4)
	snippets := reSnippet.FindAllStringSubmatch(body, 4)
	var b strings.Builder
	for i := 0; i < len(titles) && i < 4; i++ {
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

func cleanHTML(s string) string {
	s = reTags.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

// enrichLocation busca en la web si faltan etiquetas de ubicación y pide al
// modelo que las refine con los resultados. Nunca inventa si no hay evidencia.
func (c *Client) enrichLocation(ctx context.Context, ext *Extraction, catalogo []Tag) {
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
	if hasState && hasCity && !uncertainCity {
		return
	}
	if strings.TrimSpace(ext.Dependencia) == "" && strings.TrimSpace(ext.Facultad) == "" {
		return
	}
	query := strings.TrimSpace(ext.Dependencia + " " + ext.Facultad + " ubicación ciudad estado México")
	results := c.searchWeb(ctx, query)
	if results == "" {
		return
	}
	prompt := fmt.Sprintf(`Datos iniciales:
Dependencia: %s
Facultad UAGro: %s
Estado sugerido: %v
Ciudad sugerida: %v

Resultados de búsqueda web:
%s

Devuelve únicamente un JSON válido con la clave "etiquetas": una lista de etiquetas de ubicación.
Usa solo etiquetas de tipo ambito, estado o ciudad. Usa los nombres exactos del catálogo.
Si los resultados no permiten confirmar una ubicación, devuelve una lista vacía. No inventes.
Catálogo:
%s`, ext.Dependencia, ext.Facultad, hasState, hasCity, results, catalogText(catalogo))
	body := map[string]interface{}{
		"model":            c.Model,
		"temperature":      0.0,
		"max_tokens":       4000,
		"reasoning_effort": "low",
		"response_format":  map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": "Eres un asistente de ubicación geográfica para convenios. Responde solo JSON."},
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
	ext.Sugerencias = merged
}
