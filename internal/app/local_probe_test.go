package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func decodeProbe(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode probe response: %v (%s)", err, w.Body.String())
	}
	return payload
}

// The route-tracing deadline must come from the tool's own hop budget: a full
// 20-hop trace of a filtered path is legitimate and must not be cut short.
func TestTracerouteDeadlineCoversTheToolHopBudget(t *testing.T) {
	name, args, deadline := tracerouteProbeCommand("192.0.2.1")
	wantName, wantWait, hopFlag := "traceroute", traceroutePosixWait, "-m"
	if runtime.GOOS == "windows" {
		wantName, wantWait, hopFlag = "tracert", tracerouteWindowsWait, "-h"
	}
	if name != wantName {
		t.Fatalf("probe tool = %s, want %s", name, wantName)
	}
	if len(args) == 0 || args[len(args)-1] != "192.0.2.1" {
		t.Fatalf("the validated host must stay the final argument: %v", args)
	}
	hops := ""
	for index, argument := range args {
		if argument == hopFlag && index+1 < len(args) {
			hops = args[index+1]
		}
	}
	if hops != strconv.Itoa(tracerouteMaximumHops) {
		t.Fatalf("%s %q must carry the hop limit the deadline derives from", name, args)
	}
	want := time.Duration(tracerouteMaximumHops*tracerouteProbesPerHop)*wantWait + tracerouteProbeSlack
	if deadline != want {
		t.Fatalf("deadline = %v, want %v (%d hops x %d probes x %v plus slack)", deadline, want, tracerouteMaximumHops, tracerouteProbesPerHop, wantWait)
	}
	if deadline <= 45*time.Second {
		t.Fatalf("deadline %v still cuts off a full-length trace", deadline)
	}
	if note := tracerouteDeadlineNote(deadline); !strings.Contains(note, fmt.Sprintf("%d 秒", int(deadline/time.Second))) {
		t.Fatalf("the stop note must state the real limit: %q", note)
	}
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

// The streamed stop note must describe the deadline actually in force, and a
// trace that finishes inside its budget must not be described as cut short.
func TestTracerouteStreamNoteMatchesItsDeadline(t *testing.T) {
	probe, args := "", []string(nil)
	switch runtime.GOOS {
	case "windows":
		probe, args = "cmd.exe", []string{"/c", "ping -n 4 127.0.0.1 >nul & echo hop-late"}
	case "darwin", "linux":
		probe, args = "/bin/sh", []string{"-c", "sleep 2; echo hop-late"}
	default:
		t.Skipf("no portable slow probe tool on %s", runtime.GOOS)
	}
	path, err := exec.LookPath(probe)
	if err != nil {
		t.Skipf("%s not installed", probe)
	}
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	streamProbe := func(deadline time.Duration) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), deadline)
		defer cancel()
		w := httptest.NewRecorder()
		a.runTracerouteStream(ctx, w, w, probe, path, args, deadline)
		var output strings.Builder
		for _, line := range strings.Split(w.Body.String(), "\n") {
			var frame map[string]any
			if json.Unmarshal([]byte(strings.TrimSpace(line)), &frame) == nil {
				if chunk, ok := frame["c"].(string); ok {
					output.WriteString(chunk)
				}
			}
		}
		return output.String()
	}
	if output := streamProbe(time.Minute); !strings.Contains(output, "hop-late") || strings.Contains(output, "上限") {
		t.Fatalf("a trace inside its budget was cut short: %q", output)
	}
	if output := streamProbe(time.Second); !strings.Contains(output, "1 秒上限") || strings.Contains(output, "45 秒") {
		t.Fatalf("the stop note must state the live deadline: %q", output)
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
