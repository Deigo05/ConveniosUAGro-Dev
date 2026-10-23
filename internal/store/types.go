package store

import "time"

// Tag es una etiqueta del catálogo. Las de ámbito y estado son protegidas.
type Tag struct {
	ID        int64  `json:"id"`
	Nombre    string `json:"nombre"`
	Tipo      string `json:"tipo"`
	Color     string `json:"color"`
	Protegida bool   `json:"protegida"`
	Total     int    `json:"total,omitempty"`
}

// Suggestion es una etiqueta propuesta por la IA.
type Suggestion struct {
	Tipo   string `json:"tipo"`
	Nombre string `json:"nombre"`
	Nueva  bool   `json:"nueva"`
}

// ConventionSummary es una fila del listado.
type ConventionSummary struct {
	ID                    int64  `json:"id"`
	Dependencia           string `json:"dependencia"`
	FirmanteDependencia   string `json:"firmante_dependencia"`
	CargoDependencia      string `json:"cargo_dependencia"`
	FirmanteUAGro         string `json:"firmante_uagro"`
	CargoUAGro            string `json:"cargo_uagro"`
	Facultad              string `json:"facultad"`
	FechaFirma            string `json:"fecha_firma"`
	FechaVigencia         string `json:"fecha_vigencia"`
	VigenciaIndefinida    bool   `json:"vigencia_indefinida"`
	VigenciaCalculada     bool   `json:"vigencia_calculada"`
	ArchivoNombreOriginal string `json:"archivo_nombre_original"`
	Paginas               int    `json:"paginas"`
	EsEscaneado           bool   `json:"es_escaneado"`
	IAEstado              string `json:"ia_estado"`
	IAError               string `json:"ia_error,omitempty"`
	EtiquetasRevisadas    bool   `json:"etiquetas_revisadas"`
	Etiquetas             []Tag  `json:"etiquetas"`
	Estado                string `json:"estado"`
	TranscripcionEstado   string `json:"transcripcion_estado"`
	TranscripcionError    string `json:"transcripcion_error,omitempty"`
	PaginasPorTranscribir int    `json:"paginas_por_transcribir"`
	PaginasTranscritas    int    `json:"paginas_transcritas"`
	CreadoEn              string `json:"creado_en"`
	ActualizadoEn         string `json:"actualizado_en"`
	Fragmento             string `json:"fragmento,omitempty"`
}

// TranscriptionStatus es el avance de una transcripción.
type TranscriptionStatus struct {
	Estado                string `json:"estado"`
	Error                 string `json:"error,omitempty"`
	PaginasPorTranscribir int    `json:"paginas_por_transcribir"`
	PaginasTranscritas    int    `json:"paginas_transcritas"`
	Texto                 string `json:"texto,omitempty"`
}

// ConventionDetail es la ficha completa.
type ConventionDetail struct {
	ConventionSummary
	Texto              string       `json:"texto"`
	VigenciaTexto      string       `json:"vigencia_texto"`
	Notas              string       `json:"notas"`
	ArchivoGuardado    string       `json:"archivo_guardado"`
	ArchivoBytes       int64        `json:"archivo_bytes"`
	ArchivoSHA256      string       `json:"archivo_sha256"`
	EtiquetasSugeridas []Suggestion `json:"etiquetas_sugeridas"`
}

// ListFilter contiene los filtros del listado.
type ListFilter struct {
	Query          string  `json:"query"`
	TagIDs         []int64 `json:"tag_ids"`
	Vigencia       string  `json:"vigencia"`
	SoloPendientes bool    `json:"solo_pendientes"`
	Order          string  `json:"order"`
	Dir            string  `json:"dir"`
	Page           int     `json:"page"`
}

// ListResult es la página de resultados.
type ListResult struct {
	Items            []ConventionSummary `json:"items"`
	Total            int                 `json:"total"`
	TotalRegistrados int                 `json:"total_registrados"`
	Page             int                 `json:"page"`
	Pages            int                 `json:"pages"`
}

// Stats son los contadores del panel superior.
type Stats struct {
	Total               int `json:"total"`
	Vigentes            int `json:"vigentes"`
	PorVencer           int `json:"por_vencer"`
	Vencidos            int `json:"vencidos"`
	SinFecha            int `json:"sin_fecha"`
	Indefinidas         int `json:"indefinidas"`
	EtiquetasPendientes int `json:"etiquetas_pendientes"`
	Etiquetas           int `json:"etiquetas"`
}

// ConventionInput es lo que envía el formulario de edición.
type ConventionInput struct {
	ID                  int64  `json:"id"`
	Dependencia         string `json:"dependencia"`
	FirmanteDependencia string `json:"firmante_dependencia"`
	CargoDependencia    string `json:"cargo_dependencia"`
	FirmanteUAGro       string `json:"firmante_uagro"`
	CargoUAGro          string `json:"cargo_uagro"`
	Facultad            string `json:"facultad"`
	FechaFirma          string `json:"fecha_firma"`
	FechaVigencia       string `json:"fecha_vigencia"`
	VigenciaIndefinida  bool   `json:"vigencia_indefinida"`
	VigenciaCalculada   bool   `json:"vigencia_calculada"`
	VigenciaTexto       string `json:"vigencia_texto"`
	Notas               string `json:"notas"`
}

// TagInput se usa para crear o editar una etiqueta.
type TagInput struct {
	ID     int64  `json:"id"`
	Nombre string `json:"nombre"`
	Tipo   string `json:"tipo"`
	Color  string `json:"color"`
}

// AppInfo se muestra en Ajustes.
type AppInfo struct {
	Version   string `json:"version"`
	DataDir   string `json:"data_dir"`
	Database  string `json:"database"`
	GoVersion string `json:"go_version"`
}

// Today devuelve la fecha local en ISO.
func Today() string { return time.Now().Format("2006-01-02") }

// IsDate reports whether s is a valid YYYY-MM-DD date in the supported range.
func IsDate(s string) bool {
	if s == "" {
		return true
	}
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		return false
	}
	return d.Year() >= 1950 && d.Year() <= 2100
}
