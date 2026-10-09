// Package prompts guarda os prompts de runtime dos agentes, versionados no repositório
// [prompt-spec §1]. Cada prompt tem nome lógico, versão e hash do conteúdo; a corrida grava os
// três (Ref). Um teste fixa o hash de cada versão: mudar o texto exige versão nova.
package prompts

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

//go:embed fatorr/*.md
var files embed.FS

// Prompt é um prompt carregado.
type Prompt struct {
	Name    string // ex.: fatorr/plan
	Version string
	Hash    string // 12 primeiros hex do sha256 do texto (fim de linha normalizado)
	Text    string
}

// Ref identifica o prompt na corrida: nome@versão#hash.
func (p Prompt) Ref() string { return p.Name + "@" + p.Version + "#" + p.Hash }

// versions é a versão de cada prompt. Os do Fator R partem dos prompts plan e render da origem
// (agents/prompts, PROMPT_VERSION 2026.09.1) com os itens do prompt-spec §2 (D-024).
var versions = map[string]string{
	"fatorr/plan":   "2026.10.1",
	"fatorr/render": "2026.10.1",
}

// Get carrega o prompt pelo nome lógico. Nome desconhecido é erro de programação.
func Get(name string) Prompt {
	version, ok := versions[name]
	if !ok {
		panic(fmt.Sprintf("prompts: %q sem versão", name))
	}
	raw, err := files.ReadFile(name + ".md")
	if err != nil {
		panic(fmt.Sprintf("prompts: %q: %v", name, err))
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	sum := sha256.Sum256([]byte(text))
	return Prompt{Name: name, Version: version, Hash: hex.EncodeToString(sum[:])[:12], Text: text}
}

// Names lista os prompts conhecidos.
func Names() []string {
	out := make([]string, 0, len(versions))
	for n := range versions {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
