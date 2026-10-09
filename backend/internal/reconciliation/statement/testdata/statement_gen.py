"""Golden do domínio do extrato, gerado pelo próprio código do Conciliador (commit 2bc6e26).

Roda, sobre casos normais e de borda:
  - extrair_json (app/agentes/llm.py, tirado pela AST: o módulo importa o banco);
  - ExtratoExtraido e ResultadoTriagem (app/domain/schemas.py) sobre a saída do LLM, como
    ClienteLLM.complete: json.loads(extrair_json(conteudo)) e model_validate;
  - validar_extrato e dicas_correcao (app/domain/validador.py), com o JSON de model_dump_json,
    que vai no feedback da nova tentativa;
  - extrato_de_csv, metadados_txt (app/domain/csv_extrato.py) e eh_extrato,
    extrato_de_pdf_texto (app/domain/pdf_extrato.py), estes sobre o texto do PyMuPDF dos PDFs
    do cenário (../../testdata/conciliacao).

A saída do LLM só traz os tipos do schema estrito (texto, inteiro, número, nulo, lista e
objeto onde o schema pede): fora disso o porte recusa, mesmo onde o pydantic convertia (ver
schema_test.go). "guarda" marca o que o pydantic aceita e o porte recusa por limite do banco
(ordem e página fora de 1 a 2³¹−1, confiança fora de 0 a 1, valor fora de numeric(14,2),
período invertido).

Uso (Python 3.13 com pydantic 2.12.5 e PyMuPDF 1.25.4; a origem pede pydantic>=2.10 e
pymupdf>=1.24):

    python statement_gen.py <caminho>/referencias/conciliador-contabil
"""

import ast
import csv  # noqa: F401  (usado pelo extrato_de_csv da origem)
import json
import sys
from datetime import date
from decimal import Decimal, InvalidOperation
from pathlib import Path
from typing import Any

import pymupdf
from pydantic import ValidationError

if len(sys.argv) != 2:
    sys.exit(__doc__)
ORIGEM = Path(sys.argv[1]).resolve()
sys.path.insert(0, str(ORIGEM))

from app.domain.csv_extrato import extrato_de_csv, metadados_txt  # noqa: E402
from app.domain.pdf_extrato import eh_extrato, extrato_de_pdf_texto  # noqa: E402
from app.domain.schemas import ExtratoExtraido, ResultadoTriagem  # noqa: E402
from app.domain.validador import dicas_correcao, validar_extrato  # noqa: E402

AQUI = Path(__file__).resolve().parent
CENARIO = AQUI.parent.parent / "testdata" / "conciliacao"


def _da_origem(arquivo: str, nomes: list[str]) -> dict[str, Any]:
    arvore = ast.parse((ORIGEM / arquivo).read_text(encoding="utf-8"))
    corpo = [n for n in arvore.body if isinstance(n, ast.FunctionDef) and n.name in nomes]
    escopo: dict[str, Any] = {}
    exec(compile(ast.Module(body=corpo, type_ignores=[]), arquivo, "exec"), escopo)
    return escopo


extrair_json = _da_origem("app/agentes/llm.py", ["extrair_json"])["extrair_json"]

LIMITE = Decimal("1000000000000")  # numeric(14,2): |valor| < 10^12
INT32 = 2**31 - 1  # ordem e página: integer do Postgres


def guarda_extrato(e: ExtratoExtraido) -> list[str]:
    g = []
    if e.periodo_fim < e.periodo_inicio:
        g.append("periodo_invertido")
    dinheiro = [e.saldo_inicial, e.saldo_final]
    for tx in e.transacoes:
        if not (1 <= tx.ordem <= INT32):
            g.append("ordem")
        if not (Decimal(0) <= tx.confianca <= Decimal(1)):
            g.append("confianca")
        dinheiro.append(tx.valor)
        if tx.saldo_apos is not None:
            dinheiro.append(tx.saldo_apos)
    if any(abs(v) >= LIMITE for v in dinheiro):
        g.append("valor")
    if e.leiaute is not None and not (1 <= e.leiaute.pagina_inicio_tabela <= INT32):
        g.append("pagina")
    if e.leiaute is not None and any(abs(p) > INT32 for p in e.leiaute.paginas_ignorar):
        g.append("pagina")
    return sorted(set(g))


