"""Texto de referência do PyMuPDF (page.get_text()) para o porte das ferramentas do Conciliador.

A origem (conciliador-contabil, commit 2bc6e26, app/agentes/ferramentas.py) lê o PDF com o
PyMuPDF: extrair_texto_pdf devolve [page.get_text() for page in doc]. O teste Go
(pages_test.go) exige o mesmo texto, página a página, nos PDFs abaixo:

  - os desta pasta (PGDAS-D do Fator R);
  - os do cenário da Conciliação (../../../reconciliation/testdata/conciliacao);
  - PDFs sintéticos gerados aqui (mupdf/), que medem as regras de linha e espaço do MuPDF:
    intervalo na linha de base, deslocamento vertical, troca de tamanho, volta para trás,
    rotação, texto fora da página, Tc/Tw/Tz/TJ e tamanho na matriz (Tf 1).

Uso, com PyMuPDF 1.25.4 (MuPDF 1.25.5; a origem pede pymupdf>=1.24):

    python mupdf_gen.py
"""

import json
from pathlib import Path

import pymupdf

pasta = Path(__file__).resolve().parent
sinteticos = pasta / "mupdf"
sinteticos.mkdir(exist_ok=True)
conciliacao = pasta.parent.parent.parent / "reconciliation" / "testdata" / "conciliacao"

FONTE = "helv"


def largura(texto, tamanho):
    return pymupdf.get_text_length(texto, fontname=FONTE, fontsize=tamanho)


def com_insert_text(nome, desenhos, rotacao_pagina=0):
    doc = pymupdf.open()
    page = doc.new_page(width=595, height=842)
    for d in desenhos:
        x, y, texto, tamanho = d[:4]
        extra = {"rotate": d[4]} if len(d) > 4 else {}
        page.insert_text((x, y), texto, fontsize=tamanho, fontname=FONTE, **extra)
    if rotacao_pagina:
        page.set_rotation(rotacao_pagina)
    doc.save(sinteticos / f"{nome}.pdf", garbage=0, deflate=False)
    doc.close()


def pdf_cru(nome, conteudo):
    """PDF mínimo com Helvetica (fonte padrão) e o fluxo de conteúdo dado."""
    stream = conteudo.encode("latin-1")
    objs = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
        b"<< /Length %d >>\nstream\n" % len(stream) + stream + b"\nendstream",
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
    ]
    out = bytearray(b"%PDF-1.4\n")
    offsets = []
    for i, o in enumerate(objs, start=1):
        offsets.append(len(out))
        out += b"%d 0 obj\n" % i + o + b"\nendobj\n"
    xref = len(out)
    out += b"xref\n0 %d\n0000000000 65535 f \n" % (len(objs) + 1)
    for off in offsets:
        out += b"%010d 00000 n \n" % off
    out += b"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n" % (len(objs) + 1, xref)
    (sinteticos / f"{nome}.pdf").write_bytes(bytes(out))


