// Package update busca versiones nuevas en GitHub y reemplaza el ejecutable.
//
// Cada push a main compila la aplicación en GitHub Actions y publica (o
// reemplaza) el release v<VERSION> con el ejecutable y, en el texto del
// release, las líneas "commit: <sha>" y "sha256: <hash>". La aplicación
// compara su versión y su commit con ese release.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	Owner     = "Deigo05"
	Repo      = "ConveniosUAGro-Dev"
	AssetName = "ConveniosUAGro.exe"
	maxAsset  = 300 << 20
)

// Build describe el ejecutable que está corriendo.
type Build struct {
	Version string
	Commit  string
	Date    time.Time
}

// Info es el resultado de buscar actualizaciones.
type Info struct {
	Available      bool   `json:"available"`
	Pending        bool   `json:"pending"`
	CurrentVersion string `json:"current_version"`
	CurrentCommit  string `json:"current_commit"`
	Version        string `json:"version"`
	Commit         string `json:"commit"`
	Published      string `json:"published"`
	Notes          string `json:"notes"`
	URL            string `json:"url"`
	Size           int64  `json:"size"`
	CheckedAt      string `json:"checked_at"`

	downloadURL string
	sha256      string
}

// Checker consulta la API de GitHub. APIBase se puede cambiar en pruebas.
type Checker struct {
	HTTP    *http.Client
	APIBase string
	Owner   string
	Repo    string
}

// New devuelve un Checker para el repositorio de la aplicación.
func New() *Checker {
	return &Checker{
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		APIBase: "https://api.github.com",
		Owner:   Owner,
		Repo:    Repo,
	}
}

var (
	reCommit = regexp.MustCompile(`(?mi)^\s*commit:\s*([0-9a-f]{7,40})\s*$`)
	reSHA    = regexp.MustCompile(`(?mi)^\s*sha256:\s*([0-9a-f]{64})\s*$`)
)

type release struct {
	TagName     string `json:"tag_name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		Size               int64  `json:"size"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Digest             string `json:"digest"`
	} `json:"assets"`
}

// Check compara el ejecutable actual con el último release publicado.
func (c *Checker) Check(ctx context.Context, local Build) (Info, error) {
	info := Info{CurrentVersion: local.Version, CurrentCommit: local.Commit, CheckedAt: time.Now().Format(time.RFC3339)}
	var rel release
	status, err := c.getJSON(ctx, fmt.Sprintf("/repos/%s/%s/releases/latest", c.Owner, c.Repo), &rel)
	if err != nil {
		return info, err
	}
	if status == http.StatusNotFound {
		return info, nil // todavía no hay versiones publicadas
	}
	info.Version = strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
	info.URL = rel.HTMLURL
	info.Published = rel.PublishedAt
	if m := reCommit.FindStringSubmatch(rel.Body); m != nil {
		info.Commit = strings.ToLower(m[1])
	}
	if m := reSHA.FindStringSubmatch(rel.Body); m != nil {
		info.sha256 = strings.ToLower(m[1])
	}
	info.Notes = cleanNotes(rel.Body)
	for _, a := range rel.Assets {
		if strings.EqualFold(a.Name, AssetName) {
			info.downloadURL = a.BrowserDownloadURL
			info.Size = a.Size
			if info.sha256 == "" && strings.HasPrefix(a.Digest, "sha256:") {
				info.sha256 = strings.ToLower(strings.TrimPrefix(a.Digest, "sha256:"))
			}
		}
	}
	published, _ := time.Parse(time.RFC3339, rel.PublishedAt)
	info.Available = info.downloadURL != "" && isNewer(local, info.Version, info.Commit, published)

	// Si main ya tiene commits que aún no se compilan, se avisa que viene una
	// versión en camino. Un error aquí no impide informar del release.
	var head struct {
		SHA string `json:"sha"`
	}
	if st, err := c.getJSON(ctx, fmt.Sprintf("/repos/%s/%s/commits/main", c.Owner, c.Repo), &head); err == nil && st == http.StatusOK {
		h := strings.ToLower(head.SHA)
		info.Pending = h != "" && !sameCommit(h, info.Commit) && !sameCommit(h, local.Commit)
	}
	return info, nil
}

