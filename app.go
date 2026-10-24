package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"convenios-uagro-desktop/internal/ai"
	"convenios-uagro-desktop/internal/config"
	"convenios-uagro-desktop/internal/pdfutil"
	"convenios-uagro-desktop/internal/store"
	"convenios-uagro-desktop/internal/uagro"
	"convenios-uagro-desktop/internal/update"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const appVersion = "1.2.3"

// buildCommit y buildDate se fijan al compilar en GitHub Actions con
// -ldflags "-X main.buildCommit=<sha> -X main.buildDate=<RFC3339>". En una
// compilación local quedan vacíos y la búsqueda de actualizaciones solo compara
// la versión.
var (
	buildCommit string
	buildDate   string
)

// afterUpdateFlag lo recibe el ejecutable nuevo tras una actualización.
const afterUpdateFlag = "--after-update"

// App es el puente entre la interfaz y la base local.
type App struct {
	ctx   context.Context
	store *store.Store
	paths store.Paths
	cfg   config.Config

	cfgMu sync.Mutex
	// tagMu ordena los cambios de etiquetas: con varios documentos
	// procesándose a la vez, cada uno lee y reescribe sus etiquetas.
	tagMu sync.Mutex

	// doubts guarda, por convenio, las dudas de lectura de la última
	// extracción hasta que la interfaz envía las imágenes para revisarlas.
	doubtsMu sync.Mutex
	doubts   map[int64][]ai.Doubt

	updMu      sync.Mutex
	lastUpdate *update.Info
	updating   bool
}

// Bootstrap es la carga inicial de la interfaz.
type Bootstrap struct {
	Tags   []store.Tag   `json:"tags"`
	Stats  store.Stats   `json:"stats"`
	App    store.AppInfo `json:"app"`
	Config config.View   `json:"config"`
}

