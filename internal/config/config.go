// Package config guarda la configuración local del usuario.
// El archivo se mantiene fuera de la base de datos para no mezclar datos con ajustes.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultBaseURL = "https://api.deepseek.com"
	DefaultModel   = "deepseek-flash"
	// DefaultAnalysisLevel es el razonamiento bajo: en las pruebas extrajo los
	// mismos datos que el medio en un tercio del tiempo y de los tokens.
	DefaultAnalysisLevel = "low"
)

var analysisLevels = map[string]bool{"low": true, "medium": true, "high": true}

// Config es lo que se persiste en settings.json.
type Config struct {
	AIEnabled        bool   `json:"ai_enabled"`
	WebSearchEnabled bool   `json:"web_search_enabled"`
	APIKey           string `json:"api_key"`
	BaseURL          string `json:"base_url"`
	Model            string `json:"model"`
	VisionModel      string `json:"vision_model"`
	AnalysisLevel    string `json:"analysis_level"`
	UpdateCheck      bool   `json:"update_check"`
}

// View es la representación segura para la interfaz: nunca expone la clave completa.
type View struct {
	AIEnabled        bool   `json:"ai_enabled"`
	WebSearchEnabled bool   `json:"web_search_enabled"`
	HasAPIKey        bool   `json:"has_api_key"`
	APIKeyHint       string `json:"api_key_hint"`
	BaseURL          string `json:"base_url"`
	Model            string `json:"model"`
	VisionModel      string `json:"vision_model"`
	AnalysisLevel    string `json:"analysis_level"`
	UpdateCheck      bool   `json:"update_check"`
}

// SaveInput contiene solo los campos que el usuario puede cambiar.
type SaveInput struct {
	AIEnabled        *bool  `json:"ai_enabled"`
	WebSearchEnabled *bool  `json:"web_search_enabled"`
	APIKey           string `json:"api_key"`
	ClearAPIKey      bool   `json:"clear_api_key"`
	BaseURL          string `json:"base_url"`
	Model            string `json:"model"`
	VisionModel      string `json:"vision_model"`
	AnalysisLevel    string `json:"analysis_level"`
	UpdateCheck      *bool  `json:"update_check"`
}

func defaults() Config {
	return Config{
		AIEnabled:        false,
		WebSearchEnabled: true,
		BaseURL:          DefaultBaseURL,
		Model:            DefaultModel,
		VisionModel:      DefaultModel,
		AnalysisLevel:    DefaultAnalysisLevel,
		UpdateCheck:      true,
	}
}

// Default devuelve una configuración inicial sin clave.
func Default() Config { return defaults() }

// Load lee el archivo; si no existe, lo crea con valores iniciales.
func Load(path string) (Config, error) {
	cfg := defaults()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := Save(path, cfg); err != nil {
			return cfg, err
		}
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaults(), err
	}
	if !strings.Contains(string(data), `"web_search_enabled"`) {
		cfg.WebSearchEnabled = true
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = DefaultModel
	}
	if strings.TrimSpace(cfg.VisionModel) == "" {
		cfg.VisionModel = cfg.Model
	}
	if !analysisLevels[cfg.AnalysisLevel] {
		cfg.AnalysisLevel = DefaultAnalysisLevel
	}
	return cfg, nil
}

// Save escribe el archivo de forma atómica y con permisos restrictivos.
func Save(path string, cfg Config) error {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	cfg.Model = strings.TrimSpace(cfg.Model)
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	cfg.VisionModel = strings.TrimSpace(cfg.VisionModel)
	if cfg.VisionModel == "" {
		cfg.VisionModel = cfg.Model
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// Apply valida y aplica los cambios enviados por la interfaz.
func (c *Config) Apply(in SaveInput) error {
	if in.AIEnabled != nil {
		c.AIEnabled = *in.AIEnabled
	}
	if in.WebSearchEnabled != nil {
		c.WebSearchEnabled = *in.WebSearchEnabled
	}
	if in.ClearAPIKey {
		c.APIKey = ""
	} else if strings.TrimSpace(in.APIKey) != "" {
		c.APIKey = strings.TrimSpace(in.APIKey)
	}
	if strings.TrimSpace(in.BaseURL) != "" {
		c.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	}
	if strings.TrimSpace(in.Model) != "" {
		c.Model = strings.TrimSpace(in.Model)
	}
	if strings.TrimSpace(in.VisionModel) != "" {
		c.VisionModel = strings.TrimSpace(in.VisionModel)
	}
	if level := strings.TrimSpace(in.AnalysisLevel); level != "" {
		if !analysisLevels[level] {
			return errors.New("el nivel de análisis no es válido")
		}
		c.AnalysisLevel = level
	}
	if in.UpdateCheck != nil {
		c.UpdateCheck = *in.UpdateCheck
	}
	if c.VisionModel == "" {
		c.VisionModel = c.Model
	}
	if c.BaseURL == "" || (!strings.HasPrefix(c.BaseURL, "https://") && !strings.HasPrefix(c.BaseURL, "http://")) {
		return errors.New("la dirección del servicio de IA debe comenzar con https:// o http://")
	}
	if strings.ContainsAny(c.BaseURL, " \t\r\n") {
		return errors.New("la dirección del servicio de IA no es válida")
	}
	if c.Model == "" {
		return errors.New("indique el modelo de IA")
	}
	if c.VisionModel == "" {
		return errors.New("indique el modelo de visión")
	}
	return nil
}

// ToView oculta la clave y muestra solo una pista.
func (c Config) ToView() View {
	hint := ""
	if c.APIKey != "" {
		if len(c.APIKey) > 4 {
			hint = "••••••••" + c.APIKey[len(c.APIKey)-4:]
		} else {
			hint = "••••"
		}
	}
	return View{
		AIEnabled:        c.AIEnabled,
		WebSearchEnabled: c.WebSearchEnabled,
		HasAPIKey:        c.APIKey != "",
		APIKeyHint:       hint,
		BaseURL:          c.BaseURL,
		Model:            c.Model,
		VisionModel:      c.VisionModel,
		AnalysisLevel:    c.AnalysisLevel,
		UpdateCheck:      c.UpdateCheck,
	}
}