def guarda_triagem(t: ResultadoTriagem) -> list[str]:
    g = []
    if not (Decimal(0) <= t.confianca <= Decimal(1)):
        g.append("confianca")
    if t.periodo_inicio and t.periodo_fim and t.periodo_fim < t.periodo_inicio:
        g.append("periodo_invertido")
    return g


def validar_saida(modelo, conteudo: str, guarda) -> dict[str, Any]:
    """O caminho do ClienteLLM.complete: extrair_json, json.loads e model_validate."""
    try:
        dado = json.loads(extrair_json(conteudo))
    except json.JSONDecodeError:
        return {"entrada": conteudo, "resultado": "invalido"}
    try:
        obj = modelo.model_validate(dado)
    except ValidationError:
        return {"entrada": conteudo, "resultado": "invalido"}
    except (TypeError, InvalidOperation, ValueError, ArithmeticError) as exc:
        # Na origem, estes derrubavam o job; no porte, viram saída inválida.
        return {"entrada": conteudo, "resultado": "excecao:" + type(exc).__name__}
    return {"entrada": conteudo, "resultado": "ok", "saida": obj.model_dump(mode="json"), "guarda": guarda(obj)}


# --- extrair_json ---

ENTRADAS_JSON = [
    '{"a": 1}', '```json\n{"a": 1}\n```', '  ```\n{"a": [1, 2]}\n```  \n', '```json{"a":1}```',
    "```", "```\n", "``` \n{\"a\": 1}", 'Segue a hipótese:\n{"hipotese": "x", "n": 2}\nQualquer dúvida, avise.',
    "não consegui analisar", "", "   ", '[1, 2]', ' [1] ', 'prefixo [1, 2] sufixo', '}{', 'a } b { c',
    '{"a": "}"} trailing', ' {"a": 1} ', '```JSON\n{"a": 1}\n```\nmais texto', 'x {"a": 1} y {"b": 2} z',
    '```json\n```', '\n\n```json\n{"x": "```"}\n```\n\n', "{'a': 1}", '{"a": 1}}', 'texto {sem fechar',
]


# --- saída do extrator ---

def extrato(**kw: Any) -> dict[str, Any]:
    base: dict[str, Any] = {
        "banco": "itau", "agencia": "0001", "conta": "12345-6",
        "periodo_inicio": "2026-03-01", "periodo_fim": "2026-03-31",
        "saldo_inicial": "100.00", "saldo_final": "130.00",
        "transacoes": [
            {"ordem": 1, "data": "2026-03-02", "descricao": "PIX RECEBIDO", "documento": None, "valor": "50.00", "sentido": "C", "saldo_apos": "150.00", "confianca": 0.95},
            {"ordem": 2, "data": "2026-03-03", "descricao": "TARIFA", "documento": "123", "valor": "20.00", "sentido": "D", "saldo_apos": None, "confianca": 1},
        ],
        "leiaute": {"pagina_inicio_tabela": 1, "paginas_ignorar": [], "formato_data": "DD/MM/YYYY", "separador_decimal": ",",
                    "representacao_sinal": "coluna C/D", "localizacao_saldos": "cabeçalho e rodapé", "estrategia": "texto_nativo"},
    }
    for k, v in kw.items():
        if v is ...:
            base.pop(k, None)
        else:
            base[k] = v
    return base


def tx(**kw: Any) -> dict[str, Any]:
    base = {"ordem": 1, "data": "2026-03-02", "descricao": "X", "documento": None, "valor": "10.00", "sentido": "C", "saldo_apos": None, "confianca": 0.9}
    for k, v in kw.items():
        if v is ...:
            base.pop(k, None)
        else:
            base[k] = v
    return base


