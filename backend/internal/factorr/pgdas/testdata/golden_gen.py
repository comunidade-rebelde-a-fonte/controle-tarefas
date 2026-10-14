"""Golden do parser PGDAS-D gerado pelo próprio parser da origem (Fator R, commit 9dd7a74).

Roda fator_r.parsing.pgdas.parse sobre o texto das fixtures da origem (TXT decodificado como a
origem; PDF pelo pdfplumber, como extrair_texto) e sobre variações que exercitam a semântica do
re do Python (\\s, \\d e \\b Unicode, IGNORECASE, lookahead, backtracking) e as regras do parser.
O teste Go (golden_test.go) exige o mesmo resultado, campo a campo.

Uso (Python com pdfplumber 0.11.10 e pdfminer.six 20260107, as versões do uv.lock da origem):

    python golden_gen.py <fator-r>/apps/api/src <fator-r>/apps/api/fixtures/pgdas > golden.json
"""

import ast
import json
import sys
from pathlib import Path
from typing import Any

sys.path.insert(0, sys.argv[1])

from fator_r.core.uploads import detectar_mime  # noqa: E402
from fator_r.parsing.pgdas import ResultadoParse, parse  # noqa: E402
from fator_r.parsing.texto import extrair_texto  # noqa: E402

FIXTURES = Path(sys.argv[2])


def _da_origem(arquivo: str, funcao: str, **globais: Any) -> Any:
    """Função copiada da origem pela AST (o módulo inteiro exige banco, Langfuse etc.)."""
    arvore = ast.parse((Path(sys.argv[1]) / arquivo).read_text(encoding="utf-8"))
    no = next(n for n in arvore.body if isinstance(n, ast.FunctionDef) and n.name == funcao)
    escopo: dict[str, Any] = dict(globais)
    exec(compile(ast.Module(body=[no], type_ignores=[]), arquivo, "exec"), escopo)
    return escopo[funcao]


# O JSON gravado no documento (campos_json), como o agente parser_pgdas monta.
_campos_json = _da_origem("fator_r/agents/parser_pgdas.py", "_campos_json", Any=Any, ResultadoParse=ResultadoParse)


def declaratorio(receitas, folhas, rbt12, fs12, pa="01/08/2026 a 31/08/2026", meses=None, extra=()):
    meses = meses or [
        "08/2025", "09/2025", "10/2025", "11/2025", "12/2025", "01/2026",
        "02/2026", "03/2026", "04/2026", "05/2026", "06/2026", "07/2026",
    ]
    linhas = [
        f"Periodo de Apuracao: {pa}",
        "Data de abertura no CNPJ: 12/03/2019",
        f"Receita bruta acumulada nos 12 meses anteriores (RBT12) {rbt12} 0,00 {rbt12}",
        "2.2 Receitas Brutas Anteriores (R$) - Mercado Interno",
        "Mes Valor Mes Valor Mes Valor",
    ]
    for i in range(0, 12, 3):
        linhas.append(" ".join(f"{meses[j]} {receitas[j]}" for j in range(i, i + 3)))
    linhas.extend(extra)
    linhas.append("2.3 Folha de Salarios Anteriores (R$)")
    if folhas is not None:
        for i in range(0, 12, 3):
            linhas.append(" ".join(f"{meses[j]} {folhas[j]}" for j in range(i, i + 3)))
        linhas.append(f"Total FS12 {fs12}")
    linhas += ["2.4 Fator r", "Fator r = 0,20 - Anexo V"]
    return "\n".join(linhas)


PADRAO = (
    "Extrato do Simples Nacional\n"
    "CNPJ Matriz: 11.222.333/0001-81\n"
    "Nome empresarial: CLINICA EXEMPLO LTDA\n"
    "Período de Apuração (PA): 09/2026\n"
    "Receita Bruta do PA (RPA) 50.000,00 0,00 50.000,00\n"
    "Receita bruta acumulada nos doze meses anteriores ao PA (RBT12) 600.000,00\n"
    "Total de Folhas de Salários Anteriores 180.000,00\n"
    "Fator r: 0,30\n"
    "Valor Total do Débito Declarado 4.500,00\n"
    "Enquadramento Anexo III\n"
)

