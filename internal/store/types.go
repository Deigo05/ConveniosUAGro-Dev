package store

import "time"

// Tag es una etiqueta del catálogo. Las de ámbito y estado son protegidas.
type Tag struct {
	ID        int64  `json:"id"`
	Nombre    string `json:"nombre"`
	Tipo      string `json:"tipo"`
	Color     string `json:"color"`
	Protegida bool   `json:"protegida"`
	// Estado es el estado de México de una etiqueta de ciudad.
	Estado string `json:"estado,omitempty"`
	Total  int    `json:"total,omitempty"`
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
	FacultadCiudad        string `json:"facultad_ciudad"`
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
	DependenciaCorta      string `json:"dependencia_corta"`
	TipoDocumento         string `json:"tipo_documento"`
	// ArchivoNombre es el nombre útil con el que se descarga el PDF; se
	// calcula con los datos del convenio (ver FileName).
	ArchivoNombre string `json:"archivo_nombre"`
	// Duplicados cuenta los posibles duplicados sin revisar; la marca
	// desaparece cuando se borra el duplicado o se descarta el par.
	Duplicados        int  `json:"duplicados"`
	PosibleRenovacion bool `json:"posible_renovacion"`
	// Versiones cuenta las versiones anteriores en el historial; si
	// ReemplazadoPor no es cero, este convenio es una versión anterior.
	Versiones      int   `json:"versiones"`
	ReemplazadoPor int64 `json:"reemplazado_por,omitempty"`
	// NoEsConvenio indica que la IA cree que el archivo no es un convenio
	// (un acta, un oficio, una constancia…) y conviene revisarlo.
	NoEsConvenio bool `json:"no_es_convenio"`
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
	// Relacionados lista los posibles duplicados y renovaciones sin revisar.
	Relacionados []RelatedRef `json:"relacionados"`
	// Historial tiene todas las versiones del convenio (incluida esta), de la
	// más reciente a la más antigua. Vacío si no hay otras versiones.
	Historial []VersionRef `json:"historial"`
	// VersionActual es el convenio vigente del historial cuando este es una
	// versión anterior.
	VersionActual int64 `json:"version_actual,omitempty"`
}

// RelatedRef es otro convenio que parece duplicado o renovación de este.
type RelatedRef struct {
	ID            int64  `json:"id"`
	Motivo        string `json:"motivo"`
	Dependencia   string `json:"dependencia"`
	ArchivoNombre string `json:"archivo_nombre"`
	FechaFirma    string `json:"fecha_firma"`
}

// VersionRef es una versión dentro del historial de un convenio.
type VersionRef struct {
	ID                 int64  `json:"id"`
	FechaFirma         string `json:"fecha_firma"`
	FechaVigencia      string `json:"fecha_vigencia"`
	VigenciaIndefinida bool   `json:"vigencia_indefinida"`
	TipoDocumento      string `json:"tipo_documento"`
	ArchivoNombre      string `json:"archivo_nombre"`
	Actual             bool   `json:"actual"`
	ReemplazadoPor     int64  `json:"reemplazado_por,omitempty"`
}

// DuplicatePair es un par para la herramienta de revisión de duplicados.
type DuplicatePair struct {
	Motivo string            `json:"motivo"`
	A      ConventionSummary `json:"a"`
	B      ConventionSummary `json:"b"`
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
	// IncluirAnteriores muestra también las versiones reemplazadas.
	IncluirAnteriores bool `json:"incluir_anteriores"`
	// SoloNoConvenios muestra los archivos que la IA marcó como no convenio.
	SoloNoConvenios bool `json:"solo_no_convenios"`
	// ids limita el listado a esos convenios (uso interno).
	ids []int64
}

// ListResult es la página de resultados.
type ListResult struct {
	Items            []ConventionSummary `json:"items"`
	Total            int                 `json:"total"`
	TotalRegistrados int                 `json:"total_registrados"`
	Page             int                 `json:"page"`
	Pages            int                 `json:"pages"`
	// Aproximado indica que ningún convenio tenía todas las palabras y se
	// muestran los que tienen alguna.
	Aproximado bool `json:"aproximado"`
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
	VersionesAnteriores int `json:"versiones_anteriores"`
	Duplicados          int `json:"duplicados"`
	Renovaciones        int `json:"renovaciones"`
	NoConvenios         int `json:"no_convenios"`
}

// ConventionInput es lo que envía el formulario de edición.
type ConventionInput struct {
	ID                  int64  `json:"id"`
	Dependencia         string `json:"dependencia"`
	DependenciaCorta    string `json:"dependencia_corta"`
	TipoDocumento       string `json:"tipo_documento"`
	FirmanteDependencia string `json:"firmante_dependencia"`
	CargoDependencia    string `json:"cargo_dependencia"`
	FirmanteUAGro       string `json:"firmante_uagro"`
	CargoUAGro          string `json:"cargo_uagro"`
	Facultad            string `json:"facultad"`
	FacultadCiudad      string `json:"facultad_ciudad"`
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
	// Estado solo aplica a las ciudades.
	Estado string `json:"estado"`
}

// AppInfo se muestra en Ajustes.
type AppInfo struct {
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	BuildDate  string `json:"build_date"`
	DataDir    string `json:"data_dir"`
	Database   string `json:"database"`
	Executable string `json:"executable"`
	GoVersion  string `json:"go_version"`
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
