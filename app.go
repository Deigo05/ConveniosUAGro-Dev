package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"convenios-uagro-desktop/internal/ai"
	"convenios-uagro-desktop/internal/config"
	"convenios-uagro-desktop/internal/pdfutil"
	"convenios-uagro-desktop/internal/store"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const appVersion = "1.1.3"

// App es el puente entre la interfaz y la base local.
type App struct {
	ctx   context.Context
	store *store.Store
	paths store.Paths
	cfg   config.Config

	cfgMu sync.Mutex
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
}

func NewApp() (*App, error) {
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
}

func (a *App) shutdown(ctx context.Context) {
	if a.store != nil {
		_ = a.store.Close()
	}
}

func (a *App) appInfo() store.AppInfo {
	return store.AppInfo{
		Version:   appVersion,
		DataDir:   a.paths.Base,
		Database:  a.paths.DB,
		GoVersion: goruntime.Version(),
	}
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
	if err := a.store.Update(in); err != nil {
		return store.ConventionDetail{}, err
	}
	return a.store.Get(in.ID)
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
		}
		results = append(results, r)
	}
	return results
}

// SaveConvenioTags reemplaza las etiquetas y marca la clasificación como revisada.
func (a *App) SaveConvenioTags(id int64, tagIDs []int64) (store.ConventionDetail, error) {
	if err := a.store.ReplaceTags(id, tagIDs, true); err != nil {
		return store.ConventionDetail{}, err
	}
	return a.store.Get(id)
}

// AcceptSuggestions convierte las sugerencias de IA en etiquetas.
func (a *App) AcceptSuggestions(id int64) (store.ConventionDetail, error) {
	if err := a.store.AcceptSuggestions(id); err != nil {
		return store.ConventionDetail{}, err
	}
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
	return nil
}

// DownloadPDF abre el diálogo Guardar como y copia el PDF elegido.
func (a *App) DownloadPDF(id int64) (string, error) {
	if a.ctx == nil {
		return "", errors.New("la aplicación aún no está lista")
	}
	src, original, err := a.store.PDF(id)
	if err != nil {
		return "", err
	}
	defaultDir, _ := os.UserHomeDir()
	if download := filepath.Join(defaultDir, "Downloads"); dirExists(download) {
		defaultDir = download
	}
	dst, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:            "Descargar PDF",
		DefaultDirectory: defaultDir,
		DefaultFilename:  safeDownloadName(original),
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

func safeDownloadName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "convenio.pdf"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "*", "-", "?", "", "\"", "", "<", "", ">", "", "|", "-")
	name = replacer.Replace(name)
	if !strings.EqualFold(filepath.Ext(name), ".pdf") {
		name += ".pdf"
	}
	return name
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
		catalogo = append(catalogo, ai.Tag{Nombre: t.Nombre, Tipo: t.Tipo})
	}
	client := ai.New(cfg.APIKey, cfg.BaseURL, cfg.Model, cfg.VisionModel)
	client.WebSearch = cfg.WebSearchEnabled
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	res, err := client.Extract(ctx, texto, catalogo)
	if err != nil {
		_ = a.store.SetIAStatus(id, "error", err.Error())
		return store.ConventionDetail{}, err
	}
	in := store.ConventionInput{
		ID:                  id,
		Dependencia:         res.Dependencia,
		FirmanteDependencia: res.FirmanteDependencia,
		CargoDependencia:    res.CargoDependencia,
		FirmanteUAGro:       res.FirmanteUAGro,
		CargoUAGro:          res.CargoUAGro,
		Facultad:            res.Facultad,
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
	if err := a.store.SetAIResult(id, in, res.Estado, sugs, ""); err != nil {
		return store.ConventionDetail{}, err
	}
	if strings.TrimSpace(res.Facultad) != "" {
		_ = a.ensureFacultyTag(id, res.Facultad)
	}
	return a.store.Get(id)
}

// ensureFacultyTag crea, si hace falta, una etiqueta de tema con el nombre de
// la facultad y la agrega al convenio sin borrar las etiquetas existentes.
func (a *App) ensureFacultyTag(id int64, facultad string) error {
	facultad = strings.TrimSpace(facultad)
	if facultad == "" {
		return nil
	}
	tags, err := a.store.TagsForConvention(id)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(tags)+1)
	for _, t := range tags {
		if t.Tipo == "tema" && strings.EqualFold(t.Nombre, facultad) {
			return nil
		}
		ids = append(ids, t.ID)
	}
	all, err := a.store.ListTags()
	if err != nil {
		return err
	}
	var tag store.Tag
	found := false
	for _, t := range all {
		if t.Tipo == "tema" && strings.EqualFold(t.Nombre, facultad) {
			tag, found = t, true
			break
		}
	}
	if !found {
		tag, err = a.store.CreateTag(store.TagInput{Nombre: facultad, Tipo: "tema", Color: "#3A5A9C"})
		if err != nil {
			return err
		}
	}
	ids = append(ids, tag.ID)
	return a.store.ReplaceTags(id, ids, false)
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
	return a.store.FinishTranscription(id)
}

// GetTranscription devuelve el estado y el texto del convenio.
func (a *App) GetTranscription(id int64) (store.TranscriptionStatus, error) {
	return a.store.GetTranscription(id)
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
	defaultDir, _ := os.UserHomeDir()
	if download := filepath.Join(defaultDir, "Downloads"); dirExists(download) {
		defaultDir = download
	}
	dst, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:            "Descargar transcripción",
		DefaultDirectory: defaultDir,
		DefaultFilename:  fmt.Sprintf("convenio-%d.txt", id),
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
