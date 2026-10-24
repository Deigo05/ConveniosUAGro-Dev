package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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

// TestVersionFilesMatch evita publicar un release cuyo VERSION no coincide con
// la versión compilada (el actualizador compara ambas).
func TestVersionFilesMatch(t *testing.T) {
	data, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != appVersion {
		t.Fatalf("VERSION = %q, appVersion = %q", got, appVersion)
	}
	wails, err := os.ReadFile("wails.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wails), `"productVersion": "`+appVersion+`"`) {
		t.Fatalf("wails.json no tiene productVersion %s", appVersion)
	}
}

func TestExtractOpenAgreementAndNameUnification(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"{\"tipo_documento\":\"convenio general de colaboración\",\"dependencia\":\"Secretaría de Salud\",\"dependencia_corta\":\"SSA\",\"fecha_firma\":\"2024-02-01\",\"firmante_uagro\":\"Juan Ramón García\",\"facultad\":null,\"vigencia_tipo\":\"indefinida\",\"etiquetas\":[{\"tipo\":\"ambito\",\"nombre\":\"Nacional\"}],\"correcciones\":[{\"original\":\"Juan Ramón Garay\",\"correcto\":\"Juan Ramón García\"}]}"},"finish_reason":"stop"}]}`)
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
	id, err := st.Insert(pdfutil.SavedFile{NombreGuardado: "abcdefabcdefabcdefabcdefabcdef34.pdf", NombreOriginal: "s.pdf", Paginas: 2, PaginasSinTexto: 2, Escaneado: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.BeginTranscription(id, 2); err != nil {
		t.Fatal(err)
	}
	st.SaveTranscriptionPage(id, 1, "Por la UAGro comparece Juan Ramón García, Rector.")
	st.SaveTranscriptionPage(id, 2, "[Firma] Juan Ramón Garay[?]")
	a := &App{store: st, paths: paths, cfg: config.Config{AIEnabled: true, APIKey: "k", BaseURL: srv.URL, Model: "m", VisionModel: "v", AnalysisLevel: "low"}}
	res, err := a.ExtractAI(id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"reasoning_effort":"low"`) || !strings.Contains(body, "correcciones") || !strings.Contains(body, "letras de forma parecida") {
		t.Fatalf("la petición no usa el nivel ni la regla de correcciones: %s", body[:min(len(body), 400)])
	}
	if strings.Contains(res.Texto, "Garay") || !strings.Contains(res.Notas, "Garay") {
		t.Fatalf("no se unificó el nombre: texto=%q notas=%q", res.Texto, res.Notas)
	}
	abierto := false
	for _, tag := range res.Etiquetas {
		if tag.Tipo == "facultad" && tag.Nombre == store.TagConvenioAbierto {
			abierto = true
		}
	}
	if !abierto {
		t.Fatalf("falta «Convenio Abierto»: %+v", res.Etiquetas)
	}
	// El PDF subido como "Escaner…" se descarga con un nombre útil.
	if d, err := a.store.Get(id); err != nil || d.ArchivoNombre != "2024-02-01 Convenio General - SSA - Convenio Abierto.pdf" {
		t.Fatalf("nombre de descarga = %q (%v)", d.ArchivoNombre, err)
	}

	// Capturar una facultad reemplaza «Convenio Abierto»; borrarla lo restaura.
	in := store.ConventionInput{ID: id, Dependencia: res.Dependencia, Facultad: "Facultad de Turismo", VigenciaIndefinida: true}
	d, err := a.SaveConvenio(in)
	if err != nil {
		t.Fatal(err)
	}
	if facultyTag(d) != "Facultad de Turismo Acapulco" {
		t.Fatalf("etiqueta de facultad = %q", facultyTag(d))
	}
	in.Facultad = ""
	d, err = a.SaveConvenio(in)
	if err != nil {
		t.Fatal(err)
	}
	if facultyTag(d) != store.TagConvenioAbierto {
		t.Fatalf("tras borrar la facultad = %q", facultyTag(d))
	}
}

func facultyTag(d store.ConventionDetail) string {
	for _, t := range d.Etiquetas {
		if t.Tipo == "facultad" {
			return t.Nombre
		}
	}
	return ""
}

