package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"convenios-uagro-desktop/internal/config"
	"convenios-uagro-desktop/internal/pdfutil"
	"convenios-uagro-desktop/internal/store"
)

func TestTranscribePagePipeline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "test-key") {
			t.Errorf("falta autorización")
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "data:image/jpeg;base64,") {
			t.Errorf("no se envió la imagen")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"TEXTO FALSO TRANSCRITO"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("CONVENIOS_DATA_DIR", dir)
	paths, err := store.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.Insert(pdfutil.SavedFile{
		NombreGuardado:  "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee.pdf",
		NombreOriginal:  "escaneado.pdf",
		Paginas:         1,
		PaginasSinTexto: 1,
		Escaneado:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	a := &App{store: st, paths: paths, cfg: config.Config{
		AIEnabled: true, APIKey: "test-key", BaseURL: srv.URL,
		Model: "text-model", VisionModel: "vision-model",
	}}
	if err := a.BeginTranscription(id, 1); err != nil {
		t.Fatal(err)
	}
	status, err := a.TranscribePage(id, 1, "aW1hZ2Vu")
	if err != nil {
		t.Fatal(err)
	}
	if status.Estado != "completa" || !strings.Contains(status.Texto, "TEXTO FALSO") {
		t.Fatalf("estado inesperado: %+v", status)
	}
	res, err := st.List(store.ListFilter{Query: "falso"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 {
		t.Fatalf("la transcripción no entró a FTS: %d", res.Total)
	}
}

func TestExtractSignatoriesFacultyAndFacultyTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"{\"dependencia\":\"Ayuntamiento de Acapulco\",\"firmante_dependencia\":\"Ana López\",\"cargo_dependencia\":\"Presidenta\",\"firmante_uagro\":\"Luis Pérez\",\"cargo_uagro\":\"Director\",\"facultad\":\"Facultad de Turismo\",\"fecha_firma\":\"2024-03-15\",\"vigencia_tipo\":\"indefinida\",\"etiquetas\":[{\"tipo\":\"ambito\",\"nombre\":\"Nacional\"},{\"tipo\":\"estado\",\"nombre\":\"Guerrero\"},{\"tipo\":\"ciudad\",\"nombre\":\"Acapulco\"}]}"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()
	dir := t.TempDir()
	t.Setenv("CONVENIOS_DATA_DIR", dir)
	paths, err := store.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.Insert(pdfutil.SavedFile{
		NombreGuardado: "abcdefabcdefabcdefabcdefabcdef12.pdf", NombreOriginal: "x.pdf",
		Texto: "Convenio entre la UAGro y el Ayuntamiento de Acapulco.",
	})
	if err != nil {
		t.Fatal(err)
	}
	a := &App{store: st, paths: paths, cfg: config.Config{AIEnabled: true, APIKey: "k", BaseURL: srv.URL, Model: "m", VisionModel: "v", WebSearchEnabled: false}}
	res, err := a.ExtractAI(id)
	if err != nil {
		t.Fatal(err)
	}
	if res.Facultad != "Facultad de Turismo" || res.FirmanteUAGro != "Luis Pérez" || res.CargoUAGro != "Director" {
		t.Fatalf("ficha = %+v", res.ConventionSummary)
	}
	tags, err := st.TagsForConvention(id)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tag := range tags {
		if tag.Tipo == "facultad" && tag.Nombre == "Facultad de Turismo Acapulco" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no se creó la etiqueta de facultad: %+v", tags)
	}
}

func TestFacultyTagName(t *testing.T) {
	cases := []struct{ facultad, city, want string }{
		{"Turismo", "Acapulco", "Facultad de Turismo Acapulco"},
		{"Facultad de Turismo", "Acapulco", "Facultad de Turismo Acapulco"},
		{"Facultad de Ingeniería 01", "Chilpancingo", "Facultad de Ingeniería 01 Chilpancingo"},
		{"Facultad Ingeniería 02", "Acapulco", "Facultad de Ingeniería 02 Acapulco"},
		{"Facultad de Turismo Acapulco", "Acapulco", "Facultad de Turismo Acapulco"},
	}
	for _, c := range cases {
		if got := facultyTagName(c.facultad, c.city); got != c.want {
			t.Errorf("facultyTagName(%q, %q) = %q, want %q", c.facultad, c.city, got, c.want)
		}
	}
}
