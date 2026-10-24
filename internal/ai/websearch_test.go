package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// cacheSearch deja resultados en la caché de búsqueda para no salir a internet.
func cacheSearch(t *testing.T, results map[string]string) {
	t.Helper()
	search.Lock()
	if search.cache == nil {
		search.cache = map[string]string{}
	}
	for q, r := range results {
		search.cache[q] = r
	}
	search.Unlock()
	t.Cleanup(func() {
		search.Lock()
		for q := range results {
			delete(search.cache, q)
		}
		search.Unlock()
	})
}

// locationServer responde a la confirmación de ubicación según los resultados
// web que recibe en el mensaje.
func locationServer(t *testing.T, calls *int32, reply func(prompt string) string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		prompt := req.Messages[len(req.Messages)-1].Content
		out, _ := json.Marshal(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]string{"content": reply(prompt)}, "finish_reason": "stop"}},
		})
		w.Header().Set("Content-Type", "application/json")
		w.Write(out)
	}))
	t.Cleanup(srv.Close)
	return srv
}

var locationTags = []Tag{
	{Nombre: "Nacional", Tipo: "ambito"},
	{Nombre: "Guerrero", Tipo: "estado"},
	{Nombre: "Puebla", Tipo: "estado"},
	{Nombre: "Acapulco de Juárez", Tipo: "ciudad", Estado: "Guerrero"},
	{Nombre: "Huauchinango", Tipo: "ciudad", Estado: "Puebla"},
}

func TestBrandQuery(t *testing.T) {
	ext := &Extraction{Marcas: "PREGO; Salads & Wings", LugarFirma: "Acapulco de Juárez, Guerrero"}
	if q := brandQuery(ext); q != "PREGO Salads & Wings Acapulco de Juárez, Guerrero" {
		t.Fatalf("consulta = %q", q)
	}
	if q := brandQuery(&Extraction{LugarFirma: "Acapulco"}); q != "" {
		t.Fatalf("sin marcas no se busca: %q", q)
	}
}

// Si la razón social no aparece en internet, se busca por las marcas, y la
// ficha avisa que la ubicación no salió del documento.
func TestEnrichLocationSearchesBrands(t *testing.T) {
	ext := Extraction{
		EsConvenio:  true,
		Dependencia: "GRUPO PAR DE 3 S DE RL DE CV",
		Marcas:      "PREGO; Salads & Wings",
		LugarFirma:  "Acapulco de Juárez, Guerrero",
		Sugerencias: []Suggestion{{Tipo: "ambito", Nombre: "Nacional"}},
	}
	cacheSearch(t, map[string]string{
		locationQuery(&ext): "- Directorio de Empresas en México: razón social, estado, ciudad",
		brandQuery(&ext):    "- Salads&Wings | Acapulco - Facebook: Restaurant Bar\n- PREGO, Avenida Costera Miguel Alemán s/n, Acapulco de Juárez",
	})
	var calls int32
	srv := locationServer(t, &calls, func(prompt string) string {
		if !strings.Contains(prompt, "Marcas: PREGO; Salads & Wings") || !strings.Contains(prompt, "misma entidad") {
			t.Errorf("faltan datos para confirmar la entidad:\n%s", prompt)
		}
		if strings.Contains(prompt, "Salads&Wings | Acapulco") {
			return `{"etiquetas":[{"tipo":"ambito","nombre":"Nacional"},{"tipo":"estado","nombre":"Guerrero"},{"tipo":"ciudad","nombre":"Acapulco de Juárez"}]}`
		}
		return `{"etiquetas":[]}`
	})
	c := New("k", srv.URL, "m", "v")
	c.enrichLocation(context.Background(), &ext, locationTags)
	if calls != 2 {
		t.Fatalf("confirmaciones = %d, se esperaban 2 (razón social y marcas)", calls)
	}
	estado, ciudad := locationOf(ext.Sugerencias)
	if estado != "Guerrero" || ciudad != "Acapulco de Juárez" {
		t.Fatalf("ubicación = %q, %q", estado, ciudad)
	}
	if !strings.Contains(ext.Notas, "tomada de internet") || !strings.Contains(ext.Notas, "Acapulco de Juárez, Guerrero") {
		t.Fatalf("notas = %q", ext.Notas)
	}
}