def casos_extrato() -> list[str]:
    j = json.dumps
    out = [j(extrato()), "```json\n" + j(extrato()) + "\n```", "Aqui está:\n" + j(extrato(), indent=2) + "\nfim"]
    # Dinheiro como texto (o schema pede texto decimal).
    for v in ["10", "10.5", "10.005", "10.015", " 10.00 ", "1_000.00", "1e3", "-5.00", "0.00", "0.001", "abc", "1.234,56",
              "NaN", "Infinity", "-0.00", "999999999999.99", "1000000000000.00", "+7.10", "٣.٥٠", ""]:
        out.append(j(extrato(transacoes=[tx(valor=v)], saldo_inicial="0.00", saldo_final="10.00")))
    for v in ["-100.00", "0", "12.345", "abc", "1e2", None]:
        out.append(j(extrato(saldo_inicial=v)))
    for v in ["150.00", None, "abc", "-1.50"]:
        out.append(j(extrato(transacoes=[tx(saldo_apos=v)])))
    # Inteiros no lugar do dinheiro (o pydantic aceita; o JSON estrito também pode trazer).
    out.append(j(extrato(saldo_inicial=100, saldo_final=130)))
    out.append(j(extrato(transacoes=[tx(valor=10)], saldo_inicial=0, saldo_final=10)))
    # Datas.
    for d in ["2026-03-02", "2026-3-2", "02/03/2026", "2026-02-30", "2026-03-02T00:00:00", "", "0000-01-01", "9999-12-31", "2026-13-01"]:
        out.append(j(extrato(transacoes=[tx(data=d)])))
    for d in ["2026-04-01", "2026-03-01", "2026/03/31"]:
        out.append(j(extrato(periodo_fim=d)))
    # Sentido, ordem, confiança, descrição e documento.
    for s in ["C", "D", "c", "X", "", " D"]:
        out.append(j(extrato(transacoes=[tx(sentido=s)])))
    for o in [1, 0, -1, 2, 1.0, 1.5, 10**20]:
        out.append(j(extrato(transacoes=[tx(ordem=o)])))
    for c in [0, 1, 0.5, 0.123456, 1.5, -0.1, 2, None]:
        out.append(j(extrato(transacoes=[tx(confianca=c)])))
    out.append(j(extrato(transacoes=[tx(confianca=...)])))
    for desc in ["", "  com espaços  ", "ação ç"]:
        out.append(j(extrato(transacoes=[tx(descricao=desc)])))
    for doc in ["", "001", None]:
        out.append(j(extrato(transacoes=[tx(documento=doc)])))
    out.append(j(extrato(transacoes=[tx(documento=...)])))
    out.append(j(extrato(transacoes=[tx(saldo_apos=...)])))
    # Campos faltando ou a mais.
    for campo in ["banco", "agencia", "conta", "periodo_inicio", "saldo_final", "transacoes"]:
        out.append(j(extrato(**{campo: ...})))
    out.append(j(extrato(extra="ignorado", outro=[1])))
    out.append(j(extrato(transacoes=[tx(extra=1)])))
    out.append(j(extrato(transacoes=[])))
    out.append(j(extrato(transacoes={})))
    out.append(j(extrato(banco="")))
    # Leiaute.
    out.append(j(extrato(leiaute=None)))
    out.append(j(extrato(leiaute=...)))
    out.append(j(extrato(leiaute={})))
    out.append(j(extrato(leiaute={"estrategia": "render_150", "extra": 1})))
    out.append(j(extrato(leiaute={"pagina_inicio_tabela": 0})))
    out.append(j(extrato(leiaute={"pagina_inicio_tabela": 2, "paginas_ignorar": [3, 4]})))
    out.append(j(extrato(leiaute={"paginas_ignorar": ["a"]})))
    out.append(j(extrato(leiaute="texto")))
    # JSON quebrado, NaN do json.loads e valores fora do tipo.
    out += ['{"banco": "itau"', "[]", "null", '"texto"', '{"banco": NaN}', "nada de json aqui",
            j(extrato()).replace('"saldo_inicial": "100.00"', '"saldo_inicial": NaN'),
            j(extrato(transacoes=[tx(valor=10.5)])), j(extrato(banco=5)), j(extrato(transacoes=[tx(descricao=5)])),
            j(extrato(transacoes=[tx(sentido=None)])), j(extrato(transacoes=[tx(data=None)])),
            '{"banco": "a", "banco": "b"}']
    dup = j(extrato())[:-1] + ', "banco": "segundo"}'
    out.append(dup)
    return out


