"""Golden do matching, gerado pelo próprio código do Conciliador (commit 2bc6e26).

Roda, sobre casos fixos, de borda e aleatórios (semente fixa):
  - jaro e jaro_winkler (app/domain/similaridade.py);
  - cascata e buscar_candidatos (app/domain/conciliacao.py): os passes 1 a 4 e a ferramenta do
    resolvedor;
  - _ids_divergentes (app/servicos/matching.py, tirado pela AST: o módulo importa o banco), o
    pré-filtro que tira do automático as linhas perto de comprovante marcado, a NF repetida, o
    valor duplicado e a contraparte repetida;
  - marcar_transferencia_interna, marcar_estorno e marcar_bruto_liquido (app/domain/bordas.py);
  - o cenário Horizonte inteiro como o scripts/carregar_horizonte.py da origem o carrega (os dois
    extratos pelo CSV, os lançamentos de contas a pagar e a receber e os 13 comprovantes pelo
    leitor determinístico) e o rodar_matching processa.

Os ids são UUIDs determinísticos. As listas ficam na ordem de entrada: a origem decide pela ordem.

Uso (Python 3.13 com pydantic 2.12.5 e PyMuPDF 1.25.4):

    python matching_gen.py <caminho>/referencias/conciliador-contabil
"""

import __future__
import ast
import csv
import json
import random
import re
import sys
from collections import Counter, defaultdict
from datetime import date, timedelta
from decimal import Decimal
from pathlib import Path
from types import SimpleNamespace
from typing import Any
from uuid import UUID

import pymupdf

if len(sys.argv) != 2:
    sys.exit(__doc__)
ORIGEM = Path(sys.argv[1]).resolve()
sys.path.insert(0, str(ORIGEM))

from app.domain.bordas import marcar_bruto_liquido, marcar_estorno, marcar_transferencia_interna  # noqa: E402
from app.domain.conciliacao import ItemExtrato, ItemLancamento, buscar_candidatos, cascata  # noqa: E402
from app.domain.csv_extrato import extrato_de_csv, parse_brl, parse_data_br  # noqa: E402
from app.domain.dedupe import normalizar_descricao  # noqa: E402
from app.domain.schemas import ComprovanteExtraido  # noqa: E402
from app.domain.similaridade import jaro, jaro_winkler  # noqa: E402

AQUI = Path(__file__).resolve().parent
CENARIO = AQUI.parent.parent / "testdata" / "conciliacao"


def _da_origem(arquivo: str, nomes: list[str], escopo: dict[str, Any]) -> dict[str, Any]:
    arvore = ast.parse((ORIGEM / arquivo).read_text(encoding="utf-8"))
    corpo = [n for n in arvore.body if isinstance(n, ast.FunctionDef) and n.name in nomes]
    codigo = compile(ast.Module(body=corpo, type_ignores=[]), arquivo, "exec",
                     flags=__future__.annotations.compiler_flag, dont_inherit=True)
    exec(codigo, escopo)
    return escopo


divergentes = _da_origem("app/servicos/matching.py", ["_ids_divergentes"],
                         {"defaultdict": defaultdict, "UUID": UUID, "normalizar_descricao": normalizar_descricao})["_ids_divergentes"]
leitor = _da_origem("app/servicos/processar_local.py", ["_rotulo", "_emissor", "parse_comprovante_texto"],
                    {"re": re, "Decimal": Decimal, "ComprovanteExtraido": ComprovanteExtraido,
                     "parse_brl": parse_brl, "parse_data_br": parse_data_br})["parse_comprovante_texto"]


def uid(prefixo: int, n: int) -> UUID:
    return UUID(int=(prefixo << 64) + n)


# --- Serialização ---

def j_ext(e: ItemExtrato) -> dict[str, Any]:
    return {"id": str(e.id), "data": e.data.isoformat(), "valor": str(e.valor), "sentido": e.sentido, "descricao": e.descricao}


def j_lan(lan: ItemLancamento) -> dict[str, Any]:
    return {"id": str(lan.id), "data": lan.data.isoformat(), "valor": str(lan.valor), "sentido": lan.sentido,
            "contraparte": lan.contraparte}


