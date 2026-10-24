package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.0", "1.1.6", 1}, {"v1.1.6", "1.1.6", 0}, {"1.1.6", "1.10.0", -1}, {"2", "1.9.9", 1}, {"1.2.0-beta", "1.2.0", 0},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	built := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	local := Build{Version: "1.2.0", Commit: "aaaaaaa", Date: built}
	if !isNewer(local, "1.2.1", "", time.Time{}) {
		t.Fatal("una versión mayor debe ser nueva")
	}
	if isNewer(local, "1.1.9", "bbbbbbb", built.Add(time.Hour)) {
		t.Fatal("una versión menor no debe ser nueva")
	}
	if !isNewer(local, "1.2.0", "bbbbbbb", built.Add(time.Hour)) {
		t.Fatal("un commit nuevo de la misma versión debe ser nuevo")
	}
	if isNewer(local, "1.2.0", "aaaaaaa1234", built.Add(time.Hour)) {
		t.Fatal("el mismo commit no es una actualización")
	}
	if isNewer(local, "1.2.0", "bbbbbbb", built.Add(-time.Hour)) {
		t.Fatal("un release anterior a la compilación local no es nuevo")
	}
	if isNewer(Build{Version: "1.2.0"}, "1.2.0", "bbbbbbb", built) {
		t.Fatal("una compilación local sin commit solo compara versiones")
	}
}

func TestCheckAndDownload(t *testing.T) {
	exe := append([]byte("MZ"), []byte(strings.Repeat("x", 4096))...)
	sum := sha256.Sum256(exe)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"v1.3.0","html_url":"https://github.com/o/r/releases/tag/v1.3.0",
				"published_at":"2026-10-20T10:00:00Z",
				"body":"Cambios recientes:\n- feat: algo\n\ncommit: 0123456789abcdef\nsha256: %s",
				"assets":[{"name":"ConveniosUAGro.exe","size":%d,"browser_download_url":"%s/dl"}]}`,
				hex.EncodeToString(sum[:]), len(exe), srv.URL)
		case "/repos/o/r/commits/main":
			io.WriteString(w, `{"sha":"fedcba9876543210"}`)
		case "/dl":
			w.Write(exe)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := &Checker{HTTP: srv.Client(), APIBase: srv.URL, Owner: "o", Repo: "r"}
	info, err := c.Check(context.Background(), Build{Version: "1.2.0", Commit: "1111111"})
	if err != nil {
		t.Fatal(err)
	}
	if !info.Available || !info.Pending || info.Version != "1.3.0" || info.Commit != "0123456789abcdef" {
		t.Fatalf("info = %+v", info)
	}
	if strings.Contains(info.Notes, "sha256") || !strings.Contains(info.Notes, "feat: algo") {
		t.Fatalf("notas = %q", info.Notes)
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "app.exe.new")
	if err := c.Download(context.Background(), info, dst, nil); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(dir, "app.exe")
	os.WriteFile(current, []byte("MZviejo"), 0o700)
	if err := Install(current, dst); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(current)
	if string(got) != string(exe) {
		t.Fatal("el ejecutable no se reemplazó")
	}
	Cleanup(current)
	if _, err := os.Stat(current + ".old"); !os.IsNotExist(err) {
		t.Fatal("no se borró el .old")
	}

	info.sha256 = strings.Repeat("0", 64)
	if err := c.Download(context.Background(), info, dst, nil); err == nil {
		t.Fatal("se aceptó una descarga con SHA-256 distinto")
	}
}

func TestCheckWithoutReleases(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	c := &Checker{HTTP: srv.Client(), APIBase: srv.URL, Owner: "o", Repo: "r"}
	info, err := c.Check(context.Background(), Build{Version: "1.2.0"})
	if err != nil || info.Available {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}
