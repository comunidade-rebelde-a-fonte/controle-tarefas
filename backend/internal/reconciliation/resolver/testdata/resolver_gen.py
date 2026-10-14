"""Golden da hipótese do resolvedor, gerado pelo próprio código do Conciliador (commit 2bc6e26).

Roda HipoteseResolvedor (app/domain/schemas.py) sobre a saída do LLM como ClienteLLM.complete:
json.loads(extrair_json(conteudo)) e model_validate, em casos normais e de borda. A saída só traz
os tipos do schema estrito (ver o golden do extrato); "guarda" marca o que o pydantic aceita e o
porte recusa: confiança fora de 0 a 1, na hipótese ou num par.

Uso (Python 3.13 com pydantic 2.12.5):

    python resolver_gen.py <caminho>/referencias/conciliador-contabil
"""

import ast
import json
import sys
from collections import Counter
from decimal import Decimal, InvalidOperation
from pathlib import Path
from typing import Any

from pydantic import ValidationError

if len(sys.argv) != 2:
    sys.exit(__doc__)
ORIGEM = Path(sys.argv[1]).resolve()
sys.path.insert(0, str(ORIGEM))

from app.domain.schemas import HipoteseResolvedor  # noqa: E402

AQUI = Path(__file__).resolve().parent


def _da_origem(arquivo: str, nomes: list[str], escopo: dict[str, Any]) -> dict[str, Any]:
    arvore = ast.parse((ORIGEM / arquivo).read_text(encoding="utf-8"))
    corpo = [n for n in arvore.body if isinstance(n, ast.FunctionDef) and n.name in nomes]
    exec(compile(ast.Module(body=corpo, type_ignores=[]), arquivo, "exec"), escopo)
    return escopo


extrair_json = _da_origem("app/agentes/llm.py", ["extrair_json"], {})["extrair_json"]


def guarda(h: HipoteseResolvedor) -> list[str]:
    fora = [c for c in [h.confianca] + [p.confianca for p in h.pares_propostos] if not (Decimal(0) <= c <= Decimal(1))]
    return ["confianca"] if fora else []


def validar(conteudo: str) -> dict[str, Any]:
    try:
        dado = json.loads(extrair_json(conteudo))
    except json.JSONDecodeError:
        return {"entrada": conteudo, "resultado": "invalido"}
    try:
        h = HipoteseResolvedor.model_validate(dado)
    except ValidationError:
        return {"entrada": conteudo, "resultado": "invalido"}
    except (TypeError, InvalidOperation, ValueError, ArithmeticError) as exc:
        return {"entrada": conteudo, "resultado": "excecao:" + type(exc).__name__}
    return {"entrada": conteudo, "resultado": "ok", "saida": h.model_dump(mode="json"), "guarda": guarda(h)}


TX = "00000000-0000-0001-0000-000000000001"
L1, L2 = "00000000-0000-0002-0000-000000000001", "00000000-0000-0002-0000-000000000002"


def par(**kw: Any) -> dict[str, Any]:
    base: dict[str, Any] = {"extrato_transacao_id": TX, "lancamento_cliente_ids": [L1], "tipo": "1:1", "confianca": 0.8}
    for k, v in kw.items():
        if v is ...:
            base.pop(k, None)
        else:
            base[k] = v
    return base


def hipotese(**kw: Any) -> dict[str, Any]:
    base: dict[str, Any] = {
        "hipotese": "O débito de R$ 300,00 parece corresponder à soma de dois boletos.",
        "justificativa": "Mesmo fornecedor e vencimento no mesmo dia.",
        "pares_propostos": [par(lancamento_cliente_ids=[L1, L2], tipo="N:1")],
        "confianca": 0.6, "pede_humano": True,
    }
    for k, v in kw.items():
        if v is ...:
            base.pop(k, None)
        else:
            base[k] = v
    return base


def casos() -> list[str]:
    j = json.dumps
    out = [j(hipotese()), "```json\n" + j(hipotese()) + "\n```", "Hipótese:\n" + j(hipotese(), indent=2) + "\nfim", "{}"]
    for campo in ["hipotese", "justificativa", "confianca", "pares_propostos", "pede_humano"]:
        out.append(j(hipotese(**{campo: ...})))
        out.append(j(hipotese(**{campo: None})))
    out += [j(hipotese(hipotese="")), j(hipotese(hipotese=5)), j(hipotese(justificativa=[]))]
    for c in [0, 1, 0.75, "0.75", "1", 1.5, -0.1, "NaN", "abc", True, 0.123456789]:
        out.append(j(hipotese(confianca=c)))
    out += [j(hipotese(pede_humano=False)), j(hipotese(extra="ignorado"))]
    for p in [
        [], [par()], [par(), par(tipo="extrato_sem_comprovante", lancamento_cliente_ids=[])],
        [par(extrato_transacao_id=None)], [par(extrato_transacao_id=...)], [par(extrato_transacao_id=5)],
        [par(lancamento_cliente_ids=...)], [par(lancamento_cliente_ids=None)], [par(lancamento_cliente_ids=[1])],
        [par(lancamento_cliente_ids="x")], [par(tipo=...)], [par(tipo=None)], [par(tipo="qualquer coisa")],
        [par(confianca=...)], [par(confianca=None)], [par(confianca="0.9")], [par(confianca=2)], [par(confianca=-1)],
        [par(extra=1)], [5], ["x"], None,
    ]:
        out.append(j(hipotese(pares_propostos=p)))
    out += ["não é json", "[]", "null", '{"hipotese": "a"} lixo']
    return out


def main() -> None:
    golden = [validar(c) for c in casos()]
    (AQUI / "resolver_golden.json").write_text(json.dumps(golden, ensure_ascii=False, indent=1) + "\n", encoding="utf-8",
                                               newline="\n")
    print(Counter(c["resultado"] for c in golden), sum(1 for c in golden if c.get("guarda")), "com guarda")


main()
