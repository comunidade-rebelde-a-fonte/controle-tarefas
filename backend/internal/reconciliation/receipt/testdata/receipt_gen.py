"""Golden do comprovante, gerado pelo próprio código do Conciliador (commit 2bc6e26).

Roda, sobre casos normais e de borda:
  - ComprovanteExtraido (app/domain/schemas.py) sobre a saída do LLM, como ClienteLLM.complete:
    json.loads(extrair_json(conteudo)) e model_validate;
  - parse_comprovante_texto (app/servicos/processar_local.py, tirado pela AST: o módulo importa o
    banco), o leitor determinístico, sobre o texto do PyMuPDF dos 13 comprovantes do cenário
    (../../testdata/conciliacao/comprovantes), os textos dos testes da origem e variações;
  - campos_abaixo_limiar (app/agentes/extrator_comprovante.py, idem), com o limiar 0,70.

A saída do LLM só traz os tipos do schema estrito (ver o golden do extrato). "guarda" marca o que o
pydantic aceita e o porte recusa: confiança fora de 0 a 1.

O leitor da origem procura a palavra "horizonte" (a empresa da aula) para decidir o sentido; o porte
procura o nome ou o CNPJ da empresa do documento. Os textos daqui trazem o nome completo
"Construtora Horizonte Ltda", em que as duas regras coincidem; o teste Go usa essa empresa.

Uso (Python 3.13 com pydantic 2.12.5 e PyMuPDF 1.25.4):

    python receipt_gen.py <caminho>/referencias/conciliador-contabil
"""

import ast
import json
import re
import sys
from decimal import Decimal, InvalidOperation
from pathlib import Path
from types import SimpleNamespace
from typing import Any

import pymupdf
from pydantic import ValidationError

if len(sys.argv) != 2:
    sys.exit(__doc__)
ORIGEM = Path(sys.argv[1]).resolve()
sys.path.insert(0, str(ORIGEM))

from app.domain.csv_extrato import parse_brl, parse_data_br  # noqa: E402
from app.domain.schemas import ComprovanteExtraido  # noqa: E402

AQUI = Path(__file__).resolve().parent
CENARIO = AQUI.parent.parent / "testdata" / "conciliacao" / "comprovantes"


def _da_origem(arquivo: str, nomes: list[str], escopo: dict[str, Any]) -> dict[str, Any]:
    arvore = ast.parse((ORIGEM / arquivo).read_text(encoding="utf-8"))
    corpo = [n for n in arvore.body if isinstance(n, ast.FunctionDef) and n.name in nomes]
    exec(compile(ast.Module(body=corpo, type_ignores=[]), arquivo, "exec"), escopo)
    return escopo


leitor = _da_origem("app/servicos/processar_local.py", ["_rotulo", "_emissor", "parse_comprovante_texto"],
                    {"re": re, "Decimal": Decimal, "ComprovanteExtraido": ComprovanteExtraido,
                     "parse_brl": parse_brl, "parse_data_br": parse_data_br})["parse_comprovante_texto"]
abaixo = _da_origem("app/agentes/extrator_comprovante.py", ["campos_abaixo_limiar"],
                    {"Decimal": Decimal, "ComprovanteExtraido": ComprovanteExtraido,
                     "settings": SimpleNamespace(limiar_campo="0.70")})["campos_abaixo_limiar"]
extrair_json = _da_origem("app/agentes/llm.py", ["extrair_json"], {})["extrair_json"]


def guarda(c: ComprovanteExtraido) -> list[str]:
    if any(not (Decimal(0) <= v <= Decimal(1)) for v in c.confianca_por_campo.values()):
        return ["confianca"]
    return []


def validar_saida(conteudo: str) -> dict[str, Any]:
    try:
        dado = json.loads(extrair_json(conteudo))
    except json.JSONDecodeError:
        return {"entrada": conteudo, "resultado": "invalido"}
    try:
        obj = ComprovanteExtraido.model_validate(dado)
    except ValidationError:
        return {"entrada": conteudo, "resultado": "invalido"}
    except (TypeError, InvalidOperation, ValueError, ArithmeticError) as exc:
        return {"entrada": conteudo, "resultado": "excecao:" + type(exc).__name__}
    return {"entrada": conteudo, "resultado": "ok", "saida": obj.model_dump(mode="json"), "guarda": guarda(obj),
            "abaixo_limiar": abaixo(obj)}


