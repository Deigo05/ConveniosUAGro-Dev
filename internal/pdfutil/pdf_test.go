package pdfutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportSample(t *testing.T) {
	sample := os.Getenv("SAMPLE_PDF")
	if sample == "" {
		t.Skip("SAMPLE_PDF no está definido")
	}
	dir := t.TempDir()
	f, err := Import(sample, dir)
	if err != nil {
		t.Fatal(err)
	}
	if f.Paginas < 1 {
		t.Fatalf("páginas = %d", f.Paginas)
	}
	if f.Texto == "" {
		t.Fatal("no se extrajo texto")
	}
	if f.Escaneado {
		t.Fatal("el PDF de muestra no debería marcarse escaneado")
	}
	if _, err := os.Stat(filepath.Join(dir, f.NombreGuardado)); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "exportado.pdf")
	if err := Export(filepath.Join(dir, f.NombreGuardado), dst); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(dst); err != nil || st.Size() != f.Bytes {
		t.Fatalf("export incorrecto: %v %v", st, err)
	}
}

func TestRejectNonPDF(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "no.pdf")
	if err := os.WriteFile(src, []byte("hola"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(src, dir); err == nil {
		t.Fatal("se aceptó un archivo que no es PDF")
	}
}
