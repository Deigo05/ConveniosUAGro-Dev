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
		io.WriteString(w, `{"choices":[{"message":{"content":"{\"dependencia\":\"Ayuntamiento de Acapulco\",\"firmante_dependencia\":\"Ana López\",\"cargo_dependencia\":\"Presidenta\",\"firmante_uagro\":\"Luis Pérez\",\"cargo_uagro\":\"Director\",\"facultad\":\"Facultad de Turismo\",\"facultad_ciudad\":\"Acapulco\",\"fecha_firma\":\"2024-03-15\",\"vigencia_tipo\":\"indefinida\",\"etiquetas\":[{\"tipo\":\"ambito\",\"nombre\":\"Nacional\"},{\"tipo\":\"estado\",\"nombre\":\"Guerrero\"},{\"tipo\":\"ciudad\",\"nombre\":\"Acapulco\"}]}"},"finish_reason":"stop"}]}`)
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
	if res.Facultad != "Facultad de Turismo" || res.FacultadCiudad != "Acapulco" || res.FirmanteUAGro != "Luis Pérez" || res.CargoUAGro != "Director" {
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
	if !res.EtiquetasRevisadas || len(res.EtiquetasSugeridas) != 0 {
		t.Fatalf("las sugerencias no se aplicaron automáticamente: %+v", res)
	}
	for _, name := range []string{"Nacional", "Guerrero", "Acapulco"} {
		found := false
		for _, tag := range tags {
			if tag.Nombre == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("falta la etiqueta %q: %+v", name, tags)
		}
	}
}

func TestRepairFacultyTagUsesFacultyCityNotDependencyCity(t *testing.T) {
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
	id, err := st.Insert(pdfutil.SavedFile{NombreGuardado: "fedcba9876543210fedcba9876543210.pdf", NombreOriginal: "x.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Update(store.ConventionInput{ID: id, Dependencia: "Corazón Artesano Tijuana", Facultad: "Facultad de Turismo"}); err != nil {
		t.Fatal(err)
	}
	city, err := st.CreateTag(store.TagInput{Nombre: "Tijuana", Tipo: "ciudad", Color: store.ColorCiudad})
	if err != nil {
		t.Fatal(err)
	}
	bad, err := st.CreateTag(store.TagInput{Nombre: "Facultad de Turismo Tijuana", Tipo: "facultad", Color: store.ColorFacultad})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceTags(id, []int64{city.ID, bad.ID}, true); err != nil {
		t.Fatal(err)
	}
	a := &App{store: st, paths: paths}
	a.repairFacultyTags()
	tags, err := st.TagsForConvention(id)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tag := range tags {
		if tag.Tipo == "facultad" {
			if tag.Nombre != "Facultad de Turismo Acapulco" {
				t.Fatalf("etiqueta incorrecta: %+v", tag)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("no se corrigió la etiqueta: %+v", tags)
	}
	all, err := st.ListTags()
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range all {
		if tag.Tipo == "facultad" && tag.Nombre == "Facultad de Turismo Tijuana" {
			t.Fatalf("quedó la etiqueta antigua: %+v", tag)
		}
	}
}
