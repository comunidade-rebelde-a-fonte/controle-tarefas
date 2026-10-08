// Package filestore grava documentos enviados no disco local (FILE_STORAGE_DIR), fora de
// qualquer diretório servido: gravação atômica, permissão restrita ao dono e nome interno que
// nunca vem do usuário [09 §5].
package filestore

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// WriteAtomic grava data em <dir>/<name> de forma atômica (temporário + rename), com
// permissão 0600.
func WriteAtomic(dir, name string, data []byte) error {
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
		_ = tmp.Close() // o erro relevante é o da escrita
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}

// SanitizeName guarda só o nome enviado (sem caminho, nem com barra invertida) e sem
// caracteres de controle, com no máximo maxLen caracteres (os últimos, para manter a
// extensão). Vazio vira "documento". É só metadado: o arquivo no disco nunca usa esse nome.
func SanitizeName(name string, maxLen int) string {
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
	if r := []rune(name); len(r) > maxLen {
		name = string(r[len(r)-maxLen:])
	}
	return name
}
