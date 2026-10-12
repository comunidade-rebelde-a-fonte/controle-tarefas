# Registro de decisões da consolidação

Decisões tomadas durante a execução da spec (`docs/ai-spec/`), conflitos encontrados e pendências. As entradas mais recentes ficam no topo de cada seção. Decisão que muda contrato também atualiza a ADR correspondente.

## 1. Decisões

### D-038 — Dashboard consolidado: seções Fator R e Conciliação (TASK-024; 12/10/2026; quatro decisões do usuário, o resto dentro da autonomia da spec; validada pelo usuário com a TASK)

- **O que a spec pede:** o Dashboard mantém os blocos do Controle e ganha as seções Fator R e Conciliação, todo número com drill-down (PRD §8, 08 §3). O `GET /dashboard` ganha `factor_r` e `reconciliation` sem quebrar os campos (06 §6).
- **Decisões do usuário (as quatro recomendadas):**
  - **filtro de responsável:** no Fator R, vale o responsável do Fiscal, porque o Fator R decide o anexo da apuração do Simples, item da grade Fiscal. Na Conciliação, vale o do Contábil, porque a obrigação Conciliação é do Contábil. Agrupamento e regime valem nas duas;
  - **documentos:** processando, em revisão e parados por período fechado contam todas as competências, com o rótulo "todas as competências". A fila e as empresas são as da competência;
  - **prontas e bloqueadas:**
    - "prontas para fechamento" são as conciliações prontas para concluir: o extrato todo resolvido, ainda não concluídas;
    - "Contábil bloqueado" são as empresas com o Contábil aberto que usam a Conciliação e têm a obrigação Conciliação aplicável não final (a guarda do fechamento, D-037);
    - o drill-down das duas é a lista nova "Empresas" da Conciliação;
  - **alertas do Fator R (REQ-017):** ficam na seção Fator R, com os números por semáforo e as cinco primeiras empresas que exigem ação, na ordem da carteira (vermelho, amarelo e dados insuficientes). Cada uma traz a ação sugerida e o link para a aba Fator R. O "Exige atenção" segue só com obrigações e credenciais.
- **Dentro da autonomia:**
  - **Fator R:**
    - o PA é a competência do Dashboard, e os números saem da carteira (`factorr.PortfolioWith`, o motor da tela);
    - "No Anexo V" conta o vermelho, o mesmo número do KPI "no V", e "Exigem ação" são as empresas com ação diferente de manter;
    - sem tabela do Simples vigente no PA, a seção mostra o motivo e o resto do Dashboard segue;
  - **Conciliação:** processando (pendente e triado), em revisão (triagem e extração), parados por período fechado, a fila da competência (o total, as divergências, os extratos sem comprovante e os comprovantes sem extrato), as prontas para concluir e o Contábil bloqueado;
  - **números e listas batem:** cada número sai da mesma função da lista do drill-down, e o link leva os filtros. Para isso:
    - a carteira ganhou o filtro de semáforo (`semaforo`, separado por vírgula) e os filtros de empresa;
    - a fila ganhou os filtros de empresa, que exigem a competência;
    - os documentos aceitam mais de um status, e os grupos "Processando" e "Em revisão" viraram botões. Os filtros de empresa valem pelo snapshot de `filter_competence`;
    - a lista nova `GET /reconciliation/companies` virou a tela "Empresas" no submenu da Conciliação, com uma consulta só para a competência inteira;
  - **filtros de empresa:** `company.Filter`, pelo snapshot da competência, como os números das obrigações. Os documentos, de todas as competências, usam o snapshot da competência do Dashboard. As telas mostram os filtros que vieram do Dashboard e um botão para limpá-los;
  - **contagem por status dos documentos:** passou a respeitar a empresa escolhida e os filtros do Dashboard; antes, contava todos os documentos;
  - a aba Conciliação da ficha abre na competência da URL (o link da lista "Empresas").
- **AC-033 (navegação):** o menu tem 13 das 14 áreas do REQ-050. "Documentos" chega com a TASK-026, e a ordem e o agrupamento finais do menu ficam para a TASK-029 (Design System). O E2E abre cada área do menu a partir do Dashboard.
- **TEST-024:**
  - nos testes de integração (`internal/dashboard/sections_test.go`), a seção Fator R bate com a carteira e a Conciliação, com as listas do drill-down e com SQL direto, sem e com os filtros (o responsável de cada módulo, o agrupamento). O bloqueio da lista bate com a guarda do fechamento do Contábil;
  - no E2E, com um responsável próprio, os números são exatos e cada clique leva à lista com o mesmo total; as empresas do Fator R do teste são inativadas no fim, para a carteira da demonstração não mudar.

### D-037 — O fechamento do Contábil é a trava da Conciliação (TASK-023; 11/10/2026; três decisões do usuário, o resto dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Conflito com a F-009, decidido pelo usuário.** A F-009 manda recusar o fechamento do Contábil com obrigação aplicável não final; o Controle fecha sem conferir obrigação nenhuma. Decisão: **só a Conciliação, e só se usada.**
  - Fechar o Contábil exige a obrigação Conciliação (`ACCOUNTING_RECONCILIATION`) em status final, ou não aplicável, apenas quando a empresa usa a Conciliação na competência: extrato importado que a cobre ou documento da Conciliação dela (a competência do envio ou o período lido).
  - Sem Conciliação, o Contábil fecha como sempre. As outras obrigações não são conferidas.
  - A recusa é 422 `RECONCILIATION_NOT_FINAL`, com a obrigação e o status em `details`. A F-009 manda listar as pendências; como só a Conciliação é conferida, a mensagem cita só ela.
- **Onde:** a guarda fica no fechamento do Controle (`period.Service.Close`, `requireReconciliationFinal`) e vale para todo caminho (grade do Contábil, aba da ficha e API).
  - Na transação do fechamento, pega a trava exclusiva da Conciliação da empresa (a mesma da conclusão, D-036). Assim, o fechamento espera a conclusão ou a reabertura em andamento, e as gravações da Conciliação (a trava compartilhada do `RequireOpen`) esperam o fechamento.
- **A trava (AC-025)** continua em `reconciliation.RequireOpen` (TASK-007 e TASK-022). Com o Contábil fechado, toda mutação da Conciliação na competência responde 423 `PERIOD_CLOSED`, e a leitura segue.
  - O TEST-023 confere a matriz: incluir lançamento, corrigir campo do comprovante, rodada, classificação (rodar e corrigir a conta), conciliar à mão, sem comprovante, desfazer, pedir hipótese ao resolvedor e sugestão de conta, e concluir.
  - O documento que chega vira `pendencia_periodo_fechado`, sem job.
- **Fechamentos independentes (AC-026):** fechar o Contábil não fecha os outros módulos da empresa, e o Fiscal segue editável.
- **Reabertura:** pelo endpoint do Controle, sempre com motivo (sem motivo, 422 `REASON_REQUIRED`). O evento `REOPENED` guarda o motivo e, reaberto, as mutações voltam.
- **Documentos tardios (decisão do usuário: "Seguem pendentes"):** continuam em `pendencia_periodo_fechado` depois da reabertura, como na origem, até alguém reprocessar. É o botão "Reprocessar" do documento (`POST /reconciliation/documents/{id}/retry`, que já existia e confere a trava): uma decisão explícita do responsável. Na pergunta, o botão foi chamado de "Tentar de novo".
- **Tela (decisão do usuário: "Mostrar e agir"):** o cartão "Fechamento do Contábil" na aba Conciliação da ficha.
  - Mostra o estado, quem fechou e quando, e o histórico (aberto, fechado e reaberto, com o motivo).
  - Fecha e reabre com os mesmos diálogos e endpoints da grade do Contábil.
  - Usando a Conciliação, avisa que o Contábil só fecha com a obrigação Conciliação concluída.
- **API (06 §5):** `GET /companies/{id}/reconciliation/closure?competence=`, derivado do período do Contábil (`company_module_periods`), sem segundo fechamento: o período (nulo se não foi gerado), os eventos e `requires_reconciliation`. Os eventos `AccountingPeriodClosed` e `AccountingPeriodReopened` da spec são os eventos do período no Controle (`company_module_period_events`).
- **Correção:** os comentários do código e da OpenAPI da TASK-022 citavam a D-035, que ficou com o hot reload; passaram a citar a D-036.
- **E2E (TEST-023):** na competência corrente do E2E, a única em que a API gera a obrigação.
  - Fecha pela ficha; o lançamento é recusado, a rodada responde 423 e o documento tardio vira pendência.
  - Reabre com motivo (o histórico mostra o motivo); o lançamento é aceito, e o documento segue pendente, com "Reprocessar".
  - Com o documento, a empresa passou a usar a Conciliação: o fechamento é recusado até a obrigação Conciliação ir para OK. Com o axe.

### D-036 — Conclusão da Conciliação e a obrigação Conciliação do Contábil (TASK-022; 11/10/2026; quatro regras de negócio decididas pelo usuário, o resto dentro da autonomia da spec; validada pelo usuário com a TASK)

- **O que existia:** a origem não tinha conclusão. O "fechamento" (`/fechamento`) só travava a competência e reabria com justificativa, sem critério. Aqui o fechamento é o do Contábil (ADR-003, TASK-023), e a TASK-022 acrescenta a conclusão da Conciliação, a máquina de estados da F-008 (em andamento → pronta → concluída → reaberta com motivo).
  - Domínio em `backend/internal/reconciliation/completion`; a mudança na obrigação em `backend/internal/obligation` (`ApplyIntegrationStatus`); migration `0030_reconciliation_completion.sql`.
- **Decisões do usuário (as quatro recomendadas):**
  - **pronta:** o extrato todo resolvido. Ao menos um extrato importado cobre a competência, toda linha está conciliada (ou confirmada sem comprovante) e classificada, e nenhum documento nem comprovante parcial está em revisão. Comprovante sem extrato só avisa (pode ser de outra conta ou de outro mês);
  - **conclusão:** o contador confirma ("Concluir a conciliação", na aba da ficha). Na mesma transação, a obrigação Conciliação (`ACCOUNTING_RECONCILIATION`) vai para OK;
  - **depois de concluída:** a competência trava para a Conciliação até a reabertura, como o Contábil fechado. As mutações respondem 423 `RECONCILIATION_COMPLETED`, e o documento novo dela fica como pendência (`pendencia_periodo_fechado`, motivo `conciliacao_concluida`);
  - **reabertura:** sempre com motivo; a obrigação volta para Pendente.
- **Dentro da autonomia (regras do Controle preservadas):**
  - a obrigação muda só pelo serviço de obrigações (05 §9). As regras do `ChangeStatus` valem: período do módulo aberto, aplicabilidade, status do conjunto e as dependências HARD do Contábil. Com "Contábil digitado/processado" aberto, a conclusão é recusada (422 `DEPENDENCY_BLOCKED`) e nada é gravado;
  - o histórico ganha a origem `SYSTEM_INTEGRATION` (PRD §6.6; mudança aditiva no CHECK), com quem disparou e o evento causador no motivo;
  - a obrigação já final, não aplicável ou não gerada fica como está, e a conclusão registra o que aconteceu (`ja_final`, `nao_aplicavel`, `sem_obrigacao`);
  - na reabertura, a obrigação só volta para Pendente se ainda estiver no OK que a conclusão aplicou e ninguém tiver mudado o status depois (pela ordem do histórico). Senão, fica como está (`mantida`): a mudança humana no Controle prevalece;
  - com o Contábil fechado, não se conclui nem se reabre (423 `PERIOD_CLOSED`): reabre-se o Contábil antes.
- **Idempotência e concorrência (TEST-022):**
  - concluir de novo devolve o estado, sem gravar nada;
  - há uma conclusão em vigor por empresa e competência (índice único parcial), e a conclusão pega a trava exclusiva da empresa;
  - a checagem da trava (`RequireOpen`) pega a mesma trava em modo compartilhado. Assim, a gravação espera a conclusão em andamento e depois é recusada, e as gravações não se esperam entre si;
  - o relógio da conclusão é o do Controle, para o histórico da obrigação ficar numa linha do tempo só.
- **API:** `POST /companies/{id}/reconciliation/complete` e `.../reopen` (com `{reason}`), e a conclusão no resumo da ficha (`GET /companies/{id}/reconciliation`). A conclusão e a reabertura ficam no log de extração.
- **Tela:** o cartão "Conclusão da conciliação" na aba Conciliação da ficha mostra:
  - o estado, o que falta (com o caminho para a fila) e o aviso;
  - a obrigação, concluir com confirmação e reabrir com motivo;
  - o histórico das reaberturas.
  Concluída, a rodada some da aba.
- **E2E:** o TEST-021b passou a concluir a competência, ver a trava num lançamento e reabrir com motivo, com o axe. A empresa do E2E não tem a obrigação gerada em 08/2026 (a API só gera a competência corrente), então a sincronização com a obrigação real fica nos testes de integração.

### D-034 — Playbooks, métricas, aba Conciliação da ficha e contas bancárias (TASK-021c; 10/10/2026; duas decisões do usuário, o resto dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Porte** das rotas `/playbooks` e `/metricas` e da tela "Clientes e contas" do Conciliador (`app/web/rotas.py`, `playbooks.html`, `metricas.html` e `clientes.html`, commit `2bc6e26`).
  - Métricas, playbooks e o resumo da ficha em `backend/internal/reconciliation/metrics`; contas em `backend/internal/reconciliation/bankaccount`. São consultas sobre as tabelas da Conciliação: não há tabela de métrica nem migration.
  - Telas em `frontend/src/features/reconciliation` (`metrics/`, `playbooks/` e `company/`).
- **Métricas (11-observability §7, CC-22).** Filtros opcionais de empresa e competência; a origem não filtrava.
  - O percentual automático da origem (automáticas ÷ linhas do extrato, uma casa) contra a meta `META_CONCILIACAO_AUTO` (80, o padrão da origem; é só a régua do painel). As conciliações ativas por método (o "por passe" da origem) e as automáticas desfeitas pelo contador.
  - Itens por balde: a mesma contagem da fila. A origem mandava o dicionário vazio.
  - Leitura dos extratos e dos comprovantes, como o `make eval-extrator`: as tentativas são as do agente, e o leitor determinístico fica à parte. Aprovado é o extrato importado ou o comprovante completo sem campo corrigido à mão.
  - Custo por documento: todas as chamadas de modelo do documento (triagem e extração), em dólares, como o LiteLLM informa, sem conversão.
  - Correções humanas depois da leitura: campo do comprovante corrigido e "que documento é esse?", sobre os documentos lidos.
  - **Depois do fechamento (decisão do usuário):** as reaberturas do Contábil (sempre com motivo) e, à parte, os documentos que chegaram com a competência fechada; a meta é zero. Só contam as reaberturas de competências em que a empresa tem extrato ou documento na Conciliação.
  - O custo do resolvedor e do classificador por item (M4.7 da origem) e o sucesso dos playbooks por banco.
- **Playbooks (REQ-022, CC-09).** A tela da origem: todas as versões por banco, com o estado, os sucessos, as falhas e a taxa, e a receita de cada versão. Somente leitura.
- **Aba Conciliação da ficha (REQ-036, AC-034).** `GET /companies/{id}/reconciliation?competence=` (o resumo previsto no 06 §4): o Contábil aberto ou fechado (a trava), os extratos que cobrem a competência, o percentual automático, a fila por balde, a classificação e os lançamentos.
  - Na aba: rodar a conciliação (não aparece com o Contábil fechado) e os atalhos para as telas da Conciliação já filtradas pela empresa.
- **Contas bancárias (CC-11; decisão do usuário).** O cadastro da origem passa para a aba da ficha. A conta cadastrada decide a empresa dos próximos extratos dela, e o extrato dela enviado para outra empresa vai para a revisão: é a regra da D-027, que até aqui não tinha como ser usada, porque nada cadastrava conta.
  - Banco, agência e conta precisam ser exatamente os que a leitura grava. Por isso a aba lista as contas que os extratos da empresa leram e que ninguém cadastrou, para cadastrar com um clique; o formulário da origem continua.
  - A identidade é única no sistema (409 `BANK_ACCOUNT_EXISTS`, dizendo de qual empresa é). Sem espaços nas pontas, até 64, 16 e 32 caracteres (os da origem). O cadastro fica no log de extração. Não há desativação; a origem também não tinha.
