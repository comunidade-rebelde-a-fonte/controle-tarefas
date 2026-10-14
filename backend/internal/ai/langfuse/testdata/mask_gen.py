"""Golden do mascaramento do que vai para o Langfuse (TASK-009).

Roda o código do próprio Fator R de origem (apps/api/src/fator_r/tracing/client.py, commit
9dd7a74): mascarar_cnpj, _limpar_payload e mascarar_atributo, extraídos da AST do arquivo para
não importar o SDK do Langfuse. Uso, a partir de backend/internal/ai/langfuse:

    python -I testdata/mask_gen.py <caminho>/fator-r/apps/api/src/fator_r/tracing/client.py \
        > testdata/mask_golden.json
"""

import ast
import json
import re
import sys
import typing

WANTED = {"_RE_CNPJ", "_RE_CPF", "mascarar_cnpj", "CAMPOS_SENSIVEIS", "MARCA_TRUNCADO",
          "_limpar_payload", "mascarar_atributo"}


def load(path: str) -> dict[str, typing.Any]:
    tree = ast.parse(open(path, encoding="utf-8").read())
    body = []
    for node in tree.body:
        if isinstance(node, ast.Assign) and any(isinstance(t, ast.Name) and t.id in WANTED for t in node.targets):
            body.append(node)
        elif isinstance(node, ast.FunctionDef) and node.name in WANTED:
            body.append(node)
    found = {getattr(n, "name", None) or n.targets[0].id for n in body}
    missing = WANTED - found
    if missing:
        raise SystemExit(f"não encontrado na origem: {sorted(missing)}")
    ns: dict[str, typing.Any] = {"re": re, "json": json, "Any": typing.Any}
    exec(compile(ast.Module(body=body, type_ignores=[]), path, "exec"), ns)
    return ns


ns = load(sys.argv[1])

textos = [
    "11222333000181",
    "11.222.333/0001-81",
    "11 222 333 0001 81",
    "CNPJ: 11.222.333/0001-81.",
    "cnpj11222333000181",
    "11222333000181x",
    "_11222333000181",
    "é11222333000181",
    "11222333000181é",
    "111222333000181",
    "1122233300018",
    "11-222-333-0001-81",
    "11.222.333.0001.81",
    "CPF 123.456.789-09",
    "12345678909",
    "123 456 789 09",
    "123.456.789-0",
    "1234567890912",
    "12.345.678/0001-90 e 98.765.432/0001-10",
    "CNPJ 12.345.678/0001-90, sócio CPF 123.456.789-09",
    "١١٢٢٢٣٣٣٠٠٠١٨١",
    "11 222 333 0001 81",
    "11 222 333/0001 81",
    "11\u001f222\u001f333\u001f0001\u001f81",
    "123　456　789　09",
    "(11.222.333/0001-81)",
    "a11.222.333/0001-81",
    "11.222.333/0001-811",
    "valor 1.234.567,89 e conta 12345-6",
    "texto sem número",
    "",
]

payloads = [
    ({"cnpj": "11222333000181", "texto_extraido": "conteúdo do pdf" * 10}, 1024),
    ({"a": [{"arquivo_bytes": "AAAA"}, "CPF 123.456.789-09"], "n": 5, "b": True, "z": None}, 1024),
    ({"t": "x" * 50, "u": "curto"}, 10),
    ({"t": "CNPJ 11222333000181 " + "y" * 30}, 20),
    ({"conteudo_arquivo": {"aninhado": "11222333000181"}, "lista": [["11.222.333/0001-81"]]}, 1024),
    ([{"texto_extraido": "x"}, "12345678909", 3.5], 1024),
]

atributos = [
    ("x" * 5000, 1024),
    ("é" * 700, 1023),
    ("CNPJ 11.222.333/0001-81 " + "z" * 100, 50),
    ("123", 1024),
    ('"texto com cnpj 11222333000181"', 1024),
    ("não é json: 12345678909", 1024),
    ("€" * 400, 1000),
]

golden = {
    "mascarar_cnpj": [{"entrada": t, "saida": ns["mascarar_cnpj"](t)} for t in textos],
    "limpar_payload": [
        {"entrada": p, "max": m, "saida": ns["_limpar_payload"](p, m)} for p, m in payloads
    ],
    "mascarar_atributo": [
        {"entrada": a, "max_bytes": m, "saida": ns["mascarar_atributo"](a, m)} for a, m in atributos
    ],
}
sys.stdout.reconfigure(encoding="utf-8")
json.dump(golden, sys.stdout, ensure_ascii=False, indent=1)
sys.stdout.write("\n")
