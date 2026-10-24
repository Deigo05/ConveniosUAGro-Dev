package store

import (
	"encoding/csv"
	"errors"
	"os"
	"strconv"
	"strings"
)

// utf8BOM hace que Excel abra el CSV como UTF-8.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

var estadoLabel = map[string]string{
	"vigente": "Vigente", "por_vencer": "Por vencer", "vencido": "Vencido",
	"indefinida": "Indefinida", "sin_fecha": "Sin fecha",
}

// ExportCSV guarda una hoja de cálculo (UTF-8 con BOM, para que Excel respete
// los acentos) con los convenios indicados, en ese orden.
func (s *Store) ExportCSV(dst string, ids []int64) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("no hay convenios para exportar")
	}
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	defer os.Remove(tmp)
	if _, err := f.WriteString(string(utf8BOM)); err != nil {
		f.Close()
		return 0, err
	}
	w := csv.NewWriter(f)
	w.UseCRLF = true
	_ = w.Write([]string{
		"ID", "Tipo de documento", "Dependencia", "Siglas", "Firmante (dependencia)", "Cargo (dependencia)", "Firmante (UAGro)",
		"Cargo (UAGro)", "Facultad", "Ciudad de la facultad", "Fecha de firma", "Fecha de vigencia",
		"Situación", "Ámbito", "Estado", "Ciudad", "Etiqueta de facultad", "Temas", "Nombre de archivo",
		"Archivo original", "Páginas", "Versión", "Notas",
	})
	n := 0
	for _, id := range ids {
		c, err := s.Get(id)
		if err != nil {
			continue
		}
		byType := map[string][]string{}
		for _, t := range c.Etiquetas {
			byType[t.Tipo] = append(byType[t.Tipo], t.Nombre)
		}
		vigencia := c.FechaVigencia
		if c.VigenciaIndefinida {
			vigencia = "Indefinida"
		}
		row := []string{
			strconv.FormatInt(c.ID, 10), c.TipoDocumento, c.Dependencia, c.DependenciaCorta, c.FirmanteDependencia, c.CargoDependencia,
			c.FirmanteUAGro, c.CargoUAGro, c.Facultad, c.FacultadCiudad, c.FechaFirma, vigencia,
			estadoLabel[c.Estado], strings.Join(byType["ambito"], ", "), strings.Join(byType["estado"], ", "),
			strings.Join(byType["ciudad"], ", "), strings.Join(byType["facultad"], ", "),
			strings.Join(byType["tema"], ", "), c.ArchivoNombre, c.ArchivoNombreOriginal, strconv.Itoa(c.Paginas), versionLabel(c), c.Notas,
		}
		for i := range row {
			row[i] = csvSafe(row[i])
		}
		if err := w.Write(row); err != nil {
			f.Close()
			return 0, err
		}
		n++
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return 0, err
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	return n, os.Rename(tmp, dst)
}

// csvSafe evita que Excel interprete como fórmula un texto que empieza con
// =, +, - o @.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@", rune(v[0])) {
		return "'" + v
	}
	return v
}

// FilterIDs devuelve todos los ids que cumplen el filtro, en el orden del
// listado, para exportar los resultados de una búsqueda.
func (s *Store) FilterIDs(f ListFilter) ([]int64, error) {
	var ids []int64
	f.Page = 1
	for {
		res, err := s.List(f)
		if err != nil {
			return nil, err
		}
		for _, c := range res.Items {
			ids = append(ids, c.ID)
		}
		if res.Page >= res.Pages {
			return ids, nil
		}
		f.Page++
	}
}

func versionLabel(c ConventionDetail) string {
	switch {
	case c.VersionActual != 0:
		return "Anterior (reemplazada por #" + strconv.FormatInt(c.VersionActual, 10) + ")"
	case c.Versiones > 0:
		return "Vigente (" + strconv.Itoa(c.Versiones) + " anteriores)"
	}
	return "Vigente"
}
