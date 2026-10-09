# 09 — Segurança e Privacidade

## 1. Contexto

O sistema é local, usado por um único operador, mas trata dados fiscais, contábeis, financeiros e documentos. Ausência de login da aplicação principal não elimina requisitos de proteção de arquivos, secrets, logs e operações destrutivas.

## 2. Classificação de dados

| Classe | Exemplos | Tratamento |
|---|---|---|
| Cadastral | CNPJ, razão social, contatos | Banco local; evitar exposição desnecessária em logs externos |
| Fiscal | PGDAS-D, RBT12, FS12, Fator R | Retenção/auditoria; não enviar conteúdo bruto ao Langfuse quando contrato original o proíbe |
| Financeiro | extratos, comprovantes, transações, razão | Originais imutáveis; acesso local; logs sanitizados |
| Segredo | API keys, senhas MinIO/LiteLLM/Langfuse | Somente env/secret local; nunca DB/UI/log |
| Auditoria | traces, hashes, decisões, histórico | Retenção suficiente para reconstruir decisões |

## 3. Autenticação e autorização

### Aplicação principal

- [CONFIRMED] sem login e sem múltiplos usuários.
- Ator auditável implícito, por exemplo `local-user`/usuário seed fixo compatível com o modelo atual.
- Não implementar sistema de permissões como parte desta consolidação.

### Serviços auxiliares

- Langfuse mantém autenticação administrativa própria.
- MinIO mantém credenciais próprias.
- LiteLLM mantém master key/chave de acesso entre aplicação e proxy.
- n8n permanece local; não expor externamente por esta especificação.

## 4. Secrets

Nunca persistir valores reais em:
- Git;
- Markdown;
- banco de settings de IA;
- OpenAPI examples;
- logs;
- traces;
- respostas HTTP.

A UI de modelos armazena apenas provider/model/params não secretos.

## 5. Uploads

- Validar tipo/tamanho conforme cada módulo.
- Arquivo não deve ser servido diretamente por diretório público.
- Hash antes/processo de dedupe conforme domínio.
- Nome original é metadata; path/key interno não deve confiar em nome fornecido pelo usuário.
- Originais do Conciliador permanecem imutáveis/versionados no MinIO.
- Originais de outros módulos respeitam storage atual.

> **Como foi implementado na Conciliação (TASK-015; D-026).**
> - O tipo vem do conteúdo (PDF, PNG, JPEG ou texto) e o limite é de 20 MB.
> - O original vai para o MinIO com a chave `originais/<sha256>`, gravado pelo usuário da aplicação, que não pode apagar objeto nem versão (provado contra o MinIO real).
> - A leitura sai pela API, na versão gravada, com o tipo detectado, `nosniff` e sem cache. O nome enviado só aparece no Content-Disposition.

> **Como foi implementado no Inbox PGDAS-D (TASK-012; D-023).**
> - O tipo vem do conteúdo (assinatura do PDF; texto sem byte nulo, UTF-8 ou latin-1 imprimível) e o limite é de 10 MB.
> - O original fica em `FILE_STORAGE_DIR/pgdas/<sha256>` com permissão 0600 e é lido por `os.Root`. O nome enviado é só metadado.
> - O download é autenticado como as demais rotas, sai como anexo, com `nosniff` e `no-store`.
> - O PDF é lido pelo PDFium em WebAssembly, sem acesso ao disco, com 1 GiB de memória, 30 s por extração e duas extrações ao mesmo tempo.
> - O texto extraído fica no banco local. O Langfuse recebe só campos, decisão e texto do agente, mascarados (D-022).

## 6. IA

### Regras

- LLM não recebe autoridade para gravar decisão financeira/tributária final fora dos contratos definidos.
- Saída estruturada sempre validada antes de uso.
- Prompt injection em documentos não altera instruções do sistema/agente.
- Conteúdo de documento é dado não confiável.
- Ferramentas expostas ao agente são allowlisted e tipadas.
- Ferramentas de escrita devem ser evitadas; agentes retornam propostas, e serviços determinísticos decidem persistência.

### PII/financeiro no Langfuse

Preservar política original de não enviar texto integral de PDF, CNPJ/CPF em claro ou listas desnecessárias quando não forem necessários ao trace. Usar IDs internos e valores/métricas mínimos.

## 7. Operações destrutivas

Exigem aprovação humana explícita:

- [SECURITY][BLOCKER] `make db-reset` ou equivalente.
- [SECURITY][BLOCKER] `DROP`, truncate ou migration irreversível.
- [SECURITY][BLOCKER] exclusão de originais/documentos.
- [SECURITY][BLOCKER] apagar volumes Docker.
- [SECURITY][BLOCKER] operações Git remotas: push, force push, merge remoto, criação de PR/release/tag remoto.
- [SECURITY][BLOCKER] rotação/invalidação de credenciais.

## 8. Ameaças e controles

| Ameaça | Controle |
|---|---|
| LLM inventa valor | cálculo/validação determinísticos |
| Linha de extrato perdida | portão saldo R$ 0,00 |
| Duplicata de arquivo | SHA-256/dedupe |
| Conciliação ambígua | não conciliar automaticamente |
| Prompt injection no PDF | documento tratado como conteúdo; ferramentas allowlisted |
| Secret na UI | API não aceita/devolve API keys |
| Log vaza documento | sanitização + referências/hashes |
| Alteração após fechamento | `company_module_period` Contábil bloqueia mutações |
| Serviço local exposto | preferir bind loopback para consoles/infra, sem quebrar acesso local |

## 9. Retenção e exclusão

- Preservar políticas de origem dos módulos.
- Não introduzir delete automático nesta consolidação.
- Qualquer lifecycle futuro exige decisão separada.

## 10. Checklist de segurança

- [ ] Nenhum secret no diff.
- [ ] Upload com validação.
- [ ] Path traversal testado.
- [ ] API key ausente do frontend.
- [ ] Traces sanitizados.
- [ ] Portão de saldo coberto por teste negativo.
- [ ] Período fechado impede mutação.
- [ ] Jobs não serializam blobs/documentos sensíveis no payload.
