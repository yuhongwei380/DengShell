package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSecurityLockRejectsChangedPolicyWhileRunning(t *testing.T) {
	for _, change := range []string{"corrupt", "delete", "key"} {
		t.Run(change, func(t *testing.T) {
			a := lockTestApp(t)
			enableTestPassword(t, a, 0)
			if _, err := a.LockNow(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(a.store.dir, securityLockFile)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "corrupt":
				before[len(before)-1] ^= 1
				err = os.WriteFile(path, before, 0600)
			case "delete":
				err = os.Remove(path)
			case "key":
				err = os.WriteFile(filepath.Join(a.store.dir, ConfigKeyName), bytes.Repeat([]byte{0x42}, 32), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			status, err := a.Unlock(LockProof{Method: "password", Value: "1234"})
			if err == nil || !status.Locked {
				t.Fatal("changed policy/key was overwritten and unlock succeeded")
			}
			after, err := os.ReadFile(path)
			if change == "delete" {
				if !os.IsNotExist(err) {
					t.Fatal("deleted policy was silently recreated")
				}
			} else if err != nil || !bytes.Equal(before, after) {
				t.Fatal("policy bytes changed after rejected unlock", err)
			}
		})
	}
}

func TestSecurityLockRejectsStaleInstanceSettings(t *testing.T) {
	a := lockTestApp(t)
	b, err := New(a.store.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	staleGrant, err := b.AuthorizeLockSettings(LockProof{})
	if err != nil {
		t.Fatal(err)
	}
	enableTestPassword(t, a, 0)
	before, err := os.ReadFile(filepath.Join(a.store.dir, securityLockFile))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.SaveLockSettings(LockSettingsInput{Grant: staleGrant, Enabled: false}); err == nil {
		t.Fatal("stale instance disabled a newly enabled password without authorization")
	}
	after, err := os.ReadFile(filepath.Join(a.store.dir, securityLockFile))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("stale settings overwrote the current policy", err)
	}
}

func TestSecurityLockRejectsTOTPReplayAcrossInstances(t *testing.T) {
	a := lockTestApp(t)
	secret := []byte("12345678901234567890")
	now := time.Unix(1234567890, 0)
	policy := securityLockPolicy{Version: 1, Enabled: true, TOTPSecret: secret, LastTOTPStep: -1}
	a.securityLock.mu.Lock()
	err := a.saveLockPolicy(policy)
	a.securityLock.policy = policy
	a.securityLock.locked = true
	a.securityLock.now = func() time.Time { return now }
	a.securityLock.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(a.store.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	b.securityLock.mu.Lock()
	b.securityLock.now = func() time.Time { return now }
	b.securityLock.mu.Unlock()
	proof := LockProof{Method: "totp", Value: lockTOTP(secret, now.Unix()/30)}
	if _, err := a.Unlock(proof); err != nil {
		t.Fatal(err)
	}
	if status, err := b.Unlock(proof); err == nil || !status.Locked {
		t.Fatal("second instance replayed an already accepted TOTP")
	}
}