func (c *Checker) getJSON(ctx context.Context, path string, v interface{}) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.APIBase, "/")+path, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "ConveniosUAGro-actualizador")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, errors.New("no se pudo consultar GitHub; revise la conexión a internet")
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return resp.StatusCode, nil
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		return resp.StatusCode, errors.New("GitHub limitó temporalmente las consultas; intente más tarde")
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return resp.StatusCode, fmt.Errorf("GitHub respondió con error (%d)", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(v); err != nil {
		return resp.StatusCode, errors.New("GitHub devolvió una respuesta inesperada")
	}
	return resp.StatusCode, nil
}

// isNewer decide si el release es más reciente que el ejecutable. Con la
// misma versión, un commit distinto publicado después de compilar el
// ejecutable también cuenta (cada push a main reemplaza el release).
func isNewer(local Build, version, commit string, published time.Time) bool {
	switch CompareVersions(version, local.Version) {
	case 1:
		return true
	case -1:
		return false
	}
	if commit == "" || local.Commit == "" || sameCommit(commit, local.Commit) {
		return false
	}
	if local.Date.IsZero() || published.IsZero() {
		return true
	}
	return published.After(local.Date)
}

func sameCommit(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	if a == "" || b == "" {
		return false
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

// CompareVersions compara versiones "1.2.3": -1, 0 o 1.
func CompareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] > pb[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}

func versionParts(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	for i, p := range strings.SplitN(v, ".", 3) {
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

func cleanNotes(body string) string {
	body = reCommit.ReplaceAllString(body, "")
	body = reSHA.ReplaceAllString(body, "")
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	if r := []rune(body); len(r) > 4000 {
		body = string(r[:4000]) + "…"
	}
	return body
}

// Download guarda el ejecutable del release en dst y verifica su SHA-256.
func (c *Checker) Download(ctx context.Context, info Info, dst string, progress func(done, total int64)) error {
	if info.downloadURL == "" {
		return errors.New("el release no incluye el ejecutable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ConveniosUAGro-actualizador")
	client := &http.Client{Timeout: 15 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("no se pudo descargar la actualización")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("la descarga respondió con error (%d)", resp.StatusCode)
	}
	total := resp.ContentLength
	if total > maxAsset {
		return errors.New("la actualización es demasiado grande")
	}
	f, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("no se pudo escribir la actualización: %w", err)
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(dst)
		}
	}()
	hash := sha256.New()
	var done int64
	buf := make([]byte, 256<<10)
	body := io.LimitReader(resp.Body, maxAsset+1)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			hash.Write(buf[:n])
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return errors.New("la descarga se interrumpió")
		}
	}
	if done > maxAsset || done < 1024 {
		return errors.New("la actualización descargada no es válida")
	}
	if info.sha256 != "" && hex.EncodeToString(hash.Sum(nil)) != info.sha256 {
		return errors.New("la actualización descargada no coincide con la publicada (SHA-256)")
	}
	if err := f.Close(); err != nil {
		return err
	}
	head := make([]byte, 2)
	if rf, err := os.Open(dst); err == nil {
		_, _ = io.ReadFull(rf, head)
		rf.Close()
	}
	if string(head) != "MZ" {
		return errors.New("la actualización descargada no es un ejecutable de Windows")
	}
	ok = true
	return nil
}

// Install reemplaza exe por newFile. Windows permite renombrar un ejecutable
// en uso, así que el actual se aparta como .old y se borra al siguiente inicio.
func Install(exe, newFile string) error {
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("no se pudo reemplazar el ejecutable (¿carpeta protegida?): %w", err)
	}
	if err := os.Rename(newFile, exe); err != nil {
		_ = os.Rename(old, exe)
		return fmt.Errorf("no se pudo instalar la actualización: %w", err)
	}
	return nil
}

// Cleanup borra los restos de una actualización anterior.
func Cleanup(exe string) {
	_ = os.Remove(exe + ".old")
	_ = os.Remove(exe + ".new")
}