def j_lc(lan: SimpleNamespace) -> dict[str, Any]:
    return {"id": str(lan.id), "data": lan.data.isoformat() if lan.data else None,
            "valor": str(lan.valor) if lan.valor is not None else None, "sentido": lan.sentido,
            "contraparte": lan.contraparte, "documento": lan.documento, "marcas": lan.marcas}


def j_resultado(r: Any) -> dict[str, Any]:
    return {
        "matches": [{"extrato": str(m.extrato_id), "lancamentos": [str(x) for x in m.lancamento_ids], "passe": m.passe,
                     "score": str(m.score), "metodo": m.metodo} for m in r.matches],
        "divergencias": [{"extrato": str(e), "candidatos": [str(x) for x in ids], "motivo": motivo}
                         for e, ids, motivo in r.divergencias],
        "sem_comprovante": [str(x) for x in r.extrato_sem_comprovante],
        "sem_extrato": [str(x) for x in r.comprovante_sem_extrato],
    }


def j_marcas(marcas: list[Any]) -> list[dict[str, Any]]:
    return [{"tipo": m.tipo, "ids": [str(x) for x in m.ids], "detalhe": m.detalhe} for m in marcas]


# --- Jaro-Winkler ---

NOMES_E = ["PIX ENVIADO ALFA LTDA", "PAGTO FORNECEDOR ALFA LTDA", "PIX RECEBIDO BETA SA", "TED GAMA COMERCIO",
           "BOLETO DELTA", "TARIFA BANCARIA", "PIX RECEBIDO ANA LUCIA", "PAGAMENTO EPSILON", "DEB AUTOMATICO SEGURADORA"]
NOMES_L = ["Alfa Ltda", "Beta S.A.", "Gama Comercio Ltda", "Delta", "Banco", "Ana Lucia Ferreira", "Epsilon ME", "Zeta",
           "Seguradora Risco Engenharia S.A."]
UNICODE = ["  Pix  JOÃO  ", "PIX RECEBIDO - José da Silva", "Straße GROSSE", "ÇÃÕÉÍÚÂÊÔÀ çãõ", "İstanbul ı I",
           "ﬁnanciamento ﬀ", "a b", "Pagamento 🚀 ok", "한글 테스트", "", "   ", "x", "y"]
PARES = [("martha", "marhta"), ("dwayne", "duane"), ("dixon", "dicksonx"), ("abc", "abc"), ("abc", "xyz"), ("a", "a"),
         ("a", "b"), ("ab", "ba"), ("crate", "trace"), ("jones", "johnson"), ("", ""), ("abc", ""),
         ("PAGTO FORNECEDOR ALFA LTDA", "Alfa Ltda"), ("Joao", "João"), ("aaaa", "aaaab"), ("abcdefgh", "abcdefgx")]


def casos_jaro() -> list[dict[str, Any]]:
    pares = list(PARES)
    pares += [(a, b) for a in NOMES_E for b in NOMES_L]
    pares += [(a, b) for a in UNICODE for b in UNICODE[:6]]
    out = []
    for a, b in pares:
        out.append({"a": a, "b": b, "jaro": jaro(normalizar_descricao(a), normalizar_descricao(b)), "jw": jaro_winkler(a, b)})
    return out


# --- Cascata ---

VALORES = ["100.00", "200.00", "300.00", "150.00", "50.00", "250.00", "450.00"]
BASE = date(2026, 3, 1)


def caso_aleatorio(rng: random.Random, k: int) -> dict[str, Any]:
    exts = [ItemExtrato(uid(1, k * 100 + i), BASE + timedelta(days=rng.randint(0, 9)), Decimal(rng.choice(VALORES)),
                        rng.choice("DDDC"), rng.choice(NOMES_E)) for i in range(rng.randint(0, 5))]
    lans = [ItemLancamento(uid(2, k * 100 + i), BASE + timedelta(days=rng.randint(0, 9)), Decimal(rng.choice(VALORES)),
                           rng.choice("DDDC"), rng.choice(NOMES_L)) for i in range(rng.randint(0, 7))]
    limiar = rng.choice([0.86, 0.86, 0.5, 0.7])
    janela = rng.choice([3, 3, 3, 1, 0])
    max_n = rng.choice([8, 8, 3])
    return caso(exts, lans, limiar, janela, max_n)


