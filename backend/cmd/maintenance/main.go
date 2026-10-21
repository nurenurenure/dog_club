// maintenance verifies all registered private files without printing their keys.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"dogclub.local/backend/internal/platform/database"
	"dogclub.local/backend/internal/platform/filestore"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "storage/database verification failed")
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	p, err := database.New(ctx, os.Getenv("DATABASE_URL"), time.Second)
	if err != nil {
		return err
	}
	defer p.Close()
	f, err := filestore.New(os.Getenv("DOCUMENTS_DIR"))
	if err != nil {
		return err
	}
	defer f.Close()
	// Document files and dog photos share the private store; both are verified.
	rows, err := p.Query(ctx, `SELECT storage_key,size_bytes,sha256 FROM document_file UNION ALL SELECT storage_key,size_bytes,sha256 FROM dog_photo ORDER BY storage_key`)
	if err != nil {
		return err
	}
	digest := sha256.New()
	count := 0
	var size int64
	for rows.Next() {
		var b filestore.Blob
		if err = rows.Scan(&b.Key, &b.Size, &b.SHA256); err != nil {
			rows.Close()
			return err
		}
		if _, err = f.Read(ctx, b); err != nil {
			rows.Close()
			return err
		}
		fmt.Fprintf(digest, "%s %d %s\n", b.Key, b.Size, b.SHA256)
		count++
		size += b.Size
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	var version int64
	var tables int
	if err = p.QueryRow(ctx, `SELECT coalesce(max(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil {
		return err
	}
	if err = p.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'`).Scan(&tables); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"migration_version": version, "tables": tables, "registered_files": count, "registered_bytes": size, "registered_files_sha256": hex.EncodeToString(digest.Sum(nil))})
}
