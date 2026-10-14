package advisories

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, d, name, s string) {
	os.MkdirAll(filepath.Dir(filepath.Join(d, name)), 0o755)
	if err := os.WriteFile(filepath.Join(d, name), []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestComponents(t *testing.T) {
	d := t.TempDir()
	write(t, d, "go.mod", "module m\n\nrequire (\n\tgithub.com/a/b v1.2.3\n)\n")
	write(t, d, "app/build.gradle.kts", `implementation("io.ktor:ktor-server-core:2.3.0")
implementation("x:y:$ver")`)
	write(t, d, "gradle/libs.versions.toml", "[versions]\nk = \"1.9.0\"\n[libraries]\nkx = { module = \"org.jetbrains:kx\", version.ref = \"k\" }\n")
	write(t, d, "requirements.txt", "flask==2.0.0\nrequests>=2\n")
	write(t, d, "package.json", `{"dependencies":{"left-pad":"^1.3.0","loose":"*"}}`)
	got := map[string]bool{}
	for _, c := range Components(d) {
		got[c.Ecosystem+" "+c.Name+"@"+c.Version] = true
	}
	for _, w := range []string{"Go github.com/a/b@1.2.3", "Maven io.ktor:ktor-server-core@2.3.0", "Maven org.jetbrains:kx@1.9.0", "PyPI flask@2.0.0", "npm left-pad@1.3.0"} {
		if !got[w] {
			t.Errorf("missing %s in %v", w, got)
		}
	}
	if len(got) != 5 {
		t.Errorf("unpinned or variable versions leaked in: %v", got)
	}
}

func TestMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/querybatch":
			json.NewEncoder(w).Encode(map[string]any{"results": []any{
				map[string]any{"vulns": []any{map[string]string{"id": "GHSA-1"}}}, map[string]any{}}})
		case "/v1/vulns/GHSA-1":
			io := `{"id":"GHSA-1","summary":"bad","database_specific":{"severity":"CRITICAL"},"affected":[{"package":{"name":"flask","ecosystem":"PyPI"},"ranges":[{"events":[{"introduced":"0"},{"fixed":"2.2.5"}]}]}]}`
			w.Write([]byte(io))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	comps := []Component{{"PyPI", "flask", "2.0.0", "requirements.txt"}, {"PyPI", "ok", "1.0.0", "requirements.txt"}}
	fs, err := Client{URL: srv.URL}.Match(context.Background(), comps)
	if err != nil || len(fs) != 1 || fs[0].Severity != 4 || fs[0].Fix != "upgrade to 2.2.5 or later" || fs[0].Evidence != "flask@2.0.0" {
		t.Fatalf("%+v %v", fs, err)
	}
	if _, err := (Client{URL: srv.URL + "/missing"}).Match(context.Background(), comps); err == nil {
		t.Fatal("failed lookup reported as clean")
	}
}