def caso(exts: list[ItemExtrato], lans: list[ItemLancamento], limiar: float = 0.86, janela: int = 3,
         max_n: int = 8) -> dict[str, Any]:
    r = cascata(exts, lans, limiar_fuzzy=limiar, janela_dias=janela, max_n=max_n)
    return {"extratos": [j_ext(e) for e in exts], "lancamentos": [j_lan(x) for x in lans], "limiar": limiar,
            "janela": janela, "max_n": max_n, "resultado": j_resultado(r)}


def e(n: int, valor: str, dia: int, sentido: str = "D", desc: str = "alfa") -> ItemExtrato:
    return ItemExtrato(uid(1, n), date(2026, 3, dia), Decimal(valor), sentido, desc)


def lc(n: int, valor: str, dia: int, sentido: str = "D", nome: str = "Alfa Ltda") -> ItemLancamento:
    return ItemLancamento(uid(2, n), date(2026, 3, dia), Decimal(valor), sentido, nome)


def casos_cascata() -> list[dict[str, Any]]:
    fixos = [
        caso([e(1, "100.00", 10)], [lc(1, "100.00", 10)]),  # passe 1
        caso([e(1, "100.00", 10)], [lc(1, "100.00", 12)]),  # passe 2
        caso([e(1, "100.00", 10)], [lc(1, "100.00", 10), lc(2, "100.00", 10)]),  # ambíguo
        caso([e(1, "50.00", 10, desc="PAGTO FORNECEDOR ALFA LTDA")], [lc(1, "50.00", 20)], limiar=0.5),  # passe 3
        caso([e(1, "300.00", 10)], [lc(1, "100.00", 10), lc(2, "200.00", 10)]),  # passe 4
        caso([e(1, "300.00", 10)], [lc(1, "100.00", 10), lc(2, "200.00", 10), lc(3, "150.00", 10), lc(4, "150.00", 10)]),
        # Um comprovante para duas linhas iguais: a origem dá à primeira na ordem.
        caso([e(1, "100.00", 10, desc="PIX ANA"), e(2, "100.00", 10, desc="PIX BRUNO")], [lc(1, "100.00", 10, nome="Bruno")]),
        caso([e(1, "100.00", 10), e(2, "100.00", 12)], [lc(1, "100.00", 11)]),
        # Passe 4 com a mesma soma para duas linhas.
        caso([e(1, "300.00", 10), e(2, "300.00", 11)], [lc(1, "100.00", 10), lc(2, "200.00", 10)]),
        # Passe 4 limitado ao max_n.
        caso([e(1, "450.00", 10)], [lc(i, "50.00", 10) for i in range(1, 10)], max_n=8),
        caso([e(1, "450.00", 10)], [lc(i, "50.00", 10) for i in range(1, 10)], max_n=3),
        caso([], [lc(1, "1.00", 1)]),
        caso([e(1, "1.00", 1)], []),
    ]
    rng = random.Random(20261009)
    return fixos + [caso_aleatorio(rng, k) for k in range(1, 501)]


# --- Pré-filtro (_ids_divergentes) ---

MARCAS = [None, [], ["juros"], ["parcelado", "nf:NF 1"], ["iss_retido"], [{"tipo": "x"}], ["outro"], ["nf:NF 1"]]
DOCUMENTOS = [None, "NF 1", "nf 1", "NFS-E 2", "E777", "NF 1 ", "", "nfe 3"]
DESCRICOES = ["PIX RECEBIDO ANA LUCIA FERREIRA", "PIX ENVIADO ACO BRASIL DISTRIBUIDORA LTDA", "TARIFA BANCARIA PACOTE",
              "PIX RECEBIDO ANA LUCIA FERREIRA ME", "PIX ENVIADO HIDRAULICA TOTAL", "BOLETO CERAMICA SANTA RITA LTDA",
              "PAGAMENTO FOLHA", "PIX ENVIADO ACO BRASIL DISTRIB"]