- **E2E.** O TEST-021c abre a aba da ficha (com a competência e a trava), roda a conciliação, cadastra a conta (e vê a recusa da repetida), segue o atalho para as métricas e abre os playbooks, com o axe. Não usa extrato do cenário; os números das métricas são conferidos nos testes de integração, sobre dados montados no banco. A aba entrou na varredura de acessibilidade da ficha.
- **Correção da D-033:** o TEST-021b usa o extrato do Banco Beta porque o arquivo repetido é recusado e a chave de dedupe das transações é única no sistema, e não por causa da conta bancária.

### D-033 — Lançamentos, razão, plano de contas e regras da Conciliação (TASK-021b; 10/10/2026; dentro da autonomia da spec, sem regra de negócio nova; validada pelo usuário com a TASK)

- **Porte** das telas `lancamentos.html` e `razao.html` e do "Rodar matching" (`POST /matching/rodar`, `app/web/rotas.py`) do Conciliador (commit `2bc6e26`).
  - O plano de contas e as regras não tinham tela na origem (o M5 ficou pela metade, D-031): as telas são novas, sobre a API da TASK-020.
  - Telas em `frontend/src/features/reconciliation` (`entries/`, `ledger/`, `chart/`, `rules/`). Nenhuma mudança de API nem de banco.
- **Submenu.** Lançamentos, Razão, Plano de contas e Regras entram no submenu da Conciliação, depois da fila e dos documentos. A empresa escolhida acompanha a troca de seção (`company_id` na URL); os outros filtros ficam na seção.
- **Lançamentos (CC-12).**
  - A lista traz os lançamentos do cliente, os manuais e os lidos dos comprovantes, com a origem (o comprovante, com o link, ou "formulário ou CSV") e o status; o parcial leva à fila. Filtros por empresa, datas e status, 100 por página. A origem listava tudo, sem filtro.
  - O formulário tem os campos da origem (data, valor, sentido, contraparte) e os da API (CPF/CNPJ e documento). O erro aparece no campo. Depois de incluir, a data e o sentido ficam para o próximo.
  - O CSV do cliente (`data;valor;sentido;contraparte`) é tudo ou nada: recusado, a tela lista o erro de cada linha.
- **Rodar a conciliação.** O "Rodar matching" da origem rodava tudo e ia para a fila. Aqui roda a competência da empresa (D-029), na tela de lançamentos e na fila, quando a empresa e a competência estão escolhidas. O resultado mostra os baldes da rodada e a classificação, com o caminho para a fila.
- **Razão (REQ-028, AC-021, AC-022).**
  - Período por competência (de e até; sem escolha, vale a corrente), o filtro de conta da origem (a conta bancária contém o texto ou a conta contábil é igual a ele) e os totais de débito e crédito do período.
  - As colunas da origem (data, conta bancária, histórico, débito, crédito), mais a conta contábil com a origem da classificação e a contraparte.
  - Exportação CSV do razão filtrado (o arquivo da TASK-020). O XLSX é da TASK-028.
  - Drill-down (AC-021) em diálogo, na cadeia da spec: classificação (a conta, a origem, a regra ou a linha do histórico) → conciliação (método, origem, pontuação) → transação do extrato (com o link para a linha na fila, onde se desfaz ou reclassifica) → originais (o extrato e cada comprovante; o lançamento manual não tem original). A origem tinha só o link para o documento.
- **Plano de contas (CC-20).** Incluir, renomear, desativar e reativar; importar do CSV (`codigo;nome`, tudo ou nada, com o erro de cada linha) e copiar de outra empresa. Desativar pede confirmação: a conta sai das classificações novas, das regras e do histórico, e o que já foi classificado fica. Nada é apagado.
- **Regras (CC-20, CC-21).**
  - A lista vai da mais nova para a mais antiga (a ordem em que valem), com a conta, a situação e o link para a linha cuja correção originou a regra; há o filtro "só as ativas".
  - A regra nova escolhe a conta só do plano ativo, com a explicação do que cada tipo compara. Ao confirmar, a tela avisa a regra antiga substituída. Desativar pede confirmação.
  - "Aplicar às linhas pendentes" roda a classificação da competência (regra, depois histórico) e mostra o resultado.
- **E2E.** O TEST-021b usa o extrato do Banco Beta do cenário Horizonte. O do Banco A Fonte é do TEST-021a: o mesmo arquivo (sha256) é recusado, e as transações do mesmo extrato importado de novo são ignoradas, porque a chave de dedupe é única no sistema (texto corrigido na TASK-021c, D-034).
  - Lançamentos pela tela (formulário e CSV, com a recusa), a rodada, a tarifa sem comprovante (pela API), o plano (formulário, CSV, desativar e reativar), as três regras e "aplicar".
  - O razão com as contas, os totais e o filtro, a exportação (o arquivo é conferido) e o drill-down até o extrato original.
  - O axe roda em cinco telas e diálogos.
- **Fica para depois:** 021c (playbooks, métricas e a aba Conciliação da ficha).

### D-032 — Fila de revisão e documentos da Conciliação (TASK-021a; 10/10/2026; recorte, desfazer e visualizador decididos pelo usuário, o resto dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Recorte (decisão do usuário).** A TASK-021 foi dividida em três partes, cada uma com validação e commit:
  - 021a: a fila de revisão e os documentos, com as ações da fila na API (a prioridade de UX);
  - 021b: lançamentos, razão com drill-down, plano de contas e regras (a API já existe);
  - 021c: playbooks, métricas e a aba Conciliação da ficha.
- **Porte** da rota `/fila` e das ações `conciliar`, `sem-comprovante` e `desfazer` (`app/web/rotas.py`), de `candidatos_para_extrato` e `candidatos_para_comprovante` e das telas `fila.html`, `documentos.html` e `documento.html` do Conciliador (commit `2bc6e26`).
  - API em `backend/internal/reconciliation/review`; o visualizador e "que documento é esse?" em `documents`.
  - Telas em `frontend/src/features/reconciliation`.
- **Fila (08 §7).** Os baldes da origem com a contagem:
  - `revisao`: documento em revisão da triagem ou da extração, ou parado por período fechado, e comprovante parcial;
  - `divergencia`: linha livre com divergência, caso de borda (TASK-018) ou conciliação desfeita;
  - `extrato_sem`: linha livre sem marca;
  - `comprovante_sem`: comprovante completo sem extrato;
  - `classificacao` (novo): linha conciliada à espera da classificação (TASK-020).
  - Filtros por empresa, competência e balde. A origem não filtrava: listava tudo do banco.
  - O item aberto mostra o original à esquerda e, à direita, os dados, os candidatos e as ações. J e K passam de item; Esc fecha.
  - O motivo aparece em texto, com os textos da origem (`motivo_legivel`).
- **Ações da fila**, todas humanas, no log de extração, com a trava do Contábil (AC-025) e serializadas com a rodada da empresa:
  - **conciliar à mão:** a origem casava um comprovante; aqui são um ou mais (N:1), com a diferença de valor na resposta (a origem não conferia o valor). A linha passa pela classificação;
  - **"não há comprovante":** como a origem (`sem_comprovante`), também com a classificação;
  - **desfazer (decisão do usuário):** vale para a conciliação automática também, com motivo obrigatório. A origem apagava a conciliação, e só a manual. Nada é apagado: ficam quem, quando e por quê, e os comprovantes ficam livres. A linha perde a classificação e ganha a marca `desfeita`, que a rodada não troca: ela volta para a fila como divergência e não volta sozinha ao automático;
  - **corrigir o comprovante campo a campo** (REQ-023; a origem só planejava, M2.6): os campos enviados, com as regras do lançamento manual. O campo corrigido ganha confiança 1 e origem humana. Com os essenciais preenchidos, o parcial vira `extraido` e a competência roda de novo, na mesma transação;
  - **"que documento é esse?"** (a origem só listava esses documentos): o contador diz o tipo, a empresa e, no extrato, banco, agência, conta e período. Os dados que ele não informa ficam como a triagem leu. O documento volta à extração, com os mesmos portões. Documento que não é da Conciliação continua em revisão: não há estado de descarte no contrato congelado.
- **Candidatos** da origem: os lançamentos livres de mesmo valor e sentido em D±3 e os anotados pelo matching, do mais perto ao mais longe; para o comprovante, as linhas livres em D±3.
- **Original (decisão do usuário):** as páginas do PDF como imagem, renderizadas pela API com o mesmo PDFium do extrator, no DPI do extrator do comprovante (150). Assim o bbox de cada campo lido cai no lugar e é desenhado por cima. Os cabeçalhos de segurança da API não mudaram (o PDF numa moldura exigiria `X-Frame-Options: SAMEORIGIN`). Imagem aparece como está; CSV e TXT, como texto.
- **Documentos.** Envio por arrastar ou escolher, com a empresa e a competência opcionais. A tela acompanha cada documento até o worker terminar e mostra a fila parada e a falha com reenvio. A lista filtra por status e empresa. O detalhe mostra o original, os dados da triagem, a importação, os campos do comprovante (com a confiança e a correção), as tentativas e as ações.
- **Resolvedor e classificação na fila.** A hipótese do resolvedor e a sugestão do agente aparecem como sugestão, não como fato (08 §7), com aceitar e rejeitar. A correção da conta e o aceite mostram a regra proposta, com "confirmar regra".
- **Menu.** A área Conciliação entrou no menu principal (08 §2), com o submenu Fila de revisão e Documentos.
- **E2E.** `make e2e` sobe também um MinIO só do E2E: projeto compose próprio (`controle-e2e`), porta 9102, volume e credenciais de teste. Nunca usa o MinIO de dev.
  - O TEST-021a envia o extrato e um comprovante do cenário Horizonte, que o worker lê sem LLM.
  - Depois concilia à mão, desfaz com motivo, confirma "não há comprovante" e abre o documento com a página como imagem.
  - O axe roda em quatro telas e diálogos.
- **Fica para depois:** 021b (lançamentos, razão, plano e regras) e 021c (playbooks, métricas, aba da ficha).

### D-031 — Classificação contábil e razão da Conciliação (TASK-020; 10/10/2026; quatro regras de negócio decididas pelo usuário, o resto dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Porte** de `app/domain/classificacao.py`, `app/agentes/classificador.py` (com o prompt), da `SugestaoClassificacao` (`app/domain/schemas.py`), dos modelos `PlanoContas` e `RegraClassificacao` e de `app/servicos/razao.py` do Conciliador (commit `2bc6e26`).
  - **Na origem, o M5 ficou pela metade:** o motor de classificação, o agente, o plano de contas e as regras existiam, mas nada os chamava nem tinha rota ou tela; só o razão e o CSV estavam ligados. Aqui o fluxo do M5 (plan.md M5.1 a M5.7) é ligado de ponta a ponta.
  - Domínio puro em `backend/internal/reconciliation/classification`: `classificar`, `aplicar_regra`, `regra_derivada_de_correcao`, o `re.escape` e o schema da sugestão. Travado por golden gerado pelo próprio Python da origem (`testdata/classification_gen.py`): 1.260 classificações, 476 regras derivadas, 15 escapes (com o que cada padrão casa) e 43 saídas do agente.
  - Plano e regras em `backend/internal/reconciliation/chart`; classificação, correção e agente em `backend/internal/reconciliation/classify`; razão e drill-down em `backend/internal/reconciliation/ledger`.
- **Decisões do usuário (as quatro recomendadas):**
  - **onde fica a classificação:** na linha do extrato conciliada, uma conta por linha do razão. O CNPJ e a contraparte vêm dos comprovantes da conciliação, quando eles concordam (N:1 do mesmo fornecedor); a linha sem comprovante, ou com comprovantes misturados, é classificada pelo descritivo. A origem tinha a coluna no lançamento e na transação e o razão usava a do lançamento; as colunas do lançamento ficam sem uso;
  - **agente só a pedido:** a regra e o histórico rodam sozinhos ao conciliar (na rodada do matching e no aceite do resolvedor). O agente sugere quando o contador pede, como o resolvedor; `CLASSIFICADOR_AUTOMATICO` (padrão desligado) faz a rodada pedir uma vez por linha;
  - **histórico:** as classificações confirmadas dos meses anteriores ("como esse descritivo foi classificado nos meses anteriores", PRD §9), com a chave da origem (contraparte ou descritivo normalizados). Com contas diferentes para a mesma chave, vale a mais recente;
  - **regras:** a mais nova vale (testadas da mais nova para a mais antiga). A regra confirmada com o mesmo tipo e padrão de outra desativa a antiga, então a correção confirmada passa a valer.
- **Classificação automática.** Só preenche o que não foi confirmado: a consulta só grava na linha sem classificação confirmada, então nunca troca a do contador, nem em corrida. Linha sem regra nem histórico fica `agente` pendente, sem conta (o `ResultadoClassificacao(None, "agente", True)` da origem). Regra e histórico com conta desativada no plano não contam.
  - A linha guarda o que classificou: a regra, a linha do histórico ou a sugestão aceita, e quem confirmou. O banco garante a coerência (migration `0029`): a conta é do plano da empresa, e cada origem tem o que a produziu.
- **Correção humana (M5.5, REQ-027).** `PUT /reconciliation/review/{id}/classification` grava a conta com origem `manual` e o autor e devolve a regra derivada (CNPJ, senão contraparte, senão o descritivo escapado). A regra só vale quando o contador a confirma em `POST /companies/{id}/classification-rules`; vale para as próximas classificações, nunca reclassifica o que foi confirmado. A origem dizia "vale no mês seguinte": aqui vale também para as linhas que ainda esperam classificação, quando a classificação roda de novo (`POST /companies/{id}/classification/run`).
- **Agente classificador (AC-020).** Job `agent_classifier`, alias `classificador` (D-008), prompt `conciliacao/classificador.md` (versão 2026.10.1).
  - Antes do modelo, o job roda de novo a regra e o histórico: classificando, o agente não é chamado (precedência). Linha desconciliada, já confirmada ou com o Contábil fechado também não chega ao modelo.
  - Contexto: o lançamento (com o CNPJ só se for de empresa; CPF não vai ao modelo), o plano ativo e o `historico_classificacao` da tool-contract: até 10 chaves parecidas do histórico (Jaro-Winkler a partir de 0,80), sem a mesma chave. Como na origem, o modelo não chama ferramenta; `consultar_cnpj` aguarda decisão (P-007).
  - A conta sugerida precisa estar no plano ativo; fora dele, a sugestão fica sem conta e não pode ser aceita. Saída fora do schema não vira sugestão, sem nova tentativa.
  - A sugestão fica pendente em `classification_suggestions` com a corrida; só o aceite humano muda a conta da linha (origem `agente`, com o autor e a sugestão) e propõe a regra sugerida pelo agente (ou, sem ela, a derivada), que também só vale confirmada.
- **Plano de contas (M5.1) e regras (M5.2).** Por empresa: incluir, renomear, desativar (nada é apagado), importar CSV `codigo;nome` (tudo ou nada, UTF-8 ou Windows-1252) e copiar as contas ativas de outra empresa (o "padrão" da D-020). Regras por CNPJ (só os dígitos), contraparte (normalizada) ou regex do descritivo com IGNORECASE. A regex é validada ao cadastrar: só o subconjunto do `re` que o porte traduz (`pytext`, que passou a aceitar intervalos sem letra, como `[0-9]`, com IGNORECASE).
- **Razão (M5.6, M5.7; REQ-028, AC-021).** Consulta sobre as conciliações ativas, nunca tabela: reclassificar ou desfazer muda o razão porque muda a origem. Uma linha por conciliação (a origem repetia a transação em cada lançamento do N:1), com a conta bancária na coluna "conta" (como a origem), a conta contábil e a origem da classificação, a contraparte, o débito (saída) e o crédito (entrada).
  - Filtro de conta da origem: a conta bancária contém o texto, sem caixa, ou a conta contábil é igual a ele. Período por competência (`from` e `to`).
  - Drill-down: a linha leva os ids da conciliação, da transação, do original do extrato e dos comprovantes; `GET /reconciliation/reconciliations/{id}` traz a classificação (com a regra), a transação e os comprovantes com os originais.
