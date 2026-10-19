// Package filestore stores immutable private blobs. Database rows grant access;
// storage keys and filesystem paths are never part of the public API.
package filestore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"regexp"
)

const MaxSize = 10 << 20

var ErrUnavailable = errors.New("private storage unavailable")
var keyPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Store struct{ root *os.Root }
type Blob struct {
	Key    string
	Size   int64
	SHA256 string
}

func New(path string) (*Store, error) {
	if path == "" {
		return nil, ErrUnavailable
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, ErrUnavailable
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, ErrUnavailable
	}
	f, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, ErrUnavailable
	}
	info, err := f.Stat()
	f.Close()
	if err != nil || info.Mode().Perm()&0077 != 0 {
		root.Close()
		return nil, ErrUnavailable
	}
	return &Store{root}, nil
}
func (s *Store) Close() error { return s.root.Close() }
func (s *Store) Put(ctx context.Context, data []byte) (Blob, error) {
	if len(data) == 0 || len(data) > MaxSize || ctx.Err() != nil {
		return Blob{}, ErrUnavailable
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Blob{}, ErrUnavailable
	}
	b := Blob{Key: hex.EncodeToString(random[:]), Size: int64(len(data)), SHA256: Hash(data)}
	f, err := s.root.OpenFile(b.Key, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Blob{}, ErrUnavailable
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			s.root.Remove(b.Key)
		}
	}()
	if _, err = f.Write(data); err != nil {
		return Blob{}, ErrUnavailable
	}
	if err = f.Sync(); err != nil {
		return Blob{}, ErrUnavailable
	}
	if err = f.Close(); err != nil {
		return Blob{}, ErrUnavailable
	}
	directory, err := s.root.Open(".")
	if err != nil {
		return Blob{}, ErrUnavailable
	}
	err = directory.Sync()
	directory.Close()
	if err != nil || ctx.Err() != nil {
		return Blob{}, ErrUnavailable
	}
	ok = true
	return b, nil
}
func (s *Store) Read(ctx context.Context, b Blob) ([]byte, error) {
	if !keyPattern.MatchString(b.Key) || b.Size < 1 || b.Size > MaxSize || ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	info, err := s.root.Lstat(b.Key)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnavailable
	}
	f, err := s.root.Open(b.Key)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxSize+1))
	if err != nil || int64(len(data)) != b.Size || Hash(data) != b.SHA256 || ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	return data, nil
}
func (s *Store) Discard(b Blob) error {
	if !keyPattern.MatchString(b.Key) {
		return ErrUnavailable
	}
	if err := s.root.Remove(b.Key); err != nil {
		return ErrUnavailable
	}
	return nil
}
func Hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