VARIACOES = {
    # --- campos e confiança ---
    "padrao": PADRAO,
    "vazio": "",
    "so_espacos": " \t \n\n  ",
    "cnpj_dv_invalido": PADRAO.replace("0001-81", "0001-82"),
    "cnpj_todos_iguais": "CNPJ: 11.111.111/1111-11\nPA: 09/2026",
    "cnpj_sem_separadores": "CNPJ 11222333000181",
    "cnpj_minusculo_matriz": "cnpj   matriz - 11222333000181",
    "cnpj_matriz_i_sem_ponto": "CNPJ Matrız: 11.222.333/0001-81",
    "cnpj_matriz_i_com_ponto": "CNPJ MATRİZ: 11.222.333/0001-81",
    "cnpj_colado_em_letra": "xCNPJ: 11.222.333/0001-81\nCNPJ: 11.444.777/0001-61",
    "cnpj_colado_em_acento": "éCNPJ: 11.222.333/0001-81",
    "cnpj_seguido_de_digito": "CNPJ: 11.222.333/0001-812\nCNPJ: 11.444.777/0001-61 fim",
    "cnpj_seguido_de_letra": "CNPJ: 11.222.333/0001-81X",
    "cnpj_seguido_de_hifen": "CNPJ: 11.222.333/0001-81-9",
    "cnpj_espaco_fino": "CNPJ: 11.222.333/0001-81",
    "cnpj_digitos_arabes": "CNPJ: ١١.٢٢٢.٣٣٣/٠٠٠١-81",
    "cnpj_digitos_arabes_dv": "CNPJ: 11222333000١٨١",
    "pa_mes_13_depois_valido": "PA: 13/2026\nPA: 09/2026",
    "pa_primeiro_regex_tem_precedencia": "PA: 09/2026\nPeríodo de Apuração: 08/2026",
    "pa_periodo_sem_acentos": "Periodo de Apuracao 07/2026",
    "pa_maiusculas": "PERÍODO DE APURAÇÃO (PA): 07/2026",
    "pa_i_sem_ponto": "Perıodo de Apuração: 07/2026",
    "pa_colado": "PA: 09/20261\nPA:09/2026.",
    "pa_minusculo_nao_vale": "pa: 09/2026",
    "pa_intervalo_meses_diferentes": "Periodo de Apuracao: 01/08/2026 a 30/09/2026",
    "pa_intervalo_valido": "Período de Apuração (PA): 01/12/2025 a 31/12/2025",
    "pa_digitos_arabes": "PA: ٠٩/٢٠٢٦",
    "pa_ano_antigo_sem_series": "PA: 08/1899\nRBT12: 1.000,00",
    "rotulo_ultimo_numero": "RBT12: 100,00 200,00 300.000,00",
    "rotulo_sem_numero_segue_linha": "RBT12: nada aqui\nRBT12: 12.345,67",
    "rotulo_regex_em_ordem": "RBT12: 1,00\nReceita bruta acumulada nos doze meses anteriores ao PA (RBT12) 2,00",
    "rotulo_12_meses_sem_ao_pa": "Receita bruta 12 meses anteriores (RBT12) 9.999,99",
    "rpa_rotulo_curto": "RPA - 1.234,56",
    "rpa_colado_em_letra": "xRPA: 1,00\nRPA: 2,00",
    "rpa_sem_borda_final": "RPA2: 3,00",
    "fs12_total_inicio_linha": "Total FS12 50.000,00\nx Total FS12 60.000,00",
    "fs12_total_meio_da_linha": "x Total FS12 60.000,00",
    "fs12_salarios_sem_acento": "Folha de salarios (FS12) 7.000,00",
    "das_variantes": "Total do DAS 1.000,00\nValor do DAS 2.000,00",
    "das_debito_maiusculo": "VALOR TOTAL DO DÉBITO DECLARADO 3.210,99",
    "numero_ponto_decimal": "RBT12: 1234.56",
    "numero_ponto_seguido_de_digito": "RBT12: 1234.567",
    "numero_15_digitos": "RBT12: 123456789012345,67",
    "numero_16_digitos": "RBT12: 1234567890123456,78",
    "numero_milhares_grandes": "RBT12: 1.234.567.890.123.456.789,01",
    "numero_milhares_mal_formados": "RBT12: 1.23.456,00",
    "numero_digitos_arabes": "RBT12: ١٢٣,٤٥",
    "fator_percentual": "Fator r: 28,5%",
    "fator_maior_que_1": "Fator r = 30",
    "fator_fracao": "Fator r 0,2853",
    "fator_ponto": "FATOR R: 0.3",
    "fator_zero": "Fator r: 0",
    "fator_seis_casas": "Fator r: 0,123456",
    "fator_muitas_casas": "Fator r: 0,1234567",
    "fator_espaco_porcento": "Fator r: 29 %",
    "consistente_bonus": "CNPJ: 11.222.333/0001-81\nPA: 09/2026\nRBT12: 600.000,00\nFS12: 180.000,00\nFator r: 0,30",
    "inconsistente_penalidade": "PA: 09/2026\nCNPJ: 11222333000181\nRBT12: 600.000,00\nFS12: 180.000,00\nFator r: 0,10",
    "zona_intermediaria": "PA: 09/2026\nRBT12: 600.000,00\nFS12: 180.000,00\nFator r: 0,31",
    "rbt12_zero_sem_consistencia": "RBT12: 0,00\nFS12: 10,00\nFator r: 0,30",
    "fs12_zero": "RBT12: 100,00\nFS12: 0,00\nFator r: 0",
    # --- anexo ---
    "anexo_vi_nao_vira_v": "Atividade do Anexo VI",
    "anexo_linha_estruturada_manda": "Fator r = Nao se aplica - Anexo I\nCriterio: atividades sujeitas ao fator r vao para o Anexo III.",
    "anexo_estruturado_iv": "Enquadramento: Anexo IV\nTexto livre citando o Anexo V",
    "anexo_estruturado_ii": "Enquadramento: Anexo II",
    "anexo_estruturado_iii": "Atividade: Prestação de serviços - Anexo III",
    "anexo_estruturado_iiii": "Enquadramento: Anexo IIII\nAnexo V",
    "anexo_estruturado_iiv": "Enquadramento: Anexo IIV\nAnexo III",
    "anexo_estruturado_minusculo": "enquadramento anexo iii",
    "anexo_estruturado_i_sem_ponto": "Enquadramento Anexo ııı",
    "anexo_estruturado_i_com_ponto": "Enquadramento Anexo İİİ",
    "anexo_livre_i_com_ponto": "Texto Anexo İİİ aqui",
    "anexo_estruturado_sem_borda": "Atividades: Anexo III",
    "anexo_livre_seguido_de_x": "Anexo VX\nAnexo III",
    "anexo_dois_na_linha_estruturada": "Fator r: Anexo VI e Anexo III",
    "anexo_livre_colado": "xAnexo III\nAnexo V",
    # --- identificação ---
    "nome_com_dois_pontos": "Nome empresarial: CLINICA EXEMPLO LTDA",
    "nome_sem_dois_pontos": "NOME EMPRESARIAL CLINICA EXEMPLO LTDA",
    "nome_vazio_depois_preenchido": "Nome empresarial:\nNome empresarial: SEGUNDA LTDA",
    "nome_razao_social": "Razão social: CLINICA EXEMPLO LTDA",
    "nome_colado": "Nome empresarialX: CLINICA",
    "nome_longo": "Nome empresarial: " + "A" * 300,
    "nome_longo_com_espacos_no_corte": "Nome empresarial: " + "A" * 199 + "   B",
    "nome_espaco_unicode_bordas": "Nome empresarial:　NOME ",
    "sujeita_numero": "Fator r = 0,20 - Anexo V",
    "sujeita_nao_se_aplica": "Fator r = Não se aplica - Anexo III",
    "sujeita_nao_se_aplica_texto": "Folha para fator r Nao se aplica a esta atividade",
    "sujeita_sem_indicio": "Sem nenhuma menção",
    "abertura_valida": "Data de abertura no CNPJ: 12/03/2019",
    "abertura_mes_invalido_depois_valido": "Data de abertura no CNPJ: 01/13/2020\nData de abertura: 01/12/2020",
    "abertura_sem_data": "Data de abertura no CNPJ: XX/XX/XXXX",
    # --- séries anteriores ---
    "series_conferem": declaratorio(["10.000,00"] * 12, ["2.000,00"] * 12, "120.000,00", "24.000,00"),
    "series_receita_nao_soma": declaratorio(["10.000,00"] * 11 + ["11.000,00"], ["2.000,00"] * 12, "120.000,00", "24.000,00"),
    "series_tolerancia_centavo": declaratorio(["10.000,00"] * 11 + ["10.000,01"], ["2.000,00"] * 12, "120.000,00", "24.000,00"),
    "series_fora_da_janela": declaratorio(["10.000,00"] * 12, ["2.000,00"] * 12, "120.000,00", "24.000,00", pa="01/09/2026 a 30/09/2026"),
    "series_sem_folha": declaratorio(["10.000,00"] * 12, None, "120.000,00", "0,00"),
    "series_mes_invalido": declaratorio(
        ["10.000,00"] * 12, ["2.000,00"] * 12, "120.000,00", "24.000,00",
        meses=["08/2025", "09/2025", "10/2025", "11/2025", "13/2025", "01/2026",
               "02/2026", "03/2026", "04/2026", "05/2026", "06/2026", "07/2026"],
    ),
    "series_interno_e_externo_somam": declaratorio(
        ["5.000,00"] * 12, ["2.000,00"] * 12, "120.000,00", "24.000,00",
        extra=[
            "2.2 Receitas Brutas Anteriores (R$) - Mercado Externo",
            "08/2025 5.000,00 09/2025 5.000,00 10/2025 5.000,00",
            "11/2025 5.000,00 12/2025 5.000,00 01/2026 5.000,00",
            "02/2026 5.000,00 03/2026 5.000,00 04/2026 5.000,00",
            "05/2026 5.000,00 06/2026 5.000,00 07/2026 5.000,00",
        ],
    ),
    "series_ano_antigo_levanta": declaratorio(
        ["10.000,00"] * 12, ["2.000,00"] * 12, "120.000,00", "24.000,00", pa="01/08/1899 a 31/08/1899"
    ),
    "series_valores_colados": "PA: 09/2026\n2.2 Receitas Brutas Anteriores\nx08/2025 1,00 09/2025 2,00 1009/2025 3,00",
    "series_secao_numerada_encerra": "2.2 Receitas Brutas Anteriores\n08/2025 1,00\n3 Outra seção\n09/2025 2,00",
    "series_digitos_arabes": "PA: 09/2026\n2.2 Receitas Brutas Anteriores\n٠٨/٢٠٢٥ 1,00",
    "series_valor_ponto_seguido_digito": "2.2 Receitas Brutas Anteriores\n08/2025 1234.567 09/2025 10.50",
    "series_soma_gigante": "PA: 09/2026\nRBT12: 1,00\n2.2 Receitas Brutas Anteriores\n"
    + "\n".join(f"08/2025 {'9' * 15},99" for _ in range(3)),
    # --- normalização ---
    "crlf_e_cr": "PA: 09/2026\r\nCNPJ: 11.222.333/0001-81\rRBT12: 1,00",
    "nbsp_e_tabs": "CNPJ: \t11.222.333/0001-81\nRBT12:\t\t1.000,00",
    "linha_longa_cortada": "RBT12: " + "x" * 1990 + " 12,34 56,78",
    "linha_longa_com_numero_dentro": "RBT12: " + "x" * 1980 + " 12,34 56,78",
    "separadores_unicode": "CNPJ:\u000b11.222.333/0001-81 x\nPA:\u001c09/2026",
    "espaco_de_linha_unicode": "PA: 09/2026 RBT12: 1,00",
}


def resultado(r):
    return {
        "campos": _campos_json(r),
        "confianca": str(r.confianca),
        "parser_version": r.parser_version,
    }


def caso(nome, texto, origem):
    try:
        return {"nome": nome, "origem": origem, "texto": texto, "resultado": resultado(parse(texto)), "excecao": None}
    except Exception as exc:  # o Go devolve ErrParse onde a origem levanta
        return {"nome": nome, "origem": origem, "texto": texto, "resultado": None, "excecao": type(exc).__name__}


casos = []
for pasta in sorted(p for p in FIXTURES.iterdir() if p.is_dir()):
    documento = next(pasta.glob("documento.*"))
    conteudo = documento.read_bytes()
    texto = extrair_texto(conteudo, detectar_mime(conteudo))
    if texto.motivo:
        continue  # sem texto: resultado_vazio, coberto pelo teste das fixtures
    casos.append(caso(pasta.name, texto.texto, "fixture"))
for nome, texto in VARIACOES.items():
    casos.append(caso(nome, texto, "variacao"))

json.dump(casos, sys.stdout, ensure_ascii=False, indent=1, sort_keys=True)
sys.stdout.write("\n")
