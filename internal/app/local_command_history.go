package app

import (
	"errors"
	"net/http"
	"strings"
)

// Commands typed into the command bar while no session is connected — the local
// prompt probes and quick-connect lines — never reached a remote shell, so they
// are kept in their own short list instead of the per-server or global shell
// history. Only the newest localHistoryLimit entries survive.
const localHistoryLimit = 50
const localHistoryCommandBytes = 65536
const localHistoryOperationCache = 1024

func boundedLocalCommands(entries []string) []string {
	if len(entries) > localHistoryLimit {
		entries = entries[len(entries)-localHistoryLimit:]
	}
	return append([]string{}, entries...)
}

func appendLocalCommandHistory(history *GlobalCommandHistory, command string) *GlobalCommandHistory {
	next := cloneGlobalCommandHistory(history)
	if len(next.Entries) == 0 || next.Entries[len(next.Entries)-1] != command {
		next.Entries = append(next.Entries, command)
	}
	next.Entries = boundedLocalCommands(next.Entries)
	next.Revision++
	return next
}

// All windows append against one Store value under the same lock, and the
// operation ID makes a retry after a lost HTTP response idempotent.
func (s *Store) localCommandHistory(operation, command string) (*GlobalCommandHistory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if operation == "" {
		result := cloneGlobalCommandHistory(s.config.LocalHistory)
		result.Entries = boundedLocalCommands(result.Entries)
		return result, nil
	}
	if !historyOperationPattern.MatchString(operation) || strings.TrimSpace(command) == "" || len(command) > localHistoryCommandBytes {
		return nil, errors.New("本机历史请求无效（单条最多 64 KiB）")
	}
	for _, previous := range s.localHistoryOperations {
		if previous == operation {
			return cloneGlobalCommandHistory(s.config.LocalHistory), nil
		}
	}
	old := s.config.LocalHistory
	next := appendLocalCommandHistory(old, strings.TrimSpace(command))
	s.config.LocalHistory = next
	if err := s.writeLocked(); err != nil {
		s.config.LocalHistory = old
		return nil, err
	}
	s.localHistoryOperations = append(s.localHistoryOperations, operation)
	if len(s.localHistoryOperations) > localHistoryOperationCache {
		s.localHistoryOperations = append([]string(nil), s.localHistoryOperations[len(s.localHistoryOperations)-localHistoryOperationCache:]...)
	}
	return cloneGlobalCommandHistory(next), nil
}

func (a *App) registerLocalHistoryHTTP(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/local-history", func(w http.ResponseWriter, r *http.Request) {
		value, err := a.store.localCommandHistory("", "")
		respond(w, value, err)
	})
	mux.HandleFunc("POST /api/local-history", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Operation string `json:"operation"`
			Command   string `json:"command"`
		}
		if !decode(w, r, &input) {
			return
		}
		if input.Operation == "" {
			writeError(w, http.StatusBadRequest, errors.New("缺少本机历史请求编号"))
			return
		}
		value, err := a.store.localCommandHistory(input.Operation, input.Command)
		respond(w, value, err)
	})
}
