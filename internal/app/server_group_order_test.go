package app

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"
)

func TestGroupOrderPersistenceAndValidation(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range s.List().GroupNodes {
		if err := s.DeleteGroup(group.ID); err != nil {
			t.Fatal(err)
		}
	}
	add := func(name, parent string) ServerGroup {
		t.Helper()
		group, err := s.SaveGroup(ServerGroup{Name: name, ParentID: parent, Emoji: "📁", BackgroundColor: "#183F52"})
		if err != nil {
			t.Fatal(err)
		}
		return group
	}
	a, b := add("A", ""), add("B", "")
	c, d := add("C", a.ID), add("D", a.ID)
	p, err := s.Save(Profile{Name: "保留连接", Host: "192.0.2.1", User: "root", GroupID: c.ID, Secret: "keep-secret"}, false)
	if err != nil {
		t.Fatal(err)
	}
	before := cloneConnectionConfig(s.config)
	for _, input := range []struct {
		parent string
		ids    []string
	}{
		{"", []string{a.ID}}, // Stale/partial sibling set.
		{"", []string{a.ID, a.ID}},
		{"", []string{a.ID, c.ID}}, // Cross-parent move is not a reorder.
		{"", []string{a.ID, "missing"}},
		{"missing", []string{a.ID, b.ID}},
		{c.ID, nil},
	} {
		if _, err := s.ReorderGroups(input.parent, input.ids); err == nil {
			t.Fatalf("invalid order accepted: %+v", input)
		}
		if !reflect.DeepEqual(before, cloneConnectionConfig(s.config)) {
			t.Fatal("rejected order changed config")
		}
	}
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	oldDir := s.dir
	s.dir = blocked
	_, err = s.ReorderGroups("", []string{b.ID, a.ID})
	s.dir = oldDir
	if err == nil || !reflect.DeepEqual(before, cloneConnectionConfig(s.config)) {
		t.Fatal("failed persistence did not roll back")
	}
	for _, input := range []struct {
		parent string
		ids    []string
	}{{"", []string{b.ID, a.ID}}, {a.ID, []string{d.ID, c.ID}}} {
		if _, err := s.ReorderGroups(input.parent, input.ids); err != nil {
			t.Fatal(err)
		}
	}
	expected := before
	orders := map[string]int{a.ID: 2, b.ID: 1, c.ID: 2, d.ID: 1}
	for i := range expected.GroupNodes {
		expected.GroupNodes[i].Order = orders[expected.GroupNodes[i].ID]
	}
	if !reflect.DeepEqual(expected, cloneConnectionConfig(s.config)) {
		t.Fatal("reordering changed other group or connection fields")
	}
	reopened, err := OpenStore(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expected.GroupNodes, reopened.config.GroupNodes) {
		t.Fatal("order did not survive restart", err)
	}
	if stored, err := reopened.Get(p.ID); err != nil || stored.Secret != "keep-secret" || stored.GroupID != c.ID {
		t.Fatal("connection changed during order persistence", err)
	}
	// Form submissions omit order; even a stale/forged order must not reset it.
	a.Name, a.Order = "renamed", 99
	if saved, err := s.SaveGroup(a); err != nil || saved.Order != 2 {
		t.Fatal("editing reset sibling order", err)
	}
	c.ParentID = b.ID
	if saved, err := s.SaveGroup(c); err != nil || saved.Order != 0 {
		t.Fatal("moving a group retained its former sibling position", err)
	}
}

func TestGroupOrderHTTP(t *testing.T) {
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	first, _ := a.store.SaveGroup(ServerGroup{Name: "first"})
	second, _ := a.store.SaveGroup(ServerGroup{Name: "second"})
	handler := a.Handler(fstest.MapFS{"index.html": {Data: []byte("fixture")}})
	ids := []string{}
	for _, g := range a.store.List().GroupNodes {
		ids = append(ids, g.ID)
	}
	// The initial fixture may have built-in groups; include every sibling.
	ids[0], ids[len(ids)-1] = ids[len(ids)-1], ids[0]
	for _, input := range []struct {
		ids   []string
		token string
		ok    bool
	}{{ids, "", false}, {[]string{first.ID, first.ID}, a.Token(), false}, {ids, a.Token(), true}} {
		body, _ := json.Marshal(map[string]any{"parentId": "", "ids": input.ids})
		req := httptest.NewRequest("POST", "http://wails.localhost/api/group-nodes/order", bytes.NewReader(body))
		req.Header.Set("X-CloudShell-Token", input.token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if (response.Code == 200) != input.ok {
			t.Fatalf("unexpected reorder response: %d %s", response.Code, response.Body)
		}
		if input.ok {
			var groups []ServerGroup
			if err := json.Unmarshal(response.Body.Bytes(), &groups); err != nil || groupIndex(groups)[ids[0]].Order != 1 || groupIndex(groups)[second.ID].ID == "" {
				t.Fatal("incorrect order response", err)
			}
		}
	}
}

func TestServerGroupOrderBrowser(t *testing.T) {
	runIsolatedNavigationBrowser(t, "test-server-group-order-browser.mjs")
}