// ImportResult informa el resultado de cada archivo importado.
type ImportResult struct {
	Path        string `json:"path"`
	ID          int64  `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Error       string `json:"error,omitempty"`
	DuplicateID int64  `json:"duplicate_id,omitempty"`
	// Cloned indica que se copiaron los datos del original duplicado, así que
	// no hace falta volver a procesarlo con IA.
	Cloned bool `json:"cloned,omitempty"`
}

// ExportResult informa dónde se guardó una exportación.
type ExportResult struct {
	Path  string `json:"path"`
	Total int    `json:"total"`
}

func NewApp() (*App, error) {
	if hasArg(afterUpdateFlag) {
		// Da tiempo a que la versión anterior termine de cerrar la base.
		time.Sleep(1500 * time.Millisecond)
	}
	paths, err := store.DefaultPaths()
	if err != nil {
		return nil, err
	}
	st, err := store.Open(paths)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(paths.Config)
	if err != nil {
		log.Printf("configuración inválida, se usarán valores iniciales: %v", err)
		cfg = config.Default()
		_ = config.Save(paths.Config, cfg)
	}
	return &App{store: st, paths: paths, cfg: cfg}, nil
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.applyPendingSuggestions()
	if err := a.store.BackfillCityStates(); err != nil {
		log.Printf("no se pudo asignar el estado de las ciudades: %v", err)
	}
	a.repairFacultyTags()
	a.repairRequiredTags()
	a.markOpenAgreements()
	// La primera vez calcula la huella de todos los textos; luego es inmediato.
	go func() {
		a.refreshDuplicates()
		wailsruntime.EventsEmit(ctx, "duplicates:refreshed")
	}()
	go cleanupOldExecutable()
}

// refreshDuplicates recalcula las marcas de duplicados y renovaciones.
func (a *App) refreshDuplicates() {
	if err := a.store.RefreshDuplicates(); err != nil {
		log.Printf("no se pudieron recalcular los duplicados: %v", err)
	}
}

func hasArg(flag string) bool {
	for _, arg := range os.Args[1:] {
		if arg == flag {
			return true
		}
	}
	return false
}

// cleanupOldExecutable borra el .old que deja una actualización. Se reintenta
// porque el proceso anterior puede tardar unos segundos en cerrarse.
func cleanupOldExecutable() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	for i := 0; i < 10; i++ {
		update.Cleanup(exe)
		if _, err := os.Stat(exe + ".old"); os.IsNotExist(err) {
			return
		}
		time.Sleep(2 * time.Second)
	}
}

func (a *App) shutdown(ctx context.Context) {
	if a.store != nil {
		_ = a.store.Close()
	}
}

func (a *App) appInfo() store.AppInfo {
	return store.AppInfo{
		Version:    appVersion,
		Commit:     buildCommit,
		BuildDate:  buildDate,
		DataDir:    a.paths.Base,
		Database:   a.paths.DB,
		Executable: executablePath(),
		GoVersion:  goruntime.Version(),
	}
}

func executablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}

func (a *App) configView() config.View {
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()
	return a.cfg.ToView()
}

// GetBootstrap carga etiquetas, contadores, configuración y datos de la app.
func (a *App) GetBootstrap() (Bootstrap, error) {
	tags, err := a.store.ListTags()
	if err != nil {
		return Bootstrap{}, err
	}
	stats, err := a.store.Stats()
	if err != nil {
		return Bootstrap{}, err
	}
	return Bootstrap{Tags: tags, Stats: stats, App: a.appInfo(), Config: a.configView()}, nil
}

// ListConvenios devuelve la página filtrada.
func (a *App) ListConvenios(f store.ListFilter) (store.ListResult, error) {
	return a.store.List(f)
}

// GetConvenio devuelve la ficha completa.
func (a *App) GetConvenio(id int64) (store.ConventionDetail, error) {
	return a.store.Get(id)
}

// SaveConvenio guarda la captura manual.
func (a *App) SaveConvenio(in store.ConventionInput) (store.ConventionDetail, error) {
	prev, err := a.store.Get(in.ID)
	if err != nil {
		return store.ConventionDetail{}, err
	}
	if err := a.store.Update(in); err != nil {
		return store.ConventionDetail{}, err
	}
	a.tagMu.Lock()
	if strings.TrimSpace(in.Facultad) == "" && strings.TrimSpace(prev.Facultad) != "" {
		// Se borró la facultad: su etiqueta ya no corresponde.
		_ = a.removeFacultyTags(in.ID)
	}
	_ = a.applyFacultyTag(in.ID, in.Facultad, in.FacultadCiudad)
	a.tagMu.Unlock()
	a.refreshDuplicates()
	return a.store.Get(in.ID)
}

func (a *App) removeFacultyTags(id int64) error {
	tags, err := a.store.TagsForConvention(id)
	if err != nil {
		return err
	}
	keep := make([]int64, 0, len(tags))
	for _, t := range tags {
		if t.Tipo != "facultad" {
			keep = append(keep, t.ID)
		}
	}
	return a.store.ReplaceTagsAuto(id, keep)
}

// ChoosePDFs abre el diálogo nativo de archivos.
func (a *App) ChoosePDFs() ([]string, error) {
	if a.ctx == nil {
		return nil, errors.New("la aplicación aún no está lista")
	}
	return wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Seleccionar convenios en PDF",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Documentos PDF (*.pdf)", Pattern: "*.pdf"},
		},
	})
}

// ImportPDFs copia y registra los PDF seleccionados. No usa IA automáticamente:
// el usuario puede extraer los datos desde la ficha.
func (a *App) ImportPDFs(paths []string) []ImportResult {
	if len(paths) > 60 {
		paths = paths[:60]
	}
	results := make([]ImportResult, 0, len(paths))
	for _, path := range paths {
		r := ImportResult{Path: path}
		saved, err := pdfutil.Import(path, a.paths.PDFs)
		if err != nil {
			r.Error = err.Error()
			results = append(results, r)
			continue
		}
		id, err := a.store.Insert(saved)
		if err != nil {
			_ = os.Remove(filepath.Join(a.paths.PDFs, saved.NombreGuardado))
			r.Error = "no se pudo registrar en la base: " + err.Error()
			results = append(results, r)
			continue
		}
		r.ID = id
		r.Name = saved.NombreOriginal
		if dup, ok, _ := a.store.DuplicateID(saved.SHA256, id); ok {
			r.DuplicateID = dup
			// Mismo archivo ya procesado: se copian sus datos en vez de pagar
			// otra vez la transcripción y la extracción.
			if cloned, err := a.store.CloneAnalysis(dup, id); err == nil && cloned {
				r.Cloned = true
			}
		}
		results = append(results, r)
	}
	a.refreshDuplicates()
	return results
}

// SaveConvenioTags reemplaza las etiquetas y marca la clasificación como revisada.
func (a *App) SaveConvenioTags(id int64, tagIDs []int64) (store.ConventionDetail, error) {
	a.tagMu.Lock()
	err := a.store.ReplaceTags(id, tagIDs, true)
	if err == nil {
		if d, err := a.store.Get(id); err == nil {
			_ = a.applyFacultyTag(id, d.Facultad, d.FacultadCiudad)
		}
	}
	a.tagMu.Unlock()
	if err != nil {
		return store.ConventionDetail{}, err
	}
	a.refreshDuplicates()
	return a.store.Get(id)
}

// AcceptSuggestions convierte las sugerencias de IA en etiquetas.
func (a *App) AcceptSuggestions(id int64) (store.ConventionDetail, error) {
	a.tagMu.Lock()
	err := a.store.AcceptSuggestions(id)
	if err == nil {
		if d, err := a.store.Get(id); err == nil {
			_ = a.applyFacultyTag(id, d.Facultad, d.FacultadCiudad)
		}
	}
	a.tagMu.Unlock()
	if err != nil {
		return store.ConventionDetail{}, err
	}
	a.refreshDuplicates()
	return a.store.Get(id)
}

// DiscardSuggestions descarta las sugerencias; si remove, deja el convenio sin etiquetas.
func (a *App) DiscardSuggestions(id int64, remove bool) (store.ConventionDetail, error) {
	if err := a.store.DiscardSuggestions(id, remove); err != nil {
		return store.ConventionDetail{}, err
	}
	return a.store.Get(id)
}

// DeleteConvenios borra los registros y sus PDF.
func (a *App) DeleteConvenios(ids []int64) error {
	files, err := a.store.DeleteMany(ids)
	if err != nil {
		return err
	}
	for _, name := range files {
		if filepath.Base(name) != name {
			continue
		}
		_ = os.Remove(filepath.Join(a.paths.PDFs, name))
	}
	// Las marcas de los duplicados borrados ya desaparecieron (ON DELETE
	// CASCADE); se recalcula para proponer las renovaciones que siguen.
	a.refreshDuplicates()
	return nil
}

// DownloadPDF abre el diálogo Guardar como y copia el PDF elegido.
func (a *App) DownloadPDF(id int64) (string, error) {
	if a.ctx == nil {
		return "", errors.New("la aplicación aún no está lista")
	}
	src, _, err := a.store.PDF(id)
	if err != nil {
		return "", err
	}
	c, err := a.store.Get(id)
	if err != nil {
		return "", err
	}
	dst, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:            "Descargar PDF",
		DefaultDirectory: defaultDownloadDir(),
		DefaultFilename:  c.ArchivoNombre,
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Documento PDF (*.pdf)", Pattern: "*.pdf"},
		},
	})
	if err != nil || strings.TrimSpace(dst) == "" {
		return "", err
	}
	if err := pdfutil.Export(src, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// DownloadPDFs copia los PDF indicados a una carpeta, cada uno con su nombre
// útil (fecha, tipo, contraparte y facultad). No sobrescribe archivos que ya
// estén en la carpeta.
func (a *App) DownloadPDFs(ids []int64) (ExportResult, error) {
	if a.ctx == nil {
		return ExportResult{}, errors.New("la aplicación aún no está lista")
	}
	if len(ids) == 0 {
		return ExportResult{}, errors.New("no hay convenios seleccionados")
	}
	dir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:                "Carpeta donde guardar los PDF",
		DefaultDirectory:     defaultDownloadDir(),
		CanCreateDirectories: true,
	})
	if err != nil || strings.TrimSpace(dir) == "" {
		return ExportResult{}, err
	}
	used := map[string]bool{}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			used[strings.ToLower(e.Name())] = true
		}
	}
	n := 0
	var failed []string
	for _, id := range ids {
		src, _, err := a.store.PDF(id)
		if err != nil {
			failed = append(failed, fmt.Sprintf("#%d", id))
			continue
		}
		c, err := a.store.Get(id)
		if err != nil {
			failed = append(failed, fmt.Sprintf("#%d", id))
			continue
		}
		if err := pdfutil.Export(src, filepath.Join(dir, store.UniqueFileName(c.ArchivoNombre, used))); err != nil {
			failed = append(failed, c.ArchivoNombre)
			continue
		}
		n++
	}
	if n == 0 {
		return ExportResult{}, fmt.Errorf("no se pudo guardar ningún PDF (%s)", strings.Join(failed, ", "))
	}
	return ExportResult{Path: dir, Total: n}, nil
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// GetTags devuelve el catálogo completo.
func (a *App) GetTags() ([]store.Tag, error) {
	return a.store.ListTags()
}

func (a *App) CreateTag(in store.TagInput) (store.Tag, error) {
	return a.store.CreateTag(in)
}

func (a *App) UpdateTag(in store.TagInput) error {
	return a.store.UpdateTag(in)
}

func (a *App) DeleteTag(id int64) error {
	return a.store.DeleteTag(id)
}

// GetSettings devuelve la configuración sin revelar la clave.
func (a *App) GetSettings() config.View {
	return a.configView()
}

// SaveSettings guarda la configuración de IA.
func (a *App) SaveSettings(in config.SaveInput) (config.View, error) {
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()
	cfg := a.cfg
	if err := cfg.Apply(in); err != nil {
		return a.cfg.ToView(), err
	}
	if err := config.Save(a.paths.Config, cfg); err != nil {
		return a.cfg.ToView(), err
	}
	a.cfg = cfg
	return cfg.ToView(), nil
}

// ExtractAI extrae los datos del PDF y guarda las sugerencias.
func (a *App) ExtractAI(id int64) (store.ConventionDetail, error) {
	return a.extract(id, "", true)
}

// extract analiza el convenio con IA. nota se agrega a las notas (por
// ejemplo, las correcciones confirmadas con la imagen) y keepDoubts guarda
// las dudas de lectura para revisarlas después; la segunda extracción, tras
// la revisión, ya no las guarda para no repetir el ciclo.
func (a *App) extract(id int64, nota string, keepDoubts bool) (store.ConventionDetail, error) {
	a.cfgMu.Lock()
	cfg := a.cfg
	a.cfgMu.Unlock()
	if !cfg.AIEnabled {
		return store.ConventionDetail{}, errors.New("active el análisis con IA en Ajustes")
	}
	if cfg.APIKey == "" {
		return store.ConventionDetail{}, errors.New("configure una clave de IA en Ajustes")
	}
	texto, _, err := a.store.TextForAI(id)
	if err != nil {
		return store.ConventionDetail{}, err
	}
	if strings.TrimSpace(texto) == "" {
		err := errors.New("el PDF no tiene texto seleccionable; capture los datos manualmente")
		_ = a.store.SetIAStatus(id, "error", err.Error())
		return store.ConventionDetail{}, err
	}
	_ = a.store.SetIAStatus(id, "pendiente", "")
	tags, err := a.store.ExistingTags()
	if err != nil {
		return store.ConventionDetail{}, err
	}
	catalogo := make([]ai.Tag, 0, len(tags))
	for _, t := range tags {
		catalogo = append(catalogo, ai.Tag{Nombre: t.Nombre, Tipo: t.Tipo, Estado: t.Estado})
	}
	transcrito, _ := a.store.HasTranscription(id)
	client := ai.New(cfg.APIKey, cfg.BaseURL, cfg.Model, cfg.VisionModel)
	client.WebSearch = cfg.WebSearchEnabled
	client.Effort = cfg.AnalysisLevel
	if d, err := a.store.Get(id); err == nil && len(d.CreadoEn) >= 10 {
		client.ReferenceDate = d.CreadoEn[:10]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	res, err := client.Extract(ctx, texto, catalogo, transcrito)
	if err != nil {
		_ = a.store.SetIAStatus(id, "error", err.Error())
		return store.ConventionDetail{}, err
	}
	var dudas []ai.Doubt
	if keepDoubts {
		dudas = res.Dudas
	}
	a.setDoubts(id, dudas)
	notaCorrecciones := nota
	if len(res.Correcciones) > 0 {
		pedidas := make([]store.NameCorrection, 0, len(res.Correcciones))
		for _, c := range res.Correcciones {
			pedidas = append(pedidas, store.NameCorrection{Original: c.Original, Correcto: c.Correcto})
		}
		if applied, err := a.store.ApplyNameCorrections(id, pedidas); err != nil {
			log.Printf("no se pudieron unificar nombres del convenio %d: %v", id, err)
		} else if len(applied) > 0 {
			cambios := make([]string, 0, len(applied))
			for _, c := range applied {
				cambios = append(cambios, fmt.Sprintf("«%s» → «%s»", c.Original, c.Correcto))
			}
			notaCorrecciones = strings.TrimSpace(notaCorrecciones + " Transcripción: se unificaron nombres por posible error de lectura: " + strings.Join(cambios, "; ") + ".")
		}
	}
	in := store.ConventionInput{
		ID:                  id,
		Dependencia:         res.Dependencia,
		DependenciaCorta:    res.DependenciaCorta,
		TipoDocumento:       res.TipoDocumento,
		FirmanteDependencia: res.FirmanteDependencia,
		CargoDependencia:    res.CargoDependencia,
		FirmanteUAGro:       res.FirmanteUAGro,
		CargoUAGro:          res.CargoUAGro,
		Facultad:            res.Facultad,
		FacultadCiudad:      res.FacultadCiudad,
		FechaFirma:          res.FechaFirma,
		FechaVigencia:       res.FechaVigencia,
		VigenciaIndefinida:  res.VigenciaIndefinida,
		VigenciaCalculada:   res.VigenciaCalculada,
		VigenciaTexto:       res.VigenciaTexto,
		Notas:               res.Notas,
	}
	sugs := make([]store.Suggestion, 0, len(res.Sugerencias))
	for _, s := range res.Sugerencias {
		sugs = append(sugs, store.Suggestion{Tipo: s.Tipo, Nombre: s.Nombre, Nueva: s.Nueva})
	}
	if err := a.store.SetAIResult(id, in, res.Estado, sugs, notaCorrecciones); err != nil {
		return store.ConventionDetail{}, err
	}
	if err := a.store.SetAIConvenioFlag(id, res.EsConvenio); err != nil {
		log.Printf("no se pudo guardar si el convenio %d es un convenio: %v", id, err)
	}
	if err := a.applyAITags(id, len(sugs) > 0, res.Facultad, res.FacultadCiudad); err != nil {
		return store.ConventionDetail{}, err
	}
	a.refreshDuplicates()
	if res.Renovacion {
		a.linkRenewal(id, res.FechaAnterior)
	}
	return a.store.Get(id)
}

func (a *App) setDoubts(id int64, dudas []ai.Doubt) {
	a.doubtsMu.Lock()
	defer a.doubtsMu.Unlock()
	if len(dudas) == 0 {
		delete(a.doubts, id)
		return
	}
	if a.doubts == nil {
		a.doubts = map[int64][]ai.Doubt{}
	}
	a.doubts[id] = dudas
}

func (a *App) takeDoubts(id int64) []ai.Doubt {
	a.doubtsMu.Lock()
	defer a.doubtsMu.Unlock()
	d := a.doubts[id]
	delete(a.doubts, id)
	return d
}

// PagesToVerify devuelve las páginas transcritas donde la última extracción
// encontró lecturas contradictorias (como máximo 3). La interfaz las vuelve
// a renderizar con más resolución y las envía a VerifyAI.
func (a *App) PagesToVerify(id int64) []int {
	a.doubtsMu.Lock()
	dudas := a.doubts[id]
	a.doubtsMu.Unlock()
	seen := map[int]bool{}
	pages := []int{}
	for _, d := range dudas {
		for _, p := range d.Paginas {
			if seen[p] || len(pages) >= 3 {
				continue
			}
			if _, err := a.store.TranscriptionPage(id, p); err != nil {
				continue // la página tiene texto nativo o no existe
			}
			seen[p] = true
			pages = append(pages, p)
		}
	}
	sort.Ints(pages)
	return pages
}

// PageImage es una página renderizada por la interfaz (JPEG en base64).
type PageImage struct {
	Pagina int    `json:"pagina"`
	Imagen string `json:"imagen"`
}

// VerifyAI revisa con la imagen las dudas de lectura de la última extracción.
// Si la imagen confirma la lectura correcta, corrige la transcripción y vuelve
// a extraer los datos con el texto ya corregido.
func (a *App) VerifyAI(id int64, pages []PageImage) (store.ConventionDetail, error) {
	dudas := a.takeDoubts(id)
	if len(dudas) == 0 || len(pages) == 0 {
		return a.store.Get(id)
	}
	a.cfgMu.Lock()
	cfg := a.cfg
	a.cfgMu.Unlock()
	if !cfg.AIEnabled || cfg.APIKey == "" {
		return store.ConventionDetail{}, errors.New("configure y active la IA antes de revisar")
	}
	client := ai.New(cfg.APIKey, cfg.BaseURL, cfg.Model, cfg.VisionModel)
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		cambios []string
	)
	for _, p := range pages {
		var suyas []ai.Doubt
		for _, d := range dudas {
			for _, n := range d.Paginas {
				if n == p.Pagina {
					suyas = append(suyas, d)
					break
				}
			}
		}
		texto, err := a.store.TranscriptionPage(id, p.Pagina)
		if err != nil || len(suyas) == 0 {
			continue
		}
		wg.Add(1)
		go func(p PageImage, texto string, suyas []ai.Doubt) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
			defer cancel()
			cors, err := client.VerifyPage(ctx, p.Imagen, p.Pagina, texto, suyas)
			if err != nil {
				log.Printf("no se pudo revisar la página %d del convenio %d: %v", p.Pagina, id, err)
				return
			}
			pedidas := make([]store.NameCorrection, 0, len(cors))
			for _, c := range cors {
				pedidas = append(pedidas, store.NameCorrection{Original: c.Original, Correcto: c.Correcto})
			}
			mu.Lock()
			defer mu.Unlock()
			applied, err := a.store.ApplyPageCorrections(id, p.Pagina, pedidas)
			if err != nil {
				log.Printf("no se pudo corregir la página %d del convenio %d: %v", p.Pagina, id, err)
				return
			}
			for _, c := range applied {
				cambios = append(cambios, fmt.Sprintf("«%s» → «%s» (página %d)", c.Original, c.Correcto, p.Pagina))
			}
		}(p, texto, suyas)
	}
	wg.Wait()
	if len(cambios) == 0 {
		return a.store.Get(id)
	}
	sort.Strings(cambios)
	nota := "Verificado con la imagen: " + strings.Join(cambios, "; ") + "."
	return a.extract(id, nota, false)
}

// applyAITags aplica las etiquetas sugeridas (no hay paso de aprobación) y
// completa las obligatorias: la de facultad (o «Convenio Abierto») y el
// ámbito. Un archivo que no parece convenio solo lleva lo que se sugirió.
func (a *App) applyAITags(id int64, hasSuggestions bool, facultad, facultadCiudad string) error {
	a.tagMu.Lock()
	defer a.tagMu.Unlock()
	if hasSuggestions {
		if err := a.store.AcceptSuggestions(id); err != nil {
			return err
		}
	} else {
		_ = a.store.MarkTagsReviewed(id)
	}
	if flagged, err := a.store.IsFlaggedNoConvenio(id); err != nil || flagged {
		return err
	}
	_ = a.applyFacultyTag(id, facultad, facultadCiudad)
	return a.store.EnsureAmbito(id)
}

// repairRequiredTags corrige una sola vez los convenios analizados antes de
// las reglas de ubicación: les agrega el estado de su ciudad, «Nacional», la
// etiqueta de facultad o «Convenio Abierto», y marca los archivos que la IA
// ya había descrito como «no es un convenio».
func (a *App) repairRequiredTags() {
	const flag = "reparacion_etiquetas_obligatorias"
	if done, err := a.store.Flag(flag); err != nil || done {
		return
	}
	if _, err := a.store.FlagNoConveniosFromNotes(); err != nil {
		log.Printf("no se pudieron marcar los archivos que no son convenios: %v", err)
		return
	}
	ids, err := a.store.AnalyzedConventions()
	if err != nil {
		log.Printf("no se pudieron revisar las etiquetas obligatorias: %v", err)
		return
	}
	for _, id := range ids {
		d, err := a.store.Get(id)
		if err != nil {
			continue
		}
		current := make([]int64, 0, len(d.Etiquetas))
		for _, t := range d.Etiquetas {
			current = append(current, t.ID)
		}
		if err := a.store.ReplaceTagsAuto(id, current); err != nil {
			log.Printf("no se pudo corregir la ubicación del convenio %d: %v", id, err)
		}
		_ = a.applyFacultyTag(id, d.Facultad, d.FacultadCiudad)
		if err := a.store.EnsureAmbito(id); err != nil {
			log.Printf("no se pudo agregar el ámbito del convenio %d: %v", id, err)
		}
	}
	_ = a.store.SetFlag(flag)
}

// SetNoConvenio marca un archivo como «no es un convenio» o confirma que sí lo
// es; en ese caso le agrega las etiquetas obligatorias.
func (a *App) SetNoConvenio(id int64, noEs bool) (store.ConventionDetail, error) {
	if err := a.store.SetNoConvenio(id, noEs); err != nil {
		return store.ConventionDetail{}, err
	}
	if !noEs {
		a.tagMu.Lock()
		if d, err := a.store.Get(id); err == nil && (d.IAEstado == "ok" || d.IAEstado == "parcial") {
			_ = a.applyFacultyTag(id, d.Facultad, d.FacultadCiudad)
			_ = a.store.EnsureAmbito(id)
		}
		a.tagMu.Unlock()
	}
	return a.store.Get(id)
}

// applyFacultyTag deja la etiqueta de facultad correcta: la de la facultad
// capturada o, si el convenio no nombra ninguna, "Convenio Abierto" (salvo que
// el usuario haya elegido a mano otra etiqueta de facultad).
func (a *App) applyFacultyTag(id int64, facultad, facultadCiudad string) error {
	if strings.TrimSpace(facultad) == "" {
		return a.store.MarkOpenAgreement(id)
	}
	return a.ensureFacultyTag(id, facultad, facultadCiudad)
}

// markOpenAgreements etiqueta como "Convenio Abierto" los convenios ya
// procesados que no nombran facultad (incluidos los de versiones anteriores).
func (a *App) markOpenAgreements() {
	ids, err := a.store.OpenAgreementCandidates()
	if err != nil {
		log.Printf("no se pudieron revisar convenios abiertos: %v", err)
		return
	}
	for _, id := range ids {
		if err := a.store.MarkOpenAgreement(id); err != nil {
			log.Printf("no se pudo marcar el convenio %d como abierto: %v", id, err)
		}
	}
}

// ensureFacultyTag crea, si hace falta, la etiqueta de tipo facultad con el
// formato "Facultad de <nombre> <ciudad>". La ciudad es la sede de la facultad
// de la UAGro, nunca la ciudad de la dependencia.
func (a *App) ensureFacultyTag(id int64, facultad, facultadCiudad string) error {
	name, city, ok := uagro.Resolve(facultad, facultadCiudad)
	if !ok {
		// El campo no nombra una unidad académica (por ejemplo, "Rectoría"):
		// el convenio es abierto.
		return a.store.MarkOpenAgreement(id)
	}
	if strings.TrimSpace(facultadCiudad) == "" && city != "" {
		_ = a.store.SetFacultyCity(id, city)
	}
	tagName := strings.TrimSpace(name + " " + city)
	tags, err := a.store.TagsForConvention(id)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(tags)+1)
	oldFaculties := make([]int64, 0, 1)
	for _, t := range tags {
		if t.Tipo == "facultad" {
			if strings.EqualFold(t.Nombre, tagName) {
				return nil
			}
			oldFaculties = append(oldFaculties, t.ID)
			continue
		}
		ids = append(ids, t.ID)
	}
	tag, err := a.store.EnsureTag("facultad", tagName)
	if err != nil {
		return err
	}
	ids = append(ids, tag.ID)
	if err := a.store.ReplaceTagsAuto(id, ids); err != nil {
		return err
	}
	for _, old := range oldFaculties {
		_ = a.store.DeleteTagIfUnused(old)
	}
	return nil
}

// applyPendingSuggestions aplica automáticamente sugerencias pendientes de
// versiones anteriores, para que el usuario no tenga que aprobarlas.
func (a *App) applyPendingSuggestions() {
	rows, err := a.store.DB().Query("SELECT id FROM convenios WHERE etiquetas_revisadas=0 AND trim(etiquetas_sugeridas) <> ''")
	if err != nil {
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		if err := a.store.AcceptSuggestions(id); err != nil {
			log.Printf("no se pudieron aplicar sugerencias del convenio %d: %v", id, err)
		}
	}
}

// repairFacultyTags corrige etiquetas antiguas que se construyeron con la
// ciudad de la dependencia (por ejemplo, "Turismo Tijuana").
func (a *App) repairFacultyTags() {
	rows, err := a.store.DB().Query("SELECT id, facultad, facultad_ciudad FROM convenios WHERE trim(facultad) <> ''")
	if err != nil {
		log.Printf("no se pudieron revisar etiquetas de facultad: %v", err)
		return
	}
	type row struct {
		id     int64
		fac    string
		ciudad string
	}
	var pendientes []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.fac, &r.ciudad); err == nil {
			pendientes = append(pendientes, r)
		}
	}
	rows.Close()
	for _, r := range pendientes {
		if err := a.ensureFacultyTag(r.id, r.fac, r.ciudad); err != nil {
			log.Printf("no se pudo corregir la etiqueta de facultad del convenio %d: %v", r.id, err)
		}
	}
}

// BeginTranscription prepara la transcripción de las páginas escaneadas.
func (a *App) BeginTranscription(id int64, total int) error {
	a.cfgMu.Lock()
	cfg := a.cfg
	a.cfgMu.Unlock()
	if !cfg.AIEnabled || cfg.APIKey == "" {
		return errors.New("configure y active la IA antes de transcribir")
	}
	return a.store.BeginTranscription(id, total)
}

// TranscribePage envía una imagen renderizada al modelo de visión y guarda el texto.
func (a *App) TranscribePage(id int64, page int, imageBase64 string) (store.TranscriptionStatus, error) {
	a.cfgMu.Lock()
	cfg := a.cfg
	a.cfgMu.Unlock()
	if !cfg.AIEnabled || cfg.APIKey == "" {
		return store.TranscriptionStatus{}, errors.New("configure y active la IA antes de transcribir")
	}
	client := ai.New(cfg.APIKey, cfg.BaseURL, cfg.Model, cfg.VisionModel)
	client.WebSearch = cfg.WebSearchEnabled
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	texto, err := client.TranscribeImage(ctx, imageBase64, page)
	if err != nil {
		_ = a.store.FailTranscription(id, err.Error())
		return store.TranscriptionStatus{}, err
	}
	return a.store.SaveTranscriptionPage(id, page, texto)
}

// FailTranscription registra un error de transcripción.
func (a *App) FailTranscription(id int64, mensaje string) error {
	return a.store.FailTranscription(id, mensaje)
}

// FinishTranscription cierra el trabajo y actualiza el índice de búsqueda.
func (a *App) FinishTranscription(id int64) (store.TranscriptionStatus, error) {
	st, err := a.store.FinishTranscription(id)
	if err == nil {
		a.refreshDuplicates()
	}
	return st, err
}

// GetTranscription devuelve el estado y el texto del convenio.
func (a *App) GetTranscription(id int64) (store.TranscriptionStatus, error) {
	return a.store.GetTranscription(id)
}

// transcriptionFileName usa el mismo nombre útil del PDF para el .txt.
func (a *App) transcriptionFileName(id int64) string {
	c, err := a.store.Get(id)
	if err != nil {
		return fmt.Sprintf("convenio-%d.txt", id)
	}
	stem := c.ArchivoNombre
	if strings.EqualFold(filepath.Ext(stem), ".pdf") {
		stem = stem[:len(stem)-4]
	}
	return stem + " - texto.txt"
}

// DownloadTranscription guarda el texto transcrito como .txt.
func (a *App) DownloadTranscription(id int64) (string, error) {
	if a.ctx == nil {
		return "", errors.New("la aplicación aún no está lista")
	}
	st, err := a.store.GetTranscription(id)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(st.Texto) == "" {
		return "", errors.New("este convenio no tiene texto para descargar")
	}
	dst, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:            "Descargar transcripción",
		DefaultDirectory: defaultDownloadDir(),
		DefaultFilename:  a.transcriptionFileName(id),
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Texto (*.txt)", Pattern: "*.txt"},
		},
	})
	if err != nil || strings.TrimSpace(dst) == "" {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(dst), ".txt") {
		dst += ".txt"
	}
	if err := os.WriteFile(dst, []byte(st.Texto+"\n"), 0o600); err != nil {
		return "", err
	}
	return dst, nil
}

// BackupData crea un .zip con la base y los PDF.
func (a *App) BackupData() (string, error) {
	if a.ctx == nil {
		return "", errors.New("la aplicación aún no está lista")
	}
	dst, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:            "Guardar respaldo",
		DefaultDirectory: a.paths.Backups,
		DefaultFilename:  "ConveniosUAGro-respaldo-" + time.Now().Format("20060102-1504") + ".zip",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Respaldo ZIP (*.zip)", Pattern: "*.zip"},
		},
	})
	if err != nil || strings.TrimSpace(dst) == "" {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(dst), ".zip") {
		dst += ".zip"
	}
	if err := a.store.BackupZip(dst); err != nil {
		return "", err
	}
	return dst, nil
}

// OpenDataFolder abre la carpeta de datos en el explorador.
func (a *App) OpenDataFolder() error {
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", a.paths.Base)
	case "darwin":
		cmd = exec.Command("open", a.paths.Base)
	default:
		cmd = exec.Command("xdg-open", a.paths.Base)
	}
	return cmd.Start()
}

func defaultDownloadDir() string {
	dir, _ := os.UserHomeDir()
	if download := filepath.Join(dir, "Downloads"); dirExists(download) {
		return download
	}
	return dir
}

// ExportConvenios guarda un paquete .zip con los convenios indicados (todos si
// ids está vacío) para llevarlos a otra instalación.
func (a *App) ExportConvenios(ids []int64) (ExportResult, error) {
	if a.ctx == nil {
		return ExportResult{}, errors.New("la aplicación aún no está lista")
	}
	name := "Convenios-UAGro-" + time.Now().Format("20060102-1504") + ".zip"
	if len(ids) > 0 {
		name = fmt.Sprintf("Convenios-UAGro-%d-seleccionados-%s.zip", len(ids), time.Now().Format("20060102"))
	}
	dst, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:            "Exportar convenios",
		DefaultDirectory: defaultDownloadDir(),
		DefaultFilename:  name,
		Filters:          []wailsruntime.FileFilter{{DisplayName: "Paquete de convenios (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || strings.TrimSpace(dst) == "" {
		return ExportResult{}, err
	}
	if !strings.EqualFold(filepath.Ext(dst), ".zip") {
		dst += ".zip"
	}
	n, err := a.store.ExportPackage(dst, ids, appVersion)
	if err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Path: dst, Total: n}, nil
}

// ExportConveniosCSV guarda una hoja de cálculo con los convenios indicados o,
// si ids está vacío, con todos los que cumplen el filtro actual.
func (a *App) ExportConveniosCSV(ids []int64, f store.ListFilter) (ExportResult, error) {
	if a.ctx == nil {
		return ExportResult{}, errors.New("la aplicación aún no está lista")
	}
	if len(ids) == 0 {
		var err error
		if ids, err = a.store.FilterIDs(f); err != nil {
			return ExportResult{}, err
		}
	}
	if len(ids) == 0 {
		return ExportResult{}, errors.New("no hay convenios para exportar")
	}
	dst, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:            "Exportar lista de convenios",
		DefaultDirectory: defaultDownloadDir(),
		DefaultFilename:  "Convenios-UAGro-" + time.Now().Format("20060102") + ".csv",
		Filters:          []wailsruntime.FileFilter{{DisplayName: "Hoja de cálculo CSV (*.csv)", Pattern: "*.csv"}},
	})
	if err != nil || strings.TrimSpace(dst) == "" {
		return ExportResult{}, err
	}
	if !strings.EqualFold(filepath.Ext(dst), ".csv") {
		dst += ".csv"
	}
	n, err := a.store.ExportCSV(dst, ids)
	if err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Path: dst, Total: n}, nil
}

// ChooseImportFile abre el diálogo para elegir un paquete, un respaldo o una base.
func (a *App) ChooseImportFile() (string, error) {
	if a.ctx == nil {
		return "", errors.New("la aplicación aún no está lista")
	}
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Importar convenios",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Paquete, respaldo o base (*.zip, *.db)", Pattern: "*.zip;*.db"},
		},
	})
}

// ImportConvenios agrega los convenios de un paquete exportado, de un respaldo
// ZIP o de un convenios.db de otra instalación. No borra nada de la base actual.
func (a *App) ImportConvenios(path string) (store.ImportSummary, error) {
	if strings.TrimSpace(path) == "" {
		return store.ImportSummary{}, errors.New("no se eligió ningún archivo")
	}
	sum, err := a.store.ImportPackage(path)
	if err != nil {
		return sum, err
	}
	a.repairFacultyTags()
	a.markOpenAgreements()
	a.refreshDuplicates()
	return sum, nil
}

func (a *App) currentBuild() update.Build {
	b := update.Build{Version: appVersion, Commit: buildCommit}
	if t, err := time.Parse(time.RFC3339, buildDate); err == nil {
		b.Date = t
	}
	return b
}

// CheckUpdate busca una versión nueva publicada desde la rama main.
func (a *App) CheckUpdate() (update.Info, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	info, err := update.New().Check(ctx, a.currentBuild())
	if err == nil {
		a.updMu.Lock()
		a.lastUpdate = &info
		a.updMu.Unlock()
	}
	return info, err
}

// ApplyUpdate descarga la versión nueva, respalda la base, reemplaza el
// ejecutable y reinicia la aplicación. Los datos no se tocan: viven en la
// carpeta de datos, fuera del ejecutable.
func (a *App) ApplyUpdate() error {
	a.updMu.Lock()
	if a.updating {
		a.updMu.Unlock()
		return errors.New("la actualización ya está en curso")
	}
	a.updating = true
	info := a.lastUpdate
	a.updMu.Unlock()
	defer func() {
		a.updMu.Lock()
		a.updating = false
		a.updMu.Unlock()
	}()
	if info == nil || !info.Available {
		fresh, err := a.CheckUpdate()
		if err != nil {
			return err
		}
		info = &fresh
	}
	if !info.Available {
		return errors.New("ya tiene la versión más reciente")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	snapshot := filepath.Join(a.paths.Backups, fmt.Sprintf("antes-de-actualizar-%s-%s.db", appVersion, time.Now().Format("20060102-150405")))
	if err := a.store.SnapshotDB(snapshot); err != nil {
		return fmt.Errorf("no se pudo respaldar la base antes de actualizar: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	newFile := exe + ".new"
	last := -1
	err = update.New().Download(ctx, *info, newFile, func(done, total int64) {
		if total <= 0 || a.ctx == nil {
			return
		}
		if pct := int(done * 100 / total); pct != last {
			last = pct
			wailsruntime.EventsEmit(a.ctx, "update:progress", pct)
		}
	})
	if err != nil {
		return err
	}
	if err := update.Install(exe, newFile); err != nil {
		os.Remove(newFile)
		return fmt.Errorf("%v. Descárguela manualmente desde %s", err, info.URL)
	}
	cmd := exec.Command(exe, afterUpdateFlag)
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("la actualización se instaló, pero no se pudo reiniciar: ábrala de nuevo (%v)", err)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		wailsruntime.Quit(a.ctx)
	}()
	return nil
}

// OpenReleasePage abre en el navegador la página del release de GitHub.
func (a *App) OpenReleasePage(url string) error {
	prefix := fmt.Sprintf("https://github.com/%s/%s/", update.Owner, update.Repo)
	if !strings.HasPrefix(url, prefix) {
		url = prefix + "releases/latest"
	}
	wailsruntime.BrowserOpenURL(a.ctx, url)
	return nil
}

// middleware sirve los PDF locales sin exponer rutas del sistema.
func (a *App) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/pdf/") {
			raw := strings.TrimPrefix(r.URL.Path, "/pdf/")
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			path, original, err := a.store.PDF(id)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": original}))
			http.ServeFile(w, r, path)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Health se usa en pruebas automatizadas.
func (a *App) Health() string {
	return fmt.Sprintf("ok %s %s", appVersion, a.paths.Base)
}

// linkRenewal pasa al historial el convenio anterior que este renueva, según
// lo que dijo la IA. Solo usa candidatos ya propuestos como posible
// renovación (misma contraparte, tipo y facultad) y más antiguos que este.
func (a *App) linkRenewal(id int64, fechaAnterior string) {
	d, err := a.store.Get(id)
	if err != nil || d.FechaFirma == "" {
		return
	}
	var pick int64
	pickDate := ""
	for _, r := range d.Relacionados {
		if r.Motivo != store.MotivoRenovacion || r.FechaFirma == "" || r.FechaFirma >= d.FechaFirma {
			continue
		}
		if fechaAnterior != "" && r.FechaFirma == fechaAnterior {
			pick = r.ID
			break
		}
		if r.FechaFirma > pickDate {
			pick, pickDate = r.ID, r.FechaFirma
		}
	}
	if pick != 0 {
		_ = a.store.LinkVersion(pick, id)
	}
}

// ImportCheck avisa, antes de importar, si un archivo ya está registrado o
// se eligió dos veces.
type ImportCheck struct {
	Path          string `json:"path"`
	Name          string `json:"name"`
	DuplicateID   int64  `json:"duplicate_id,omitempty"`
	DuplicateName string `json:"duplicate_name,omitempty"`
	// Repetido es el nombre del otro archivo de la misma selección con el
	// mismo contenido.
	Repetido string `json:"repetido,omitempty"`
	Error    string `json:"error,omitempty"`
}

// CheckImport calcula el SHA-256 de cada archivo y lo busca en la base.
func (a *App) CheckImport(paths []string) []ImportCheck {
	out := make([]ImportCheck, 0, len(paths))
	seen := map[string]string{}
	for _, path := range paths {
		c := ImportCheck{Path: path, Name: filepath.Base(path)}
		sum, err := fileSHA256(path)
		if err != nil {
			c.Error = "no se pudo leer el archivo"
			out = append(out, c)
			continue
		}
		if first, ok := seen[sum]; ok {
			c.Repetido = first
		} else {
			seen[sum] = c.Name
		}
		if dup, ok, _ := a.store.DuplicateID(sum, 0); ok {
			c.DuplicateID = dup
			if d, err := a.store.Get(dup); err == nil {
				c.DuplicateName = d.Dependencia
				if c.DuplicateName == "" {
					c.DuplicateName = d.ArchivoNombre
				}
			}
		}
		out = append(out, c)
	}
	return out
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ListDuplicates devuelve los pares de posibles duplicados y renovaciones.
func (a *App) ListDuplicates() ([]store.DuplicatePair, error) {
	return a.store.ListDuplicates()
}

// DismissDuplicate marca dos convenios como distintos.
func (a *App) DismissDuplicate(x, y int64) error {
	return a.store.DismissDuplicate(x, y)
}

// LinkVersion pasa older al historial de newer (newer es la versión renovada).
func (a *App) LinkVersion(older, newer int64) error {
	return a.store.LinkVersion(older, newer)
}

// UnlinkVersion saca un convenio del historial y lo vuelve a mostrar como vigente.
func (a *App) UnlinkVersion(id int64) error {
	return a.store.UnlinkVersion(id)
}