- **Exportação (REQ-029, AC-022).** `GET /companies/{id}/ledger/export?format=csv`: as colunas da origem (`data;conta;historico;debito;credito`) e as do AC-022 no fim, determinística; campo com ponto e vírgula vai entre aspas (a origem não escapava). O XLSX fica para a TASK-028.
- **Auditoria.** Toda ação humana (plano, regras, correção, pedido, aceite e rejeição da sugestão) vai para o log de extração. Corrida `classificador` no tracer, com a decisão `sugestao_classificacao` (códigos e contagens, sem nomes).
- **Fica para depois:**
  - TASK-021: as telas (plano, regras, fila com a classificação e a sugestão, razão com drill-down) e as ações da fila que conciliam à mão, que também passam pela classificação;
  - TASK-028: o XLSX e a comparação com fixture;
  - o resolvedor continua sem o `historico_classificacao` (o contexto da origem não o tinha).

### D-030 — Resolvedor de exceções da Conciliação (TASK-019; 10/10/2026; plano aceito pelo usuário, o resto dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Porte** de `app/agentes/resolvedor.py`, do `HipoteseResolvedor` (`app/domain/schemas.py`), do `contexto_resolvedor` e das rotas `pedir_hipotese`, `aceitar_hip` e `rejeitar_hip` (`app/web/rotas.py`) do Conciliador (commit `2bc6e26`).
  - Domínio puro em `backend/internal/reconciliation/resolver`: o schema com a validação do pydantic, o contexto que o modelo recebe e o plano do aceite.
  - O schema é travado por golden gerado pelo próprio Python da origem (`testdata/resolver_gen.py`): 56 saídas do modelo, 28 válidas e 28 recusadas. O contexto e o aceite têm testes escritos a partir do código da origem.
  - Agente, API e aceite em `backend/internal/reconciliation/proposals`.
- **O resolvedor só propõe (REQ-025, AC-019).** O agente grava a proposta pendente (a hipótese como a origem a gravava, com a corrida no tracer) e nada mais.
  - Só duas consultas criam conciliação: a da rodada determinística (AC-018) e a do aceite humano. Um teste lê o código e prova que a do aceite só é chamada pelo aceite e que o agente não chama nenhuma.
  - Nem a confiança alta nem `pede_humano: false` conciliam: o teste de ponta a ponta confere que o banco fica sem conciliação até o aceite.
- **Pedido assíncrono (REQ-046).** A origem chamava o modelo dentro da requisição. Aqui, `POST /reconciliation/review/{id}/proposal` enfileira o job `agent_resolver` e responde 202.
  - Com um pedido da mesma linha na fila, devolve o mesmo job (`already_queued`), sem novo registro no log.
  - O job confere de novo a linha: conciliada enquanto esperava, ou com o Contábil fechado, termina sem chamar o modelo.
  - Saída fora do schema: a corrida fica com erro e o job morre sem nova tentativa, porque cada chamada tem custo. A origem também não gravava proposta. Alias desligado na tela de IA: o job morre sem chamar o modelo.
- **Contexto (`contexto_resolvedor`).** Os mesmos blocos da origem: a linha com o motivo da divergência; os candidatos (livres, de mesmo valor e sentido em D±3, mais os anotados pelo matching); até 15 comprovantes soltos próximos; as linhas irmãs; a observação e as regras da origem. Três diferenças:
  - só a empresa da linha. A origem olhava os lançamentos e as linhas de todos os clientes do banco;
  - a linha leva também as `marcas` (divergências e casos de borda da TASK-018), e cada comprovante leva o `status`, porque agora o parcial chega ao contexto;
  - sem ferramentas: como na origem, o sistema monta o contexto e o modelo não chama nada. `historico_classificacao` chega com a classificação (TASK-020); `consultar_cnpj` aguarda decisão (P-007).
- **Aceite (`aceitar_hip`).** Cada par vira conciliação com origem `manual`, o método `resolvedor` (ou `sem_comprovante`, no par de extrato sem comprovante sem lançamento), o autor e a proposta. O N:1 vai em `reconciliation_items`, como na TASK-018; a origem gravava uma linha de conciliação por lançamento.
  - Como na origem, id inventado, fora do formato ou já conciliado é ignorado.
  - Diferenças: linha ou lançamento de outra empresa também é ignorado. Sem nenhum par conciliável, o aceite responde 409 `PROPOSAL_NOTHING_TO_ACCEPT` e a proposta segue pendente; a origem a marcava aceita com zero conciliações.
  - Serializado com a rodada do matching da empresa (o mesmo lock). Trava do Contábil (AC-025) em cada competência das linhas.
- **Rejeição (`rejeitar_hip`).** A proposta fica `rejeitada`, com o autor e a data. Diferença: proposta já decidida responde 409 `PROPOSAL_DECIDED`; a origem rejeitava de novo até a aceita.
- **Auditoria.** O pedido, o aceite e a rejeição vão para o log de extração como ação humana (o `_auditar` da origem).
  - Corrida `resolvedor` no tracer: gatilho `pedido` (com quem pediu) ou `rodada` (automático), o contexto resumido na entrada (motivo e contagens, sem descritivos) e a decisão `proposta_resolvedor`.
  - A decisão traz os ids que o modelo citou fora do contexto (`ids_fora_do_contexto`), que o aceite ignora se não forem da empresa e livres.
- **Resolvedor automático (`RESOLVEDOR_AUTOMATICO`, padrão desligado).** O 06 §4 prevê a rodada enfileirando o resolvedor "se configurado"; a origem só o chamava a pedido.
  - Ligado, a rodada pede hipótese uma só vez para cada linha livre da competência sem proposta nem pedido anterior. A rejeitada e a de job com falha não voltam sozinhas; o contador pede de novo na fila.
  - A rodada devolve `resolver_requested`. O padrão é desligado pelo custo de provedor por linha.
- **Prompt** `conciliacao/resolvedor.md` (versão 2026.10.1): o texto da origem, com a entrada que o sistema envia no lugar das ferramentas e a regra de que o conteúdo da entrada é dado, não instrução. Saída estruturada com `json_schema` estrito; o `tipo` fica na lista do prompt da origem.
- **API.** `POST /reconciliation/review/{id}/proposal` (202), `GET /reconciliation/review/{id}/proposals`, `POST /reconciliation/review/{id}/accept-proposal` e `/reject-proposal` (corpo `{proposal_id}`). O `{id}` é a linha do extrato, como nas outras ações da fila do 06 §4.
- **Banco.** Migration `0028`: índices da proposta por linha e dos jobs por tipo e chave (o "uma só vez por linha" do automático). A chave da proposta para a corrida já existia (0022).
- **Fica para depois:**
  - TASK-020: o `historico_classificacao` como entrada do resolvedor, se a classificação o pedir;
  - TASK-021: a fila na interface, com a hipótese visível como sugestão (08 §Fila de revisão);
  - eval do resolvedor com provedor real: não há golden de hipóteses na origem; fica para a avaliação humana (TASK-024).

### D-029 — Matching da Conciliação: passes 1 a 4, pré-filtro e bordas (TASK-018; 09/10/2026; regra dos passes por decisão do usuário, ADR-008; o resto dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Porte** de `app/domain/{conciliacao,similaridade,bordas}.py` e do `rodar_matching` com o pré-filtro `_ids_divergentes` (`app/servicos/matching.py`) do Conciliador (commit `2bc6e26`).
  - Domínio puro em `backend/internal/reconciliation/matching`: Jaro-Winkler, a cascata, o pré-filtro, as marcações de borda e o `buscar_candidatos` (a ferramenta do resolvedor, para a TASK-019).
  - Travado por golden gerado pelo próprio Python da origem (`testdata/matching_gen.py`): 175 pares de Jaro-Winkler (bit a bit), 513 cascatas, 300 pré-filtros, 200 casos de borda, 150 buscas de candidatos e o cenário Horizonte inteiro, como o `carregar_horizonte.py` o carrega.
  - Rodada no banco em `backend/internal/reconciliation/reconcile`.
- **Três mudanças no contrato congelado dos passes, aprovadas pelo usuário (ADR-008):**
  - ambiguidade dos dois lados: o comprovante disputado por duas linhas não vai para a primeira da ordem; vira divergência, e o passe seguinte ainda pode resolver (o 3, pelo nome). No passe 4, a solução que divide lançamento com a de outra linha também é divergência;
  - casos de borda (transferência entre contas, estorno em até 7 dias, bruto × líquido até R$ 20,00 em D±3) são marcados e saem do automático. A origem tinha as funções, mas não as chamava;
  - só entram no automático os lançamentos extraídos e manuais; o parcial espera a revisão.
- **Fidelidade.** Sem disputa, o resultado é o da origem: no golden, os 500 casos sem disputa saem idênticos e os 13 com disputa saem mútuos. O cenário Horizonte inteiro, pelo fluxo real (contas a pagar e a receber pela API, os 2 extratos e os 13 comprovantes pelos leitores determinísticos, a rodada no worker), dá o mesmo resultado da origem: 25 casamentos no passe 1, as mesmas 14 linhas tiradas pelo pré-filtro com o motivo (parcelado, juros no boleto, ISS retido, NF em vários PDFs, contraparte repetida) e a disputa da Escola Crescer (o título a receber e o comprovante).
- **Rodada.** Por empresa e competência: as linhas livres da competência contra os lançamentos livres da empresa (o passe 3 da origem não olha a data). A origem rodava sobre todas as linhas e todos os clientes do banco.
  - O pré-filtro olha todos os lançamentos da empresa, como a origem olhava os do cliente; as bordas olham as linhas a até 7 dias da competência, conciliadas ou não.
  - Conciliação automática com origem `auto`, passe, método e score da origem, sem autor (o banco exige, D-020); N:1 em `reconciliation_items`.
  - Marcas da fila em `statement_transactions.flags`: `{tipo: "divergencia", motivo, candidatos, concorrentes}` (o formato da origem, mais as linhas concorrentes) e `{tipo: "borda", motivo, relacionados}`. A rodada troca as marcas derivadas da rodada anterior em vez de acumular (a origem repetia a divergência a cada rodada); a linha conciliada fica sem elas; marcas de outro tipo ficam.
  - Idempotente e serializada por empresa (lock transacional): rodar de novo não concilia de novo nem duplica marca.
  - Trava do Contábil (AC-025): competência fechada, a API responde 423 e o job não faz nada.
  - O passe 4 tem orçamento de combinações por linha; passando dele, a linha vira divergência (`passe_4_extenso`) em vez de prender o job. A origem enumerava sem limite. A soma é feita em centavos, com o corte dos ramos que passam do valor (valores positivos), sem mudar o resultado.
- **Quando roda.**
  - `POST /companies/{id}/reconciliation/run?competence=AAAA-MM` (06 §4), na hora, com os baldes da competência: conciliado, divergente (marcado), extrato sem comprovante, comprovante sem extrato e aguardando revisão (parciais). O resolvedor para os remanescentes vem na TASK-019.
  - O job `reconciliation_run` (04 §7), que o worker enfileira quando importa extrato ou grava comprovante, como a origem rodava o matching depois de cada envio. Sem chave de dedupe: a rodada é idempotente, e o gatilho não pode se perder com outra rodada em andamento. Lançamento manual não dispara a rodada (a origem também não); o usuário roda pela API.
- **Configuração.** `LIMIAR_FUZZY` (0,86, o da origem), na API e no worker; maior que 0 e até 1.
- **Fica para depois:**
  - TASK-019: o resolvedor para os remanescentes (`buscar_candidatos` já portado);
  - TASK-021: a fila (baldes, marcas, conciliar e desfazer à mão), que também roda a competência depois da correção de um parcial;
  - TASK-022: a sincronização com a obrigação Conciliação.

### D-028 — Extração de comprovantes e lançamentos manuais da Conciliação (TASK-017; 09/10/2026, dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Porte** de `app/domain/schemas.py` (`ComprovanteExtraido`), `app/agentes/extrator_comprovante.py`, `app/servicos/comprovantes.py`, do leitor de comprovante de `app/servicos/processar_local.py` e das rotas `/lancamentos` e `/lancamentos/csv` do Conciliador (commit `2bc6e26`).
  - Domínio puro em `backend/internal/reconciliation/receipt`: schema com a validação do pydantic, `campos_abaixo_limiar` e o leitor determinístico (`parse_comprovante_texto`). Travado por golden gerado pelo próprio Python da origem (`testdata/receipt_gen.py`): 77 saídas do LLM e 36 textos, entre eles os 13 comprovantes do cenário lidos pelo PyMuPDF.
  - A validação no estilo do pydantic, que o extrato e o comprovante dividem, foi para `internal/platform/pymodel`.
  - No worker (`pipeline`): o leitor no job de triagem e o job `receipt_extract`.
- **Leitor determinístico (CC-08, o atalho `processar_local` para comprovantes)**, no job de triagem e sem LLM:
  - só lê o PDF com texto que diz "comprovante" e não é extrato. A origem tentava ler qualquer PDF com uma data e um valor;
  - a origem procurava a palavra "horizonte" (a empresa da aula) no pagador e no recebedor para decidir o sentido. O porte procura a empresa do documento pelo nome: razão social e nome fantasia, sem a natureza jurídica, em palavras inteiras, sem acento nem caixa. Nos textos do golden as duas regras coincidem;
  - a empresa é a do envio ou, sem ela, a única empresa cadastrada cujo CNPJ está impresso no comprovante;
  - a leitura só vale com a empresa achada no comprovante (pagadora, recebedora ou dona da "conta debitada") e com os campos essenciais válidos. Senão, o documento segue para o triador e o extrator. A origem gravava qualquer leitura e, sem achar a empresa, supunha entrada com confiança 0,80, acima do limiar;
  - os 13 comprovantes do cenário saem iguais aos da origem, do PDF ao lançamento (teste de ponta a ponta).
- **Extrator (REQ-023, CC-13).** Roteamento da M3.3 da origem, com até 2 chamadas ao alias `extrator`:
  - o PDF com texto vai primeiro como texto (até 16.000 caracteres, como na origem). Se a saída vier fora do schema ou com campo essencial fraco, vai de novo com as páginas renderizadas a 150 DPI (até 3) e o texto;
  - PDF sem texto vai renderizado; foto ou print, como estão. Vale a leitura com menos campos a revisar;
  - **divergências:** a origem fazia uma chamada só e, sem texto, mandava a marca "[imagem N bytes]" no lugar da imagem. O modelo também não sabia quem era o cliente: aqui a mensagem leva a empresa do documento (razão social e CNPJ), de que o sentido depende;
  - saída estruturada com `json_schema` estrito. O schema pede também `alertas`, no vocabulário da origem (`parcelado`, `juros`, `iss_retido`, `nf:<número>`); marca fora dele é descartada. As marcas que o código lê do texto (as mesmas do leitor) somam-se às do agente;
  - guarda a mais, como no extrato: confiança fora de 0 a 1 recusa a saída, assim como os tipos fora do schema estrito.
- **Regras de campo (M3.4)**, depois do schema:
  - campo essencial (data, valor, sentido, contraparte) nulo ou abaixo do `LIMIAR_CAMPO` (0,70, o da origem) deixa o lançamento `parcial`, fora do razão, e o documento em `revisao_extracao` (`campos_abaixo_limiar`, com os campos e o limiar);
  - o porte acrescenta: valor ≤ 0 ou fora de numeric(14,2) fica nulo e vai à revisão; contraparte em branco, idem. A origem gravava o valor negativo;
  - `contraparte_doc` fica só com os dígitos. Sem 11 (CPF) ou 14 (CNPJ), como o CPF mascarado do PIX, fica nulo, sem mandar à revisão: não é campo essencial na origem.
- **Gravação (CC-13, CC-14)** na transação do job: um lançamento por comprovante (índice único, migration `0027`), com a confiança e a origem (página, trecho, bbox) de cada campo, as marcas e a corrida do extrator.
  - **Duplicata de leiaute diferente (M3.5):** mesmo valor, mesma data e contraparte normalizada é gravada e sinalizada, nunca descartada. O porte compara só na mesma empresa; a origem comparava com todos os lançamentos.
  - **Empresa:** a do envio ou a do CNPJ impresso. Sem nenhuma, ou com mais de uma cadastrada no comprovante: `revisao_extracao` (`empresa_nao_identificada`), sem lançamento.
  - **Trava (AC-025):** Contábil fechado na competência da data do comprovante deixa o documento em `pendencia_periodo_fechado`, sem lançamento.
