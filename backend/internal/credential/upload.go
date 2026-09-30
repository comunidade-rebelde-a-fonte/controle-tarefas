package credential

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/db"
	"controle/internal/store"
	"controle/internal/user"
)

const maxFileNameLength = 200

// FileInput é o documento recebido no upload. Version é opcional (0 = sem checagem).
type FileInput struct {
	Name    string
	Data    []byte
	Version int32
}

// newStorageKey gera um identificador aleatório (UUID v4) para o arquivo no disco.
func newStorageKey() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// sanitizeFileName guarda só o nome (sem caminho) e sem caracteres de controle.
func sanitizeFileName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == "/" {
		name = "documento"
	}
	if r := []rune(name); len(r) > maxFileNameLength {
		name = string(r[len(r)-maxFileNameLength:])
	}
	return name
}

// writeFile grava o conteúdo em <dir>/<key> de forma atômica (temporário + rename).
func writeFile(dir, key string, data []byte) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return err
	}
	// Após o rename o temporário não existe mais; a remoção só limpa falhas no meio do caminho.
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o640); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, key))
}

// removeFile apaga o arquivo; ausência não é erro.
func (s *Service) removeFile(key *string) {
	if key == nil || *key == "" || s.storageDir == "" {
		return
	}
	if err := os.Remove(filepath.Join(s.storageDir, filepath.Base(*key))); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Sem conteúdo nem nome do arquivo no log [AGENTS §6.4].
		s.logRemoveError(err)
	}
}

// UploadFile valida e grava o documento vigente do tipo, substituindo o anterior
// [PRD §19.1, CA-020]. O arquivo só vai para o disco depois de aprovado pela validação.
func (s *Service) UploadFile(ctx context.Context, companyID, typ string, in FileInput) (Control, error) {
	if int64(len(in.Data)) > MaxFileSize {
		return Control{}, fileTooLarge()
	}
	accepted, err := ValidateFile(typ, in.Name, in.Data)
	if err != nil {
		return Control{}, err
	}
	thresholds, err := s.thresholds(ctx)
	if err != nil {
		return Control{}, err
	}
	if err := s.ensureCompany(ctx, companyID); err != nil {
		return Control{}, err
	}
	key, err := newStorageKey()
	if err != nil {
		return Control{}, err
	}
	if err := writeFile(s.storageDir, key, in.Data); err != nil {
		return Control{}, err
	}

	now := s.clock.Now()
	uid := user.FromContext(ctx)
	name := sanitizeFileName(in.Name)
	size := int64(len(in.Data))
	var rec store.CredentialControl
	var previous *string
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.EnsureCredentialControl(ctx, store.EnsureCredentialControlParams{CompanyID: companyID, Type: typ, UpdatedBy: &uid, UpdatedAt: now}); err != nil {
			return err
		}
		cur, err := q.GetCredentialControlForUpdate(ctx, store.GetCredentialControlForUpdateParams{CompanyID: companyID, Type: typ})
		if err != nil {
			return err
		}
		if in.Version != 0 && in.Version != cur.Version {
			return apperr.VersionConflict().WithDetails(map[string]any{"current_version": cur.Version})
		}
		previous = cur.FileStorageKey
		rec, err = q.SetCredentialFile(ctx, store.SetCredentialFileParams{
			CompanyID: companyID, Type: typ, FileStorageKey: &key, FileName: &name, FileMimeType: &accepted.MimeType,
			FileSizeBytes: &size, FileUploadedAt: &now, UpdatedBy: &uid,
		})
		return err
	})
	if err != nil {
		s.removeFile(&key)
		return Control{}, err
	}
	s.removeFile(previous)
	return s.toControl(typ, &rec, thresholds), nil
}

// DeleteFile remove o documento vigente do tipo (os demais dados permanecem).
func (s *Service) DeleteFile(ctx context.Context, companyID, typ string) (Control, error) {
	thresholds, err := s.thresholds(ctx)
	if err != nil {
		return Control{}, err
	}
	if err := s.ensureCompany(ctx, companyID); err != nil {
		return Control{}, err
	}
	now := s.clock.Now()
	uid := user.FromContext(ctx)
	var rec store.CredentialControl
	var previous *string
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		cur, err := q.GetCredentialControlForUpdate(ctx, store.GetCredentialControlForUpdateParams{CompanyID: companyID, Type: typ})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && cur.FileStorageKey == nil) {
			return apperr.NotFound("FILE_NOT_FOUND", "Não há documento vigente para remover.")
		}
		if err != nil {
			return err
		}
		previous = cur.FileStorageKey
		rec, err = q.ClearCredentialFile(ctx, store.ClearCredentialFileParams{CompanyID: companyID, Type: typ, UpdatedBy: &uid, UpdatedAt: now})
		return err
	})
	if err != nil {
		return Control{}, err
	}
	s.removeFile(previous)
	return s.toControl(typ, &rec, thresholds), nil
}

func fileTooLarge() error {
	return apperr.BadRequest("FILE_TOO_LARGE", "O arquivo excede o limite de 10 MB.")
}