func TestExtractLinksDeclaredRenewal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"{\"tipo_documento\":\"Convenio General\",\"dependencia\":\"Secretaría de Salud\",\"fecha_firma\":\"2024-02-01\",\"vigencia_tipo\":\"indefinida\",\"etiquetas\":[],\"renovacion\":{\"es\":true,\"fecha_anterior\":\"2021-02-01\"}}"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()
	t.Setenv("CONVENIOS_DATA_DIR", t.TempDir())
	paths, err := store.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	old, err := st.Insert(pdfutil.SavedFile{NombreGuardado: "11111111111111111111111111111111.pdf", NombreOriginal: "viejo.pdf", SHA256: "viejo", Texto: "Convenio 2021"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Update(store.ConventionInput{ID: old, Dependencia: "Secretaría de Salud", TipoDocumento: "Convenio General", FechaFirma: "2021-02-01"}); err != nil {
		t.Fatal(err)
	}
	id, err := st.Insert(pdfutil.SavedFile{NombreGuardado: "22222222222222222222222222222222.pdf", NombreOriginal: "Escaner1.pdf", SHA256: "nuevo", Texto: "Convenio que renueva el firmado en 2021"})
	if err != nil {
		t.Fatal(err)
	}
	a := &App{store: st, paths: paths, cfg: config.Config{AIEnabled: true, APIKey: "k", BaseURL: srv.URL, Model: "m", VisionModel: "v", AnalysisLevel: "low"}}
	res, err := a.ExtractAI(id)
	if err != nil {
		t.Fatal(err)
	}
	if res.Versiones != 1 || len(res.Historial) != 2 {
		t.Fatalf("la renovación no quedó en el historial: versiones=%d %+v", res.Versiones, res.Historial)
	}
	list, err := st.List(store.ListFilter{})
	if err != nil || list.Total != 1 || list.Items[0].ID != id {
		t.Fatalf("el listado debe mostrar solo la versión nueva: %+v %v", list.Items, err)
	}
}

// Casos reales: la IA escribe el nombre antiguo de la facultad y omite el
// ámbito; la etiqueta debe usar el nombre oficial y llevar «Nacional». Un
// archivo que no es convenio queda marcado y sin etiqueta de facultad.
func TestExtractRequiredTagsAndNoConvenio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(body), "Escritura número 77") {
			io.WriteString(w, `{"choices":[{"message":{"content":"{\"es_convenio\":false,\"dependencia\":\"Asociación Civil\",\"tipo_documento\":\"Otro\",\"etiquetas\":[{\"tipo\":\"estado\",\"nombre\":\"Guerrero\"}]}"},"finish_reason":"stop"}]}`)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"{\"es_convenio\":true,\"dependencia\":\"Hotel Playa\",\"facultad\":\"Unidad Académica de Turismo\",\"facultad_ciudad\":\"Acapulco de Juárez\",\"fecha_firma\":\"2024-03-15\",\"vigencia_tipo\":\"indefinida\",\"etiquetas\":[{\"tipo\":\"estado\",\"nombre\":\"Guerrero\"},{\"tipo\":\"ciudad\",\"nombre\":\"San Jerónimo de Juárez\"}]}"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()
	t.Setenv("CONVENIOS_DATA_DIR", t.TempDir())
	paths, err := store.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	insert := func(name, text string) int64 {
		id, err := st.Insert(pdfutil.SavedFile{NombreGuardado: name, NombreOriginal: "x.pdf", Texto: text})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	conv := insert("abcdefabcdefabcdefabcdefabcdef12.pdf", "Convenio entre la UAGro y Hotel Playa.")
	acta := insert("abcdefabcdefabcdefabcdefabcdef13.pdf", "Escritura número 77: acta constitutiva de una asociación civil.")
	a := &App{store: st, paths: paths, cfg: config.Config{AIEnabled: true, APIKey: "k", BaseURL: srv.URL, Model: "m", VisionModel: "v"}}

	if _, err := a.ExtractAI(conv); err != nil {
		t.Fatal(err)
	}
	tags, err := st.TagsForConvention(conv)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, tag := range tags {
		got[tag.Tipo] = tag.Nombre
	}
	want := map[string]string{"ambito": "Nacional", "estado": "Guerrero", "ciudad": "San Jerónimo de Juárez", "facultad": "Facultad de Turismo Acapulco"}
	for tipo, nombre := range want {
		if got[tipo] != nombre {
			t.Fatalf("etiquetas = %v, se esperaba %s=%q", got, tipo, nombre)
		}
	}

	res, err := a.ExtractAI(acta)
	if err != nil {
		t.Fatal(err)
	}
	if !res.NoEsConvenio {
		t.Fatal("el acta debía quedar marcada como «no es convenio»")
	}
	tags, _ = st.TagsForConvention(acta)
	for _, tag := range tags {
		if tag.Tipo == "facultad" {
			t.Fatalf("un archivo que no es convenio no lleva etiqueta de facultad: %+v", tags)
		}
	}
	// El usuario confirma que sí es un convenio: recibe las obligatorias.
	res, err = a.SetNoConvenio(acta, false)
	if err != nil || res.NoEsConvenio {
		t.Fatalf("confirmación = %+v, %v", res.ConventionSummary, err)
	}
	got = map[string]string{}
	for _, tag := range res.Etiquetas {
		got[tag.Tipo] = tag.Nombre
	}
	if got["ambito"] != "Nacional" || got["facultad"] != "Convenio Abierto" {
		t.Fatalf("etiquetas tras confirmar = %v", got)
	}
}
