package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"convenios-uagro-desktop/internal/ai"
	"convenios-uagro-desktop/internal/config"
	"convenios-uagro-desktop/internal/pdfutil"
	"convenios-uagro-desktop/internal/store"
)

func newTestApp(t *testing.T) (*App, int64) {
	t.Helper()
	t.Setenv("CONVENIOS_DATA_DIR", t.TempDir())
	paths, err := store.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	id, err := st.Insert(pdfutil.SavedFile{NombreGuardado: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.pdf", NombreOriginal: "c.pdf", Paginas: 1, Texto: "texto del convenio"})
	if err != nil {
		t.Fatal(err)
	}
	return &App{store: st, paths: paths, cfg: config.Default()}, id
}

// Cambiar la facultad a un texto que no nombra una facultad quita la etiqueta
// anterior y deja el convenio como «Convenio Abierto».
func TestFacultyChangeToUnrecognizedMarksOpen(t *testing.T) {
	a, id := newTestApp(t)
	d, err := a.SaveConvenio(store.ConventionInput{ID: id, Facultad: "Facultad de Turismo", FacultadCiudad: "Acapulco"})
	if err != nil {
		t.Fatal(err)
	}
	if got := facultyTag(d); !strings.Contains(got, "Turismo") {
		t.Fatalf("etiqueta inicial = %q", got)
	}
	d, err = a.SaveConvenio(store.ConventionInput{ID: id, Facultad: "Rectoría"})
	if err != nil {
		t.Fatal(err)
	}
	if got := facultyTag(d); got != store.TagConvenioAbierto {
		t.Fatalf("tras cambiar a Rectoría la etiqueta es %q", got)
	}
}

// Un settings.json dañado no se sobrescribe: se aparta y se avisa.
func TestDamagedSettingsAreKept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"api_key": "clave-secreta", roto`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, aviso := loadConfig(path)
	if cfg.APIKey != "" || aviso == "" {
		t.Fatalf("cfg=%+v aviso=%q", cfg, aviso)
	}
	matches, _ := filepath.Glob(path + ".danado-*")
	if len(matches) != 1 {
		t.Fatalf("no se apartó la copia dañada: %v", matches)
	}
	if data, _ := os.ReadFile(matches[0]); !strings.Contains(string(data), "clave-secreta") {
		t.Fatal("la copia apartada no conserva el contenido original")
	}
	if _, err := config.Load(path); err != nil {
		t.Fatalf("el settings.json nuevo no es válido: %v", err)
	}
	// Sin archivo no hay aviso.
	if _, aviso := loadConfig(filepath.Join(dir, "otro.json")); aviso != "" {
		t.Fatalf("aviso inesperado: %q", aviso)
	}
}

// Con la IA apagada, VerifyAI falla sin perder las dudas pendientes.
func TestVerifyAIKeepsDoubtsWhenAIOff(t *testing.T) {
	a, id := newTestApp(t)
	a.setDoubts(id, []ai.Doubt{{Paginas: []int{1}, Dato: "firmante", Lecturas: []string{"A", "B"}}})
	if _, err := a.VerifyAI(id, []PageImage{{Pagina: 1, Imagen: "aW1hZ2Vu"}}); err == nil {
		t.Fatal("se esperaba error con la IA apagada")
	}
	if !a.hasDoubts(id) {
		t.Fatal("las dudas se perdieron")
	}
}

// Agregar la extensión no reemplaza un archivo que el diálogo no confirmó.
func TestWithExtensionDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	if got, err := withExtension(filepath.Join(dir, "a.zip"), ".zip"); err != nil || got != filepath.Join(dir, "a.zip") {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if got, err := withExtension(filepath.Join(dir, "b"), ".zip"); err != nil || got != filepath.Join(dir, "b.zip") {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c.zip"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := withExtension(filepath.Join(dir, "c"), ".zip"); err == nil {
		t.Fatal("se iba a sobrescribir c.zip sin confirmación")
	}
}

// El contador «No son convenios» coincide con su filtro: sin versiones anteriores.
func TestNoConvenioCountExcludesOldVersions(t *testing.T) {
	a, id := newTestApp(t)
	newer, err := a.store.Insert(pdfutil.SavedFile{NombreGuardado: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.pdf", NombreOriginal: "d.pdf", Paginas: 1, Texto: "otro texto"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetNoConvenio(id, true); err != nil {
		t.Fatal(err)
	}
	if err := a.store.LinkVersion(id, newer); err != nil {
		t.Fatal(err)
	}
	st, err := a.store.Stats()
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.store.List(store.ListFilter{SoloNoConvenios: true})
	if err != nil {
		t.Fatal(err)
	}
	if st.NoConvenios != len(res.Items) {
		t.Fatalf("contador=%d lista=%d", st.NoConvenios, len(res.Items))
	}
}
