"""Gera o golden da chave de dedupe a partir do código Python da origem (TEST-007).

Roda app/domain/dedupe.py do Conciliador (commit 2bc6e26) sobre descrições com casos difíceis
de Unicode e grava entradas e saídas em dedupe_golden.json. O teste Go exige igualdade exata.

Uso (Python 3.12, só biblioteca padrão):

    python dedupe_gen.py <caminho>/referencias/conciliador-contabil
"""

import json
import sys
from datetime import date
from decimal import Decimal
from pathlib import Path

if len(sys.argv) != 2:
    sys.exit(__doc__)
sys.path.insert(0, str(Path(sys.argv[1]).resolve()))

from app.domain.dedupe import chave_dedupe, normalizar_descricao, ordens_no_dia  # noqa: E402

DESCRICOES = [
    "  Pix  JOÃO  ", "PIX RECEBIDO - José da Silva", "TARIFA BANCÁRIA\tPACOTE\nMENSAL",
    "ﬁnanciamento ﬀ ﬃ ﬆ", "Straße GROSSE", "ÇÃÕÉÍÚÂÊÔÀ çãõ", "Ångström Ω Å", "İstanbul ı I",
    "ǅǆǄ", "x\u00a0y", "a\u2003b\u3000c", "a\u1680b", "a\x1cb\x1dc\x1ed\x1fe", "a\u200bb",
    "ﬁ\u0301", "Ⅻ Ⅷ", "①② ⑳", "ＡＢＣ ｄｅｆ", "ꭰꮀ", "ΣΑΣ ς", "Pagamento 🚀 ok",
    "A\u030a e\u0301e\u0300", "ŉ ǰ ΐ", "한글 테스트", "ส้มตำ", "مَرْحَبًا", "שָׁלוֹם", "हिन्दी",
    "TED 123.456.789-00 CPF", "", "   ", "\u2028linha\u2029par", "DOC\x85TED", "x\u202fy\u205fz",
    "º ª ¹²³ ½", "ℌℍℎ", "Ⓐⓑ", "™ ℠", "ㄱㄴ", "ᄀ", "Ǳ", "ﬓ", "ᾳ ᾼ", "ẞ", "K Å",
]


def main() -> None:
    casos = []
    contas = ["12345-6", "000123", "9"]
    valores = ["0.01", "10.00", "1234567.89", "999999999999.99", "10.5", "7"]
    for i, desc in enumerate(DESCRICOES):
        conta = contas[i % len(contas)]
        valor = Decimal(valores[i % len(valores)]).quantize(Decimal("0.01"))
        data = date(2026, 1 + i % 12, 1 + i % 28)
        sentido = "C" if i % 2 else "D"
        ordem = 1 + i % 3
        casos.append({
            "conta": conta, "data": data.isoformat(), "valor": str(valor), "sentido": sentido,
            "descricao": desc, "ordem_no_dia": ordem,
            "normalizada": normalizar_descricao(desc),
            "chave": chave_dedupe(conta, data, valor, sentido, desc, ordem),
        })
    datas = [date(2026, 3, 1), date(2026, 3, 1), date(2026, 3, 2), date(2026, 3, 1), date(2026, 3, 2), date(2026, 3, 3)]
    saida = {
        "fonte": "conciliador-contabil 2bc6e26 (app/domain/dedupe.py)",
        "casos": casos,
        "ordens_no_dia": {"datas": [d.isoformat() for d in datas], "esperado": ordens_no_dia(datas)},
    }
    destino = Path(__file__).resolve().parent / "dedupe_golden.json"
    destino.write_text(json.dumps(saida, ensure_ascii=False, indent=1) + "\n", encoding="utf-8", newline="\n")
    print(f"{destino.name}: {len(casos)} casos")


if __name__ == "__main__":
    main()
