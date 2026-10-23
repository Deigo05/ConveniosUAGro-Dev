package store

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"convenios-uagro-desktop/internal/pdfutil"
)

func TestAIResultAndBackup(t *testing.T) {
	st, paths := testStore(t)
	name := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.pdf"
	if err := os.WriteFile(filepath.Join(paths.PDFs, name), []byte("%PDF-1.4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := st.Insert(pdfutil.SavedFile{NombreGuardado: name, NombreOriginal: "x.pdf", Bytes: 9})
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().AddDate(3, 0, 0).Format("2006-01-02")
	err = st.SetAIResult(id, ConventionInput{
		ID: id, Dependencia: "Contraparte", FechaFirma: "2024-01-01",
		FechaVigencia: future, VigenciaCalculada: true, VigenciaTexto: "3 años",
	}, "ok", []Suggestion{{Tipo: "ambito", Nombre: "Nacional"}}, "nota IA")
	if err != nil {
		t.Fatal(err)
	}
	d, err := st.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if d.Dependencia != "Contraparte" || d.IAEstado != "ok" || d.FechaVigencia != future || !d.VigenciaCalculada {
		t.Fatalf("resultado IA inesperado: %+v", d)
	}
	if len(d.EtiquetasSugeridas) != 1 || d.EtiquetasRevisadas {
		t.Fatalf("sugerencias inesperadas: %+v", d)
	}

	backup := filepath.Join(t.TempDir(), "respaldo.zip")
	if err := st.BackupZip(backup); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(backup)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	foundDB, foundPDF := false, false
	for _, f := range zr.File {
		if f.Name == "convenios.db" {
			foundDB = true
		}
		if f.Name == "pdfs/"+name {
			foundPDF = true
		}
	}
	if !foundDB || !foundPDF {
		t.Fatalf("faltan archivos en el respaldo: db=%v pdf=%v", foundDB, foundPDF)
	}
}
