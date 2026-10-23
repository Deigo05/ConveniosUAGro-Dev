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

func TestAskAIPipeline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"Hay un convenio en Acapulco [Convenio #1, página 1]."},"finish_reason":"stop"}]}`)
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
		NombreGuardado: "ffffffffffffffffffffffffffffffff.pdf", NombreOriginal: "ask.pdf",
		Paginas: 1, PaginasTexto: []string{"Convenio de colaboración con el Ayuntamiento de Acapulco."},
		Texto: "Convenio de colaboración con el Ayuntamiento de Acapulco.",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = id
	a := &App{store: st, paths: paths, cfg: config.Config{AIEnabled: true, APIKey: "test-key", BaseURL: srv.URL, Model: "text", VisionModel: "vision"}}
	res, err := a.AskAI("¿Qué convenio hay en Acapulco?")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Fuentes) == 0 || !strings.Contains(res.Respuesta, "Acapulco") {
		t.Fatalf("respuesta = %+v", res)
	}
}
