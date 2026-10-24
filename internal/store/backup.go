package store

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BackupZip crea un respaldo completo (base + PDF) en un archivo .zip.
func (s *Store) BackupZip(dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	tmpDB := filepath.Join(s.paths.Backups, fmt.Sprintf(".backup-%d.db", time.Now().UnixNano()))
	_ = os.Remove(tmpDB)
	quoted := strings.ReplaceAll(tmpDB, "'", "''")
	if _, err := s.db.Exec("VACUUM INTO '" + quoted + "'"); err != nil {
		return fmt.Errorf("no se pudo preparar la base para el respaldo: %w", err)
	}
	defer os.Remove(tmpDB)

	tmpZip := dst + ".tmp"
	_ = os.Remove(tmpZip)
	zf, err := os.Create(tmpZip)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(zf)
	ok := false
	defer func() {
		zw.Close()
		zf.Close()
		if !ok {
			os.Remove(tmpZip)
		}
	}()

	if err := addFileToZip(zw, tmpDB, "convenios.db"); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.paths.PDFs)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".pdf") {
			continue
		}
		if err := addFileToZip(zw, filepath.Join(s.paths.PDFs, e.Name()), "pdfs/"+e.Name()); err != nil {
			return err
		}
	}
	readme, err := zw.Create("LEEME.txt")
	if err != nil {
		return err
	}
	fmt.Fprintf(readme, "Respaldo de Convenios UAGro\r\nGenerado: %s\r\n\r\nContenido:\r\n- convenios.db: base de datos\r\n- pdfs/: documentos originales\r\n\r\nPara restaurar, cierre la aplicación y copie convenios.db y la carpeta pdfs/ dentro de la carpeta de datos de la aplicación.\r\n", time.Now().Format("2006-01-02 15:04"))
	if err := zw.Close(); err != nil {
		return err
	}
	if err := zf.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpZip, dst); err != nil {
		return err
	}
	ok = true
	return nil
}

func addFileToZip(zw *zip.Writer, src, name string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	hdr.Name = filepath.ToSlash(name)
	hdr.Method = zip.Deflate
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, in)
	return err
}

// SnapshotDB guarda una copia consistente de la base (sin PDF) en dst.
func (s *Store) SnapshotDB(dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	_ = os.Remove(dst)
	_, err := s.db.Exec("VACUUM INTO '" + strings.ReplaceAll(dst, "'", "''") + "'")
	return err
}
