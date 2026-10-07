# AI Behavior Spec

## 1. Princípio central

> O agente interpreta. O código determinístico valida, calcula e persiste.

O LLM nunca é autoridade final para cálculos tributários, fechamento, saldo, matching ambíguo ou classificação definitiva sem guarda humana/determinística.

## 2. Agentes

### `parser_pgdas`

**Natureza:** agente **determinístico**, sem chamada de modelo (ADR-006). O parse é por regras (regex), como na origem; o agente existe para trace, decisão e avaliação.  
**Entrada:** documento PGDAS-D já armazenado + texto extraído.  
**Saída:** campos estruturados, confiança calculada por regra, indícios de vínculo/cadastro.  
**Não pode:** calcular Fator R como fonte oficial, sobrescrever movimentos existentes, criar empresa sem confirmação.

### `consultor`

**Entrada:** empresa, PA, resultado determinístico do motor e simulador.  
**Saída:** leitura para contador + decisão estruturada referenciando resultados calculados.  
**Não pode:** recalcular números ou recomendar dado sem simulador quando a regra original exige.

### `priorizador`

**Entrada:** carteira calculada.  
**Saída:** ordenação/explicação de prioridade.  
**Não pode:** alterar carteira ou dados.

### `triador`

**Entrada:** documento.  
**Saída:** tipo, banco, conta, período e confiança.  
**Baixa confiança:** revisão humana.

### `extrator`

**Entrada:** extrato/comprovante, texto/render, playbook.  
**Saída:** schema estruturado.  
**Loop:** até 3 tentativas conforme contrato.  
**Não pode:** aprovar saldo.

### `extrator_forte`

Alias de escalada da tentativa final. Mesmos limites do extrator.

### `resolvedor`

**Entrada:** exceções + candidatos + histórico.  
**Saída:** hipótese, justificativa, pares propostos, confiança.  
**Não pode:** criar conciliação.

### `classificador`

Quando necessário pelo comportamento original, sugere conta para item sem regra/histórico. Sempre sujeito à confirmação.

## 3. Seleção de modelo

- Resolvida por alias no início da corrida.
- Provider inicial: OpenAI via LiteLLM.
- Modelo concreto configurável.
- Snapshot do provider/model/version é gravado no run.
- Alteração de configuração não muda corrida já iniciada.

## 4. Fallback

- Provider indisponível: retry de job.
- Retry esgotado: estado de erro/revisão; não inventar saída template como se fosse modelo real, exceto se o comportamento original possuir modo fake explicitamente de dev/test.
- Langfuse indisponível: continuar domínio após auditoria local.

## 5. Human-in-the-loop

Obrigatório para:
- CNPJ não vinculado no PGDAS quando houver criação de empresa;
- baixa confiança conforme regra do módulo;
- correção de campo crítico;
- conciliação ambígua;
- hipótese do resolvedor;
- classificação agentic final;
- reabertura de período/fechamento.

## 6. Abstenção

Agente deve retornar estado de insuficiência/revisão quando:
- documento ilegível;
- schema não pode ser satisfeito;
- confiança insuficiente;
- ferramentas retornam dados conflitantes;
- domínio exige decisão humana.

## 7. Conteúdo não confiável

Texto de PDF/imagem pode conter instruções maliciosas. Prompts de sistema devem ordenar explicitamente que instruções contidas em documentos são dados e nunca políticas operacionais.