- **Lançamentos manuais (CC-12)** em `backend/internal/reconciliation/entries`: `POST /reconciliation/entries` (formulário), `POST /reconciliation/entries/import` (CSV) e `GET /reconciliation/entries` (lista com filtros).
  - Status `manual`, com o autor e a intervenção humana no log de extração (a origem registrava só a do formulário). Trava do Contábil em cada competência.
  - Empresa obrigatória; no CSV a origem não tinha cliente. Data ISO, valor positivo com até 2 casas (ponto ou vírgula), sentido C ou D, documento com 11 ou 14 dígitos.
  - CSV `data;valor;sentido;contraparte` da origem (cabeçalho opcional), tudo ou nada, com o erro de cada linha. A origem pulava a linha curta e derrubava a importação na primeira data ou valor ilegível. Aceita UTF-8 e Windows-1252.
- **Documento.** O detalhe traz o lançamento (`entry`). O reprocessamento vale para o comprovante em revisão sem lançamento; com lançamento, responde 409, porque a correção é campo a campo na fila (TASK-021).
- **Prompt** `conciliacao/extrator_comprovante.md` (versão 2026.10.1): o texto da origem com os itens do prompt-spec §2, a entrada que o sistema envia, a empresa do documento e as marcas. Saíram as ferramentas que o modelo não chama: o render é do código e a `consultar_cnpj` não existe.
- **Auditoria (CC-10).** Cada tentativa vai para o log de extração, com as regras de campo no lugar do validador; a leitura determinística, como `leitor_local` (tentativa 0).
  - Corrida `extrator_comprovante` no tracer, com a decisão `extracao_comprovante`. Nome da contraparte não entra na decisão, que vai ao Langfuse.
- **Relatório (M3.9).** Gabarito dos 13 comprovantes do cenário (`testdata/golden/comprovantes/gabarito.json`, com os dados do gerador da origem).
  - Mede a acurácia por campo (data, valor, sentido, contraparte, documento da contraparte e tipo), as chamadas, o custo e a latência.
  - `make test` roda com o gateway simulado. `make eval-extrator` passa a avaliar também o comprovante, com custo de provedor real: **não rodado**, depende do seu aceite.
- **Configuração.** `LIMIAR_CAMPO` no worker (compose e `.env.example`).
- **Não portado:** `reler_comprovantes_soltos` (a origem não o chamava de lugar nenhum) e a `consultar_cnpj` (P-007, aguarda decisão).
- **AC-020** (classificação) é da TASK-020; esta TASK entrega os lançamentos que ela classifica.
- **Fica para depois:**
  - TASK-018: o matching lê as marcas (parcelado, juros, ISS retido, NF repetida) para não conciliar sozinho;
  - TASK-021: a fila campo a campo, com o trecho e o bbox, e as telas dos lançamentos.

### D-027 — Triagem e extração do extrato da Conciliação (TASK-016; 09/10/2026, dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Porte** de `app/agentes/{triador,extrator_extrato,ferramentas,llm}.py`, `app/domain/{schemas,validador,money,csv_extrato,pdf_extrato}.py` e `app/servicos/{pipeline,importacao,processar_local}.py` do Conciliador (commit `2bc6e26`).
  - Domínio puro em `backend/internal/reconciliation/statement`: schema do extrato e da triagem com a validação do pydantic, validador, dicas e leitores determinísticos. Travado por golden gerado pelo próprio Python da origem (`testdata/statement_gen.py`, 256 casos) e por testes de propriedade do validador (TEST-016).
  - Processamento no worker em `backend/internal/reconciliation/pipeline`: os jobs `reconciliation_triage` (nasce no envio) e `statement_extract` (o nome do 04 §7). O comprovante triado ganha o `receipt_extract`, que espera na fila até a TASK-017 (D-019).
- **Ferramentas de PDF (CC-03).** A origem usa o PyMuPDF; aqui é o mesmo PDFium em WebAssembly do PGDAS-D (`internal/platform/pdftext`).
  - `extrair_texto_pdf`: texto por página no formato do `page.get_text()` do PyMuPDF (cada célula da tabela numa linha), que o leitor determinístico exige. As regras de linha e espaço foram medidas contra o PyMuPDF 1.25.4, sem copiar código do MuPDF (AGPL).
  - O golden tem 67 PDFs (o cenário da origem, o PGDAS-D e 46 sintéticos); 65 saem idênticos. As 2 diferenças estão documentadas: o PDFium descarta o texto repetido no mesmo lugar ("negrito falso") e ordena pela posição os objetos de texto de uma mesma linha.
  - `renderizar_pagina`: PNG em 36 a 600 DPI, sobre fundo branco; página gigante cai de DPI para caber em 25 milhões de pixels.
  - O `cache_render` (render guardado no storage) não foi portado: a origem não o usava.
- **Leitor determinístico do PDF (CC-08, o atalho `processar_local`)**, agora no job de triagem e sem LLM: o texto no leiaute do cenário (Banco A Fonte, Banco Beta) que fecha o saldo importa com tentativas = 0, como na origem. Três diferenças:
  - só lê com o banco nomeado no texto (a origem assumia `a_fonte` para qualquer banco);
  - se não ler ou não fechar, o documento segue para o triador, em vez de derrubar o job ou ir direto para a revisão sem passar pelo agente;
  - o `garantir_horizonte` não foi portado: ele criava a empresa da aula para todo PDF; aqui ninguém cria empresa. O leitor de comprovante fica para a TASK-017 e o cenário Horizonte, para a TASK-027.
- **Triador (REQ-019, CC-04).** Classifica pelo texto das 3 primeiras páginas (até 12.000 caracteres), com o limiar `LIMIAR_TRIAGEM` (0,70, o da origem). Abaixo dele, ou tipo desconhecido, o documento vai para `revisao_triagem` com o diagnóstico.
  - PDF sem texto vai como a imagem da 1ª página a 72 DPI; foto ou print, como a própria imagem; CSV/TXT, como o conteúdo. A origem mandava ao modelo um texto vazio, ou só a marca "[imagem pagina 0 N bytes]", embora o prompt prometesse a imagem.
  - Saída fora do schema: a tentativa do job falha (`TRIAGE_INVALID_OUTPUT`) e a fila tenta de novo, como na origem.
  - Alias desligado na tela de IA: `revisao_triagem` (`agente_desligado`), sem gastar tentativas. PDF ilegível: `documento_ilegivel`.
- **Extrator (REQ-020, CC-05).** Até 3 tentativas: texto nativo; render a 150 DPI; render a 300 DPI com o `extrator_forte`.
  - Da 2ª em diante vão o extrato anterior, o resultado do validador (o mesmo JSON do `model_dump_json` da origem, byte a byte) e as dicas.
  - **Divergência principal:** nas tentativas 2 e 3, a origem mandava só o tamanho do PNG (`render_pagina0_bytes=…`), sem a imagem e sem o texto: o modelo não via o documento, e o loop de correção não tinha como funcionar. Aqui as páginas (até 10) vão como imagem, como o PRD §6.3 e os prompts pedem. O formato com imagem foi provado contra o LiteLLM real em `make test-litellm`.
  - Sem camada de texto, a 1ª tentativa já vai com as páginas renderizadas (o prompt mandava pedir o render, e não há chamada de ferramenta). Foto ou print vão como imagem em todas as tentativas.
  - Saída inválida conta como tentativa perdida: JSON quebrado, fora do schema ou dinheiro como número (`TentativaParseError`). Na origem, parte dessas saídas (float no valor, texto não numérico) derrubava o job.
  - Guardas a mais, que o pydantic aceitava e o banco recusaria: ordem e página de 1 a 2³¹−1, confiança de 0 a 1, valor dentro de numeric(14,2) e período em ordem. Tipos fora do schema estrito (booleano como número, texto como inteiro, data como timestamp) também são recusados.
  - Saída estruturada com `json_schema` estrito, como o consultor (D-024); a origem pedia `json_object`.
  - O log grava o alias efetivo: `extrator_forte` na 3ª tentativa (a origem gravava `extrator`).
- **Portão (REQ-021, CC-06, AC-015, AC-016).** `statement.Validate` é o `validar_extrato` (6 regras, tolerância R$ 0,00) e responde também à ferramenta `validar_saldo`.
  - Sem o portão aprovado, nada é gravado e o documento vai para `revisao_extracao` com o diagnóstico da última tentativa.
  - O banco tem ainda o `CHECK (validation_ok)` da D-020.
- **Importação (CC-07)** atômica, na transação do job, com a chave de dedupe da origem (a transação repetida fica de fora). O porte acrescenta:
  - **empresa:** a conta bancária cadastrada decide; sem ela, vale a do envio; sem nenhuma, `revisao_extracao` (`empresa_nao_identificada`). Conta de outra empresa: `conta_de_outra_empresa`;
  - **trava (AC-025):** Contábil fechado em alguma competência do extrato (das transações ou do período) deixa o documento em `pendencia_periodo_fechado`, sem importar. A origem não tinha empresa nem essa trava na importação.
- **CSV (CC-08).** O CSV do banco (cabeçalho `data;descricao;documento;tipo;valor;saldo`) é lido pelo leitor determinístico no job de extração, sem o extrator.
  - Banco, agência e conta vêm da triagem (ou da revisão humana da TASK-021); o período, da triagem ou da competência do envio. Faltando algum: `metadados_ausentes`.
  - A origem só usava o CSV no script do cenário (`carregar_horizonte.py`, TASK-027).
  - O `metadados_txt` foi portado com golden; a origem não o usava em nenhum fluxo.
- **Playbooks (REQ-022, CC-09, AC-017).** O playbook válido do banco vai na 1ª tentativa: as instruções no prompt de sistema e o playbook como dado na mensagem.
  - Falhou na 1ª tentativa, ele é invalidado e as tentativas seguintes redescobrem. A versão nova só nasce com o portão aprovado; o playbook usado com sucesso ganha um sucesso.
  - Leitura determinística aprovada também cria ou conta playbook, como na origem.
  - A origem anexava ao prompt de sistema só a linha `PLAYBOOK: banco=… pagina=… decimal=… sinal=…`; aqui vão todos os campos do schema congelado, como dado e não como instrução.
- **Prompts** em `internal/ai/prompts/conciliacao/` (triador, extrator, correção e o bloco do playbook): os textos da origem com os itens do prompt-spec §2 e a entrada que o sistema de fato envia. A versão e o hash ficam fixados em teste.
- **Auditoria (CC-10).** O log de extração grava cada tentativa: alias, modelo real, hash e versão do prompt, saída bruta, validador, tokens, custo e latência. Grava também a leitura determinística (`leitor_local`, tentativa 0) e o reprocessamento (`humano`, com o autor).
  - Triador e extrator têm corrida no tracer (D-022) com a decisão. O documento aponta a corrida do triador; a importação, a do extrator (migration `0026`).
  - Agência e conta não entram na decisão, que vai ao Langfuse.
- **API.** `POST /reconciliation/documents/{id}/retry` (06 §4) reprocessa:
  - a revisão de triagem e a pendência de período fechado, que voltam à triagem;
  - a revisão de extração, que volta à extração;
  - o documento com o job encerrado sem sucesso, que repete o mesmo job.
  - Ele não ignora portão nenhum. Recusa com 409 se o documento já está importado ou na fila, e com 423 se o Contábil está fechado.
  - O detalhe passa a trazer a importação, as tentativas e a corrida do triador.
- **Relatório do golden (CC-25, eval-plan §3).** O golden é o cenário da origem (2 extratos, com o gabarito tirado do CSV pelo `extrato_de_csv`).
  - O relatório pula os leitores determinísticos e traz aprovação na 1ª tentativa, tentativas, custo, latência e as diferenças para o gabarito.
  - `make test` roda o relatório com o gateway simulado. `make eval-extrator` roda com o LiteLLM do ambiente, com custo de provedor real: **não rodado**, depende do seu aceite.
- **Configuração.** `LIMIAR_TRIAGEM` no worker. No compose, o worker recebe a chave do MinIO da aplicação (teste: a de administrador continua fora).
- **Fica para depois:**
  - TASK-017: extração de comprovante, o leitor determinístico de comprovante e a releitura;
  - TASK-021: telas da revisão ("que documento é esse?"), dos playbooks e das métricas;
  - TASK-027: cenário Horizonte no seed-demo.

### D-026 — Ingestão manual da Conciliação com original no MinIO (TASK-015; 09/10/2026, dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Porte** de `app/servicos/ingestao.py` e `app/storage.py` do Conciliador (commit `2bc6e26`), em `backend/internal/reconciliation/documents`.
- **Endpoints** nos caminhos do 06 §4, no lugar do `POST /ingestao/arquivo` da origem:
  - `POST /reconciliation/documents`: multipart com o arquivo no campo `file` e, opcionais, `company_id` e `competence`. Responde 202 `{document_id, job_id, status}`.
  - `GET /reconciliation/documents`: filtros de status, empresa, tipo, banco e competência, com a contagem por status.
  - `GET /reconciliation/documents/{id}`: hash, versão do original, triagem, diagnóstico e o estado do job [AC-014].
  - `GET /reconciliation/documents/{id}/file`: a versão gravada do original, para ver (inline, como a rota `/documentos/{id}/pdf` da origem) ou baixar (`?download=1`), com `nosniff` e sem cache.
  - Um arquivo por envio; a origem aceitava vários e redirecionava para a lista HTML. A interface (TASK-021) envia um a um.
- **Original imutável e versionado [REQ-043, 09 §5].**
  - A chave no bucket é só o SHA-256 (`originais/<hash>`). A origem usava `originais/<hash>/<nome enviado>`, e o 09 §5 proíbe o nome do usuário na chave.
  - A API grava com o usuário da aplicação, que a política do `docker/minio/init.sh` impede de apagar objeto ou versão. O documento guarda a versão do objeto (`object_version_id`), e a leitura usa essa versão.
  - Cliente S3 próprio, só com a biblioteca padrão: AWS Signature V4, conferida contra os exemplos da documentação da AWS e contra o MinIO real. Não tem operação de apagar, como o `Storage` da origem. Sem dependência nova.
- **Dedupe pelo SHA-256, como na origem:** o mesmo arquivo é recusado com 409 `DUPLICATE_DOCUMENT`, com o documento existente em `details.document_id`. Não grava documento, job nem versão nova.
  - O estado `duplicado` continua no contrato (D-020), mas a origem também nunca o gravava: o arquivo repetido não vira documento.
- **Tipo pelo conteúdo, até 20 MB.**
  - Aceita PDF, imagem PNG ou JPEG (foto ou print de comprovante) e texto (CSV/TXT de extrato), com o charset detectado (UTF-8 ou windows-1252).
  - A origem aceitava qualquer arquivo e não tinha limite. Aqui o resto dá 415 e o excesso dá 413.
- **Fila [REQ-046].**
  - O documento nasce `pendente`, com o job `reconciliation_triage` na mesma transação e a coluna `job_id` (migration `0025`).
  - O processamento (triagem e extração) chega com a TASK-016. Até lá, o job espera na fila sem gastar tentativa, porque o worker só adquire os tipos que sabe executar (D-019).
  - O atalho da origem que processava na hora o PDF com texto (`processar_local`) passa para o job, na TASK-016: a API não espera o processamento [AC-014].
