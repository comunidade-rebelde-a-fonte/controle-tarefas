"""Golden da classificação contábil, gerado pelo próprio código do Conciliador (commit 2bc6e26).

Roda app/domain/classificacao.py (classificar, aplicar_regra, regra_derivada_de_correcao), o
re.escape do Python (a regra derivada do descritivo) e a SugestaoClassificacao
(app/domain/schemas.py) sobre a saída do LLM como ClienteLLM.complete:
json.loads(extrair_json(conteudo)) e model_validate. Na sugestão, "guarda" marca o que o pydantic
aceita e o porte recusa: confiança fora de 0 a 1.

As regex dos casos ficam no subconjunto do re que o porte traduz (pytext.Compile); padrão fora dele
é recusado ao cadastrar a regra.

Uso (Python 3.13 com pydantic 2.12.5):

    python classification_gen.py <caminho>/referencias/conciliador-contabil
"""

import ast
import itertools
import json
import re
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

from app.domain.classificacao import HistoricoItem, Regra, classificar, regra_derivada_de_correcao  # noqa: E402
from app.domain.dedupe import normalizar_descricao  # noqa: E402
from app.domain.schemas import SugestaoClassificacao  # noqa: E402

AQUI = Path(__file__).resolve().parent


def _da_origem(arquivo: str, nomes: list[str], escopo: dict[str, Any]) -> dict[str, Any]:
    arvore = ast.parse((ORIGEM / arquivo).read_text(encoding="utf-8"))
    corpo = [n for n in arvore.body if isinstance(n, ast.FunctionDef) and n.name in nomes]
    exec(compile(ast.Module(body=corpo, type_ignores=[]), arquivo, "exec"), escopo)
    return escopo


extrair_json = _da_origem("app/agentes/llm.py", ["extrair_json"], {})["extrair_json"]

# --- Entradas: CNPJ, contraparte e descritivo como chegam da conciliação ---

CNPJS = ["12345678000199", "98765432000110", None, ""]
CONTRAPARTES = ["Alfa Comércio Ltda", "ALFA COMERCIO  LTDA", "Beta Serviços", "  Gama  ", "Escola Crescer", None, ""]
DESCRITIVOS = [
    "PIX ENVIADO ALFA COMERCIO LTDA",
    "Tarifa Pacote Serviços",
    "TARIFA PACOTE SERVICOS",
    "Pix Recebido Escola Crescer",
    "TED 123.456 (REF 7)",
    "Boleto Beta Serviços - NF 991",
    "DÉBITO AUTOMÁTICO ÁGUA",
    "",
    None,
]

REGRAS = [
    Regra("cnpj", "12345678000199", "2.01.01"),
    Regra("cnpj", "98765432000110", "2.01.02"),
    Regra("contraparte", "alfa comercio ltda", "2.01.03"),
    Regra("contraparte", "Beta  Serviços", "3.01.01"),
    Regra("contraparte", "Escola Crescer", "1.01.05"),
    Regra("regex_descritivo", "tarifa", "4.01.01"),
    Regra("regex_descritivo", r"^pix\s+recebido", "1.01.01"),
    Regra("regex_descritivo", r"\bTED\b", "1.01.02"),
    Regra("regex_descritivo", r"[0-9]{3}\.[0-9]{3}", "1.01.03"),
    Regra("regex_descritivo", r"(?:boleto|pix) (?:beta|alfa)", "2.01.04"),
    Regra("regex_descritivo", "servi[çc]os", "4.01.02"),
    Regra("regex_descritivo", "água|agua", "4.01.03"),
    Regra("regex_descritivo", re.escape("TED 123.456 (REF 7)"), "1.01.04"),
    Regra("regex_descritivo", "nf [0-9]+$", "2.01.05"),
]

HISTORICOS = [
    [],
    [HistoricoItem(normalizar_descricao("Alfa Comércio Ltda"), "9.01")],
    [HistoricoItem(normalizar_descricao("Tarifa Pacote Serviços"), "9.02"), HistoricoItem(normalizar_descricao("Tarifa Pacote Serviços"), "9.03")],
    [HistoricoItem(normalizar_descricao("Gama"), "9.04"), HistoricoItem("", "9.05")],
    [HistoricoItem(normalizar_descricao("Pix Recebido Escola Crescer"), "9.06"), HistoricoItem(normalizar_descricao("escola crescer"), "9.07")],
]


def subconjuntos_de_regras() -> list[list[Regra]]:
    """Listas de regras em várias ordens: a primeira que casa vale (a ordem é do chamador)."""
    out: list[list[Regra]] = [[], REGRAS, list(reversed(REGRAS))]
    for tipo in ["cnpj", "contraparte", "regex_descritivo"]:
        out.append([r for r in REGRAS if r.tipo == tipo])
    out.append([r for r in REGRAS if r.tipo != "cnpj"])
    out.append([REGRAS[5], REGRAS[2], REGRAS[0]])
    return out


def regra_json(r: Regra) -> dict[str, str]:
    return {"tipo": r.tipo, "padrao": r.padrao, "conta_codigo": r.conta_codigo}