def comprovante(**kw: Any) -> dict[str, Any]:
    base: dict[str, Any] = {
        "data": "2026-08-05", "valor": "2000.00", "sentido": "C", "contraparte": "Ana Lucia Ferreira",
        "contraparte_doc": None, "documento": "E777", "tipo_operacao": "pix", "alertas": ["parcelado"],
        "confianca_por_campo": {"data": 0.98, "valor": 0.99, "sentido": 0.9, "contraparte": 0.95, "contraparte_doc": 0,
                                "documento": 0.8, "tipo_operacao": 1},
        "origem_por_campo": {"valor": {"pagina": 1, "trecho": "R$ 2.000,00", "bbox": [10, 20.5, 110, 40]},
                             "data": {"pagina": 1, "trecho": "05/08/2026", "bbox": None}},
    }
    for k, v in kw.items():
        if v is ...:
            base.pop(k, None)
        else:
            base[k] = v
    return base


def conf(**kw: Any) -> dict[str, Any]:
    base: dict[str, Any] = {"data": 0.9, "valor": 0.9, "sentido": 0.9, "contraparte": 0.9}
    base.update(kw)
    return {k: v for k, v in base.items() if v is not ...}


def casos_saida() -> list[str]:
    j = json.dumps
    out = [j(comprovante()), "```json\n" + j(comprovante()) + "\n```", "Segue:\n" + j(comprovante(), indent=2) + "\nok", "{}"]
    for v in ["2000", "1.234,56", "abc", "0.00", "-5.00", " 7.5 ", "1e3", "NaN", "Infinity", "", "999999999999.99", "1000000000000.00"]:
        out.append(j(comprovante(valor=v)))
    out += [j(comprovante(valor=None)), j(comprovante(valor=10)), j(comprovante(valor=10.5)), j(comprovante(valor=...))]
    for d in ["2026-08-05", "05/08/2026", "2026-02-30", "2026-08-05T00:00:00", "", None]:
        out.append(j(comprovante(data=d)))
    for s in ["C", "D", "c", "X", None, ""]:
        out.append(j(comprovante(sentido=s)))
    for campo in ["contraparte", "contraparte_doc", "documento", "tipo_operacao"]:
        out.append(j(comprovante(**{campo: None})))
        out.append(j(comprovante(**{campo: 5})))
    out.append(j(comprovante(tipo_operacao="darf")))
    for a in [[], ["juros", "nf:NF 1"], None, "parcelado", [1]]:
        out.append(j(comprovante(alertas=a)))
    out.append(j(comprovante(alertas=...)))
    for c in [conf(), conf(data=0.69), conf(valor=...), conf(sentido=0.7), conf(contraparte=0.1, data=0.2), {},
              conf(data="0.95"), conf(data=None), conf(data=1.5), conf(valor=-0.1), conf(extra=0.5), conf(data=True)]:
        out.append(j(comprovante(confianca_por_campo=c)))
    out.append(j(comprovante(confianca_por_campo=None)))
    out.append(j(comprovante(confianca_por_campo=[1])))
    for o in [{}, {"valor": {}}, {"valor": {"pagina": 2}}, {"valor": {"bbox": [1, 2]}},
              {"valor": {"bbox": ["a"]}}, {"valor": {"trecho": None}}, {"valor": None}, {"valor": {"pagina": 1.0, "extra": 1}},
              {"x": {"pagina": 3, "trecho": "t", "bbox": [0.5, 1.25, 2, 3]}}]:
        out.append(j(comprovante(origem_por_campo=o)))
    out.append(j(comprovante(origem_por_campo=None)))
    out.append(j(comprovante(extra="ignorado")))
    out += ["não é json", "[]", "null", '{"valor": NaN}', '{"data": "2026-08-05", "valor": "1.00"} lixo']
    return out