def caso_divergentes(rng: random.Random, k: int) -> dict[str, Any]:
    exts = [ItemExtrato(uid(1, k * 100 + i), BASE + timedelta(days=rng.randint(0, 9)), Decimal(rng.choice(VALORES)),
                        rng.choice("DDC"), rng.choice(DESCRICOES)) for i in range(rng.randint(0, 6))]
    lans = []
    for i in range(rng.randint(0, 6)):
        marcas = rng.choice(MARCAS)
        lans.append(SimpleNamespace(
            id=uid(2, k * 100 + i),
            data=None if rng.random() < 0.15 else BASE + timedelta(days=rng.randint(0, 9)),
            valor=None if rng.random() < 0.1 else Decimal(rng.choice(VALORES)),
            sentido=rng.choice("DDC"), contraparte=rng.choice(NOMES_L), documento=rng.choice(DOCUMENTOS),
            marcas=list(marcas) if marcas is not None else None,
        ))
    skip_e, skip_l, motivos = divergentes(exts, lans)
    return {"extratos": [j_ext(x) for x in exts], "lancamentos": [j_lc(x) for x in lans],
            "pular_extratos": sorted(str(x) for x in skip_e), "pular_lancamentos": sorted(str(x) for x in skip_l),
            "motivos": {str(k2): v for k2, v in motivos.items()}}


# --- Bordas ---

def caso_bordas(rng: random.Random, k: int) -> dict[str, Any]:
    exts = [ItemExtrato(uid(1, k * 100 + i), BASE + timedelta(days=rng.randint(0, 12)), Decimal(rng.choice(VALORES[:4])),
                        rng.choice("DC"), rng.choice(NOMES_E)) for i in range(rng.randint(0, 6))]
    lans = [ItemLancamento(uid(2, k * 100 + i), BASE + timedelta(days=rng.randint(0, 12)),
                           Decimal(rng.choice(VALORES[:4])) + Decimal(rng.choice(["0.00", "5.00", "20.00", "20.01"])),
                           rng.choice("DC"), rng.choice(NOMES_L)) for i in range(rng.randint(0, 4))]
    return {"extratos": [j_ext(x) for x in exts], "lancamentos": [j_lan(x) for x in lans],
            "transferencia_interna": j_marcas(marcar_transferencia_interna(exts)),
            "estorno": j_marcas(marcar_estorno(exts)),
            "bruto_liquido": j_marcas(marcar_bruto_liquido(exts, lans))}


# --- buscar_candidatos (a ferramenta do resolvedor) ---

def caso_candidatos(rng: random.Random, k: int) -> dict[str, Any]:
    lans = [ItemLancamento(uid(2, k * 100 + i), BASE + timedelta(days=rng.randint(0, 9)), Decimal(rng.choice(VALORES)),
                           rng.choice("DC"), rng.choice(NOMES_L)) for i in range(rng.randint(0, 8))]
    valor, data, sentido = Decimal(rng.choice(VALORES)), BASE + timedelta(days=rng.randint(0, 9)), rng.choice("DC")
    janela = rng.choice([3, 3, 0, 5])
    r = buscar_candidatos(lans, valor=valor, data=data, sentido=sentido, janela_dias=janela)
    return {"lancamentos": [j_lan(x) for x in lans], "valor": str(valor), "data": data.isoformat(), "sentido": sentido,
            "janela": janela, "candidatos": [str(x.id) for x in r]}


# --- Cenário Horizonte (carregar_horizonte.py + rodar_matching) ---

CONTAS = [("a_fonte", "1234", "56789-0", "extrato_construtora_horizonte_2026-08.csv"),
          ("beta", "0021", "33412-7", "extrato_banco_beta_2026-08.csv")]


def texto_pdf(caminho: Path) -> str:
    doc = pymupdf.open(caminho)
    try:
        return "\n".join(p.get_text() for p in doc)
    finally:
        doc.close()


