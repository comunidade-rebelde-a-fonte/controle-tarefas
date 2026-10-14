"""Gera o golden do motor Fator R a partir do código Python da origem (TEST-010, ADR-007).

Roda o domínio original (fator_r/domain: calcular, montar_carteira, simular) sobre casos
determinísticos e grava entradas e saídas em golden.json. O teste Go (golden_test.go) roda o
motor portado nas mesmas entradas e exige igualdade dígito a dígito: str() do Python contra
String() do pacote decimal, ou seja, mesmo valor e mesmo expoente.

Uso (Python 3.12, só biblioteca padrão; a origem fica congelada em referencias/, TASK-002):

    python golden_gen.py <caminho>/referencias/fator-r/apps/api/src

Os casos são reproduzíveis (semente fixa). Rodar de novo com a mesma origem gera o mesmo arquivo.
"""

import json
import platform
import random
import subprocess
import sys
from datetime import date
from decimal import Decimal
from pathlib import Path

SEMENTE = 20261007
CSV = Path(__file__).resolve().parents[4] / "seed" / "simples_tables_2018.csv"
SAIDA = Path(__file__).resolve().parent / "golden.json"

if len(sys.argv) != 2:
    sys.exit(__doc__)
ORIGEM = Path(sys.argv[1]).resolve()
sys.path.insert(0, str(ORIGEM))

from fator_r.core.competencia import somar_meses  # noqa: E402
from fator_r.domain.carteira import EmpresaCarteira, montar_carteira  # noqa: E402
from fator_r.domain.fator_r import MovimentoMes, Politica, calcular  # noqa: E402
from fator_r.domain.janela import janela  # noqa: E402
from fator_r.domain.simulador import ParametrosSimulacao, simular  # noqa: E402
from fator_r.domain.tabelas import Faixa, TabelaAnexo, TabelasVigentes  # noqa: E402

rng = random.Random(SEMENTE)
D = Decimal


def comp(d: date) -> str:
    return f"{d.year:04d}-{d.month:02d}"


def dcomp(s: str) -> date:
    ano, mes = s.split("-")
    return date(int(ano), int(mes), 1)


def texto(v: object) -> object:
    """Decimal → str() do Python; datas → AAAA-MM; o resto como está."""
    if isinstance(v, Decimal):
        return str(v)
    if isinstance(v, date):
        return comp(v)
    if isinstance(v, tuple):
        return [texto(x) for x in v]
    return v


def tabelas() -> TabelasVigentes:
    linhas = CSV.read_text(encoding="utf-8").strip().splitlines()[1:]
    por_anexo: dict[str, list[Faixa]] = {"III": [], "V": []}
    for linha in linhas:
        anexo, faixa, ate, nominal, deduzir, _inicio, _fim = linha.split(",")
        por_anexo[anexo].append(Faixa(int(faixa), D(ate), D(nominal), D(deduzir)))
    inicio = date(2018, 1, 1)
    return TabelasVigentes(
        anexo_iii=TabelaAnexo("III", inicio, tuple(por_anexo["III"])),
        anexo_v=TabelaAnexo("V", inicio, tuple(por_anexo["V"])),
    )


TABELAS = tabelas()


def dinheiro(minimo_centavos: int, maximo_centavos: int) -> str:
    c = rng.randint(minimo_centavos, maximo_centavos)
    return f"{c // 100}.{c % 100:02d}"


def movimento(m: date, receita: str, pro_labore: str, salarios: str, cpp: str, fgts: str, pgdas: bool) -> list[object]:
    """Formato compacto: [competência, receita, pró-labore, salários, CPP, FGTS, pgdas]."""
    return [comp(m), receita, pro_labore, salarios, cpp, fgts, 1 if pgdas else 0]


def para_python(movs: list[list[object]]) -> list[MovimentoMes]:
    # Na origem, a folha total do PGDAS-D vai inteira em salarios, com o resto zerado; o Go
    # recebe o mesmo valor em folha_total_declarada_pgdas.
    return [
        MovimentoMes(dcomp(str(m[0])), D(str(m[1])), D(str(m[2])), D(str(m[3])), D(str(m[4])), D(str(m[5])),
                     "pgdas" if m[6] else "manual")
        for m in movs
    ]