ORIGEM_TESTES = {
    "TED_RECEBIDA": """Banco Comercial Sul
Comprovante de transferência TED
Data e hora
04/08/2026 10:05
Valor
R$ 120.000,00
Remetente
Incorporadora Vale Verde Ltda
CNPJ
23.456.789/0001-01
Conta debitada
Banco Comercial Sul (555) Ag 0310 CC 44.120-8
Favorecido
Construtora Horizonte Ltda
CNPJ
12.345.678/0001-90
Conta de crédito
Banco A Fonte (777) Ag 1234 CC 56789-0
Descrição
NF 1041 - Medicao 03 Obra Residencial Aurora
""",
    "ORDEM_BANCARIA": """Prefeitura Municipal de Rio Claro - Tesouraria
Ordem bancária / Comprovante de TED
Data
12/08/2026 13:20
Ordem bancária
2026OB004512
Favorecido
Construtora Horizonte Ltda
CNPJ
12.345.678/0001-90
Documento fiscal
NFS-e 1044 - Pavimentacao Rua das Flores - parcela 2
Valor bruto
R$ 100.000,00
(-) ISS retido na fonte 5%
R$ 5.000,00
Valor líquido creditado
R$ 95.000,00
""",
    "DEBITO_AUTOMATICO": """Banco A Fonte
Comprovante de débito automático
Data do débito
19/08/2026
Valor debitado
R$ 2.780,00
Empresa conveniada
Seguradora Risco Engenharia S.A.
Titular da conta
Construtora Horizonte Ltda
Conta debitada
Banco A Fonte (777) Ag 1234 CC 56789-0
""",
    "PIX_RECEBIDO": """Banco Digital Nuvem
Comprovante de transferência PIX
Data e hora
05/08/2026 09:12
Valor
R$ 2.000,00
Pagador
Ana Lucia Ferreira
Recebedor
Construtora Horizonte Ltda
""",
}


def texto_pdf(caminho: Path) -> str:
    doc = pymupdf.open(caminho)
    try:
        return "\n".join(p.get_text() for p in doc)
    finally:
        doc.close()


def casos_leitor() -> list[str]:
    fixtures = [texto_pdf(p) for p in sorted(CENARIO.glob("*.pdf"))]
    pix = ORIGEM_TESTES["PIX_RECEBIDO"]
    variacoes = [
        "Comprovante\nsem data nem valor",
        "Comprovante de pagamento\nData do pagamento\n10/08/2026\nPagador\nConstrutora Horizonte Ltda\n",
        "Comprovante de pagamento\nValor pago\nR$ 1.234,56\n",
        "Comprovante\npago em 03/09/2026 o total de R$ 1.500,00 ao fornecedor",
        pix.replace("Pagador\nAna Lucia Ferreira\n", ""),
        pix.replace("Recebedor\n", "Favorecido\n"),
        pix + "Parcela 2 de 3\n",
        pix + "NFS-e 2201 serviço\n",
        pix + "(+) Juros\nR$ 10,00\n",
        pix + "(+) Juros\nR$ 0,00\n",
        pix + "(+) juros 1% ao mês\n",
        pix + "ISS retido 2%\n",
        "Comprovante de pagamento de salários (lote)\nData do processamento\n01/08/2026 07:30\nTipo\nFolha de pagamento - competência 07/2026\n"
        "Conta debitada\nBanco A Fonte (777) Ag 1234 CC 56789-0\nValor total do lote\nR$ 68.450,00\n",
        "Comprovante de arrecadação\nData do pagamento\n20/08/2026\nValor total pago\nR$ 9.400,00\nTipo de documento\nDARF\n",
        "Banco X - Agência Centro\nComprovante de TED\nData\n02/09/2026\nValor\nR$ 50,00\nRemetente\nFulano de Tal\n",
        "Comprovante de boleto\nData\n31/02/2026\nValor\nR$ 10,00\n",
        "Comprovante PIX\nData e hora\n05/08/2026\nValor\nR$ 0,00\nPagador\nConstrutora Horizonte Ltda\nRecebedor\nAlguém\n",
        "COMPROVANTE DE PAGAMENTO DE BOLETO\nDATA DO PAGAMENTO\n15/08/2026\nVALOR PAGO\nR$ 300,00\nBENEFICIÁRIO\nLoja Y\nPAGADOR\nConstrutora Horizonte Ltda\n",
        "Comprovante PIX\nData e hora\n٠٥/٠٨/٢٠٢٦ 10:00\nValor\nR$ ١٠,٠٠\nRecebedor\nConstrutora Horizonte Ltda\nPagador\nMaria\n",
    ]
    return fixtures + list(ORIGEM_TESTES.values()) + variacoes


def main() -> None:
    golden = {"saida": [validar_saida(c) for c in casos_saida()], "leitor": []}
    for texto in casos_leitor():
        try:
            c = leitor(texto)
            golden["leitor"].append({"entrada": texto, "resultado": "ok" if c else "nenhum",
                                     "saida": c.model_dump(mode="json") if c else None})
        except Exception as exc:  # a origem deixa a exceção subir
            golden["leitor"].append({"entrada": texto, "resultado": "erro:" + type(exc).__name__})
    (AQUI / "receipt_golden.json").write_text(json.dumps(golden, ensure_ascii=False, indent=1) + "\n", encoding="utf-8", newline="\n")
    from collections import Counter
    print({k: Counter(c["resultado"] for c in v) for k, v in golden.items()})


main()
