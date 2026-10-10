# Tool Contract — Agentes

## 1. Regras gerais

Ferramentas são funções tipadas expostas aos agentes. O agente não recebe acesso arbitrário a SQL, filesystem ou shell.

## 2. Conciliação

### `extrair_texto_pdf`
Entrada: referência do documento/páginas.  
Saída: texto por página + metadados.  
Sem escrita de domínio.

### `renderizar_pagina`
Entrada: documento, página, DPI permitido.  
Saída: referência segura da imagem/render.  
Sem escrita de domínio.

### `validar_saldo`
Entrada: schema extraído.  
Saída: exatamente o mesmo `ResultadoValidacao` usado pelo portão de persistência.  
Regra: não duplicar implementação de cálculo.

> **Como foi implementado (TASK-016; D-027).** Como na origem, o sistema chama as ferramentas e envia o resultado ao agente; o modelo não faz chamada de ferramenta.
> - `extrair_texto_pdf` e `renderizar_pagina`: `pdftext.Extractor.Pages` e `Render`, só leitura.
> - `validar_saldo`: a mesma `statement.Validate` do portão.

### `buscar_candidatos`
Entrada: company/conta/período/valor/data/sentido conforme contrato.  
Saída: candidatos somente leitura.

> **Na TASK-018 (D-029):** o domínio está portado com golden da origem (`matching.Candidates`: mesmo valor e sentido em D±3). A ferramenta do resolvedor chega com a TASK-019.

### `historico_classificacao`
Entrada: company + contraparte/descritivo.  
Saída: histórico somente leitura.

### `consultar_cnpj`
Entrada: identificação parcial permitida.  
Saída: dados de apoio.  
Para demo/local, se integração externa não existir no projeto original, usar fonte/fixture prevista; não inventar chamada externa nova sem decisão.

> **Na TASK-017 (D-028):** não implementada; a origem nunca a teve. A forma (consulta só local ou fonte externa) aguarda a decisão do usuário (P-007). O extrator não busca CNPJ de memória: `contraparte_doc` só quando visível por inteiro.

## 3. Fator R

### `obter_factor_r`
Entrada: company + PA.  
Saída: resultado do motor determinístico.

### `simular_correcao`
Entrada: company + PA + parâmetros.  
Saída: resultado determinístico persistível.

### `obter_carteira`
Entrada: PA/filtros.  
Saída: carteira calculada.

### `obter_empresa_factor_r`
Entrada: company.  
Saída: settings/série/resumo necessários ao agente, sem secrets.

## 4. Escritas

Por padrão, agentes não recebem tools que alteram dados finais. Se um fluxo exigir gravação de proposta/trace, a escrita é executada pelo serviço orquestrador após validar schema e regras.

## 5. Erros

Tools retornam erros estruturados e sanitizados. O agente não deve receber stack trace com secret/DSN.
