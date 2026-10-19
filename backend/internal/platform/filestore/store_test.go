package filestore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateImmutableStorageAndIntegrity(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "files")
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	data := []byte("private data")
	b, err := s.Put(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Read(ctx, b)
	if err != nil || string(got) != string(data) {
		t.Fatal("round trip failed")
	}
	info, err := os.Stat(filepath.Join(dir, b.Key))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("unsafe file permissions")
	}
	if err = os.WriteFile(filepath.Join(dir, b.Key), []byte("altered data"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Read(ctx, b); err == nil {
		t.Fatal("corruption accepted")
	}
	if _, err = s.Read(ctx, Blob{Key: "../secret", Size: 1}); err == nil {
		t.Fatal("traversal accepted")
	}
	external := filepath.Join(t.TempDir(), "private")
	if err = os.WriteFile(external, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("a", 64)
	if err = os.Symlink(external, filepath.Join(dir, key)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Read(ctx, Blob{Key: key, Size: 6, SHA256: Hash([]byte("secret"))}); err == nil {
		t.Fatal("symlink accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = s.Put(cancelled, data); err == nil {
		t.Fatal("cancelled write accepted")
	}
	if err = s.Discard(b); err != nil {
		t.Fatal(err)
	}
	unsafe := t.TempDir()
	if err = os.Chmod(unsafe, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err = New(unsafe); err == nil {
		t.Fatal("public directory accepted")
	}
}
