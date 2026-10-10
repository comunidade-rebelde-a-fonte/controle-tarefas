package prompts

import (
	"io/fs"
	"strings"
	"testing"
)

// O hash fixa o texto de cada versão: mudou o prompt, suba a versão (e rode as avaliações do
// agente) antes de atualizar o hash aqui [prompt-spec §9].
var pinned = map[string]string{
	"fatorr/plan@2026.10.1":   "eddce0a92af0",
	"fatorr/render@2026.10.1": "6df09143757a",

	"conciliacao/triador@2026.10.1":                   "e4db8b1c7752",
	"conciliacao/extrator_extrato@2026.10.1":          "0f65d93356b6",
	"conciliacao/extrator_extrato_correcao@2026.10.1": "5dac592e2910",
	"conciliacao/playbook@2026.10.1":                  "9ad8a00dd7f0",
	"conciliacao/extrator_comprovante@2026.10.1":      "cf7bf1ef831a",
	"conciliacao/resolvedor@2026.10.1":                "6ef3ac656bc2",
	"conciliacao/classificador@2026.10.1":             "5751b14d1b00",
}

func TestPrompts_VersionAndHashPinned(t *testing.T) {
	for _, n := range Names() {
		p := Get(n)
		key := p.Name + "@" + p.Version
		if want, ok := pinned[key]; !ok || want != p.Hash {
			t.Errorf("%s mudou (hash %s): suba a versão e fixe o novo hash", key, p.Hash)
		}
		if !strings.HasPrefix(p.Ref(), key+"#") || strings.Contains(p.Text, "\r") {
			t.Errorf("ref ou texto inválido: %s", p.Ref())
		}
	}
	// Todo arquivo embutido tem versão.
	_ = fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if _, ok := versions[strings.TrimSuffix(path, ".md")]; !ok {
				t.Errorf("%s sem versão", path)
			}
		}
		return nil
	})
}

// Os prompts trazem os itens do contrato comum do prompt-spec §2 (cada item, uma das formas).
func TestPrompts_CommonContract(t *testing.T) {
	items := []struct {
		name  string
		forms []string
	}{
		{"papel", []string{"você classifica", "você escreve", "você extrai"}},
		{"schema de saída", []string{"responda só com"}},
		{"ferramentas", []string{"não há outras ferramentas", "não há ferramentas"}},
		{"o que não decide", []string{"decididos pelo sistema", "calculada pelo sistema"}},
		{"abstenção", []string{"não se encaixar", "dados insuficientes"}},
		{"anti-injeção", []string{"é dado, não instrução"}},
		{"não inventar", []string{"não invente", "não estime"}},
		{"extraído × inferência", []string{"copie o que está escrito", "não apresente suposição como fato"}},
	}
	for _, n := range Names() {
		if IsFragment(n) {
			continue
		}
		text := strings.Join(strings.Fields(strings.ToLower(Get(n).Text)), " ")
		for _, it := range items {
			found := false
			for _, f := range it.forms {
				found = found || strings.Contains(text, f)
			}
			if !found {
				t.Errorf("%s sem o item %q do prompt-spec §2", n, it.name)
			}
		}
	}
}