def triagem(**kw: Any) -> str:
    base: dict[str, Any] = {"tipo": "extrato", "banco": "itau", "agencia": "0001", "conta": "12345-6",
                            "periodo_inicio": "2026-03-01", "periodo_fim": "2026-03-31", "confianca": 0.92,
                            "justificativa": "cabeçalho do banco"}
    for k, v in kw.items():
        if v is ...:
            base.pop(k, None)
        else:
            base[k] = v
    return json.dumps(base)


def casos_triagem() -> list[str]:
    out = [triagem(), "```json\n" + triagem() + "\n```"]
    for t in ["extrato", "comprovante", "desconhecido", "Extrato", "outro", ""]:
        out.append(triagem(tipo=t))
    for c in [0, 1, 0.7, 0.69999, 0.123456789, 1.2, -0.5, "0.8", None]:
        out.append(triagem(confianca=c))
    out.append(triagem(confianca=...))
    for campo in ["banco", "agencia", "conta", "periodo_inicio", "periodo_fim", "justificativa"]:
        out.append(triagem(**{campo: None}))
        out.append(triagem(**{campo: ...}))
    out.append(triagem(periodo_inicio="2026-04-01"))
    out.append(triagem(periodo_fim="31/03/2026"))
    out.append(triagem(banco=7))
    out.append(triagem(extra="x"))
    out.append(triagem(tipo=...))
    out += ["{}", "[]", "sem json", '{"tipo": "extrato", "confianca": NaN}']
    return out


# --- validador ---

def ext_valido(si: str, sf: str, txs: list[tuple[int, str, str, str]], ini: str = "2026-03-01", fim: str = "2026-03-31") -> ExtratoExtraido:
    return ExtratoExtraido.model_validate({
        "banco": "itau", "agencia": "1", "conta": "1", "periodo_inicio": ini, "periodo_fim": fim,
        "saldo_inicial": si, "saldo_final": sf,
        "transacoes": [{"ordem": o, "data": d, "descricao": "x", "valor": v, "sentido": s} for (o, d, v, s) in txs],
    })


