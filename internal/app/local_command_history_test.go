package app

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func localHistoryStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLocalCommandHistoryKeepsOnlyTheNewestFifty(t *testing.T) {
	s := localHistoryStore(t)
	for i := 0; i < 60; i++ {
		if _, err := s.localCommandHistory(randomID(), fmt.Sprintf("ping host-%02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.localCommandHistory("", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{}
	for i := 10; i < 60; i++ {
		want = append(want, fmt.Sprintf("ping host-%02d", i))
	}
	if !reflect.DeepEqual(got.Entries, want) {
		t.Fatalf("local history is not the newest 50 in order: %q", got.Entries)
	}
	// The command bar's own list must never leak into the shell history views.
	if len(s.List().CommandHistory.Entries) != 0 {
		t.Fatal("local prompt commands leaked into the global command history")
	}
	// A hand-edited or imported oversized list is trimmed on the way out.
	s.config.LocalHistory = &GlobalCommandHistory{Entries: make([]string, 0, 80)}
	for i := 0; i < 80; i++ {
		s.config.LocalHistory.Entries = append(s.config.LocalHistory.Entries, fmt.Sprintf("cmd %d", i))
	}
	trimmed, err := s.localCommandHistory("", "")
	if err != nil || len(trimmed.Entries) != 50 || trimmed.Entries[0] != "cmd 30" {
		t.Fatalf("oversized stored list not trimmed: %d %q %v", len(trimmed.Entries), trimmed.Entries[0], err)
	}
}

func TestLocalCommandHistoryConsecutiveDedupKeepsEarlierEntries(t *testing.T) {
	s := localHistoryStore(t)
	for _, command := range []string{"ping a.invalid", "ping b.invalid", "ping b.invalid"} {
		if _, err := s.localCommandHistory(randomID(), command); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.localCommandHistory(randomID(), "ping a.invalid"); err != nil {
		t.Fatal(err)
	}
	got, err := s.localCommandHistory("", "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Entries, []string{"ping a.invalid", "ping b.invalid", "ping a.invalid"}) {
		t.Fatalf("unexpected local history: %q", got.Entries)
	}
}

func TestLocalCommandHistoryRetryWindowAndRestart(t *testing.T) {
	s := localHistoryStore(t)
	operation := randomID()
	if _, err := s.localCommandHistory(operation, "ssh -p 2222 root@192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	// A window that lost its response retries with the same operation ID.
	retry, err := s.localCommandHistory(operation, "ssh -p 2222 root@192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(retry.Entries) != 1 || retry.Entries[0] != "ssh -p 2222 root@192.0.2.1" {
		t.Fatalf("replayed append duplicated the command: %q", retry.Entries)
	}
	for _, invalid := range []struct{ operation, command string }{
		{"too-short", "ping a.invalid"},
		{randomID(), "   "},
		{randomID(), strings.Repeat("x", localHistoryCommandBytes+1)},
	} {
		if _, err := s.localCommandHistory(invalid.operation, invalid.command); err == nil {
			t.Fatal("invalid local history request accepted")
		}
	}
	if _, err := s.localCommandHistory(randomID(), "traceroute 192.0.2.2"); err != nil {
		t.Fatal(err)
	}
	// Public snapshots must not alias the stored slice.
	public := s.List().LocalHistory
	public.Entries[0] = "tampered public copy"
	reopened, err := OpenStore(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.List().LocalHistory; len(got.Entries) != 2 || got.Entries[0] != "ssh -p 2222 root@192.0.2.1" {
		t.Fatalf("local history lost or aliased across restart: %q", got.Entries)
	}
}

func TestLocalCommandHistoryConcurrentWindowsKeepEveryCommand(t *testing.T) {
	s := localHistoryStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := s.localCommandHistory(randomID(), fmt.Sprintf("ping window-%02d.invalid", i)); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	got, err := s.localCommandHistory("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 30 {
		t.Fatalf("lost commands from concurrent windows: %d", len(got.Entries))
	}
}
