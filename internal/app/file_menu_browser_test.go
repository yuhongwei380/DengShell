package app

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The right-click menu is the only place that offers file commands, and it keeps
// direct download apart from the archive. The same page also proves that the
// lrzsz bridge (rz / sz) reaches the real upload and cancel globals. This needs
// no SSH or SFTP fixture: the page fabricates a session and stubs the desktop
// download bridge.
// DENGSHELL_PUPPETEER points to puppeteer-core's ESM entry point.
func TestFileMenuBrowser(t *testing.T) {
	module := os.Getenv("DENGSHELL_PUPPETEER")
	if module == "" {
		t.Skip("set DENGSHELL_PUPPETEER for browser integration")
	}
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	server := httptest.NewUnstartedServer(a.Handler(os.DirFS("../../web")))
	defer server.Close()
	a.baseURL = "http://" + server.Listener.Addr().String()
	server.Start()
	script, err := filepath.Abs("../../scripts/tests/test-file-menu-browser.mjs")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", script, a.BrowserURL(), module)
	output, err := cmd.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
}