def casos_validador() -> list[ExtratoExtraido]:
    d = "2026-03-02"
    casos = [
        ext_valido("100.00", "130.00", [(1, d, "50.00", "C"), (2, d, "20.00", "D")]),
        ext_valido("100.00", "150.01", [(1, d, "50.00", "C")]),
        ext_valido("0.00", "20.00", [(1, d, "10.00", "C"), (3, d, "10.00", "C")]),
        ext_valido("0.00", "10.00", [(1, "2026-04-02", "10.00", "C")]),
        ext_valido("0.00", "0.00", []),
        ext_valido("50.00", "40.00", []),
        ext_valido("-100.00", "-50.00", [(1, d, "50.00", "C")]),
        # Dicas: diferença igual a uma linha (pulo ou duplicata).
        ext_valido("100.00", "180.00", [(1, d, "50.00", "C"), (2, d, "30.00", "C"), (3, d, "30.00", "C")]),
        # Ordem de grandeza: diferença = 100× e 0,01× a primeira linha.
        ext_valido("0.00", "1.00", [(1, d, "100.00", "C")]),
        ext_valido("0.00", "100.00", [(1, d, "1.00", "C")]),
        ext_valido("0.00", "0.00", [(1, d, "0.50", "C"), (2, d, "50.50", "D")]),
        # Diferença ≥ 100 dentro e fora da faixa 99..101 × max(primeira linha, 1).
        ext_valido("0.00", "0.00", [(1, d, "1.00", "C"), (2, d, "100.00", "C")]),
        ext_valido("0.00", "0.00", [(1, d, "0.50", "C"), (2, d, "101.00", "C")]),
        ext_valido("0.00", "0.00", [(1, d, "0.50", "C"), (2, d, "101.50", "C")]),
        ext_valido("0.00", "0.00", [(1, d, "2.00", "C"), (2, d, "150.00", "C")]),
        ext_valido("0.00", "0.00", [(1, d, "2.00", "C"), (2, d, "300.00", "C")]),
        ext_valido("1000.00", "0.00", [(1, d, "3.33", "C")]),
        ext_valido("0.00", "-0.01", [(1, d, "0.01", "D"), (2, d, "0.01", "D")]),
        ext_valido("0.00", "0.00", [(5, d, "1.00", "C"), (2, d, "1.00", "D"), (9, d, "3.00", "C"), (7, d, "3.00", "D")]),
        ext_valido("0.00", "0.00", [(1, "2026-02-28", "1.00", "C"), (2, "2026-03-31", "1.00", "D"), (3, "2026-03-01", "2.00", "C"), (4, "2026-04-01", "2.00", "D")]),
        ext_valido("999999999999.99", "0.00", [(1, d, "999999999999.99", "D")]),
    ]
    # Linhas rejeitadas só aparecem sem o schema (model_construct): o validador é puro.
    tx_ruim = ExtratoExtraido.model_construct(
        banco="x", agencia="1", conta="1", periodo_inicio=date(2026, 3, 1), periodo_fim=date(2026, 3, 31),
        saldo_inicial=Decimal("0.00"), saldo_final=Decimal("0.00"), leiaute=None,
        transacoes=[
            ExtratoExtraido.model_fields["transacoes"].annotation.__args__[0].model_construct(
                ordem=1, data=date(2026, 3, 2), descricao="x", documento=None, valor=Decimal("0.00"), sentido="C", saldo_apos=None, confianca=Decimal("0")),
            ExtratoExtraido.model_fields["transacoes"].annotation.__args__[0].model_construct(
                ordem=2, data=date(2026, 3, 2), descricao="x", documento=None, valor=Decimal("5.00"), sentido="X", saldo_apos=None, confianca=Decimal("0")),
            ExtratoExtraido.model_fields["transacoes"].annotation.__args__[0].model_construct(
                ordem=3, data=date(2026, 3, 2), descricao="x", documento=None, valor=Decimal("-5.00"), sentido="Y", saldo_apos=None, confianca=Decimal("0")),
        ],
    )
    casos.append(tx_ruim)
    return casos


# --- CSV, TXT e texto do PDF ---

def csv_texto(linhas: list[str], cab: str = "data;descricao;documento;tipo;valor;saldo") -> str:
    return "\n".join([cab] + linhas) + "\n"


