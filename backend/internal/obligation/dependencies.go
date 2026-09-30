package obligation

import (
	"slices"
	"strings"
)

// Modos de dependência [PRD §38.15].
const (
	DependencyHard = "HARD" // Contábil: bloqueia a conclusão fora de ordem (RN-011)
	DependencySoft = "SOFT" // DCTFWeb: alerta forte com exceção registrada (RN-012)
)

// StatusNoMovement satisfaz dependências e pode ser marcado sem checar pré-requisitos.
const StatusNoMovement = "NO_MOVEMENT"

// Dependency é uma aresta tipo → pré-requisito.
type Dependency struct {
	TypeID   string
	PrereqID string
	Mode     string
}

// DepInstance é o estado de uma instância da mesma empresa e competência.
type DepInstance struct {
	TypeID     string `json:"obligation_type_id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	StatusCode string `json:"status_code"`
	Applicable bool   `json:"-"`
	IsFinal    bool   `json:"-"`
	Order      int    `json:"-"`
}

// Prerequisites devolve os pré-requisitos do tipo no modo informado. Com transitive, inclui
// os pré-requisitos dos pré-requisitos (todas as etapas anteriores da cadeia contábil).
func Prerequisites(typeID, mode string, deps []Dependency, transitive bool) []string {
	var out []string
	queue := []string{typeID}
	seen := map[string]bool{typeID: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range deps {
			if d.TypeID != cur || d.Mode != mode || seen[d.PrereqID] {
				continue
			}
			seen[d.PrereqID] = true
			out = append(out, d.PrereqID)
			if transitive {
				queue = append(queue, d.PrereqID)
			}
		}
	}
	return out
}

// pendingAmong devolve os pré-requisitos aplicáveis ainda não finais, na ordem do catálogo.
// Pré-requisito não aplicável (ou inexistente na competência) é ignorado.
func pendingAmong(prereqs []string, instances map[string]DepInstance) []DepInstance {
	var pending []DepInstance
	for _, id := range prereqs {
		inst, ok := instances[id]
		if !ok || !inst.Applicable || inst.IsFinal {
			continue
		}
		pending = append(pending, inst)
	}
	slices.SortFunc(pending, func(a, b DepInstance) int { return a.Order - b.Order })
	return pending
}

// CheckHard devolve as etapas anteriores (HARD, transitivas) que impedem concluir o tipo:
// aplicáveis e não finais. "Sem movimento" é final e satisfaz a dependência [PRD §18.2].
func CheckHard(typeID string, deps []Dependency, instances map[string]DepInstance) []DepInstance {
	return pendingAmong(Prerequisites(typeID, DependencyHard, deps, true), instances)
}

// CheckSoft devolve os componentes SOFT pendentes (diretos) do tipo [PRD §16.4].
func CheckSoft(typeID string, deps []Dependency, instances map[string]DepInstance) []DepInstance {
	return pendingAmong(Prerequisites(typeID, DependencySoft, deps, false), instances)
}

// joinNames junta nomes em português: "A", "A e B", "A, B e C".
func joinNames(items []DepInstance) string {
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.Name
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " e " + names[len(names)-1]
}

// BlockedMessage é a mensagem de bloqueio da dependência contábil [PRD §18.3].
func BlockedMessage(target string, pending []DepInstance) string {
	if len(pending) == 1 {
		return "Não é possível concluir " + target + ". A etapa " + pending[0].Name + " ainda não foi concluída."
	}
	return "Não é possível concluir " + target + ". As etapas " + joinNames(pending) + " ainda não foram concluídas."
}

// SoftWarningMessage é o alerta forte da DCTFWeb [PRD §16.4].
func SoftWarningMessage(target string, pending []DepInstance) string {
	return target + " possui pré-requisitos pendentes: " + joinNames(pending) + "."
}
