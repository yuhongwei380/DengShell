package app

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSerialProfileNormalizationAndValidation(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Save(Profile{Name: "Console", Protocol: "serial", SerialPort: "  COM7  "}, false)
	if err != nil {
		t.Fatal(err)
	}
	// Host mirrors the device name so search and history keep working.
	if p.SerialPort != "COM7" || p.Host != "COM7" {
		t.Fatalf("serial port not mirrored: %+v", p)
	}
	if p.BaudRate != 115200 || p.DataBits != 8 || p.Parity != "none" || p.StopBits != 1 || p.FlowControl != "none" {
		t.Fatalf("serial defaults missing: %+v", p)
	}
	if p.User != "" || p.Port != 0 || p.Auth != "" || p.ProxyID != "" || p.Protocol != "serial" {
		t.Fatalf("network fields leaked into serial profile: %+v", p)
	}
	again, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := again.Get(p.ID)
	if err != nil || got.SerialPort != "COM7" || got.BaudRate != 115200 {
		t.Fatal("serial profile did not roundtrip", err)
	}
	// A serial profile is valid without user, port, or auth fields.
	if _, err := again.Save(Profile{Name: "USB", Protocol: "serial", SerialPort: "/dev/ttyUSB0", BaudRate: 9600, Parity: "even", StopBits: 2, FlowControl: "rtscts"}, false); err != nil {
		t.Fatal("valid serial profile rejected:", err)
	}
	for _, bad := range []Profile{
		{Name: "bad", Protocol: "serial"},
		{Name: "bad", Protocol: "serial", SerialPort: "COM1", BaudRate: 99},
		{Name: "bad", Protocol: "serial", SerialPort: "COM1", BaudRate: 5000000},
		{Name: "bad", Protocol: "serial", SerialPort: "COM1", DataBits: 9},
		{Name: "bad", Protocol: "serial", SerialPort: "COM1", Parity: "crc"},
		{Name: "bad", Protocol: "serial", SerialPort: "COM1", StopBits: 3},
		{Name: "bad", Protocol: "serial", SerialPort: "COM1", FlowControl: "xoff"},
		{Name: "", Protocol: "serial", SerialPort: "COM1"},
		{Name: "bad", Protocol: "serial", SerialPort: "COM1\nCOM2"},
	} {
		if _, err := again.Save(bad, false); err == nil {
			t.Fatalf("accepted invalid serial profile %+v", bad)
		}
	}
}

func TestSerialHTTPAndStartGuards(t *testing.T) {
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	handler := a.Handler(fstest.MapFS{"index.html": {Data: []byte("fixture")}})
	do := func(method, path string, input any) *httptest.ResponseRecorder {
		t.Helper()
		var body []byte
		if input != nil {
			body, _ = json.Marshal(input)
		}
		req := httptest.NewRequest(method, "http://wails.localhost"+path, bytes.NewReader(body))
		req.Header.Set("X-CloudShell-Token", a.Token())
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	response := do("GET", "/api/serial/ports", nil)
	if response.Code != 200 {
		t.Fatalf("serial port enumeration: %d %s", response.Code, response.Body)
	}
	var listing struct {
		Ports    []string     `json:"ports"`
		Sessions []SerialInfo `json:"sessions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if listing.Ports == nil || listing.Sessions == nil || len(listing.Sessions) != 0 {
		t.Fatalf("unexpected port listing: %+v", listing)
	}
	if response = do("POST", "/api/serial/sessions", map[string]string{"profileId": "missing"}); response.Code == 200 {
		t.Fatal("unknown profile started a serial session")
	}
	ssh, err := a.store.Save(Profile{Name: "ssh", Host: "localhost", User: "root"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.startSerial(ssh.ID); err == nil || !strings.Contains(err.Error(), "串口") {
		t.Fatal("SSH profile accepted as serial:", err)
	}
	ghost, err := a.store.Save(Profile{Name: "ghost", Protocol: "serial", SerialPort: "dengshell-no-such-port"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.startSerial(ghost.ID); err == nil {
		t.Fatal("nonexistent serial port opened without error")
	}
	if len(a.serialSessions()) != 0 {
		t.Fatal("failed opens left serial sessions registered")
	}
	if response = do("GET", "/api/serial/sessions/does-not-exist", nil); response.Code == 200 {
		t.Fatal("unknown serial session reported alive")
	}
	// DELETE is idempotent so a closed tab can always release its session id.
	if response = do("DELETE", "/api/serial/sessions/does-not-exist", nil); response.Code != 200 {
		t.Fatalf("serial session delete: %d %s", response.Code, response.Body)
	}
	a.securityLock.mu.Lock()
	a.securityLock.locked = true
	a.securityLock.mu.Unlock()
	if _, err := a.startSerial(ghost.ID); err == nil {
		t.Fatal("serial session started while locked")
	}
}