- **Documento tardio (F-009).** Com `company_id` e `competence` no envio e o Contábil fechado nessa competência, o original é guardado e o documento fica como `pendencia_periodo_fechado`, sem job, como no `ingerir_arquivo` da origem. Não é recusado com 423, porque guardar o documento não é mutação da conciliação (AC-025).
- **MinIO fora ou não configurado:** 503 `STORAGE_UNAVAILABLE`, sem gravar documento nem job e sem guardar o original em outro lugar (11 §9). Configuração em `S3_ENDPOINT`, `S3_REGION`, `S3_ACCESS_KEY`, `S3_SECRET_KEY` e `S3_BUCKET`. No compose, só a API e o `minio-init` recebem a chave da aplicação, e a de administrador nunca chega à API (teste).
- **n8n [REQ-045].**
  - O workflow `n8n/workflows/ingestao.json` foi refeito: observa `/data/inbox` (pasta `n8n/inbox`), lê o arquivo e envia no campo `file` para `$env.CONTROLE_API_URL/api/v1/reconciliation/documents`. A origem apontava para `http://app:8000/ingestao/arquivo` sem campo de arquivo e com login, e não funcionava.
  - O n8n 2.x desliga por padrão o gatilho de pasta: o compose religa só esse nó (`NODES_EXCLUDE` mantém o de executar comando desligado) e libera `$env` para o endereço da API.
  - O gatilho usa polling, porque a pasta montada do Windows não avisa mudanças ao container, e espera o arquivo terminar de ser gravado.
  - Provado num n8n descartável da mesma imagem: o arquivo solto na pasta chegou ao endpoint no campo `file`, com o nome original.
  - `make n8n-import` importa e ativa o workflow no n8n do ambiente. Não foi rodado: depende do seu aceite.
- **Fica para a TASK-016:** `POST /reconciliation/documents/{id}/retry` (06 §4). Ele só faz sentido com o processamento, que define quais estados de revisão aceitam novo processamento. Hoje, o job encerrado com falha volta pela `POST /jobs/{id}/retry`. *(Feito na TASK-016, D-027.)*
- **Ponto para a TASK-021:** a API responde com `X-Frame-Options: DENY`. Se a fila de revisão embutir o original numa moldura, essa rota vai precisar de `SAMEORIGIN`.

### D-025 — Telas do inbox PGDAS-D, do simulador, dos agentes e da observabilidade (TASK-014b; 09/10/2026, dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Porte** das telas `inbox`, `inbox/[id]`, `agentes`, `observabilidade` e `observabilidade/traces/[id]` e dos componentes `Simulador`, `Chat` e `CadastroPeloExtrato` do Fator R (commit `9dd7a74`), em `frontend/src/features/factor-r`.
  - A tela só formata o que a API devolve: decimal por string, half-even, sem float.
- **Navegação.** A área Fator R ganhou um submenu com Carteira, Inbox PGDAS-D, Agentes e Observabilidade, o mesmo menu da origem.
  - Rotas: `/factor-r`, `/factor-r/inbox`, `/factor-r/inbox/:id`, `/factor-r/agents`, `/factor-r/runs` e `/factor-r/runs/:traceId`.
  - A observabilidade fica no Fator R porque o 11 §6 trata essas métricas como do Fator R. As da Conciliação (11 §7) vêm com a TASK-021; o detalhe da corrida é genérico e pode ser reaproveitado lá.
  - O submenu leva o PA escolhido entre a Carteira e os Agentes.
- **Inbox.**
  - Envio por arrastar ou escolher arquivos, vários de uma vez. O envio volta na hora (202) e a tela acompanha cada documento até o worker terminar, consultando a cada 1,5 s só enquanto há documento em processamento. Na origem, o envio esperava a leitura.
  - Estados com ícone e texto (08 §6): na fila, processando, nova tentativa, falha no processamento, extraído, em revisão, vinculado e rejeitado.
  - Fila parada (11 §9): documento na fila sem worker ativo mostra o aviso (`worker_online` de `GET /jobs/{id}`). Job com falha mostra o motivo e pode voltar à fila (`POST /jobs/{id}/retry`), ou o documento pode ser rejeitado.
  - O detalhe mostra também os meses anteriores declarados e se conferem com o RBT12 e a FS12; a origem não mostrava.
  - Vínculo manual com busca de empresa ativa pelo nome, código ou CNPJ (a origem listava todas as empresas num select).
  - **Cadastro pelo extrato no formulário canônico da base**, com o CNPJ travado e os dados lidos já preenchidos: razão social, regime Simples Nacional, Fator R e início da atividade.
    - O analista completa o código interno e o início do atendimento, que a base exige; na origem eram 2 cliques.
    - CNPJ já cadastrado oferece vincular o extrato à empresa existente, como na origem.
- **Ficha → aba Fator R** (08 §6): simulador com histórico, consultor da empresa, extratos vinculados e corridas da empresa.
  - Simulador e consultor só aparecem para empresa sujeita ao Fator R, como na origem.
  - Na grade, o mês lançado pelo extrato leva ao documento.
  - API: `GET /factor-r/pgdas` ganhou o filtro `company_id`.
- **Simulador.** Os percentuais digitados viram fração por string, sem float (origem: `percentualParaTaxa`). Meta vazia é a da política; o aviso "INSS sem teto" continua. O erro de campo da API aparece no próprio campo.
- **Observabilidade.**
  - Painel: corridas em 24 h e no total, sem nota humana, acerto ouro e humano, custo e tokens do LLM em 30 dias e a tabela por agente. O "Custo do Claude" da origem virou "Custo do LLM".
  - Corridas filtráveis por agente, resultado, sem nota humana e empresa (o filtro de empresa vem da ficha), de 50 em 50.
  - Detalhe com metadados (modelo, alias, versão da configuração, tokens e custo, prompts versionados, erro e Langfuse), os passos com as chamadas ao LLM e o detalhe de cada passo, entrada, saída, decisão, nota ouro e nota humana. Na origem os spans vinham do Langfuse; aqui vêm da trilha local (D-022).
  - Nota humana: erro exige comentário na tela, na API e no banco.
- **E2E (TEST-014).**
  - A suíte sobe também o worker, para o inbox.
  - O gateway LiteLLM simulado passou a responder `/v1/chat/completions` de forma determinística: a intenção por palavras-chave e um texto sem números, que passa pelo validador. Assim o caminho com LLM roda de ponta a ponta sem provedor real (FR-20; origem: `LLM_PROVIDER=fake`).
  - A empresa cadastrada pelo extrato no teste é inativada no fim, para a carteira da demonstração não mudar.
  - A varredura de acessibilidade (axe) passou a cobrir o inbox, o documento, o diálogo de cadastro pelo extrato, os agentes, a observabilidade, o detalhe da corrida e o resultado do simulador. Os blocos JSON do detalhe, que rolam, recebem foco pelo teclado.
- **Contrato.** `input`, `output` e `decision` da corrida e `data` da decisão passaram a `additionalProperties: true` no OpenAPI. São objetos livres, e o tipo gerado para o frontend era um objeto vazio.
- **Correção.** Na aba Fator R, a grade mensal (TASK-014a) alargava a coluna da página e criava rolagem horizontal; agora a coluna encolhe e a grade rola dentro do cartão.
- **Fica para as TASKs do plano:** Dashboard com o Fator R (AC-012, TASK-024), ficha como hub (AC-034, TASK-025) e Design System InovaSala aplicado (AC-035, TASK-029).

### D-024 — Simulador persistido e agentes consultor e priorizador (TASK-013 e TASK-006b; 08/10/2026, dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Porte** de `services/simulacao.py`, `agents/{consultor,priorizador,etapas,intencao,guardas,render,llm}.py` e `api/{simulations,agents}.py` do Fator R (commit `9dd7a74`), no pacote `backend/internal/factorr/agents`.
  - Todo número vem do motor e do simulador portados na TASK-010; o LLM só classifica a intenção (span `plan`) e redige o texto (span `render`) [AC-010].
- **Simulador persistido.**
  - Tabela `factor_r_simulations` (migration `0024`), a parte que faltava da TASK-006b. Guarda parâmetros e resultado exatos em JSON, mais colunas tipadas dos campos do 05 §4.
  - `trace_id` obrigatório: toda simulação nasce numa corrida do consultor e é ela que autoriza um "corrigir".
  - `POST /companies/{id}/factor-r/simulations` roda numa corrida `consultor`, gatilho `ficha`, sem LLM. Valem os padrões da origem: meta da política, 100% do reforço em pró-labore, INSS 11%, IRRF 27,5% e 12 meses.
  - A resposta arredonda só na exibição (centavos e 6 casas), como `para_exibicao`.
  - O histórico (`GET`) traz o texto da corrida; na origem, vinha vazio.
- **Agentes** em `POST /factor-r/agents/consult` e `/prioritize`, nos caminhos da spec (a origem usava `/agents/consultor/chat` e `/agents/priorizador/chat`).
  - Resposta: `texto`, `decisao` e `trace_id`.
  - Mantido da origem:
    - a empresa citada é resolvida localmente, nunca pelo LLM; o `company_ref` do LLM é buscado como na origem, por `ILIKE` no nome ou parte do CNPJ;
    - a intenção `priorizar` dentro do consultor monta a fila na própria corrida;
    - empresa não identificada deixa a corrida em revisão;
    - os textos de template;
    - os tipos de decisão `consultor_resposta`, `simulacao` e `fila_priorizacao`.
  - A busca local compara com a razão social e também com o nome fantasia; a origem só tinha "nome".
- **LLM pelo LiteLLM**, com os aliases `consultor` e `priorizador` (OpenAI no início). Na origem era o SDK da Anthropic.
  - O plan pede saída estruturada (`json_schema` estrito), no lugar do tool use da origem. A saída passa pelas mesmas regras do `Intencao` da origem:
    - intenção conhecida;
    - `pa` no padrão `AAAA-MM`, que vira nulo fora de 2000-01 a 2100-12;
    - `meta` de 0,28 a menos de 1;
    - o que falha cai no fallback.
  - O `make test-litellm` passou a provar que o LiteLLM real repassa o `json_schema` ao provedor.
  - Limites: 512 tokens para classificar e 1024 para redigir. A configuração do alias na tela de IA (`max_tokens`, `temperature`) vence esses padrões.
  - Cada chamada vira uma geração no trace, com tokens e custo. O modelo resolvido é o snapshot da corrida.
  - Sem LLM, o agente segue sem chamar o gateway, com intenção por palavras-chave e texto pelo template (o modo determinístico da origem). Isso vale para:
    - gateway não configurado;
    - alias desligado ou sem modelo;
    - erro, saída inválida ou recusa.
  - A corrida registra o motivo em `metadata.llm_indisponivel` e no span.
- **Guardas.**
  - O texto do LLM é validado contra a decisão, porte de `validar_texto`. Reprova o texto que traz:
    - número ou data que não está na decisão;
    - número por extenso;
    - recomendação que contradiz o veredito.
  - Texto reprovado vira o template, e o span `render` registra os órfãos.
  - "Corrigir" só com uma simulação gravada com esse veredito: 409 `RECOMMENDATION_WITHOUT_SIMULATION`, com corrida de erro.
  - Toda resposta termina com o disclaimer do PGDAS-D.
- **Paridade provada por golden gerado pelo próprio código da origem.**
  - Validador do texto e formatadores (`render.py` e `money.py`): 40 casos.
  - Classificador por palavras-chave e empresa citada (`intencao.py`).
  - Validação da saída do LLM pelo `Intencao` da origem, com pydantic 2.13.5, a versão do `uv.lock`.
  - Para isso, o `pytext.Compile` passou a aceitar `\w`, lookbehind de um caractere e o `$` do Python. O pacote `decimal` ganhou `ParsePython` (o `Decimal(str)` do Python), `Normalize`, `Abs` e `Key`.
  - Diferenças onde a origem levantava exceção e derrubava a corrida:
    - valor `NaN` ou `Infinity` numa decisão;
    - número que não cabe nos 28 dígitos ao arredondar.
    - Aqui o valor só não entra nos números aceitos.
- **Prompts versionados** em `backend/internal/ai/prompts/fatorr/{plan,render}.md` [prompt-spec §1].
  - Versão 2026.10.1, a partir da 2026.09.1 da origem, com os itens do prompt-spec §2 que faltavam: formato de saída, ferramentas, o que não decide, abstenção, instrução contra injeção, extraído × inferência.
  - Um teste fixa o hash de cada versão. A corrida grava nome, versão e hash dos dois prompts (`prompt_version` e `metadata.prompts`).
- **Rotina semanal do priorizador.**
  - Tarefa do worker, conferida a cada 15 minutos. Aos domingos, a partir de `ROTINA_PRIORIZADOR_HORA` (padrão 7, como na origem), registra a fila uma vez por semana, sem LLM (gatilho `rotina`).
  - A semana conta de segunda 00:00 no fuso `APP_TIMEZONE`; na origem era UTC.
  - Se o worker estiver parado na hora, a rotina roda quando ele voltar, ainda no domingo. Na origem, o cron perdia a semana.
- **Respostas de erro.** Validação de campo segue a base (400 `VALIDATION_ERROR`; a origem dava o 422 do FastAPI). Sem tabela do Simples vigente: 422 `SIMPLES_TABLE_NOT_IN_FORCE`, como na origem.

### D-023 — Inbox PGDAS-D e agente parser_pgdas (TASK-012 e TASK-006b; 08/10/2026, dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Porte do Inbox do Fator R** (commit `9dd7a74`): `parsing/{texto,isolado,pgdas}.py`, `agents/parser_pgdas.py`, `api/inbox.py`, `core/uploads.py` e `services/ouro_pgdas.py`.
  - Pacotes:
    - `internal/factorr/pgdas`: o parser, função pura sobre o texto;
    - `internal/platform/pdftext`: o texto do PDF e do TXT;
    - `internal/factorr/inbox`: upload, job, agente e decisões do analista;
    - `internal/platform/pytext`: as regex do Python.
  - O que se manteve da origem:
    - `PARSER_VERSION` 2026.09.3, pesos dos campos, bônus e penalidade de consistência;
    - limiar de confiança e tolerância da nota ouro, lidos da política (`factor_r_policy`);
    - motivos de revisão e sugestão de cadastro;
    - textos do agente, sempre com o disclaimer do PGDAS-D;
    - gatilhos `upload`, `vinculo_manual`, `rejeicao` e `cadastro_pelo_extrato`, e os tipos de decisão.
  - Regras de gravação mantidas [AC-008, AC-009]:
    - grava só em competência vazia e nunca sobrescreve;
    - os meses anteriores ao PA só entram com a série conferida contra o RBT12 e a FS12 declarados;
    - a folha entra só como o total declarado e conferido, sem inventar divisão;
    - atividade sem fator r entra com folha zero;
    - CNPJ fora da carteira ou confiança abaixo do limiar vai para revisão, sem escrita.
- **Paridade provada por golden.**
  - Parser: golden gerado pelo próprio `parse` da origem (`backend/internal/factorr/pgdas/testdata/golden_gen.py`) nas 14 fixtures e em 109 variações de borda (Unicode, IGNORECASE, lookahead, séries mensais). Os 122 casos dão o mesmo resultado.
  - Acerto nas fixtures: 108 de 112 campos (96%; meta da origem: 80%). Os 4 erros são as lacunas conhecidas do DAS no layout declaratório (T-504 da origem).
  - Texto do PDF: byte a byte igual ao do pdfplumber 0.11.10, no Windows e no Linux (`backend/internal/platform/pdftext/testdata/text_gen.py`).
- **Regex do Python no Go (`pytext.Compile`).**
  - O regexp do Go trata `\s`, `\d`, `\b` e o IGNORECASE só em ASCII e não tem lookahead.
  - A tradução usa as classes Unicode do Python e iguala i, I, ı e İ no IGNORECASE. O `\b` e o `(?!…)` viram marcas conferidas depois de cada correspondência candidata.
  - O único padrão que exigiria backtracking com lookahead, `(I{1,3}|IV|V)(?![IVX])`, é lido por código.