def casos_csv() -> list[dict[str, Any]]:
    fonte = (CENARIO / "extratos" / "extrato_construtora_horizonte_2026-08.csv").read_text(encoding="utf-8")
    beta = (CENARIO / "extratos" / "extrato_banco_beta_2026-08.csv").read_text(encoding="utf-8")
    linhas = ["01/08/2026;TARIFA;1;D;-89,90;85.340,22", "02/08/2026;PIX;2;C;1.000,00;86.340,22"]
    conteudos = [
        fonte, beta, csv_texto(linhas), csv_texto(linhas).replace("\n", "\r\n"),
        csv_texto(["01/08/2026;  TARIFA  ;;d;89,90;10,00"]),
        csv_texto(['01/08/2026;"PIX; COM PONTO E VÍRGULA";7;C;"1.000,00";"1.010,00"']),
        csv_texto(['01/08/2026;"aspas ""duplas""";;C;R$ 5,00;R$ 15,00']),
        csv_texto(["01/08/2026;X;;C;-;10,00"]), csv_texto(["01/08/2026;X;;C;;10,00"]),
        csv_texto(["01/08/2026;X;;X;5,00;10,00"]), csv_texto(["01/08/2026;X;;C;5,00"]),
        csv_texto(["01/08/2026;X;;C;5,00;10,00;extra"]), csv_texto(["2026-08-01;X;;C;5,00;10,00"]),
        csv_texto(["31/02/2026;X;;C;5,00;10,00"]), csv_texto(["1/8/2026;X;;C;5,00;10,00"]),
        csv_texto(["01/08/2026;X;;C;abc;10,00"]), csv_texto(["01/08/2026;X;;C;5.000;10,00"]),
        csv_texto([]), "", csv_texto(linhas, cab="data;descricao;tipo;valor;saldo"),
        csv_texto(linhas, cab="Data;Descricao;Documento;Tipo;Valor;Saldo"),
        csv_texto(["01/08/2026;X;;C;5,00;10,00", "", "02/08/2026;Y;;D;1,00;9,00"]),
        "﻿" + csv_texto(linhas), csv_texto(["01/08/2026;X;;C;0,00;10,00"]),
        csv_texto(["01/08/2026;X;;C;5,00;10,00"]) + "02/08/2026;\"sem fechar;;C;1,00;11,00\n",
        csv_texto(["01/08/2026;X;;C;5,00;10,00", "02/08/2026;Y;;D;1,00;9,00", "02/08/2026;Z;;C;3,00;12,00"]),
        csv_texto([" 01 / 08 / 2026 ;X;;C;5,00;10,00"]),
    ]
    casos = []
    for c in conteudos:
        caso: dict[str, Any] = {"entrada": c, "banco": "a_fonte", "agencia": "1234", "conta": "56789-0", "inicio": "2026-08-01", "fim": "2026-08-31"}
        try:
            e = extrato_de_csv(c, banco="a_fonte", agencia="1234", conta="56789-0", periodo_inicio=date(2026, 8, 1), periodo_fim=date(2026, 8, 31))
            caso.update(resultado="ok", saida=e.model_dump(mode="json"))
        except Exception as exc:  # a origem deixa a exceção subir
            caso.update(resultado="erro:" + type(exc).__name__)
        casos.append(caso)
    return casos


def casos_txt() -> list[dict[str, Any]]:
    txt = (CENARIO / "extratos" / "extrato_construtora_horizonte_2026-08.txt").read_text(encoding="utf-8")
    out = []
    for c in [txt, "Agencia: 0021 Conta: 33412-7", "AGENCIA:0021   CONTA:  33412-7 x", "agencia: 1\nconta: 2", "Agência: 1 Conta: 2",
              "Agencia: Conta: 2", "nada", "Agencia: 1 Conta: 2"]:
        try:
            ag, conta = metadados_txt(c)
            out.append({"entrada": c, "resultado": "ok", "agencia": ag, "conta": conta})
        except Exception as exc:
            out.append({"entrada": c, "resultado": "erro:" + type(exc).__name__})
    return out


def texto_pdf(nome: str) -> str:
    doc = pymupdf.open(nome)
    try:
        return "\n".join(p.get_text() for p in doc)
    finally:
        doc.close()


