"""Golden do validador do texto do agente e dos formatadores, gerado pelo próprio código da origem.

Roda fator_r.agents.render.validar_texto e fator_r.core.money.formatar_brl /
formatar_percentual_br (Fator R, commit 9dd7a74; só biblioteca padrão) sobre os casos abaixo. O
teste Go (render_test.go) exige o mesmo resultado, órfão a órfão.

Uso: python render_gen.py <fator-r>/apps/api/src > render_golden.json
"""

import json
import sys

sys.path.insert(0, sys.argv[1])

from fator_r.agents.render import DISCLAIMER, validar_texto  # noqa: E402
from fator_r.core.money import formatar_brl, formatar_percentual_br  # noqa: E402

DECISAO = {
    "empresa": "Beta Engenharia",
    "pa": "2026-09",
    "fator_r": "0.240000",
    "rbt12": "600000.00",
    "reforco_mensal_meta": "3000.00",
    "horizonte_meses": 12,
    "veredito": "corrigir",
}
NAO_FORCAR = {"economia_12m": "43740.00", "horizonte_meses": 12, "veredito": "nao_forcar", "custo": "0.00"}
STATUS = {
    "intencao": "status_empresa",
    "empresa": "Clínica 2000 Ltda",
    "pa": "2026-09",
    "status": "ok",
    "motivo": None,
    "fator_r": "0.300000",
    "anexo": "III",
    "semaforo": "verde",
    "rbt12": "600000.00",
    "fs12": "180000.00",
    "gap_12m_28": "0.00",
    "reforco_mensal_28": "0.00",
    "gap_12m_meta": "0.00",
    "reforco_mensal_meta": "0.00",
    "meta_operacional": "0.300000",
    "economia_12m": "36000.00",
    "meses_faltantes": ["2026-01", "2026-02"],
}
FILA = {
    "intencao": "priorizar",
    "pa": "2026-09",
    "fila": [
        {"posicao": 1, "company_id": "0b6f7d1e-0000-4000-8000-000000000001", "empresa": "Vermelha",
         "semaforo": "vermelho", "fator_r": "0.100000", "economia_12m": "45000.00", "acao": "Corrigir folha"},
        {"posicao": 2, "company_id": "0b6f7d1e-0000-4000-8000-000000000002", "empresa": "Amarela 3",
         "semaforo": "amarelo", "fator_r": "0.290000", "economia_12m": None, "acao": "Monitorar"},
    ],
}
ESPECIAIS = {
    # Sem "NaN" e "Infinity": na origem, eles levantam InvalidOperation dentro de valores_permitidos.
    "sublinhado": "1_000",
    "arabe": "١٢",
    "expoente": "1e3",
    "negativo_zero": "-0",
    "grande": "12345678901234567890.50",
    "fracao_um": "1",
    "fracao": "0.5",
    "bool": True,
    "nada": None,
    "lista": ["2025-12", 7, "x"],
    "competencia_com_quebra": "2026-08\n",
}

