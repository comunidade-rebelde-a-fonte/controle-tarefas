"""Gabaritos do golden do extrator (tests/golden da origem), gerados pelo código do Conciliador.

Convenção da origem (tests/golden/README.md): extratos/<banco>/<AAAA-MM>.pdf e o .json gabarito
com saldo_inicial, saldo_final e transacoes[] (ordem, data, descricao, valor, sentido). Aqui o
gabarito de cada PDF do cenário (fictício, já anonimizado) sai do CSV do mesmo extrato pelo
extrato_de_csv da origem (app/domain/csv_extrato.py, commit 2bc6e26), o gabarito da aula.

Uso (Python 3.13 com pydantic 2.12.5):

    python gabarito_gen.py <caminho>/referencias/conciliador-contabil
"""

import json
import shutil
import sys
from datetime import date
from pathlib import Path

if len(sys.argv) != 2:
    sys.exit(__doc__)
sys.path.insert(0, str(Path(sys.argv[1]).resolve()))

from app.domain.csv_extrato import extrato_de_csv  # noqa: E402

AQUI = Path(__file__).resolve().parent
CENARIO = AQUI.parent / "conciliacao" / "extratos"
EXTRATOS = [
    ("a_fonte", "1234", "56789-0", "extrato_construtora_horizonte_2026-08"),
    ("beta", "0021", "33412-7", "extrato_banco_beta_2026-08"),
]

for banco, agencia, conta, nome in EXTRATOS:
    destino = AQUI / "extratos" / banco
    destino.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(CENARIO / f"{nome}.pdf", destino / "2026-08.pdf")
    e = extrato_de_csv((CENARIO / f"{nome}.csv").read_text(encoding="utf-8"), banco=banco, agencia=agencia, conta=conta,
                       periodo_inicio=date(2026, 8, 1), periodo_fim=date(2026, 8, 31))
    d = e.model_dump(mode="json")
    gabarito = {
        "banco": d["banco"], "agencia": d["agencia"], "conta": d["conta"],
        "periodo_inicio": d["periodo_inicio"], "periodo_fim": d["periodo_fim"],
        "saldo_inicial": d["saldo_inicial"], "saldo_final": d["saldo_final"],
        "transacoes": [{k: t[k] for k in ("ordem", "data", "descricao", "valor", "sentido")} for t in d["transacoes"]],
    }
    (destino / "2026-08.json").write_text(json.dumps(gabarito, ensure_ascii=False, indent=1) + "\n", encoding="utf-8", newline="\n")
    print(banco, len(gabarito["transacoes"]), gabarito["saldo_inicial"], "->", gabarito["saldo_final"])