// Si la web solo corrige la ortografía de un domicilio que sí consta, no hace
// falta avisar.
func TestEnrichLocationNoNoteWhenDocumentHasAddress(t *testing.T) {
	ext := Extraction{
		EsConvenio:  true,
		Dependencia: "C.B.T.i.s. No. 86",
		Domicilio:   "Calle Reforma 10, Huachinango, Pue.",
		Sugerencias: []Suggestion{{Tipo: "ambito", Nombre: "Nacional"}, {Tipo: "estado", Nombre: "Puebla"}, {Tipo: "ciudad", Nombre: "Huachinango", Nueva: true}},
	}
	cacheSearch(t, map[string]string{locationQuery(&ext): "- CBTis 86 Huauchinango, Puebla"})
	var calls int32
	srv := locationServer(t, &calls, func(string) string {
		return `{"etiquetas":[{"tipo":"estado","nombre":"Puebla"},{"tipo":"ciudad","nombre":"Huauchinango"}]}`
	})
	c := New("k", srv.URL, "m", "v")
	c.enrichLocation(context.Background(), &ext, locationTags)
	if _, ciudad := locationOf(ext.Sugerencias); ciudad != "Huauchinango" {
		t.Fatalf("ciudad = %q", ciudad)
	}
	if ext.Notas != "" {
		t.Fatalf("no debía avisar: %q", ext.Notas)
	}
}

// Sin evidencia de que sea la misma entidad, no se agrega ubicación.
func TestEnrichLocationKeepsEmptyWithoutEvidence(t *testing.T) {
	ext := Extraction{
		EsConvenio:          true,
		Dependencia:         "Jardín de Eventos Lua",
		FirmanteDependencia: "Jacqueline Cerón Falcón",
		LugarFirma:          "Acapulco de Juárez, Guerrero",
		Sugerencias:         []Suggestion{{Tipo: "ambito", Nombre: "Nacional"}},
	}
	cacheSearch(t, map[string]string{locationQuery(&ext): "- Jardín Lua, Nuevo Casas Grandes, Chihuahua: salón de eventos"})
	var calls int32
	srv := locationServer(t, &calls, func(prompt string) string {
		if !strings.Contains(prompt, "Representante: Jacqueline Cerón Falcón") {
			t.Errorf("falta el representante:\n%s", prompt)
		}
		return `{"etiquetas":[]}`
	})
	c := New("k", srv.URL, "m", "v")
	c.enrichLocation(context.Background(), &ext, locationTags)
	if estado, ciudad := locationOf(ext.Sugerencias); estado != "" || ciudad != "" || ext.Notas != "" {
		t.Fatalf("no debía ubicarla: %q, %q, %q", estado, ciudad, ext.Notas)
	}
}

// Si no se puede consultar internet, se informa en vez de dejar la ubicación
// vacía sin explicación.
func TestEnrichLocationReportsUnavailable(t *testing.T) {
	search.Lock()
	prev := search.blockedUntil
	search.blockedUntil = time.Now().Add(time.Hour)
	search.Unlock()
	t.Cleanup(func() {
		search.Lock()
		search.blockedUntil = prev
		search.Unlock()
	})
	var calls int32
	srv := locationServer(t, &calls, func(string) string { return `{"etiquetas":[]}` })
	c := New("k", srv.URL, "m", "v")
	ext := Extraction{EsConvenio: true, Dependencia: "Empresa Sin Domicilio", Sugerencias: []Suggestion{{Tipo: "ambito", Nombre: "Nacional"}}}
	if !c.enrichLocation(context.Background(), &ext, locationTags) {
		t.Fatal("debía avisar que no se pudo consultar internet")
	}
	if calls != 0 {
		t.Fatalf("no debía llamar al modelo sin resultados: %d", calls)
	}
}

func TestNoteMissingLocation(t *testing.T) {
	nacional := []Suggestion{{Tipo: "ambito", Nombre: "Nacional"}}
	cases := []struct {
		name               string
		ext                Extraction
		webSearch, noInter bool
		want               string
	}{
		{"sin internet", Extraction{EsConvenio: true, Sugerencias: nacional}, true, true, "no se pudo consultar internet"},
		{"no encontrada", Extraction{EsConvenio: true, Sugerencias: nacional}, true, false, "ni se confirmó en internet"},
		{"sin búsqueda web", Extraction{EsConvenio: true, Sugerencias: nacional}, false, false, "agréguela a mano"},
		{"con estado", Extraction{EsConvenio: true, Sugerencias: []Suggestion{{Tipo: "estado", Nombre: "Guerrero"}}}, true, false, ""},
		{"internacional", Extraction{EsConvenio: true, Sugerencias: []Suggestion{{Tipo: "ambito", Nombre: "Internacional"}}}, true, false, ""},
		{"no es convenio", Extraction{Sugerencias: nacional}, true, false, ""},
	}
	for _, tc := range cases {
		ext := tc.ext
		noteMissingLocation(&ext, tc.webSearch, tc.noInter)
		if tc.want == "" && ext.Notas != "" || tc.want != "" && !strings.Contains(ext.Notas, tc.want) {
			t.Errorf("%s: notas = %q", tc.name, ext.Notas)
		}
	}
}