- **Extração do PDF: dependência nova.**
  - `github.com/klippa-app/go-pdfium` v1.21.1 (MIT) com `github.com/tetratelabs/wazero` v1.12.0 (Apache-2.0): o PDFium compilado para WebAssembly, sem cgo. O binário cresce cerca de 6 MB.
  - O texto é montado pelo mesmo algoritmo do `extract_text()` do pdfplumber (palavras e linhas agrupadas com tolerância de 3 pt).
  - Alternativas descartadas:
    - `ledongthuc/pdf`: não tem as métricas das fontes base-14, e o texto sai inutilizável;
    - chamar o Python: fora da stack.
  - Limites da origem mantidos:
    - 30 s por extração, com interrupção do WebAssembly;
    - 1 GiB de memória;
    - 2 extrações ao mesmo tempo;
    - 2 milhões de caracteres.
  - O PDFium roda sem acesso ao disco (o padrão do go-pdfium monta o disco inteiro na sandbox).
  - O `MAX_PAGINAS = 20` da origem não era aplicado; aqui também não.
  - Diferenças:
    - quando o pdfminer falhava, a origem caía no pypdf; aqui o PDFium lê direto, e PDF que ele não abre continua `pdf_ilegivel`;
    - caractere sem Unicode no PDF: o pdfminer escrevia `(cid:N)`, o PDFium o omite;
    - exceção do parser (ex.: PA com ano anterior a 1900 e tabelas mensais) vira `extracao_excedeu_limites`, como acontecia com o processo isolado da origem;
    - PA inválido na hora do vínculo (ano anterior a 1900): falha permanente do job e 422 `PGDAS_PA_INVALIDO` na API, sem gravar nada; a origem levantava exceção (500).
- **Upload assíncrono [REQ-046, AC-014].**
  - `POST /factor-r/pgdas` guarda o original e enfileira o job `pgdas_parse` (202 com `document_id`, `job_id` e `status`). Na origem o upload processava na hora (201).
  - O worker faz a corrida `upload` na transação do job. Falha permanente vai para `dead` com código estável (`PGDAS_FILE_MISSING`, `PGDAS_DOCUMENT_NOT_FOUND`, `INVALID_PAYLOAD`, `PGDAS_PA_INVALIDO`), e o documento mostra o estado do job.
  - O mesmo arquivo enviado de novo devolve o documento existente (200, `duplicate = true`), sem novo job.
  - Armazenamento:
    - o arquivo fica em `FILE_STORAGE_DIR/pgdas/<sha256>` (origem: `upload_dir/<firm_id>/<sha256>`), com permissão 0600;
    - a leitura passa por `os.Root`;
    - o nome enviado é só metadado (sem caminho nem caracteres de controle);
    - a API e o worker já compartilham `./storage` no compose.
  - Respostas e download:
    - 413 `FILE_TOO_LARGE` e 415 `UNSUPPORTED_FILE`, como na origem;
    - o download sai como anexo, com `nosniff` e sem cache.
- **Tabela `factor_r_pgdas_documents` (TASK-006b) [05 §4].**
  - Colunas tipadas só para o que se consulta (`cnpj_extraido`, `pa`, `confidence`). O resultado completo do parser fica em `parsed_json`, o `campos_json` da origem.
  - Texto extraído em `extracted_text`: fica no banco local e nunca vai ao Langfuse.
  - Também guarda `job_id` e `trace_id`.
  - CHECKs:
    - empresa só no documento vinculado;
    - motivo só em revisão e rejeição;
    - depois do upload, toda decisão tem trace [AC-029].
  - A FK `factor_r_movements.pgdas_document_id` chega aqui.
  - `factor_r_simulations` fica para a TASK-013, junto com o simulador.
- **Folha nos movimentos.** O total declarado vai em `folha_total_declarada_pgdas` (D-018), com a divisão zerada. O mês do PA e os meses de atividade sem fator r ficam com essa coluna nula, e a folha do mês fica zero, como o `salarios = 0` da origem.
- **Rotas [06 §3].**
  - Listagem com contagem por status.
  - Detalhe com campos, identificação, séries, sugestão de cadastro, job e texto da última corrida.
  - Download do original.
  - `POST …/link`.
  - `POST …/review`: é o `/reject` da origem, com `decisao = rejeitar`.
  - `POST …/create-company`:
    - usa o cadastro canônico (`company.Service`, as mesmas validações de `POST /companies`);
    - o início de atividade vai para a configuração do Fator R;
    - CNPJ já cadastrado responde com o `CNPJ_CONFLICT` da base, com a empresa em `details.conflicting_company`, no lugar do `cnpj_ja_cadastrado` da origem.
  - A validação de campo segue a base: 400 `VALIDATION_ERROR`, onde a origem dava o 422 do FastAPI.
- **Vínculo de domínio da corrida [AC-029].** Com o vínculo, `ai_runs.company_id` passa a apontar a empresa. No cadastro pelo extrato, isso é feito por `Run.LinkCompany`, o `run.trace.company_id` da origem.
- **Teste.** A infraestrutura de teste agora repete a subida do Postgres quando o Docker Desktop não responde à detecção do provedor. É uma falha transitória com dezenas de pacotes subindo containers ao mesmo tempo, a mesma raiz do BL-005.

### D-022 — Auditoria local de IA e sincronização com o Langfuse (TASK-009; 08/10/2026, dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Porte do tracing do Fator R** (`tracing/{tracer,client,sync,scores}.py`, commit `9dd7a74`), com os nomes do 05 §8.
  - Tabelas:
    - `ai_runs`, que corresponde a `agent_traces`;
    - `ai_decisions` (`agent_decisions`);
    - `ai_gold_evals` (`evals_gold`);
    - `ai_human_evals` (`evals_human`).
  - Vocabulários da origem:
    - status `ok`, `error` e `needs_review`;
    - nota humana `acerto`, `parcial` e `erro`;
    - nota ouro `ok`, `erro` e `ouro_indisponivel`.
  - Contrato do Langfuse da origem:
    - o trace leva o nome do agente;
    - spans só `plan`, `parse`, `tool`, `decide` e `render`, mais as gerações do LLM;
    - scores `gold_<campo>` (BOOLEAN), `gold_acerto` (NUMERIC) e `human_eval` (CATEGORICAL).
- **Semântica preservada:**
  - a corrida nasce na transação do agente;
  - decisão de agente só se grava por `RecordDecision`, e o banco exige `trace_id`;
  - um erro desfaz as decisões e grava a corrida de erro, com o código e nunca a mensagem;
  - a corrida que termina sem `Finish` fica como erro `NOT_FINISHED`;
  - a nota "erro" exige comentário, no código e no banco.
- **Diferenças da origem:**
  - **Envio sem SDK:** OTLP/HTTP em JSON e a API de scores. O Langfuse 4.53 aceita o formato (confirmado por sonda e pelo `make test-langfuse`), sem dependência nova.
  - **Spans guardados na trilha local:** o detalhe da corrida não depende do Langfuse. Na origem, os spans vinham da API do Langfuse e ficavam "indisponíveis" com ele fora. O reenvio manda o trace completo; na origem, só um resumo de metadados.
  - **Fila de jobs como outbox:**
    - o job `langfuse_sync` entra na fila na mesma transação da corrida;
    - se o Langfuse falha, o status fica `failed`, com o erro sanitizado;
    - o worker reenvia periodicamente: `LANGFUSE_RETRY_SECONDS`, padrão 5 minutos, 50 por vez, como a origem.
  - **IDs fixos:**
    - o span raiz é derivado do trace;
    - cada score usa o ID da própria nota;
    - o `gold_acerto` usa um UUID derivado do trace.
    - O reenvio não cria observação nem score novos (provado no Langfuse real).
    - O Langfuse v4 grava cada envio e consolida depois as versões da mesma observação (ClickHouse). Até isso acontecer, a API pública pode listar a mesma observação duas vezes, com o mesmo ID. Na origem, cada reenvio criava um span raiz novo.
  - **Notas com status de sincronização próprio:** na origem, o `create_score` era "dispara e esquece".
  - **Comentário da nota humana:** vai ao Langfuse com CNPJ e CPF mascarados; na origem ia cru.
  - **Painel:** custo e tokens vêm da trilha local (informados pelo gateway), e não da API de métricas do Langfuse.
  - **Sem multiescritório:** as etiquetas são agente, gatilho e módulo; não há `firm:<id>`.
  - **Gancho genérico na fila (`jobs.AfterRollback`):** num job, a corrida de erro é gravada depois do rollback da tentativa, junto com a falha do job.
- **Mascaramento.**
  - Porte de `mascarar_cnpj`, `_limpar_payload` e `mascarar_atributo`, com as classes Unicode do Python (`\d`, `\s` e `\b`; pacote `pytext`, que também passou a servir a chave de dedupe da Conciliação).
  - Provado por golden gerado com o próprio código da origem (`testdata/mask_gen.py`).
- **Rede (ponto aberto da D-013).**
  - Rede externa `controle-obs`, criada pelo Makefile (`make obs-net`, pré-requisito de `dev` e `langfuse-up`).
  - Ficam nela o worker e o `langfuse-web` (apelido `langfuse`), com `HOSTNAME=0.0.0.0` no `langfuse-web`, como no compose oficial; sem isso, o Next.js só atende na rede padrão dele.
  - O Langfuse continua publicado só em `127.0.0.1`. Funciona também no Linux.
- **Segredos.** `LANGFUSE_SECRET_KEY` vai só para o worker (um teste confere no compose). A API recebe apenas `LANGFUSE_PUBLIC_URL` e `LANGFUSE_PROJECT_ID`, para montar o link do trace.
- **Tela.** Traces e avaliações entram na TASK-014b (plano). Aqui fica a API do 06 §9, mais `GET /ai/summary` com as métricas do painel (FR-18).

### D-021 — Configuração dos modelos de IA (TASK-008; 08/10/2026, dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Tabela `ai_model_aliases` [05 §8]**, criada na migration `0021_ai_model_aliases.sql`.
  - Uma linha por alias do `ai.Catalog`, criada pela própria migration (versão 1). Um teste confere que tabela e catálogo não divergem.
  - `provider` e `model` nulos significam "vale o padrão do `.env`" (`<ALIAS>_MODEL`); a escolha da tela vence o padrão.
    - O `.env` continua sendo o padrão da instalação (D-014, D-015).
    - A tela mostra de onde vem cada modelo: `settings` (escolhido na tela), `env` (padrão do ambiente) ou `none` (sem modelo).
  - Parâmetros: lista fechada com `temperature` (0 a 2) e `max_tokens` (1 a 1.000.000).
    - A lista é conferida no código e por CHECK no banco.
    - Nenhum outro parâmetro entra, inclusive `api_key`, nem por SQL direto.
  - Nome de modelo com cara de chave (`sk-…`, `pk-…`, `rk-…`) é recusado no código e no banco, sem ecoar o valor.
  - Histórico em `ai_model_alias_history`: uma linha por versão (o quê, quem e quando), gravada na mesma transação, como os demais históricos do Controle. Ainda sem tela.
- **API [06 §7].**
  - `GET /settings/ai/models` traz:
    - os aliases na ordem do catálogo;
    - a escolha da tela, o padrão do ambiente e o modelo das novas corridas, com a origem;
    - os parâmetros, ligado/desligado, a versão e a última alteração;
    - `gateway_configured`.
  - `PUT /settings/ai/models/{alias}` substitui a configuração, com `version` (409 `VERSION_CONFLICT`).
    - Mudança no modelo efetivo ou nos parâmetros é conferida no gateway antes de gravar. Reprovada: 422 `AI_MODEL_INVALID`, com as conferências em `details.checks`. Gateway fora do ar ou não configurado: 503 (`AI_GATEWAY_UNAVAILABLE`, `AI_GATEWAY_NOT_CONFIGURED`).
    - Ligar e desligar não dependem do gateway.
    - Campo fora do contrato, como `api_key`, é recusado com 400.
  - `POST /settings/ai/models/{alias}/validate` confere sem gravar.
- **Validação no gateway (regra do D-014).**
  - Consulta `GET /utils/model_info` do LiteLLM, que lê só a tabela de modelos do gateway. Não chama o provedor e não tem custo, então não há "chamada real" a sinalizar [06 §7].
  - Conferências:
    - o gateway conhece o modelo;
    - é um modelo de chat;
    - aceita saída estruturada (todos os agentes);
    - aceita imagens, para triador, extrator e extrator_forte, que leem a página renderizada quando o PDF não tem texto;
    - aceita os parâmetros escolhidos, e `max_tokens` cabe no limite do modelo.
  - Reprovação bloqueia a gravação. Com `drop_params`, o gateway descartaria em silêncio o JSON Schema ou o parâmetro não aceito.
  - **Limitação conhecida:** um modelo que a versão fixada do LiteLLM não conhece (ex.: `gpt-6.1-sol`) só pode ser escolhido depois de atualizar o LiteLLM. É o mesmo motivo de travar a tabela de modelos (D-014).
- **Resolução [AC-027, F-010].**
  - `aliases.Resolver.Resolve(ctx, db, alias)` aplica a configuração gravada sobre o `.env` e devolve o snapshot `ai.Resolved`: modelo, origem, parâmetros e versão.
  - As corridas dos agentes (TASK-012 em diante) resolvem por aqui, no início. A corrida em andamento mantém o snapshot.
  - Alias desligado não resolve (`ErrAliasDisabled`).
- **Chaves [REQ-047].**
  - A tela não tem campo de credencial.
  - A validação usa só a chave do gateway (`LITELLM_API_KEY`).
  - Os erros do gateway chegam sanitizados.
  - TEST-008 confere todas as respostas da tela.
- **E2E.** A suíte sobe um gateway LiteLLM simulado (`e2e/support/fake-litellm.mjs`), com a mesma rota e os mesmos corpos do LiteLLM v1.104.0. O contrato do gateway real fica com `make test-litellm`.

### D-020 — Schema da Conciliação (TASK-007; 08/10/2026, dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Porte do schema do Conciliador** (`app/db/models.py`, commit `2bc6e26`) para a migration `0020_reconciliation.sql`.
  - Tabelas e colunas em inglês, na convenção do projeto-base. O 05 §5 nomeia as tabelas em inglês e descreve os campos sem fixar nomes.
  - Os vocabulários da origem ficam como estavam:
    - estados do documento (contrato congelado): `pendente`, `triado`, `extraido`, `importado`, `revisao_triagem`, `revisao_extracao`, `duplicado` e `pendencia_periodo_fechado`;
    - sentido: `C` e `D`;
    - métodos: `passe_1` a `passe_4`, `manual`, `sem_comprovante` e `resolvedor`;
    - origem da conciliação: `auto` e `manual`;
    - origem da classificação: `regra`, `historico`, `agente` e `manual`;
    - tipos de regra: `cnpj`, `contraparte` e `regex_descritivo`;
    - estados da proposta: `pendente`, `aceita` e `rejeitada`;
    - estados do lançamento: `pendente`, `parcial`, `extraido` e `manual`;
    - tipos de documento: `extrato`, `comprovante` e `desconhecido`.

  | Origem | Consolidado |
  |---|---|
  | `cliente` | `companies` (canônica, REQ-002) |
  | `usuario` | `users` |
  | `conta_bancaria` | `bank_accounts` |
  | `documento_origem` | `reconciliation_documents` |
  | `extrato_importacao` | `statement_imports` |
  | `extrato_transacao` | `statement_transactions` |
  | `lancamento_cliente` | `client_entries` |
  | `conciliacao` | `reconciliations` + `reconciliation_items` |
  | `plano_contas` | `chart_accounts` |
  | `regra_classificacao` | `classification_rules` |
  | `playbook_banco` | `bank_playbooks` (schema lógico congelado mantido) |
  | `extracao_log` | `extraction_logs` |
  | `proposta_resolvedor` | `resolver_proposals` |
  | `periodo_fechamento` | **não existe**: a trava é o fechamento do Contábil em `company_module_periods` (REQ-033, 05 §6) |
  | `job` | `jobs` (TASK-005) |
  | `cnpj_cache` | adiado: a consulta de CNPJ (CC-15) é decidida na TASK-017 |

- **Empresa em tudo.**
  - Toda entidade tem `company_id`. Só o documento pode ficar sem empresa durante a triagem; sem empresa definida, ele não é importado e fica em revisão.
  - Transação, lançamento, conciliação, item e proposta usam FKs compostas com `company_id`, então um vínculo nunca cruza empresas.
  - Na origem, a transação não tinha cliente, e o lançamento feito à mão ou por CSV podia ficar sem cliente.
- **O portão também no banco.**
  - `statement_imports.validation_ok` tem `CHECK (validation_ok)`: importação reprovada não existe (AC-015).
  - `attempts` vai de 0 a 3; 0 é a importação determinística, como na origem.
