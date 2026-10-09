"""Golden da intenção do pedido (span plan), gerado pelo próprio código da origem.

- classificar_por_palavras e empresa_citada (agents/intencao.py): o fallback sem LLM;
- Intencao.model_validate (agents/llm.py, pydantic 2.13.5 do uv.lock): a validação da saída
  estruturada do LLM, inclusive o que vira None e o que é recusado.

As funções saem da origem pela AST (os módulos inteiros importam o SDK da Anthropic).
O teste Go (intent_test.go) exige o mesmo resultado.

Uso: python intent_gen.py <fator-r>/apps/api/src > intent_golden.json
     (com pydantic==2.13.5 no PYTHONPATH)
"""

import ast
import json
import re
import sys
import unicodedata
from decimal import Decimal
from pathlib import Path
from typing import Any, Literal

from pydantic import BaseModel, Field, ValidationError, field_validator

sys.path.insert(0, sys.argv[1])
SRC = Path(sys.argv[1])


def _da_origem(arquivo: str, nomes: list[str], escopo: dict[str, Any]) -> None:
    arvore = ast.parse((SRC / arquivo).read_text(encoding="utf-8"))
    corpo = [
        n
        for n in arvore.body
        if (isinstance(n, (ast.FunctionDef, ast.ClassDef)) and n.name in nomes)
        or (isinstance(n, (ast.Assign, ast.AnnAssign)) and any(
            isinstance(t, ast.Name) and t.id in nomes
            for t in (n.targets if isinstance(n, ast.Assign) else [n.target])
        ))
    ]
    exec(compile(ast.Module(body=corpo, type_ignores=[]), arquivo, "exec"), escopo)


escopo: dict[str, Any] = {
    "Any": Any, "Literal": Literal, "Decimal": Decimal, "BaseModel": BaseModel, "Field": Field,
    "field_validator": field_validator, "re": re, "unicodedata": unicodedata,
}
_da_origem("fator_r/agents/llm.py", ["TipoIntencao", "Intencao"], escopo)
_da_origem("fator_r/agents/intencao.py", ["_normalizar", "PALAVRAS", "empresa_citada", "classificar_por_palavras"], escopo)
Intencao = escopo["Intencao"]
classificar = escopo["classificar_por_palavras"]
citada = escopo["empresa_citada"]


def _intencao(i: Any) -> dict[str, Any]:
    return {"intencao": i.intencao, "company_ref": i.company_ref, "pa": i.pa,
            "meta": None if i.meta is None else str(i.meta)}


EMPRESAS = ["Clínica Alfa Ltda", "Beta Engenharia", "Alfa", "Beta Engenharia e Serviços", "Ação Já ME", "MÉDICOS 2000"]

MENSAGENS = [
    "simule a clinica alfa ltda para 09/2026 com meta de 32%",
    "o que priorizar esta semana?",
    "como está a Beta Engenharia?",
    "qual a situação da Beta Engenharia em 09/2026?",
    "explique o que é fator r",
    "o que é fator r?",
    "Por que o anexo mudou?",
    "Quais empresas atacar primeiro na carteira?",
    "situação em 2026-09 da beta engenharia e serviços",
    "simular pró-labore para Ação Já ME, meta 30%",
    "meta: 29,5 % para a alfa",
    "meta de 27% não vale",
    "meta 100%",
    "meta 3%",
    "meta de 28%",
    "meta 28,1 %",
    "pa 13/2026 e 12/2026",
    "pa 09/1999",
    "pa 1999-12 e 2101-01",
    "período 2026-13",
    "x09/2026 e 10/20261 e 11/2026",
    "status da MÉDICOS 2000 e da Alfa",
    "fila? reforço? correção?",
    "como funciona o reforço?",
    "nenhuma palavra conhecida",
    "",
    "Simulação para BETA ENGENHARIA",
    "porque? por que? porque",
    "situação ٠٩/٢٠٢٦",
]

LLM_SAIDAS = [
    {"intencao": "status_empresa", "company_ref": "Beta Engenharia", "pa": "2026-09", "meta": None},
    {"intencao": "simular", "company_ref": None, "pa": None, "meta": 0.3},
    {"intencao": "simular", "company_ref": None, "pa": None, "meta": 0.30},
    {"intencao": "simular", "company_ref": None, "pa": None, "meta": 0.295},
    {"intencao": "simular", "company_ref": None, "pa": None, "meta": 0.28},
    {"intencao": "simular", "company_ref": None, "pa": None, "meta": 0.27},
    {"intencao": "simular", "company_ref": None, "pa": None, "meta": 1},
    {"intencao": "simular", "company_ref": None, "pa": None, "meta": 0.999999},
    {"intencao": "explicar", "company_ref": None, "pa": "1999-12", "meta": None},
    {"intencao": "explicar", "company_ref": None, "pa": "2101-01", "meta": None},
    {"intencao": "explicar", "company_ref": None, "pa": "2100-12", "meta": None},
    {"intencao": "explicar", "company_ref": None, "pa": "2000-01", "meta": None},
    {"intencao": "explicar", "company_ref": None, "pa": "0001-01", "meta": None},
    {"intencao": "explicar", "company_ref": None, "pa": "2026-13", "meta": None},
    {"intencao": "explicar", "company_ref": None, "pa": "2026-9", "meta": None},
    {"intencao": "explicar", "company_ref": None, "pa": "09/2026", "meta": None},
    {"intencao": "explicar", "company_ref": None, "pa": "2026-09\n", "meta": None},
    {"intencao": "explicar", "company_ref": None, "pa": "٢٠٢٦-09", "meta": None},
    {"intencao": "priorizar", "company_ref": "", "pa": None, "meta": None},
    {"intencao": "inventada", "company_ref": None, "pa": None, "meta": None},
    {"intencao": "PRIORIZAR", "company_ref": None, "pa": None, "meta": None},
    {"intencao": "priorizar"},
    {"intencao": "priorizar", "extra": "ignorado"},
    {"company_ref": None, "pa": None, "meta": None},
    {"intencao": "simular", "company_ref": 123, "pa": None, "meta": None},
    {"intencao": "simular", "company_ref": None, "pa": 202609, "meta": None},
    {"intencao": "simular", "company_ref": None, "pa": None, "meta": "0.30"},
    {"intencao": "simular", "company_ref": None, "pa": None, "meta": True},
]


def _valida(saida: dict[str, Any]) -> dict[str, Any]:
    try:
        return {"ok": True, **_intencao(Intencao.model_validate(saida))}
    except ValidationError:
        return {"ok": False}


resultado = {
    "empresas": EMPRESAS,
    "palavras": [{"mensagem": m, **_intencao(classificar(m, EMPRESAS))} for m in MENSAGENS],
    "citada": [{"mensagem": m, "empresa": citada(m, EMPRESAS)} for m in MENSAGENS],
    "llm": [{"saida": s, "resultado": _valida(s)} for s in LLM_SAIDAS],
}
json.dump(resultado, sys.stdout, ensure_ascii=False, indent=1, sort_keys=True)
sys.stdout.write("\n")