def casos_classificar() -> list[dict[str, Any]]:
    """Cada caso aponta a lista de regras e o histórico pelo índice (listas_regras, historicos)."""
    out = []
    listas = subconjuntos_de_regras()
    entradas = list(itertools.product(CNPJS, CONTRAPARTES, DESCRITIVOS))
    # Regras e histórico alternados; depois, sem regra, cada histórico com todas as entradas.
    combinacoes = [(e, i % len(listas), i % len(HISTORICOS)) for i, e in enumerate(entradas)]
    combinacoes += [(e, 0, h) for h in range(1, len(HISTORICOS)) for e in entradas]
    for (cnpj, contraparte, descritivo), li, hi in combinacoes:
        r = classificar(cnpj=cnpj, contraparte=contraparte, descritivo=descritivo, regras=listas[li], historico=HISTORICOS[hi])
        out.append({
            "cnpj": cnpj, "contraparte": contraparte, "descritivo": descritivo, "regras": li, "historico": hi,
            "conta_codigo": r.conta_codigo, "origem": r.origem, "pendente": r.pendente,
        })
    return out


def casos_derivada() -> list[dict[str, Any]]:
    out = []
    for cnpj, contraparte, descritivo in itertools.product(CNPJS, CONTRAPARTES, DESCRITIVOS + ESPECIAIS):
        r = regra_derivada_de_correcao(cnpj=cnpj, contraparte=contraparte, descritivo=descritivo, conta_codigo="5.01")
        out.append({"cnpj": cnpj, "contraparte": contraparte, "descritivo": descritivo, "regra": regra_json(r)})
    return out


ESPECIAIS = [
    "PIX (ENVIADO) R$ 1.234,56 + tarifa*",
    "a-b|c^d$e",
    "tab\there\nnova linha\r\x0b\x0c",
    "#hash ~til &e {1,2} [x] ?ponto",
    "Ação Çedilha ÚÍ",
    "barra \\ invertida",
    "aspas \" e ' e `crase`",
    "%_=:;<>!@,/",
]


def casos_escape() -> list[dict[str, Any]]:
    out = []
    for d in [x for x in DESCRITIVOS + ESPECIAIS if x]:
        escapado = re.escape(d)
        outros = [x for x in DESCRITIVOS + ESPECIAIS if x]
        out.append({
            "texto": d, "escapado": escapado,
            # O padrão derivado casa com o próprio descritivo e com quais outros (IGNORECASE).
            "casa": [o for o in outros if re.search(escapado, o, flags=re.IGNORECASE)],
        })
    return out


# --- SugestaoClassificacao ---


def guarda(s: SugestaoClassificacao) -> list[str]:
    return [] if Decimal(0) <= s.confianca <= Decimal(1) else ["confianca"]


def validar(conteudo: str) -> dict[str, Any]:
    try:
        dado = json.loads(extrair_json(conteudo))
    except json.JSONDecodeError:
        return {"entrada": conteudo, "resultado": "invalido"}
    try:
        s = SugestaoClassificacao.model_validate(dado)
    except ValidationError:
        return {"entrada": conteudo, "resultado": "invalido"}
    except (TypeError, InvalidOperation, ValueError, ArithmeticError) as exc:
        return {"entrada": conteudo, "resultado": "excecao:" + type(exc).__name__}
    return {"entrada": conteudo, "resultado": "ok", "saida": s.model_dump(mode="json"), "guarda": guarda(s)}


def sugestao(**kw: Any) -> dict[str, Any]:
    base: dict[str, Any] = {
        "conta_codigo": "4.01.01", "conta_nome": "Despesas bancárias",
        "justificativa": "O descritivo parece ser tarifa do pacote de serviços do banco.",
        "regra_sugerida": {"tipo": "regex_descritivo", "padrao": "tarifa pacote"},
        "confianca": 0.7,
    }
    for k, v in kw.items():
        if v is ...:
            base.pop(k, None)
        else:
            base[k] = v
    return base


def casos_sugestao() -> list[str]:
    j = json.dumps
    out = [j(sugestao()), "```json\n" + j(sugestao()) + "\n```", "Sugestão:\n" + j(sugestao(), indent=2) + "\nfim", "{}"]
    for campo in ["conta_codigo", "conta_nome", "justificativa", "regra_sugerida", "confianca"]:
        out.append(j(sugestao(**{campo: ...})))
        out.append(j(sugestao(**{campo: None})))
    out += [j(sugestao(conta_codigo=401)), j(sugestao(conta_codigo="")), j(sugestao(conta_nome=["x"])), j(sugestao(justificativa=""))]
    for r in [{}, {"tipo": None, "padrao": None}, {"tipo": "cnpj", "padrao": "12345678000199"}, {"outra": [1, 2, {"a": True}]},
              "texto", [1], 5, True]:
        out.append(j(sugestao(regra_sugerida=r)))
    for c in [0, 1, 0.75, "0.75", "1", 1.5, -0.1, "NaN", "abc", True, 0.123456789, "1e-3"]:
        out.append(j(sugestao(confianca=c)))
    out += [j(sugestao(extra="ignorado")), "não é json", "[]", "null", '{"justificativa": "a", "confianca": 0.1} lixo']
    return out


def main() -> None:
    golden = {
        "listas_regras": [[regra_json(r) for r in lista] for lista in subconjuntos_de_regras()],
        "historicos": [[{"chave": h.chave, "conta_codigo": h.conta_codigo} for h in hist] for hist in HISTORICOS],
        "classificar": casos_classificar(),
        "derivada": casos_derivada(),
        "escape": casos_escape(),
        "sugestao": [validar(c) for c in casos_sugestao()],
    }
    (AQUI / "classification_golden.json").write_text(json.dumps(golden, ensure_ascii=False, indent=1) + "\n",
                                                     encoding="utf-8", newline="\n")
    print({k: len(v) for k, v in golden.items() if k not in ("listas_regras", "historicos")},
          Counter(c["origem"] for c in golden["classificar"]),
          Counter(c["resultado"] for c in golden["sugestao"]))


main()