- **Dedupe.**
  - A chave segue a origem: sha256 de `conta|data|valor|sentido|descrição normalizada|ordem no dia`, única no sistema.
  - Foi portada para Go (`reconciliation.DedupeKey`). Um golden gerado pelo código Python da origem prova igualdade caractere a caractere em 45 casos difíceis de Unicode.
- **Conciliação N:1 com integridade.**
  - Os vínculos ficam em `reconciliation_items`, e não em arrays JSON (05 §5).
  - Há uma conciliação ativa por transação e um lançamento em no máximo uma conciliação ativa, ambos garantidos por índices únicos parciais.
  - "Sem comprovante" é uma conciliação sem itens.
  - Origem `auto` exige passe e não tem autor; origem `manual` exige autor.
  - Desfazer não apaga, ao contrário da origem: marca `undone_at` e o autor e libera os itens. A auditoria se mantém, e o razão é projeção das conciliações ativas.
- **Outras unicidades.**
  - Conta bancária única no sistema. Na origem a unicidade era por cliente, mas a importação buscava a conta só por banco, agência e conta, o que ficava ambíguo.
  - No máximo um playbook válido por banco; a origem já assumia isso.
  - Uma importação por documento.
- **Log de extração.** O custo é gravado em dólares com 8 casas. O `MONEY` de 2 casas da origem zerava o custo de cada chamada.
- **Trava.** `reconciliation.RequireOpen` consulta o fechamento do módulo Contábil e devolve 423 `PERIOD_CLOSED`, o mesmo das obrigações.
  - Fiscal fechado não trava a Conciliação (AC-026).
  - Sem período gerado, não há trava.
  - Os serviços das TASK-015 em diante usam a trava em cada mutação (AC-025).

### D-019 — Fila de jobs em PostgreSQL (TASK-005; 08/10/2026, dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Porte da fila do Conciliador** (`app/servicos/fila.py` e `app/workers/runner.py`, commit `2bc6e26`):
  - aquisição atômica em ordem de chegada com `SELECT … FOR UPDATE SKIP LOCKED`;
  - 3 tentativas por padrão (`JOB_MAX_TENTATIVAS`);
  - espera de 2ⁿ segundos entre tentativas (limitada a 1 h);
  - estado terminal;
  - espera de 1 s com a fila vazia (`JOB_POLL_SECONDS`);
  - job `ping` de verificação.
  - As transições são as mesmas da origem; um teste confere os casos de `test_fila.py`.
- **Nomes do 05 §7:** tabela `jobs`; estados `pending`, `running`, `retry`, `done` e `dead` (`dead_letter` na origem); colunas `kind`, `attempts`, `max_attempts`, `run_after`, `last_error_code` e `last_error_message`.
- **Diferenças da origem, todas para não perder nem duplicar job:**
  - **Worker que cai.** A tentativa conta na aquisição, e a posse do job tem prazo (`JOB_LEASE_SECONDS`, padrão 300). O worker renova a posse a cada 1/3 do prazo enquanto executa. Sem renovação, outro worker devolve o job à fila com o código `WORKER_LOST` e a mesma regra de tentativas.
    - Na origem, um job em `running` de um worker que caiu ficava assim para sempre.
  - **Efeito uma vez por aquisição [06 §11].** O handler grava na mesma transação que marca o job como concluído, e a conclusão exige que o worker ainda tenha a posse. Uma tentativa que falha ou perde a posse desfaz os próprios efeitos.
  - **Enfileiramento idempotente.** Com `dedupe_key`, existe um job ativo por (`kind`, `dedupe_key`), garantido por índice único parcial. O job pode ser gravado na transação de quem chama, para nascer junto com o dado que o motivou.
  - **Tipos sem handler.** O worker só adquire os tipos que sabe executar. Um tipo sem handler espera na fila sem gastar tentativa; na origem, ele falhava até `dead_letter`.
  - **Pânico e erros.** Pânico no handler vira falha da tentativa (`HANDLER_PANIC`), sem derrubar o worker. O handler pode dar um código à falha (`Coded`) ou declará-la permanente (`Permanent`), e a falha permanente vai direto para `dead`.
  - **Mensagem de erro sanitizada.** Sem chaves, tokens e quebras de linha, até 300 caracteres. O sanitizador passou a ser compartilhado (`internal/platform/redact`) com o cliente do LiteLLM.
- **API [06 §8].**
  - `GET /jobs/{id}` traz o estado, as tentativas, os horários e o erro sanitizado, sem o payload.
  - `worker_online` diz se algum worker deu sinal nos últimos 30 s (tabela `worker_heartbeats`). Assim a interface pode mostrar "fila parada" (runbook 11 §9).
  - `POST /jobs/{id}/retry` vale só para `dead`: zera as tentativas e registra quem reenviou e quando. Não pula portões de domínio.
- **Encerramento ordenado.** O job em andamento tem 30 s para terminar; depois disso, a tentativa é registrada como `WORKER_SHUTDOWN` e o job volta para a fila.
- **Relógio.** A fila usa o relógio real, que é tempo operacional (espera entre tentativas, posse, batimento), mesmo com `APP_FIXED_TIME`, que congela só as datas de negócio.
- **Configuração:** `WORKER_CONCURRENCY` (padrão 1, como a origem), `JOB_MAX_TENTATIVAS`, `JOB_POLL_SECONDS` e `JOB_LEASE_SECONDS`.
- **Evidências:**
  - testes contra Postgres real: 60 jobs com 6 workers concorrentes, cada job executado uma vez; espera, `dead` e sanitização; efeitos desfeitos; posse vencida e posse renovada; encerramento ordenado;
  - detector de corrida limpo (Linux, `-race`);
  - teste manual no ambiente de dev com o worker real: job concluído; com o worker parado, job pendente e `worker_online: false`; ao religar, concluído.

### D-018 — Decisões técnicas da fatia do Fator R (TASK-006a, 010, 011 e 014a; 07/10/2026, dentro da autonomia da spec)

- **Decimal exato com a semântica do Python (ADR-007).**
  - Tipo `decimal.Decimal` sobre `cockroachdb/apd/v3`, com 28 dígitos e half-even.
  - A divisão exata vai ao expoente ideal e o zero é escrito como no Python, porque o apd difere nesses dois pontos.
  - Na API, decimal é string em ponto fixo. O motor recebe os valores com a escala da coluna.
  - Evidência: golden gerado pelo código Python da origem, com 233 cálculos, 18 carteiras e 160 simulações idênticos dígito a dígito.
- **Folha do PGDAS-D sem divisão.**
  - A folha total declarada fica em `folha_total_declarada_pgdas` (05 §4), não mais em `salarios` como na origem. Quando existe, é a folha do mês na FS12.
  - Um CHECK no banco exige `origem = 'pgdas'` e a divisão zerada, o que impede dupla contagem.
  - O lançamento manual ("Detalhar folha") apaga a folha total e grava a divisão.
  - Um teste prova que o resultado é igual ao da convenção da origem, com e sem a CPP na política.
- **Modelo de dados.**
  - **Configuração por empresa:** os padrões são os da origem (`quantidade_socios` 1, `honorario_mensal` 0, ambos NOT NULL), e não os nulos do 05 §4. Empresa sem configuração não tem linha. `pacote_comercial` nulo equivale ao `none` do 05 §4.
  - **Movimentos:** ganham `version` (concorrência otimista, padrão do projeto-base) e `pgdas_document_id`, cuja FK chega na TASK-006b, como na origem. `folha_mes` é coluna gerada.
  - **Tabelas do Simples:** `factor_r_simples_tables`, carregadas pelo `make seed` a partir do CSV idêntico ao da origem. Só insere; nunca altera uma vigência gravada.
  - **Política:** `factor_r_policy` tem uma linha só, com os padrões da origem (meta 30%, limiar do parser 0,40, piso R$ 6.000,00, CPP integra a FS12, tolerância ouro 1%). Não há tela de edição, porque na origem ela era da Fase 2.
- **Nomes.**
  - Colunas como no 05 §4. O JSON usa os mesmos nomes e, no cálculo e na carteira, os nomes da origem (`rbt12`, `fs12`, `semaforo`…).
  - Assim, o AGENTS.md §8.4 (campos em inglês) cede ao 05 §4 (D-007).
  - Os identificadores Go ficam em inglês, com o nome da origem no comentário.
- **Erros.**
  - Parâmetro de query ou caminho inválido → 400 `VALIDATION_ERROR` (convenção do projeto-base; a origem usava 422).
  - PA sem tabela do Simples vigente → 422 `SIMPLES_TABLE_NOT_IN_FORCE`, como na origem.
  - Dinheiro de entrada: string, ≥ 0, até 12 dígitos inteiros e 2 casas, como o `Dinheiro` da origem. Número JSON é recusado.
- **Interface.**
  - Carteira no menu "Fator R" e aba "Fator R" na ficha. O PA fica na URL (`?pa=`) e o padrão é a competência corrente.
  - A grade mensal acompanha a janela do PA; na origem ela tinha navegação própria de 12 em 12 meses.
  - "Como foi calculado" mostra o passo a passo das contas (cálculo explicável, 08 §6).
  - Semáforo com ícone e texto; disclaimer nas duas telas (FR-19).
  - A formatação é half-even por string, sem float.
- **Trava de contrato.** `TestOpenAPI_DocumentsEveryRoute` falha se alguma rota da API não estiver no `openapi.yaml`.
- **Demonstração.** O `make seed-demo` grava dados do Fator R para as 5 empresas sujeitas do seed. No PA corrente, cobrem:
  - vermelho, com meses faltantes;
  - amarelo;
  - verde, com meses do PGDAS-D;
  - dados insuficientes.

### D-017 — "Sujeita ao Fator R" é o `companies.factor_r` (07/10/2026; diverge do 05 §4; validada pelo usuário com a fatia em 08/10/2026)

- **Conflito:** o 05 §4 lista `sujeita_factor_r` em `company_factor_r_settings`. O projeto-base já tem `companies.factor_r`, o indicador "Fator R: Sim/Não" do Simples (PRD §17.2). Ele é copiado para o snapshot das competências e editado na aba Folha.
- **Por que não duplicar:** duas colunas seriam semântica paralela, o que o 05 §3 e o REQ-002 proíbem.
- **Decisão:**
  - Não existe coluna de sujeição na configuração do Fator R.
  - O indicador continua sendo editado só pelo `PATCH /companies/{id}` (aba Folha), que preserva histórico e snapshot.
  - A aba Fator R mostra o valor e leva até a aba Folha.
- **Carteira:** empresa ativa, com `factor_r = true` e no Simples Nacional no PA.
  - Na origem toda empresa era do Simples. No projeto-base, o `factor_r` pode continuar `true` depois de uma mudança de regime, porque a tela só o esconde.
  - O filtro de regime garante que só empresas do Simples entram.
  - O cálculo informa `monitorada` e `regime_no_pa`, e a aba explica por que a empresa está fora da carteira.

### D-016 — Fatia vertical do Fator R (07/10/2026)

- **Decisão do usuário:** adiantar a primeira tela de outro sistema ("só tem um dos 3 sistemas ali") e validar a fatia inteira de uma vez ("segue a fatia e valido inteira").
- **Ordem:** TASK-006a → TASK-010 → TASK-011 → TASK-014a. Um commit local por TASK, depois da validação.
- **Divisão das TASKs.**
  - **TASK-006:** a 006a traz configuração, movimentos, política e tabelas do Simples. A 006b traz documentos PGDAS-D e simulações, e fica com as TASK-012/013, porque depende da fila de jobs (TASK-005) e do trace local (TASK-009).
  - **TASK-014:** a 014a traz carteira, ficha com cálculo, grade mensal, configuração e disclaimer. A 014b traz inbox PGDAS-D, simulador, traces e avaliações.
  - **Por que o simulador espera:** na origem, `corrigir` só vale com a simulação persistida num trace (CLAUDE §3.11).
  - O motor do simulador já foi portado e provado na TASK-010 (REQ-013).
- Registrado no plano (13 §13).

### D-015 — Q-003 decidida: modelos padrão dos aliases (07/10/2026)

- **Decisão do usuário:** confirmados os modelos propostos na TASK-004.

  | Uso | Modelo | Aliases |
  |---|---|---|
  | barato | `openai/gpt-6-luna` | triador, consultor, priorizador |
  | forte com visão | `openai/gpt-6-sol` | extrator, resolvedor, classificador |
  | mais forte | `openai/gpt-6-astra` | extrator_forte |

- Continuam trocáveis no `.env` e, a partir da TASK-008, pela tela Configurações → IA.

### D-014 — Roteamento de modelos de IA (TASK-004, 07/10/2026; decisões técnicas dentro da autonomia da spec)

- **A aplicação resolve o alias e chama o gateway com o modelo concreto.**
  - O `ai.Resolver` devolve `provedor/modelo` no início de cada corrida; o resultado é o snapshot gravado nela.
  - O cliente (`internal/ai/litellm`) envia esse modelo pela rota `openai/*` (ou `anthropic/*`) do `litellm/config.yaml`.
  - Com isso, a corrida em andamento mantém o modelo do início (F-010), e a troca feita na tela (TASK-008) vale para as novas corridas sem reiniciar nada (AC-027).
  - Os aliases continuam declarados no gateway, também com o modelo do `.env`, como compatibilidade com o Conciliador.
- **Chaves.**
  - Nenhum alias tem chave própria (antes: `ANTHROPIC_API_KEY` fixa em todos). O LiteLLM usa a variável do provedor do modelo.
  - As chaves dos provedores existem só no container do LiteLLM. API e worker usam a chave do gateway (`LITELLM_API_KEY`), guardada como `config.Secret`, que se mascara em print, log e JSON.
  - Dois testes travam essas propriedades: `TestCompose_ProviderKeysOnlyInGateway` e `TestSecret_NeverPrinted`.
- **Modelos padrão (Q-003; propostos aqui e confirmados pelo usuário no D-015).** Família OpenAI gpt-6, que é a mais nova no LiteLLM fixado, mantendo o escalonamento dos sistemas de origem:

  | Uso | Modelo | Aliases |
  |---|---|---|
  | barato | `gpt-6-luna` | `triador`, `consultor`, `priorizador` |
  | forte com visão | `gpt-6-sol` | `extrator`, `resolvedor`, `classificador` |
  | mais forte, 3ª tentativa | `gpt-6-astra` | `extrator_forte` |

  Todos têm visão e saída estruturada segundo o próprio gateway. Ficam num único lugar do compose (âncora `x-ai-models`, usada por LiteLLM, API e worker). `TestCatalog_MatchesGatewayAndCompose` confere que código, `litellm/config.yaml`, `docker-compose.yml` e `.env.example` não divergem.
- **D-008 aplicado:** alias `classificador` com o mesmo modelo do `resolvedor`. No Conciliador, o agente de classificação usava o próprio alias `resolvedor`.
- **Tabela de modelos do LiteLLM travada** (`LITELLM_LOCAL_MODEL_COST_MAP=True`).
  - Sem isso, o gateway baixa a tabela do GitHub a cada início, ou seja, chamada externa e comportamento que muda sozinho.
  - Evidência: a tabela embutida não conhece o `gpt-6.1-sol` e o trata como sem suporte a saída estruturada, o que, com `drop_params`, poderia descartar o JSON Schema em silêncio.
  - **Regra para a TASK-008:** a validação da tela de IA deve consultar o gateway e avisar quando o modelo escolhido não tiver suporte a saída estruturada ou a visão, se o alias precisar.
- **Teste de alias simulado (TEST-004).**
  - O `mock_response` do LiteLLM vindo do cliente só funciona com chave virtual, que exige banco.
  - Por isso a evidência é: testes unitários do resolver e do cliente (`make test`) mais `make test-litellm`, que sobe o LiteLLM real (mesma imagem e mesmo `config.yaml`) diante de um provedor OpenAI simulado no próprio teste. Sem chamada externa e sem custo.
- **Cliente do gateway:**
  - poucas novas tentativas só em falhas transitórias (rede, 408, 429, 5xx), como os clientes de origem;
  - erro sempre sanitizado (sem chaves nem `Bearer`);
  - custo do gateway guardado como decimal textual, nunca `float`;
  - saída estruturada por JSON Schema (`strict`), que continua validada pelo código antes de qualquer uso.

