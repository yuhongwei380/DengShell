package app

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Exercise actual header geometry and the saved-profile actions in an isolated
// browser. No real connections, user profiles, or remote servers are used.
func TestPinnedNavBrowser(t *testing.T) {
	runIsolatedNavigationBrowser(t, "test-pinned-nav-browser.mjs")
}

func TestSessionNavigationBrowser(t *testing.T) {
	runIsolatedNavigationBrowser(t, "test-session-navigation-browser.mjs")
}

func runIsolatedNavigationBrowser(t *testing.T, scriptName string) {
	t.Helper()
	module, chrome := os.Getenv("DENGSHELL_PUPPETEER"), os.Getenv("CHROME_BIN")
	if module == "" || chrome == "" {
		t.Skip("set DENGSHELL_PUPPETEER and CHROME_BIN for pinned bar browser integration")
	}
	root := t.TempDir()
	a, err := New(filepath.Join(root, "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	server := httptest.NewUnstartedServer(a.Handler(os.DirFS("../../web")))
	a.baseURL = "http://" + server.Listener.Addr().String()
	server.Start()
	defer server.Close()
	fixture, err := json.Marshal(map[string]string{"url": a.BrowserURL(), "token": a.Token()})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "browser-fixture.json"), fixture, 0600); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs(filepath.Join("../../scripts/tests", scriptName))
	if err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("node", script, root, chrome, module).CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
}