def gerar_sinteticos():
    t = 10.0
    w = largura("AB", t)
    for k in [0.0, 0.1, 0.14, 0.16, 0.5, 0.79, 0.81, 1.0, -0.1, -0.5, -0.7, -0.81, -2.0]:
        com_insert_text(f"intervalo_{k}", [(100, 100, "AB", t), (100 + w + k * t, 100, "CD", t)])
    for k in [0.3, 0.79, 0.81, -0.3, -0.81, 1.5]:
        com_insert_text(f"base_{k}", [(100, 100, "AB", t), (100 + w, 100 + k * t, "CD", t)])
    for gap in [2.9, 3.1, 15.9, 16.1]:
        com_insert_text(f"maior_{gap}", [(100, 100, "AB", 10), (100 + w + gap, 100, "CD", 20)])
    for gap in [1.4, 1.6, 7.9, 8.1]:
        com_insert_text(f"menor_{gap}", [(100, 100, "AB", 20), (100 + largura("AB", 20) + gap, 100, "CD", 10)])
    com_insert_text("espaco_explicito", [(100, 100, "AB ", t), (100 + largura("AB ", t) + 3, 100, "CD", t)])
    com_insert_text("rotacao_mista", [(100, 100, "AB", t), (100 + w, 100, "CD", t, 90)])
    com_insert_text("rotacao_90", [(100, 100, "AB", t, 90), (100, 100 - w, "CD", t, 90)])
    com_insert_text("pagina_girada", [(100, 100, "AB", t), (100 + w + 5, 100, "CD", t)], rotacao_pagina=90)
    com_insert_text("colunas", [(100, 100, "A1", t), (100 + largura("A1", t) + 3, 100, "B1", t), (300, 100, "C1", t)])
    com_insert_text("volta_linha", [(100, 100, "L1", t), (100, 120, "L2", t), (130, 100, "L1b", t)])
    com_insert_text("fora_da_pagina", [(100, 100, "DENTRO", t), (700, 100, "FORA", t), (100, -50, "ACIMA", t)])
    com_insert_text("acentos", [(100, 100, "Agência Histórico — R$ 1.234,56 ç ã õ", t)])
    # Fluxos crus: tamanho na matriz, espaçamentos e ajustes do TJ.
    pdf_cru("tf1_matriz", "BT /F1 1 Tf 10 0 0 10 100 700 Tm (AB) Tj ET BT /F1 1 Tf 10 0 0 10 200 700 Tm (CD) Tj ET")
    pdf_cru("tf1_matriz_perto", "BT /F1 1 Tf 10 0 0 10 100 700 Tm (AB) Tj 0.3 0 Td (CD) Tj ET")
    pdf_cru("tc_espacamento", "BT /F1 10 Tf 3 Tc 100 700 Td (ABCD) Tj ET")
    pdf_cru("tc_grande", "BT /F1 10 Tf 9 Tc 100 700 Td (ABCD) Tj ET")
    pdf_cru("tw_palavras", "BT /F1 10 Tf 20 Tw 100 700 Td (AB CD) Tj ET")
    pdf_cru("tz_escala", "BT /F1 10 Tf 50 Tz 100 700 Td (ABCD) Tj ( EF) Tj ET")
    pdf_cru("tj_ajustes", "BT /F1 10 Tf 100 700 Td [(AB) -200 (CD) -900 (EF) 300 (GH)] TJ ET")
    pdf_cru("duas_linhas_td", "BT /F1 10 Tf 100 700 Td (Linha um) Tj 0 -12 Td (Linha dois) Tj ET")
    pdf_cru("negrito_falso", "BT /F1 10 Tf 100 700 Td (Negrito) Tj ET BT /F1 10 Tf 100.4 700 Td (Negrito) Tj ET")
    pdf_cru("render_invisivel", "BT /F1 10 Tf 3 Tr 100 700 Td (Invisivel) Tj ET BT /F1 10 Tf 0 Tr 100 680 Td (Visivel) Tj ET")
    pdf_cru("cm_escala", "q 2 0 0 2 0 0 cm BT /F1 5 Tf 50 350 Td (AB) Tj ( CD) Tj ET Q")


def paginas(caminho):
    doc = pymupdf.open(caminho)
    try:
        return [p.get_text() for p in doc], [[round(p.rect.width, 3), round(p.rect.height, 3)] for p in doc]
    finally:
        doc.close()


def main():
    gerar_sinteticos()
    alvos = sorted(pasta.glob("*.pdf")) + sorted(sinteticos.glob("*.pdf")) + sorted(conciliacao.rglob("*.pdf"))
    casos = []
    for caminho in alvos:
        textos, tamanhos = paginas(caminho)
        casos.append({"pdf": caminho.relative_to(pasta).as_posix() if caminho.is_relative_to(pasta) else "../../../reconciliation/testdata/conciliacao/" + caminho.relative_to(conciliacao).as_posix(),
                      "paginas": textos, "tamanhos": tamanhos})
    (pasta / "mupdf_golden.json").write_text(json.dumps(casos, ensure_ascii=False, indent=1) + "\n", encoding="utf-8", newline="\n")
    print(len(casos), "PDFs; PyMuPDF", pymupdf.VersionBind, "MuPDF", pymupdf.VersionFitz)


main()