### D-013 — Infraestrutura consolidada da TASK-003 (07/10/2026, decisões técnicas dentro da autonomia da spec)

A spec autoriza alterar Docker Compose e Makefile sem aprovação prévia (15-agent-execution-prompt); as escolhas abaixo ficam registradas para revisão.

- **MinIO.**
  - **Imagem:** `cgr.dev/chainguard/minio`, fixada por digest (MinIO `RELEASE.2026-09-22`).
  - **Motivo:** a MinIO deixou de publicar imagens da edição comunitária. `quay.io/minio/minio` e `quay.io/minio/mc` não existem mais, nem `latest` nem tags `RELEASE.*`. Por isso o compose do Conciliador de origem **não sobe hoje**.
  - O Langfuse do Fator R já usava a imagem da Chainguard; a mesma imagem serve aos dois MinIO e ao `minio-init` (ela traz `sh` e `mc`).
  - A Chainguard gratuita só publica `latest`, então a versão é fixada pelo digest.
- **Original imutável também no storage.**
  - Antes, o Conciliador usava a credencial raiz no app. Agora o `minio-init` cria um usuário da aplicação (`S3_ACCESS_KEY`).
  - A política desse usuário permite gravar, ler e listar versões, e **nega** apagar objeto, apagar versão, apagar o bucket, suspender o versionamento e mudar ciclo de vida ou política.
  - A credencial de administrador (`S3_ROOT_*`) só é usada pelo `minio-init`.
- **`minio-init` avulso.** Roda como etapa do `make dev` (`docker compose run --rm minio-init`) e é idempotente. Nenhum serviço depende do MinIO para subir: com ele fora, API e worker continuam saudáveis e só o upload da Conciliação falha (runbook 11 §9; testado).
- **Langfuse em compose dedicado** (`docker-compose.langfuse.yml`, projeto `controle-langfuse`).
  - Comandos: `make langfuse-up` ou `make dev-all`; `make dev-down` para tudo.
  - Motivo: são cerca de 2,4 GiB de RAM só para ele (RISK-008), e o domínio não depende dele (AC-030).
  - Organização e projeto passam a "Controle Integrado" / `controle-integrado-dev`.
  - Ganhou healthchecks no web e no worker.
- **Worker:**
  - subcomando `worker` do mesmo binário, em processo e container separados (04 §7);
  - healthcheck HTTP próprio em `/health`, porta 8090 só dentro do container;
  - hot reload com `.air.worker.toml` e diretório de build próprio (`tmp-worker/`).
- **Portas novas** publicadas só em `127.0.0.1` (09 §8). As portas existentes (Postgres, API, SPA) ficaram como estavam.
- **Versões fixadas:**

  | Serviço | Imagem |
  |---|---|
  | LiteLLM | `ghcr.io/berriai/litellm:v1.104.0` |
  | n8n | `n8nio/n8n:2.42.4` |
  | Langfuse | `langfuse:4.53.0` e `langfuse-worker:4.53.0` |
  | ClickHouse | `25.12.11.4` |
  | Redis | `7.4.11-bookworm` |
  | Postgres do Langfuse | `17.11-bookworm` |

  A origem usava `latest`/`main-latest` no Conciliador e só a versão maior (`4`, `7`, `17`) no Langfuse.
- **Preservado como na origem, adaptado depois:**
  - `litellm/config.yaml` (aliases e padrões Anthropic do Conciliador) → OpenAI configurável na TASK-004;
  - `n8n/workflows/ingestao.json`, que aponta para `http://app:8000/ingestao/arquivo` → endpoint novo na TASK-015. A pasta `n8n/inbox`, que o workflow observa e a origem não montava, agora é montada.
- **`.env.example`:** valores `dev-only` que funcionam localmente, na convenção do projeto-base. Não são segredos reais; para qualquer uso fora da máquina local, gerar valores próprios.
- **Ponto aberto para a TASK-009:**
  - API e worker (projeto `controle`) precisam alcançar o Langfuse (projeto `controle-langfuse`). Hoje estão em redes diferentes, e o Langfuse só publica em `127.0.0.1`.
  - Opções: rede externa compartilhada entre os dois projetos, ou `host.docker.internal`. Esta funciona no Docker Desktop, mas não no Linux com bind em loopback.
  - A decisão vai junto com a sincronização dos traces.
  - **Resolvido na D-022 (TASK-009):** rede externa `controle-obs`, criada pelo Makefile.

### D-012 — Agente `echo` fora do porte (07/10/2026)

- **Decisão do usuário:** o agente `echo` do Fator R (FR-23), ferramenta de dev para exercitar o tracer, não é portado. O tracer fica coberto por testes automatizados na TASK-009.

### D-011 — Capacidades não citadas pela spec entram no escopo (07/10/2026)

- **Decisão do usuário:** "se precisamos das capacidades, temos que desenvolver." Toda capacidade da origem não citada pela spec (`fontes.md` §4) é desenvolvida.
- **Distribuição:** `docs/ai-spec/13-implementation-plan.md` §12, assim:
  - FR-03 → TASK-014;
  - FR-07 → TASK-011/014;
  - FR-15 → TASK-013;
  - FR-19 → TASK-013/014;
  - CC-08 → TASK-016;
  - CC-09 → TASK-021;
  - CC-12 → TASK-017/021;
  - CC-22 → TASK-021/024.
- **Nova TASK-031 (M6, antes da TASK-030):** execução local completa fora do modo dev + backup e restauração de todos os armazenamentos (FR-21). A lacuna veio do próprio projeto-base: o `docker-compose.prod.yml` sobe só `app` + `db`, e o `scripts/backup.sh` cobre só o banco e os arquivos de procurações. Sem essa TASK, o produto consolidado instalado fora do modo dev ficaria sem worker, MinIO, LiteLLM, n8n e Langfuse.

### D-010 — Corrigir o baseline antes da TASK-003 (07/10/2026)

- **Decisão do usuário:** corrigir BL-001 (acessibilidade `target-size` no Dashboard) e BL-002 (`source-map-js`) agora, num commit `fix` separado, antes da TASK-003.
- **Motivo:** o teste de acessibilidade varre todas as telas num caso só, então enquanto falha esconde violações novas. Com o baseline todo verde, qualquer falha futura é regressão.
- **Execução:** a verificação da correção revelou mais dois problemas pré-existentes que também impediam um baseline verde e confiável. Os dois entraram no mesmo commit, com a mesma finalidade:
  - BL-003: asserção instável de aviso no E2E;
  - BL-004: detecção do Docker pelo testcontainers no Windows sob paralelismo.

  Detalhes e evidências em `baseline.md` §7.

### D-009 — Parser PGDAS-D determinístico: C-001 resolvido com a opção A (07/10/2026)

- **Decisão do usuário:** opção A, ou seja, portar o parser determinístico (regex), sem chamada de modelo em nenhuma etapa. `parser_pgdas` continua sendo o agente de trace, decisão e avaliação, mas não tem alias no LiteLLM.
- **Registro:** ADR-006. Documentos da spec ajustados: `CLAUDE.md` (IA), `04-system-design` §8, `07-flows` F-002, `10-env-and-secrets` §5, `ai-behavior-spec` §2, `prompt-spec` §3, `eval-plan` §2, `13-implementation-plan` (TASK-004).

### D-008 — Alias `classificador` (proposta técnica, TASK-004)

- **Contexto:** a spec lista `classificador` "quando necessário". No Conciliador, o agente de classificação usa o alias `resolvedor` (`app/agentes/classificador.py`).
- **Proposta:** criar o alias `classificador` apontando, por padrão, para o mesmo modelo do `resolvedor`. O comportamento de origem não muda, e o modelo passa a ser trocável de forma independente pela tela de IA.
- **Status:** aplicado na TASK-004 (D-014).

### D-007 — Precedência sobre o `AGENTS.md` do projeto-base (07/10/2026)

- **Contexto:** o `AGENTS.md` do `controle-tarefas` proíbe filas, dependências fora da lista aprovada e integrações externas (§6.1, §6.2), e define commits em inglês no formato `T-Mx-NN` (§8.8). A consolidação exige fila/worker, LiteLLM, Langfuse, MinIO e n8n.
- **Decisão:** vale a ordem de prioridade do `CLAUDE.md`: instrução do usuário > `docs/ai-spec/*` > `CLAUDE.md` > documentos existentes do `controle-tarefas`. O `AGENTS.md` continua valendo para o que a consolidação não altera: regras de domínio (§7), convenções de código, API, testes e segurança (§6.3–§6.5, §8.2–§8.7). O arquivo não é editado nem apagado.
- **Origem:** `CLAUDE.md` (Prioridade de instruções) e 00-index (Arquivos de agente).

### D-006 — Chave da OpenAI (07/10/2026)

- **Decisão do usuário:** a chave será entregue por arquivo, na hora de usar. Ela nunca entra no repositório, no banco, em log ou na API. Testes padrão não precisam dela (NFR-007).

### D-005 — Design System InovaSala entregue: Q-001 resolvida (07/10/2026)

- **Material:**
  - `docs/design-system/design-system-inovasala.html`, idêntico (mesmo SHA-256 `a48e7fa0…`) a `fator-r/docs/design-system-inovasala.html`;
  - `docs/design-system/design-system-inovasala-luminous.md`, versão operacional com tokens em JSON, regras e checklist.
- **Logos:** ficam em `C:\Users\mauro\OneDrive\Documentos\CEATH\Inovasala\Inova Sala SVG\` (versões branca e preta, com e sem "by Ceath"). Só entram no repositório na TASK-029.
- **Referência de adaptação** do sistema visual (pensado para páginas de marketing) a uma interface operacional densa: o próprio Fator R já fez esse trabalho no commit `31e7fd4` ("aplicar tema visual CEATH com tokens e gate de cores"), documentado em `fator-r/.claude/sdd/features/DESIGN_TEMA_CEATH.md`.
- **Efeito:** o AC-035 deixa de estar bloqueado por falta de material e passa a ser trabalho normal da TASK-029. A ADR-005 foi atualizada.
- **Observação:** o repositório é público. Os dois arquivos ficam públicos no primeiro push; o HTML já é público no `fator-r`.

### D-004 — Cadência de validação (07/10/2026)

- **Decisão do usuário:** parar ao fim de cada TASK para validação. A TASK-001 e a TASK-002 foram executadas juntas, porque só registram estado e não alteram código.

### D-003 — Ambiente de execução no Windows (07/10/2026)

- **Decisão do usuário:** opção A, ou seja, Git Bash + GNU Make no Windows, com o código em `C:\Desenvolvimento`.
- **Aplicado:**
  - GNU Make 4.4.1 (`ezwinports.make`, via winget);
  - Docker Desktop (WSL2);
  - clone com `core.autocrlf=false`;
  - `.env` local com `HTTP_PORT=8088`, porque a 8080 está ocupada por outro processo do usuário.

### D-002 — Git (07/10/2026)

- **Decisão do usuário:**
  - branch local `feat/consolidacao` a partir de `develop`;
  - um commit local por TASK, **depois** da validação do usuário;
  - mensagens em Conventional Commits em português;
  - nenhuma operação remota (push, PR, merge, tag) sem pedido explícito.
- Para o trabalho da consolidação, isso substitui o formato `T-Mx-NN` e as branches `task/...` do `AGENTS.md` §8.8 (instrução do usuário tem prioridade 1). Também é compatível com a DT-02 do projeto-base: nada vai para `main`.

### D-001 — Organização do workspace (07/10/2026)

- **Decisão do usuário:**
  - o repositório final é o `controle-tarefas`, não um repositório novo;
  - o workspace fica assim:

```text
C:\Desenvolvimento\controle-a-fonte\
  controle-tarefas\                  repositório de trabalho (branch feat/consolidacao)
  referencias\fator-r\               só leitura, commit 9dd7a74 (TASK-002)
  referencias\conciliador-contabil\  só leitura, commit 2bc6e26 (TASK-002)
  spec\                              pacote original da spec (cópia versionada aqui)
```

## 2. Conflitos

### C-001 — `parser_pgdas`: extração por LLM (spec) × parser determinístico (origem) — **resolvido (D-009, ADR-006)**

- **Spec de consolidação:**
  - F-002 diz "Worker resolve alias `parser_pgdas` no LiteLLM → extrair campos + confiança";
  - `prompt-spec` §3 define um prompt de extração;
  - `ai-behavior-spec` §2 trata o agente como extrator;
  - o `CLAUDE.md` lista `parser_pgdas` como alias de modelo.
- **Origem (Fator R):**
  - o `CLAUDE.md` §3.2 diz "O parse é determinístico. A extração do PGDAS-D é por regras (regex e heurísticas). O LLM nunca extrai campos de documento na v1";
  - no `plan.md` §7.3, a etapa `plan` do `parser_pgdas` é "sem LLM" e o `render` é template;
  - o código confirma: `parsing/pgdas.py` abre com "regras e regex, sem LLM";
  - a meta de ≥ 80% de acerto ouro mede esse parser determinístico.
- **Por que é material:** usar LLM para extrair é uma mudança de comportamento e de risco que a regra soberana da própria spec proíbe ("não substituir validação determinística por LLM", "PRD de origem é contrato comportamental"). Afeta a TASK-004 (lista de aliases), a TASK-012 (parser) e o eval-plan §2.
- **Opções:**
  - **A (recomendada):** portar o parser determinístico para Go, com a mesma regex, confiança e versão, provado pelo golden das 14 fixtures. `parser_pgdas` continua sendo o nome do agente, mas sem chamada de modelo; o alias não é criado, ou fica reservado e desabilitado.
  - **B:** parser determinístico primeiro; LLM só como sugestão para documento que o regex não lê, sempre em `needs_review` e sem gravar nada. Isso é capacidade nova e exige ADR.
  - **C:** extração por LLM como a spec descreve. Exige ADR, golden novo e aceite explícito da troca de comportamento.
- **Status:** resolvido em 07/10/2026 com a opção A (D-009, ADR-006).

## 3. Pendências

| ID | Pendência | Situação |
|---|---|---|
| P-001 | BL-001 (a11y `target-size` no Dashboard, `baseline.md` §3) | aprovado: corrigir antes da TASK-003 (D-010) |
| P-002 | BL-002 (`source-map-js`, npm audit alto) | aprovado: corrigir antes da TASK-003 (D-010) |
| P-003 | Capacidades não citadas pela spec | aprovado: entram no escopo (D-011) |
| P-004 | FR-23 agente `echo` | aprovado: não portar (D-012) |
| P-005 | D-017: sujeição ao Fator R pelo `companies.factor_r` e carteira só com empresas do Simples no PA | validada pelo usuário com a fatia (08/10/2026) |
| P-006 | Exibição do fator com 2 casas, como na origem: 0,279999 aparece como "28,00%" ao lado de "Anexo V" (o anexo e o semáforo usam o valor exato) | observação para a TASK-029; sem mudança de regra |
| P-007 | `consultar_cnpj` (CC-15, tool-contract §2): a origem nunca a implementou e o contrato proíbe chamada externa nova sem decisão. Recomendação: consulta só local (o cadastro de empresas e os CNPJs já vistos em lançamentos da mesma empresa, por nome normalizado e com resultado único), aplicada pelo código quando o comprovante não traz o documento da contraparte. Alternativa: fonte pública de CNPJ, que é chamada externa nova | aguarda decisão do usuário (TASK-017, D-028) |

### Pendências herdadas (permanecem pendentes; não resolver por suposição)

| Origem | Pendência |
|---|---|
| Controle de Tarefas | Q1–Q8 de `docs/decisoes.md` (valores provisórios em `docs/pendencias.md`), conforme Q-002 do intake |
| Consolidação | ~~Q-003: modelo OpenAI padrão de cada alias~~ — **decidida em 07/10/2026 (D-015)** |
| Fator R | P-04: conferência do CSV das tabelas do Simples contra a LC 123/2006 |
| Fator R | P-05: proporcionalização de empresa com menos de 12 meses (hoje devolve `empresa_nova_regra_pendente`) |
| Fator R | P-07: planilha de aceite (2 empresas × 12 meses); T-707: conferência com empresa real |
| Conciliador | golden com extratos reais (gate M0) e as questões abertas 1, 4, 5 e 6 do PRD |