CASOS = [
    # Origem: test_render_com_numeros_da_decisao_passa / numero_orfao / M2.
    ("ok_frase_completa", "A Beta Engenharia está com Fator R de 24,00% no PA 2026-09, com RBT12 de R$ 600.000,00.", DECISAO),
    ("ok_reforco", "Reforço mensal de R$ 3.000,00 por 12 meses leva à meta.", DECISAO),
    ("ok_formas", "Fator R em 24% e receita de 600000.00.", DECISAO),
    ("orfao_inventado", "A economia estimada é de R$ 43.740,00.", DECISAO),
    ("orfao_arredondado", "O Fator R está em 25%.", DECISAO),
    ("orfao_competencia", "Situação do PA 2026-10.", DECISAO),
    ("m2_extenso", "A economia é de trinta mil reais.", NAO_FORCAR),
    ("m2_mil", "Economia de R$ 12 mil no ano.", NAO_FORCAR),
    ("m2_colado", "Código x45000 aplicado.", NAO_FORCAR),
    ("m2_sinal", "Perda de R$-99.000,00.", NAO_FORCAR),
    ("m2_contradiz", "Vale corrigir a folha agora.", NAO_FORCAR),
    ("m2_coerente", "Não forçar: economia de R$ 43.740,00 em 12 meses.", NAO_FORCAR),
    # Veredito.
    ("corrigir_contradito", "Não vale a pena; não compensa mexer.", DECISAO),
    ("corrigir_coerente", "Vale corrigir: reforço de R$ 3.000,00.", DECISAO),
    ("recomendamos", "Recomendamos a corrigir a folha.", {**NAO_FORCAR, "veredito": "ja_na_meta"}),
    ("convem", "Convém corrigir.", NAO_FORCAR),
    ("sem_veredito", "Vale corrigir e não forçar.", {"economia_12m": "1.00"}),
    # Datas.
    ("data_barra", "Em 09/2026 o Fator R foi 24,00%.", DECISAO),
    ("data_barra_orfa", "Em 10/2026 o Fator R foi 24,00%.", DECISAO),
    ("data_lista", "Faltam 2026-01 e 01/2026 e 2026-03.", STATUS),
    ("data_colada", "Código x2026-09 e 2026-091.", DECISAO),
    ("data_mes_invalido", "No 2026-13 nada.", DECISAO),
    # Números e formas.
    ("percentual_inteiro", "Fator R de 30% e meta de 30,00%.", STATUS),
    ("percentual_fracao", "Fator 0.300000 e 0,3 e 0.3.", STATUS),
    ("milhar_sem_virgula", "RBT12 de 600.000 e FS12 de 180.000.", STATUS),
    ("milhar_negativo", "Valor de -1.000 e 1.2345.", STATUS),
    ("zero_e_menos_zero", "Gap de 0 e -0 e 0,00.", STATUS),
    ("economia", "Economia de R$ 36.000,00 em 12 meses.", STATUS),
    ("unicode_digitos", "Valor ١٢ e 12.", ESPECIAIS),
    ("especiais", "Mil? 1000 e 1.000 e 1_000 e 0,5 e 50% e 100%.", ESPECIAIS),
    ("grande", "Total 12345678901234567890,50 e 12.345.678.901.234.567.890,50.", ESPECIAIS),
    ("lista_e_nada", "Item 7 e 2025-12 e 2026-08.", ESPECIAIS),
    ("fila", "Priorize Vermelha (economia R$ 45.000,00) e Amarela 3; 2 empresas em 2026-09.", FILA),
    ("fila_posicao", "1. Vermelha; 2. Amarela 3 com 10% e 29%.", FILA),
    ("nome_com_numero", "A Clínica 2000 Ltda está bem.", STATUS),
    ("extenso_variantes", "Duzentos e quinhentas; MILHÕES e milhao, cento e onze.", STATUS),
    ("extenso_dentro_de_palavra", "Família vinteana e milhares.", STATUS),
    ("disclaimer", "Fator R de 30,00%.\n\n" + DISCLAIMER, STATUS),
    ("vazio", "", STATUS),
    ("sem_numeros", "Situação estável.", {}),
]

FORMATOS = ["600000.00", "0", "-0", "1234.5", "-1234.565", "0.005", "999.995", "1E+3", "12345678901234567890.123"]
PERCENTUAIS = ["0.240000", "0.3", "1", "0", "-0.1234", "0.12345"]

saida = {
    "casos": [
        {"nome": nome, "texto": texto, "decisao": decisao, "valido": v.valido, "orfaos": list(v.orfaos)}
        for nome, texto, decisao in CASOS
        for v in [validar_texto(texto, decisao)]
    ],
    "brl": {f: formatar_brl(f) for f in FORMATOS} | {"null": formatar_brl(None)},
    "percentual": {p: formatar_percentual_br(p) for p in PERCENTUAIS} | {"null": formatar_percentual_br(None)},
    "disclaimer": DISCLAIMER,
}
json.dump(saida, sys.stdout, ensure_ascii=False, indent=1, sort_keys=True)
sys.stdout.write("\n")