def cenario() -> dict[str, Any]:
    extratos: list[ItemExtrato] = []
    for banco, agencia, conta, arquivo in CONTAS:
        ext = extrato_de_csv((CENARIO / "extratos" / arquivo).read_text(encoding="utf-8"), banco=banco, agencia=agencia,
                             conta=conta, periodo_inicio=date(2026, 8, 1), periodo_fim=date(2026, 8, 31))
        for t in ext.transacoes:
            extratos.append(ItemExtrato(uid(3, len(extratos) + 1), t.data, t.valor, t.sentido, t.descricao))
    lans: list[SimpleNamespace] = []

    def novo(**kw: Any) -> None:
        lans.append(SimpleNamespace(id=uid(4, len(lans) + 1), **kw))

    with (CENARIO / "contabil" / "contas_a_pagar.csv").open(encoding="utf-8") as f:
        for row in csv.DictReader(f, delimiter=";"):
            m = re.search(r"(\d{2}/\d{2}/\d{4})", row["data_pagamento"])
            if not m:
                continue
            novo(data=parse_data_br(m.group(1)), valor=parse_brl(row["valor_pago"]), sentido="D",
                 contraparte=row["fornecedor"], documento=row["documento"], marcas=None, origem="pagar")
    with (CENARIO / "contabil" / "contas_a_receber.csv").open(encoding="utf-8") as f:
        for row in csv.DictReader(f, delimiter=";"):
            novo(data=parse_data_br(row["vencimento"]), valor=parse_brl(row["valor"]), sentido="C", contraparte=row["cliente"],
                 documento=row["nf"], marcas=None, origem="receber")
    for pdf in sorted((CENARIO / "comprovantes").glob("*.pdf")):
        c = leitor(texto_pdf(pdf))
        novo(data=c.data, valor=c.valor, sentido=c.sentido, contraparte=c.contraparte, documento=c.documento,
             marcas=list(c.alertas), origem=pdf.name)
    # rodar_matching: lançamentos completos, pré-filtro e cascata com o limiar da configuração.
    lancamentos = [ItemLancamento(x.id, x.data, x.valor, x.sentido, x.contraparte or "") for x in lans
                   if x.data and x.valor and x.sentido]
    skip_e, skip_l, motivos = divergentes(extratos, lans)
    r = cascata([x for x in extratos if x.id not in skip_e], [x for x in lancamentos if x.id not in skip_l],
                limiar_fuzzy=float("0.86"))
    return {"extratos": [j_ext(x) for x in extratos], "lancamentos": [j_lc(x) | {"origem": x.origem} for x in lans],
            "pular_extratos": sorted(str(x) for x in skip_e), "pular_lancamentos": sorted(str(x) for x in skip_l),
            "motivos": {str(k): v for k, v in motivos.items()}, "resultado": j_resultado(r)}


def main() -> None:
    rng = random.Random(4242)
    golden = {
        "jaro": casos_jaro(),
        "cascata": casos_cascata(),
        "divergentes": [caso_divergentes(rng, k) for k in range(1, 301)],
        "bordas": [caso_bordas(rng, k) for k in range(1, 201)],
        "candidatos": [caso_candidatos(rng, k) for k in range(1, 151)],
        "cenario": cenario(),
    }
    (AQUI / "matching_golden.json").write_text(json.dumps(golden, ensure_ascii=False, separators=(",", ":")) + "\n", encoding="utf-8",
                                               newline="\n")
    passes = Counter(m["passe"] for c in golden["cascata"] for m in c["resultado"]["matches"])
    motivos = Counter(d["motivo"] for c in golden["cascata"] for d in c["resultado"]["divergencias"])
    cen = golden["cenario"]["resultado"]
    print({"jaro": len(golden["jaro"]), "cascata": len(golden["cascata"]), "passes": dict(sorted(passes.items())),
           "divergencias": dict(motivos), "divergentes": len(golden["divergentes"]), "bordas": len(golden["bordas"]),
           "cenario": {"extratos": len(golden["cenario"]["extratos"]), "lancamentos": len(golden["cenario"]["lancamentos"]),
                       "matches": Counter(m["passe"] for m in cen["matches"]), "divergencias": len(cen["divergencias"]),
                       "pulados": len(golden["cenario"]["pular_extratos"]), "sem_comprovante": len(cen["sem_comprovante"])}})


main()
