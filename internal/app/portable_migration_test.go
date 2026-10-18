package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPortableDataMigrationKeepsSettingsAndAssets(t *testing.T) {
	root := t.TempDir()
	old, e := OpenStore(root)
	if e != nil {
		t.Fatal(e)
	}
	a := old.List().Appearance
	a.Theme = "dark"
	if _, e = old.SaveAppearance(a); e != nil {
		t.Fatal(e)
	}
	if e = os.MkdirAll(filepath.Join(root, "assets", "backgrounds"), 0700); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(root, "assets", "backgrounds", "custom.png"), []byte("image"), 0600)
	target := filepath.Join(root, "data")
	os.MkdirAll(filepath.Join(target, "docs"), 0700)
	os.WriteFile(filepath.Join(target, "docs", "README.txt"), []byte("instructions"), 0600)
	if e = MigratePortableData(target); e != nil {
		t.Fatal(e)
	}
	next, e := OpenStore(target)
	if e != nil {
		t.Fatal(e)
	}
	if next.List().Appearance.Theme != "dark" {
		t.Fatal("lost theme")
	}
	if b, e := os.ReadFile(filepath.Join(target, "assets", "backgrounds", "custom.png")); e != nil || string(b) != "image" {
		t.Fatal("lost asset", e)
	}
	if _, e = os.Stat(filepath.Join(root, EncryptedConfigName)); !os.IsNotExist(e) {
		t.Fatal("old file remains")
	}
	if e = MigratePortableData(target); e != nil {
		t.Fatal(e)
	}
}

func TestPortableDataMigrationKeepsEncryptedConfigAndKey(t *testing.T) {
	root := t.TempDir()
	old, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Save(Profile{Name: "fixture", Host: "fixture.invalid", User: "root", Secret: "portable-secret"}, false); err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile(filepath.Join(root, ConfigKeyName))
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := os.ReadFile(filepath.Join(root, EncryptedConfigName))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte("portable-secret")) {
		t.Fatal("fixture config was not encrypted")
	}
	target := filepath.Join(root, "data")
	if err := MigratePortableData(target); err != nil {
		t.Fatal(err)
	}
	for name, before := range map[string][]byte{ConfigKeyName: key, EncryptedConfigName: encrypted} {
		after, err := os.ReadFile(filepath.Join(target, name))
		if err != nil || !bytes.Equal(after, before) {
			t.Fatalf("migration lost or changed %s: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("old %s remains after successful migration: %v", name, err)
		}
	}
	next, err := OpenStore(target)
	if err != nil {
		t.Fatal(err)
	}
	got, err := next.Get(next.List().Servers[0].ID)
	if err != nil || got.Secret != "portable-secret" {
		t.Fatalf("migrated config cannot be read with migrated key: %v", err)
	}
	if err := MigratePortableData(target); err != nil {
		t.Fatal(err)
	}
}
