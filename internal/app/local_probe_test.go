package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

func decodeProbe(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode probe response: %v (%s)", err, w.Body.String())
	}
	return payload
}

func TestLocalProbeRejectsInvalidTargets(t *testing.T) {
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for _, tc := range []struct{ path, verb, field, value string }{
		{"/api/local-probe/nslookup", "nslookup", "host", "bad host;rm"},
		{"/api/local-probe/traceroute", "traceroute", "host", "-h 127.0.0.1"},
		{"/api/local-probe/nslookup", "nslookup", "host", ""},
	} {
		w := issueRequest(t, a, "POST", tc.path, map[string]any{tc.field: tc.value})
		if w.Code != 400 {
			t.Fatalf("%s accepted %q: %s", tc.verb, tc.value, w.Body.String())
		}
	}
	for _, raw := range []string{"ftp://example.com", "file:///etc/passwd", "not a url", "http://"} {
		w := issueRequest(t, a, "POST", "/api/local-probe/curl", map[string]any{"url": raw})
		if w.Code != 400 {
			t.Fatalf("curl accepted %q: %s", raw, w.Body.String())
		}
	}
	for _, tc := range []struct{ field, value string }{
		{"host", "bad host;rm"},
		{"host", ""},
	} {
		w := issueRequest(t, a, "POST", "/api/local-probe/dig", map[string]any{tc.field: tc.value})
		if w.Code != 400 {
			t.Fatalf("dig accepted %q: %s", tc.value, w.Body.String())
		}
	}
	w := issueRequest(t, a, "POST", "/api/local-probe/dig", map[string]any{"host": "example.com", "type": "SVCB"})
	if w.Code != 400 {
		t.Fatalf("dig accepted record type SVCB: %s", w.Body.String())
	}
}

func TestLocalProbeDigIPLiteral(t *testing.T) {
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	w := issueRequest(t, a, "POST", "/api/local-probe/dig", map[string]any{"host": "127.0.0.1", "type": "A"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	payload := decodeProbe(t, w)
	output, _ := payload["output"].(string)
	if payload["ok"] != true || !strings.Contains(output, "IN\tA\t127.0.0.1") || !strings.Contains(output, "QUESTION SECTION") {
		t.Fatalf("unexpected dig output: %q", output)
	}
	w = issueRequest(t, a, "POST", "/api/local-probe/dig", map[string]any{"host": "127.0.0.1", "type": "AAAA"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	payload = decodeProbe(t, w)
	output, _ = payload["output"].(string)
	if !strings.Contains(output, "(无记录)") {
		t.Fatalf("IPv4 literal should have no AAAA answer: %q", output)
	}
}

func TestLocalProbeCurlDefaultsToHTTP(t *testing.T) {
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("scheme-less ok"))
	}))
	defer server.Close()
	// Real curl accepts scheme-less targets and defaults to http; pass the
	// bare host:port (strip the http:// prefix) to prove the same defaulting.
	w := issueRequest(t, a, "POST", "/api/local-probe/curl", map[string]any{"url": strings.TrimPrefix(server.URL, "http://")})
	if w.Code != 200 {
		t.Fatalf("scheme-less curl target rejected: %s", w.Body.String())
	}
	payload := decodeProbe(t, w)
	output, _ := payload["output"].(string)
	if payload["ok"] != true || !strings.Contains(output, "scheme-less ok") {
		t.Fatalf("unexpected curl output: %q", output)
	}
}

func TestLocalProbeNslookupIPLiteral(t *testing.T) {
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	w := issueRequest(t, a, "POST", "/api/local-probe/nslookup", map[string]any{"host": "127.0.0.1"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	payload := decodeProbe(t, w)
	output, _ := payload["output"].(string)
	if payload["ok"] != true || !strings.Contains(output, "127.0.0.1") || !strings.Contains(output, "本身即 IP") {
		t.Fatalf("unexpected nslookup output: %q", output)
	}
}

func TestLocalProbeCurlFetchesLoopback(t *testing.T) {
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Probe", "yes")
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write([]byte("hello from probe"))
	}))
	defer server.Close()
	w := issueRequest(t, a, "POST", "/api/local-probe/curl", map[string]any{"url": server.URL})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	payload := decodeProbe(t, w)
	output, _ := payload["output"].(string)
	if payload["ok"] != true || !strings.Contains(output, "200") || !strings.Contains(output, "hello from probe") || !strings.Contains(output, "X-Probe: yes") {
		t.Fatalf("unexpected curl output: %q", output)
	}
	w = issueRequest(t, a, "POST", "/api/local-probe/curl", map[string]any{"url": server.URL, "head": true})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	head := decodeProbe(t, w)
	headOutput, _ := head["output"].(string)
	if !strings.Contains(headOutput, "200") || strings.Contains(headOutput, "hello from probe") {
		t.Fatalf("HEAD curl should show headers only: %q", headOutput)
	}
}

func TestLocalProbeTracerouteStreamsNDJSON(t *testing.T) {
	name := "traceroute"
	if runtime.GOOS == "windows" {
		name = "tracert"
	}
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not installed", name)
	}
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	data, _ := json.Marshal(map[string]any{"host": "127.0.0.1"})
	req := httptest.NewRequest("POST", "http://wails.localhost/api/local-probe/traceroute", bytes.NewReader(data))
	req.Header.Set("X-CloudShell-Token", a.Token())
	req.Header.Set("X-DevShell-Probe-Stream", "1")
	w := httptest.NewRecorder()
	a.Handler(fstest.MapFS{"index.html": {Data: []byte("fixture")}}).ServeHTTP(w, req)
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/x-ndjson") {
		t.Fatalf("stream traceroute content type = %q body=%s", ct, w.Body.String())
	}
	var output strings.Builder
	done := false
	for _, line := range strings.Split(w.Body.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var frame map[string]any
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("frame %q: %v", line, err)
		}
		if chunk, ok := frame["c"].(string); ok {
			output.WriteString(chunk)
		}
		if frame["done"] == true {
			done = true
		}
	}
	if !done {
		t.Fatalf("missing done frame: %s", w.Body.String())
	}
	if !strings.Contains(output.String(), "127.0.0.1") {
		t.Fatalf("no route output streamed: %q", output.String())
	}
}
