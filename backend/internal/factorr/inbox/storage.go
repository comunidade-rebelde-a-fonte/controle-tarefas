package inbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"controle/internal/platform/filestore"
)

var shaRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Storage guarda os originais em <FILE_STORAGE_DIR>/pgdas/<sha256>: caminho derivado só do hash
// (caminho_seguro da origem), fora de qualquer diretório servido, com permissão 0600. Original
// nunca é sobrescrito nem apagado depois de ter documento.
type Storage struct{ dir string }

// NewStorage cria o armazenamento sob o diretório base.
func NewStorage(baseDir string) Storage { return Storage{dir: filepath.Join(baseDir, "pgdas")} }

func fileRef(sha string) string { return "pgdas/" + sha }

func (s Storage) path(sha string) (string, error) {
	if !shaRe.MatchString(sha) {
		return "", fmt.Errorf("inbox: hash inválido")
	}
	return filepath.Join(s.dir, sha), nil
}

// Save grava o arquivo se ainda não existe (o mesmo conteúdo dá o mesmo caminho). created
// indica se esta chamada o gravou.
func (s Storage) Save(sha string, data []byte) (created bool, err error) {
	p, err := s.path(sha)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(p); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := filestore.WriteAtomic(s.dir, sha, data); err != nil {
		return false, err
	}
	return true, nil
}

// Read lê o original. A leitura passa por os.Root: o caminho não sai da pasta do inbox.
func (s Storage) Read(sha string) ([]byte, error) {
	if _, err := s.path(sha); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return root.ReadFile(sha)
}

// Remove apaga um arquivo que ficou sem documento (falha entre a gravação e o registro, como
// a revisão B6 da origem). Ausência não é erro.
func (s Storage) Remove(sha string) error {
	if _, err := s.path(sha); err != nil {
		return err
	}
	root, err := os.OpenRoot(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if err := root.Remove(sha); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