def serie_aleatoria(pa: date, perfil: str) -> list[list[object]]:
    j = janela(pa)
    receita_base = {
        "pequena": (100_00, 15_000_00),
        "media": (10_000_00, 60_000_00),
        "grande": (50_000_00, 300_000_00),
        "limite": (380_000_00, 420_000_00),
        "zerada": (0, 0),
    }[perfil]
    proporcao = rng.choice([0.05, 0.15, 0.25, 0.27, 0.28, 0.29, 0.30, 0.31, 0.35, 0.5])
    movs: list[list[object]] = []
    meses = list(j.meses)
    if rng.random() < 0.3:
        meses += [pa, somar_meses(j.inicio, -1)]  # fora da janela: ignorados
    for m in meses:
        if rng.random() > 0.88:
            continue
        receita = dinheiro(*receita_base)
        folha_alvo = int(D(receita) * D(str(proporcao)) * 100)
        if rng.random() < 0.15:  # mês do PGDAS-D: folha total sem divisão
            movs.append(movimento(m, receita, "0.00", dinheiro(0, max(folha_alvo, 1)), "0.00", "0.00", True))
            continue
        # Folha em torno da proporção sorteada, para cobrir os três semáforos.
        pro_labore_c = int(folha_alvo * rng.uniform(0.3, 0.7))
        salarios_c = max(0, int((folha_alvo - pro_labore_c) * rng.uniform(0.85, 1.0)))
        fgts_c = int(salarios_c * 0.08)
        cpp_c = rng.randint(0, max(folha_alvo // 10, 1))
        centavos = [pro_labore_c, salarios_c, cpp_c, fgts_c]
        pro_labore, salarios, cpp, fgts = (f"{c // 100}.{c % 100:02d}" for c in centavos)
        movs.append(movimento(m, receita, pro_labore, salarios, cpp, fgts, False))
    return movs


METAS = ["0.300000", "0.300000", "0.300000", "0.280000", "0.290000", "0.320000", "0.350000", "0.30", "0.999999"]


def politica_aleatoria() -> dict[str, object]:
    return {"cpp": rng.random() < 0.6, "meta": rng.choice(METAS)}


def pa_aleatorio() -> date:
    return date(rng.randint(2019, 2027), rng.randint(1, 12), 1)


def inicio_aleatorio(pa: date) -> str | None:
    j = janela(pa)
    sorteio = rng.random()
    if sorteio < 0.55:
        return None
    if sorteio < 0.70:
        return comp(somar_meses(j.inicio, -rng.randint(1, 60)))
    if sorteio < 0.75:
        return comp(j.inicio)
    if sorteio < 0.90:
        return comp(somar_meses(j.inicio, rng.randint(1, 11)))
    if sorteio < 0.95:
        return comp(pa)
    return comp(somar_meses(pa, rng.randint(1, 6)))


def resultado(r: object) -> dict[str, object]:
    campos = [
        "pa", "janela_inicio", "janela_fim", "meses_preenchidos", "meses_faltantes", "status", "motivo",
        "rbt12", "fs12", "fator_r", "anexo", "semaforo", "folha_minima_28", "folha_minima_meta",
        "gap_12m_28", "reforco_mensal_28", "gap_12m_meta", "reforco_mensal_meta", "aliquota_efetiva_iii",
        "aliquota_efetiva_v", "economia_12m", "meta_operacional", "cpp_integra_fs12",
    ]
    out = {c: texto(getattr(r, c)) for c in campos}
    out["tabela_vigencia_inicio"] = getattr(r, "tabela_vigencia_inicio").isoformat()
    return out


def caso_calculo(nome: str, pa: date, inicio: str | None, politica: dict[str, object], movs: list[list[object]]) -> dict[str, object]:
    r = calcular(
        pa=pa,
        inicio_atividade=dcomp(inicio) if inicio else None,
        movimentos=para_python(movs),
        tabelas=TABELAS,
        politica=Politica(cpp_das_integra_fs12=bool(politica["cpp"]), meta_operacional=D(str(politica["meta"]))),
    )
    return {
        "nome": nome, "pa": comp(pa), "inicio_atividade": inicio, "politica": politica,
        "movimentos": movs, "esperado": resultado(r), "_r": r,
    }


def serie_constante(pa: date, receita: str, folha: str, **kw: object) -> list[list[object]]:
    return [movimento(m, receita, folha, "0.00", "0.00", "0.00", False) for m in janela(pa).meses]


def casos_de_fronteira() -> list[dict[str, object]]:
    pa = date(2026, 9, 1)
    padrao = {"cpp": True, "meta": "0.300000"}
    casos = []
    # Corte legal e meta exatos e a um centavo: receita 50.000,00/mês.
    for folha in ["13999.99", "14000.00", "14000.01", "14999.99", "15000.00", "15000.01"]:
        casos.append(caso_calculo(f"fronteira folha {folha}", pa, None, padrao, serie_constante(pa, "50000.00", folha)))
    for meta, folha in [("0.320000", "16000.00"), ("0.320000", "15999.99"), ("0.280000", "14000.00"), ("0.999999", "49999.95")]:
        casos.append(caso_calculo(f"meta {meta} folha {folha}", pa, None, {"cpp": False, "meta": meta},
                                  serie_constante(pa, "50000.00", folha)))
    # Limites das faixas do RBT12 (soma de 12 meses).
    for total in ["180000.00", "180000.12", "360000.00", "360000.12", "720000.00", "1800000.00", "3600000.00",
                  "4800000.00", "4800000.12", "0.12"]:
        mensal = D(total) / 12
        receita = f"{mensal:.2f}"
        casos.append(caso_calculo(f"rbt12 {total}", pa, None, padrao, serie_constante(pa, receita, f"{mensal * D('0.29'):.2f}")))
    # RBT12 = 0, sem movimentos, só fora da janela, valores máximos da coluna.
    casos.append(caso_calculo("rbt12 zero", pa, None, padrao, serie_constante(pa, "0.00", "5000.00")))
    casos.append(caso_calculo("sem movimentos", pa, None, padrao, []))
    casos.append(caso_calculo("só fora da janela", pa, None, padrao,
                              [movimento(pa, "999999.00", "1.00", "0.00", "0.00", "0.00", False)]))
    casos.append(caso_calculo("máximo numeric(14,2)", pa, None, padrao,
                              serie_constante(pa, "999999999999.99", "999999999999.99")))
    # Empresa nova: 1, 5, 11, 12 e 13 meses; início igual ao PA e depois dele.
    for meses in [1, 5, 11, 12, 13]:
        inicio = comp(somar_meses(pa, -meses))
        casos.append(caso_calculo(f"empresa com {meses} meses", pa, inicio, padrao, serie_constante(pa, "50000.00", "15000.00")))
    casos.append(caso_calculo("início no PA", pa, comp(pa), padrao, serie_constante(pa, "50000.00", "15000.00")))
    # PGDAS-D: folha total declarada, com e sem a CPP na política.
    for cpp in [True, False]:
        movs = [movimento(m, "50000.00", "0.00", "14000.00", "0.00", "0.00", True) for m in janela(pa).meses]
        casos.append(caso_calculo(f"pgdas cpp={cpp}", pa, None, {"cpp": cpp, "meta": "0.300000"}, movs))
    # Janeiro vira o ano.
    casos.append(caso_calculo("PA janeiro", date(2027, 1, 1), None, padrao, serie_constante(date(2027, 1, 1), "30000.00", "9000.00")))
    return casos


def gerar() -> dict[str, object]:
    calculos = casos_de_fronteira()
    for i in range(200):
        pa = pa_aleatorio()
        perfil = rng.choice(["pequena", "media", "media", "grande", "limite", "zerada"])
        calculos.append(caso_calculo(f"aleatório {i} ({perfil})", pa, inicio_aleatorio(pa), politica_aleatoria(),
                                     serie_aleatoria(pa, perfil)))

    nomes = ["Alfa", "alfa", "Álvaro", "Beta", "Íris", "Zeta", "Ômega", "Gama", "Gama", "Delta", "Épsilon", "Kappa"]
    carteiras = []
    for i in range(18):
        pa = pa_aleatorio()
        politica = politica_aleatoria()
        empresas = []
        for k in range(rng.randint(0 if i == 0 else 1, 18)):
            perfil = rng.choice(["pequena", "media", "grande", "limite", "zerada"])
            empresas.append({
                "id": f"e{i}-{k}",
                "nome": rng.choice(nomes),
                "honorario": rng.choice(["0", "0.00", dinheiro(0, 500_000)]),
                "inicio_atividade": inicio_aleatorio(pa),
                "movimentos": serie_aleatoria(pa, perfil),
            })
        c = montar_carteira(
            pa=pa,
            empresas=[EmpresaCarteira(e["id"], e["nome"], "00000000000000", None, D(e["honorario"]),  # type: ignore[arg-type]
                                      dcomp(e["inicio_atividade"]) if e["inicio_atividade"] else None)
                      for e in empresas],
            movimentos={e["id"]: para_python(e["movimentos"]) for e in empresas},
            tabelas=TABELAS,
            politica=Politica(cpp_das_integra_fs12=bool(politica["cpp"]), meta_operacional=D(str(politica["meta"]))),
        )
        carteiras.append({
            "pa": comp(pa), "politica": politica, "empresas": empresas,
            "esperado": {
                "ordem": [linha.empresa.id for linha in c.linhas],
                "acoes": [linha.acao for linha in c.linhas],
                "kpis": {k: texto(v) for k, v in vars(c.kpis).items()},
            },
        })

    simulacoes = []
    for _ in range(160):
        indice = rng.randrange(len(calculos))
        p = {
            "meta": rng.choice(["0.28", "0.30", "0.300000", "0.32", "0.35", "0.4"]),
            "fracao_pro_labore": rng.choice(["0", "0.25", "0.5", "0.75", "1", "0.3333"]),
            "inss": rng.choice(["0.11", "0.20", "0.14"]),
            "irrf": rng.choice(["0", "0.075", "0.15", "0.225", "0.275"]),
            "horizonte_meses": rng.randint(1, 36),
            "piso_economia_anual": rng.choice(["0", "6000.00", "20000", "50000.00"]),
        }
        s = simular(calculos[indice]["_r"], ParametrosSimulacao(
            meta=D(p["meta"]), fracao_pro_labore=D(p["fracao_pro_labore"]), inss=D(p["inss"]), irrf=D(p["irrf"]),
            horizonte_meses=int(p["horizonte_meses"]), piso_economia_anual=D(p["piso_economia_anual"]),
        ))
        campos = ["status", "motivo", "veredito", "fator_atual", "reforco_12m", "reforco_mensal",
                  "pro_labore_extra_mensal", "pro_labore_extra_horizonte", "custo_inss", "custo_irrf",
                  "custo_total", "economia_12m", "economia_horizonte", "liquido"]
        simulacoes.append({"calculo": indice, "parametros": p, "esperado": {c: texto(getattr(s, c)) for c in campos}})

    for caso in calculos:
        del caso["_r"]
    try:
        commit = subprocess.run(["git", "-C", str(ORIGEM), "rev-parse", "--short", "HEAD"],
                                capture_output=True, text=True, check=True).stdout.strip()
    except (OSError, subprocess.CalledProcessError):
        commit = "desconhecido"
    return {
        "fonte": f"fator-r {commit} (apps/api/src/fator_r/domain)",
        "python": platform.python_version(),
        "semente": SEMENTE,
        "calculos": calculos,
        "carteiras": carteiras,
        "simulacoes": simulacoes,
    }


def escrever(dados: dict[str, object]) -> None:
    """JSON compacto com um caso por linha: arquivo menor e diff legível."""

    def compacto(v: object) -> str:
        return json.dumps(v, ensure_ascii=False, separators=(",", ":"))

    partes = []
    for chave, valor in dados.items():
        if isinstance(valor, list):
            itens = ",\n".join(compacto(item) for item in valor)
            partes.append(f"{compacto(chave)}:[\n{itens}\n]")
        else:
            partes.append(f"{compacto(chave)}:{compacto(valor)}")
    SAIDA.write_text("{\n" + ",\n".join(partes) + "\n}\n", encoding="utf-8", newline="\n")


if __name__ == "__main__":
    dados = gerar()
    escrever(dados)
    print(f"{SAIDA.name}: {len(dados['calculos'])} cálculos, {len(dados['carteiras'])} carteiras, "
          f"{len(dados['simulacoes'])} simulações ({dados['fonte']}, Python {dados['python']})")