def casos_pdf() -> list[dict[str, Any]]:
    fonte = texto_pdf(CENARIO / "extratos" / "extrato_construtora_horizonte_2026-08.pdf")
    beta = texto_pdf(CENARIO / "extratos" / "extrato_banco_beta_2026-08.pdf")
    textos = [fonte, beta]
    textos += [texto_pdf(p) for p in sorted((CENARIO / "comprovantes").glob("*.pdf"))[:4]]
    textos += [
        beta.replace("Banco Beta", "Banco Gama"),
        beta.replace("SALDO ANTERIOR", "SALDO INICIAL"),
        beta.replace("01/08/2026 a 31/08/2026", "agosto de 2026"),
        beta.replace("Agência 0021 - Conta 33412-7", "Ag 0021 Cc 33412-7"),
        beta.replace("12.640,00 D", "12.640,00 d"),
        beta.replace("Saldo final em 31/08/2026: R$ 22.325,00", "Saldo final\n22.300,00"),
        beta.replace("01/08/2026\nSALDO ANTERIOR\n15.000,00\n", ""),
        fonte.replace("Agência\n1234", "Agência\n1234-"),
        fonte.replace("85.430,12", "85.430,1"),
        fonte.replace("260801", "DOC"),
        "extrato conta corrente sem saldo",
        "SALDO ANTERIOR mas sem a palavra",
        "Extrato\nSALDO ANTERIOR\nAgência\n1\nConta corrente\n2\n01/01/2026 a 31/01/2026\n",
        "Extrato\nSALDO ANTERIOR\nAgência\n1\nConta corrente\n2\n01/01/2026 a 31/01/2026\n02/01/2026\nPIX\n9\n10,00 C\n10,00\n",
        "Extrato\nsaldo anterior\nAgência\n1\nConta corrente\n2\n01/01/2026 a 31/01/2026\n02/01/2026\nPIX\n9\n10,00 C\n10,00\nSaldo final: 9,00\n",
        "Conta corrente extrato\nSALDO ANTERIOR\nAgência 7 - Conta 8\n01/01/2026 a 31/01/2026\n02/01/2026\nPIX\n9\n10,00 C\n10,00\n",
        "Extrato\nSALDO ANTERIOR\nAgência\n1\nConta corrente\n2\n31/02/2026 a 31/01/2026\n02/01/2026\nPIX\n9\n10,00 C\n10,00\n",
        "Extrato\nSALDO ANTERIOR\nAgência\n1\nConta corrente\n2\n01/01/2026 a 31/01/2026\n02/01/2026\nPIX\n9\n0,00 C\n10,00\n",
        "Extrato\nSALDO ANTERIOR\nAgência\n١\nConta corrente\n٢\n٠١/٠١/٢٠٢٦ a 31/01/2026\n02/01/2026\nPIX\n9\n10,00 C\n10,00\n",
    ]
    out = []
    for t in textos:
        caso: dict[str, Any] = {"entrada": t, "eh_extrato": eh_extrato(t)}
        try:
            caso.update(resultado="ok", saida=extrato_de_pdf_texto(t).model_dump(mode="json"))
        except Exception as exc:
            caso.update(resultado="erro:" + type(exc).__name__)
        out.append(caso)
    return out


def main() -> None:
    golden = {
        "extrair_json": [{"entrada": e, "saida": extrair_json(e)} for e in ENTRADAS_JSON],
        "extrato": [validar_saida(ExtratoExtraido, c, guarda_extrato) for c in casos_extrato()],
        "triagem": [validar_saida(ResultadoTriagem, c, guarda_triagem) for c in casos_triagem()],
        "validador": [],
        "csv": casos_csv(),
        "txt": casos_txt(),
        "pdf": casos_pdf(),
    }
    for e in casos_validador():
        r = validar_extrato(e)
        golden["validador"].append({
            "entrada": {
                "periodo_inicio": e.periodo_inicio.isoformat(), "periodo_fim": e.periodo_fim.isoformat(),
                "saldo_inicial": str(e.saldo_inicial), "saldo_final": str(e.saldo_final),
                "transacoes": [{"ordem": t.ordem, "data": t.data.isoformat(), "valor": str(t.valor), "sentido": t.sentido} for t in e.transacoes],
            },
            "json": r.model_dump_json(),
            "dicas": dicas_correcao(e, r),
        })
    (AQUI / "statement_golden.json").write_text(json.dumps(golden, ensure_ascii=False, indent=1) + "\n", encoding="utf-8", newline="\n")
    print({k: len(v) for k, v in golden.items()})


main()
