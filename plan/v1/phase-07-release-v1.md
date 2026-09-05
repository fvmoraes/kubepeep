# Fase 7 — Validação e preparação da release 1.0

**Prioridade:** P0. **Entrada:** F0–F6 completas com matriz R/U preenchida. **Saídas distintas:** versão pronta localmente; publicação aprovada e executada pelo usuário. Esta fase não autoriza push ou publicação automática pelo agente.

A distribuição atual usa `scripts/release.sh` e `.github/workflows/release.yml`, com builds nativos Wails, pacotes, checksums e SBOM. Tags oficiais seguem `1.0.0`, **sem prefixo `v`**. Não recriar GoReleaser ou copiar a matriz antiga sem conferir o pipeline vigente.

## Tarefas locais

- [ ] **V7-01 — Revisão da matriz.** Conferir R01–R36 e U01–U12 contra o binário final; cada requisito obrigatório precisa de evidência no commit avaliado. U12 (cadastro em lote sem acesso administrativo) é bloqueante, mesmo com todas as novas famílias prontas. Manter backlog explícito e não chamar destino desabilitado de recurso entregue.
- [ ] **V7-02 — Regressão visual e acessível.** Comparar shell, Overview e páginas ao estilo da imagem KubePeep.png conservada no Project Brain, conforme a [direção aprovada](../reference/direcao-visual-e-premissa-de-acesso.md). Percorrer todas as páginas nos tamanhos 1280×720, 1366×768, 1440×900 e 1920×1080; verificar 2560×1440 adicionalmente. Teclado/Tab/Escape, foco em drawer/dialog/paleta, zoom, contraste, nome KubePeep/versão do build e nenhum scroll horizontal global. Validar também aparência útil com RBAC restrito e dados parciais. Screenshots só como evidência local sanitizada.
- [ ] **V7-03 — Testes integrados.** Executar `make verify` e race checks aplicáveis. E2E mockado cobre UI; harness `test/kind/` cobre cluster real restritivo e recursos novos com fixtures sintéticas. Repetir os cenários da F0: colagem em lote → preview → salvar → selecionar → consultar recursos permitidos, sem list/get Namespace nem cluster-admin; misturar namespaces permitidos/negados e comprovar persistência, limites e cobertura parcial. Testar ainda sem acesso, contexto sem scope, cluster offline e Metrics API ausente.
- [ ] **V7-04 — Ciclo local e Wails.** CLI/status/stop/start/doctor, única instância, execução sem Node em runtime, instalação ativa/inativa e inicialização sob demanda. Build e smoke desktop nativo: frontend embutido, deep links, versão, troca de seleção, logs/exec/port-forward, fechamento da janela e cleanup. Cross-build não prova execução nativa.
- [ ] **V7-05 — Dados e segurança.** Rodar gate de segurança, auditoria de dependências conforme versões fixadas em `verify.yml` e inspeção negativa de browser storage, SQLite/WAL/backups/logs/archives. Usar sentinelas sintéticas; confirmar ausência de credenciais, Secret/YAML/logs persistidos e arquivos locais em staging/artefatos. Não apagar dados do usuário como “limpeza”.
- [ ] **V7-06 — Documentação final.** Alinhar README, API, arquitetura, produto, RBAC, dados, segurança, design-system, instalação/download e troubleshooting à matriz implementada. Links locais válidos, convenções e matriz de plataformas iguais ao pipeline real; limitações explícitas. Preparar changelog/notas `1.0.0` com ferramenta existente, sem invocar publicação.
- [ ] **V7-07 — Candidate reproduzível.** Fixar commit e manifesto de versões/hash; gerar assets de teste fora do Git. Revisar como produzir candidate sem tag final, commit remoto ou alteração de `latest`: o workflow atual publica em push de `main`/dispatch e não é um dry-run. Implementar suporte de validação isolada quando necessário antes de usá-lo para candidate. Não presumir que o script atual suporta SemVer prerelease.
- [ ] **V7-08 — Contrato dos pacotes.** Conferir matriz real do workflow, nomes/casing de assets, conteúdo, arquitetura, checksums e SBOM. Smoke de CLI/desktop e instalação/upgrade/remoção preservando dados, checksum adulterado bloqueado e rollback em runners nativos disponíveis. Registrar plataformas ainda sem execução, sem declarar gate verde por compilação.
- [ ] **V7-09 — Entrega local revisável.** Commit final com identidade noreply aprovada e `scripts/security_check.sh HEAD`; árvore limpa de alterações pendentes. Apresentar SHA, matriz de checks, limitações, notas/manifesto e ações externas ainda necessárias. Salvar contexto no Project Brain. Não criar/pushar tags ou executar workflow com escrita remota por conta própria.

## Gates externos de publicação

Somente após decisão explícita do usuário sobre a candidate concreta. Esses passos não bloqueiam a conclusão da organização documental atual, mas são obrigatórios para declarar **1.0 publicada**.

| ID | Gate | Evidência exigida |
| --- | --- | --- |
| V7-10 | Candidate imutável e instaladores (herança F8-42) | instalação/upgrade contra os assets exatos da candidate, SHA-256 validado e execução nativa por plataforma anunciada |
| V7-11 | Fechamento de qualidade (herança F8-46) | R/U desta matriz mais gates de runtime/instalação/segurança, com testes do commit efetivamente empacotado; nenhuma evidência antiga usada como execução nova |
| V7-12 | Canais canônicos de download (herança F8-48) | comandos publicados baixam os assets esperados da versão aprovada; redirects, casing, arquitetura e checksums testados |
| V7-13 | Publicação final | plano de versionamento produz `1.0.0`; tag imutável aponta para conteúdo aprovado; builds e verificação do commit empacotado passam; release/latest só atualizados no passo autorizado |

O pipeline pode criar commit de metadados distinto do SHA de origem. Verificar esse comportamento e exigir teste do conteúdo final empacotado; CI verde do commit anterior não prova uma alteração posterior. Não mover tag imutável para “consertar” uma release.

## Critério de saída

**Pronta localmente:** V7-01–09 e F0–F6 concluídas, U12 comprovado, referência visual atendida, artefatos/limitações rastreáveis e nenhum push executado. **Publicada:** V7-10–13 também concluídos após autorização. Gate pendente permanece explicitamente pendente; não marcar a fase publicada por falta de ambiente remoto.

**Rollback:** antes da publicação, corrigir por novo commit e gerar nova candidate. Depois, seguir procedimento de rollback/update validado preservando dados e tags imutáveis; revogar/republicar assets ou mover canais remotos exige a decisão do usuário.
