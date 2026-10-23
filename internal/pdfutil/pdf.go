// Package pdfutil guarda y analiza PDF sin dependencias nativas (sin CGO).
package pdfutil

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/ledongthuc/pdf"
)

const (
	MaxBytes         = 100 << 20 // 100 MB
	MaxPages         = 2000
	ScannedThreshold = 200
)

// SavedFile describe un PDF ya copiado a la carpeta de datos.
type SavedFile struct {
	NombreGuardado  string
	NombreOriginal  string
	Bytes           int64
	SHA256          string
	Paginas         int
	PaginasSinTexto int
	Texto           string
	Escaneado       bool
}

// Import valida, copia y extrae el texto de un PDF.
func Import(src, destDir string) (SavedFile, error) {
	var out SavedFile
	info, err := os.Stat(src)
	if err != nil {
		return out, fmt.Errorf("no se pudo leer el archivo: %w", err)
	}
	if info.IsDir() {
		return out, errors.New("seleccione un archivo PDF, no una carpeta")
	}
	if info.Size() == 0 {
		return out, errors.New("el archivo está vacío")
	}
	if info.Size() > MaxBytes {
		return out, fmt.Errorf("el archivo supera el límite de %d MB", MaxBytes/(1024*1024))
	}

	in, err := os.Open(src)
	if err != nil {
		return out, err
	}
	defer in.Close()

	head := make([]byte, 5)
	if _, err := io.ReadFull(in, head); err != nil || string(head) != "%PDF-" {
		return out, errors.New("el archivo no es un PDF válido")
	}
	if _, err := in.Seek(0, io.SeekStart); err != nil {
		return out, err
	}

	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return out, err
	}
	tmp, err := os.CreateTemp(destDir, ".import-*.pdf")
	if err != nil {
		return out, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, hash), in)
	if err != nil {
		tmp.Close()
		return out, err
	}
	if err := tmp.Close(); err != nil {
		return out, err
	}

	name, err := randomName()
	if err != nil {
		return out, err
	}
	final := filepath.Join(destDir, name)
	if err := os.Rename(tmpName, final); err != nil {
		return out, err
	}

	out = SavedFile{
		NombreGuardado: name,
		NombreOriginal: cleanName(filepath.Base(strings.ReplaceAll(src, "\\", "/"))),
		Bytes:          n,
		SHA256:         hex.EncodeToString(hash.Sum(nil)),
	}
	texto, paginas, paginasSinTexto, err := extractText(final)
	if err != nil {
		// El PDF puede ser válido para el visor aunque esta librería no lo entienda.
		out.Paginas = 0
		out.PaginasSinTexto = 0
		out.Texto = ""
		out.Escaneado = true
		return out, nil
	}
	out.Paginas = paginas
	out.PaginasSinTexto = paginasSinTexto
	out.Texto = texto
	out.Escaneado = visibleChars(texto) < ScannedThreshold
	return out, nil
}

func extractText(path string) (texto string, paginas int, paginasSinTexto int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("no se pudo leer el PDF: %v", r)
		}
	}()
	f, reader, err := pdf.Open(path)
	if err != nil {
		return "", 0, 0, err
	}
	defer f.Close()
	n := reader.NumPage()
	if n < 1 {
		return "", 0, 0, errors.New("el PDF no tiene páginas")
	}
	if n > MaxPages {
		return "", n, 0, fmt.Errorf("el PDF tiene %d páginas; el máximo es %d", n, MaxPages)
	}
	var b strings.Builder
	for i := 1; i <= n; i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			paginasSinTexto++
			continue
		}
		s, err := page.GetPlainText(nil)
		if err != nil || visibleChars(s) < 30 {
			paginasSinTexto++
		}
		if err != nil {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(s)
	}
	return strings.TrimSpace(b.String()), n, paginasSinTexto, nil
}

func visibleChars(s string) int {
	n := 0
	for _, r := range s {
		if !unicode.IsSpace(r) && !unicode.IsControl(r) {
			n++
		}
	}
	return n
}

func randomName() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b) + ".pdf", nil
}

var reControl = regexp.MustCompile(`[\x00-\x1f\x7f]`)

func cleanName(name string) string {
	name = strings.TrimSpace(reControl.ReplaceAllString(name, ""))
	if name == "" {
		name = "convenio.pdf"
	}
	if len([]rune(name)) > 180 {
		name = string([]rune(name)[:180])
	}
	return name
}

// Export copia un PDF a la ruta elegida por el usuario.
func Export(src, dst string) error {
	if !strings.EqualFold(filepath.Ext(dst), ".pdf") {
		dst += ".pdf"
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
