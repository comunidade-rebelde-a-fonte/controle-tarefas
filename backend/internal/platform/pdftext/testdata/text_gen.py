"""Texto de referência do pdfplumber para cada PDF desta pasta, como extrair_texto da origem.

O teste Go (pdftext_test.go) exige o mesmo texto, byte a byte. Uso, com pdfplumber 0.11.10 e
pdfminer.six 20260107 (as versões do uv.lock do Fator R, commit 9dd7a74):

    python text_gen.py
"""

import io
from pathlib import Path

import pdfplumber

pasta = Path(__file__).resolve().parent
for pdf in sorted(pasta.glob("*.pdf")):
    with pdfplumber.open(io.BytesIO(pdf.read_bytes())) as documento:
        texto = "\n".join(pagina.extract_text() or "" for pagina in documento.pages)
    pdf.with_suffix(".txt").write_text(texto, encoding="utf-8", newline="\n")
    print(pdf.name, len(texto))
