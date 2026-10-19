package documents

import (
	"context"
	"encoding/json"
	"time"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/platform/filestore"
)

// Preparation and file I/O happen outside WithAccess; finalization rechecks the
// principal and source/version. An uncertain commit keeps the private blob.
func (s *Service) Upload(ctx context.Context, p auth.Principal, id int32, c Correction, name, media string, data []byte) (Document, error) {
	if err := correction(&c); err != nil {
		return Document{}, err
	}
	if err := validateFile(name, media, data); err != nil {
		return Document{}, err
	}
	if err := s.writable(ctx, p, id, c.Version); err != nil {
		return Document{}, err
	}
	return s.persist(ctx, p, id, c, File{Name: name, MediaType: media}, data, nil)
}
func (s *Service) writable(ctx context.Context, p auth.Principal, id int32, version int64) error {
	return s.with(ctx, p, []string{"staff", "veterinarian"}, func(ctx context.Context, q *repository, p auth.Principal) error {
		d, err := q.get(ctx, id)
		if err != nil {
			return err
		}
		if err = writeAllowed(p, d); err != nil {
			return err
		}
		if d.Status != "active" || d.Version != version {
			return ErrConflict
		}
		return nil
	})
}
func (s *Service) Generate(ctx context.Context, p auth.Principal, id int32, in Generate) (Document, error) {
	if err := correction(&in.Correction); err != nil {
		return Document{}, err
	}
	var snapshot sourceSnapshot
	err := s.with(ctx, p, []string{"staff", "veterinarian"}, func(ctx context.Context, q *repository, p auth.Principal) error {
		d, err := q.get(ctx, id)
		if err != nil {
			return err
		}
		if err = writeAllowed(p, d); err != nil {
			return err
		}
		if d.Status != "active" || d.Version != in.Version {
			return ErrConflict
		}
		snapshot, err = s.source(ctx, q, p, d, in, true)
		return err
	})
	if err != nil {
		return Document{}, err
	}
	data, err := render(snapshot, in.Template)
	if err != nil {
		return Document{}, err
	}
	source, err := json.Marshal(snapshot)
	if err != nil {
		return Document{}, ErrUnavailable
	}
	input, err := json.Marshal(in)
	if err != nil {
		return Document{}, ErrUnavailable
	}
	f := File{Name: in.Template + ".pdf", MediaType: "application/pdf", Template: &in.Template, SourceInput: input, Snapshot: source}
	return s.persist(ctx, p, id, in.Correction, f, data, &in)
}
func (s *Service) persist(ctx context.Context, p auth.Principal, id int32, c Correction, f File, data []byte, in *Generate) (Document, error) {
	blob, err := s.files.Put(ctx, data)
	if err != nil {
		return Document{}, ErrUnavailable
	}
	f.Key, f.Size, f.SHA256 = blob.Key, blob.Size, blob.SHA256
	var d Document
	commitAttempt := false
	err = s.with(ctx, p, []string{"staff", "veterinarian"}, func(ctx context.Context, q *repository, p auth.Principal) error {
		before, err := q.get(ctx, id)
		if err != nil {
			return err
		}
		if err = writeAllowed(p, before); err != nil {
			return err
		}
		if before.Status != "active" || before.Version != c.Version {
			return ErrConflict
		}
		if in != nil {
			latest, err := s.source(ctx, q, p, before, *in, true)
			if err != nil {
				return err
			}
			b, err := json.Marshal(latest)
			if err != nil {
				return ErrUnavailable
			}
			if !equalJSON(b, f.Snapshot) {
				return ErrConflict
			}
		}
		if err = q.bump(ctx, id, c.Version); err != nil {
			return err
		}
		f.DocumentID = id
		f.DocumentVersion = c.Version + 1
		if _, err = q.addFile(ctx, p, f); err != nil {
			return err
		}
		d, err = q.get(ctx, id)
		if err != nil {
			return err
		}
		if err = q.record(ctx, p, d, "file_added", c.Reason, &before); err != nil {
			return err
		}
		commitAttempt = true
		return nil
	})
	if err != nil && !commitAttempt {
		_ = s.files.Discard(blob)
	}
	d.Expired = d.ExpiresOn != nil && *d.ExpiresOn < s.now().UTC().Format(time.DateOnly)
	return d, err
}
func (s *Service) Download(ctx context.Context, p auth.Principal, id int32, fileID int64) (File, []byte, error) {
	d, err := s.Get(ctx, p, id)
	if err != nil {
		return File{}, nil, err
	}
	var f *File
	for i := range d.Files {
		if d.Files[i].ID == fileID {
			f = &d.Files[i]
			break
		}
	}
	if f == nil {
		return File{}, nil, ErrNotFound
	}
	data, err := s.files.Read(ctx, filestore.Blob{Key: f.Key, Size: f.Size, SHA256: f.SHA256})
	if err != nil {
		return File{}, nil, ErrUnavailable
	}
	// Recheck revocation/ownership after potentially slow disk I/O.
	current, err := s.Get(ctx, p, id)
	if err != nil {
		return File{}, nil, err
	}
	if current.Version != d.Version {
		return File{}, nil, ErrConflict
	}
	return *f, data, nil
}
