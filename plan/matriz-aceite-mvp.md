# Matriz de aceite do MVP

Esta matriz liga os 27 critérios do prompt às fases responsáveis e à evidência
observada. Em 2026-08-25, 23 critérios possuem prova local/nativa aplicável e
quatro permanecem abertos por dependerem de Kind ou execução
nativa atual. Os relatórios das Fases 4–8 registram o detalhe e não inventam
URLs de CI.

| ID | Status | Critério | Fase | Tarefas responsáveis | Evidência mínima esperada |
| --- | --- | --- | --- | --- | --- |
| MVP-01 | [x] | `kubePeep` inicia a aplicação local | 3 | F3-06, F3-12–15, F3-49 | `internal/cli/production_test.go` e black-box nativo da [Fase 3](../docs/research/phase3-evidence.md) |
| MVP-02 | [x] | O navegador abre automaticamente | 3 e 8 | F3-14, F3-41, F8-41 | adapter testado e run nativo `31392541114` da Fase 3; smoke atual dos archives segue como gate complementar |
| MVP-03 | [x] | O frontend está embutido no binário | 3 | F3-36–37, F3-41 | teste de produção serve a SPA embutida e preserva fallback History API |
| MVP-04 | [x] | Não há Node.js em runtime | 3 e 8 | F3-41, F8-41 | lifecycle Linux atual passou com `PATH` sem Node.js; prova nativa dos archives segue separada |
| MVP-05 | [x] | O usuário pode selecionar um contexto | 4 | F4-10–14, F4-51 | testes de service/handler e `ContextSelector.test.tsx`; [Fase 4](../docs/research/phase4-evidence.md) |
| MVP-06 | [x] | O usuário pode cadastrar um namespace | 4 | F4-19–23, F4-53 | serviço/SQLite/handler e UI cobrem escopo `single` |
| MVP-07 | [x] | O usuário pode cadastrar vários namespaces de uma vez | 4 | F4-15–23, F4-53 | parser + transação única + editor cobrem `list` |
| MVP-08 | [x] | Duplicados são removidos corretamente | 4 | F4-16, F4-20–22, F4-53 | `parser_test.go`, constraints SQLite e testes React |
| MVP-09 | [x] | Entradas inválidas são informadas | 4 | F4-17–18, F4-21, F4-53 | DNS label, formatos estritos, API e editor testados |
| MVP-10 | [ ] | “Todos os namespaces” funciona respeitando RBAC | 4 | F4-24–29, F4-49, F4-56 | serviços locais passaram; falta executar com/sem `list namespaces` no Kind real |
| MVP-11 | [x] | O dashboard mostra pods problemáticos | 5 | F5-19–23, F5-56, F5-59 | classificadores Go + componente React + cenário Playwright parcial |
| MVP-12 | [x] | O dashboard mostra pods com mais restarts | 5 | F5-14–18, F5-56, F5-59 | três tipos de container, ordenação/thresholds e tabela Playwright |
| MVP-13 | [x] | O dashboard mostra workloads degradados | 5 | F5-24–28, F5-56, F5-59 | testes dos cinco kinds e UI de blocos independentes |
| MVP-14 | [x] | O dashboard mostra eventos `Warning` | 5 | F5-29–32, F5-56, F5-59 | agrupamento/contador/timestamp e tabela Playwright |
| MVP-15 | [x] | Usuário autorizado consegue ver logs | 6 | F6-21–29, F6-57 | leitura/follow autorizados nos testes Go e catálogo por capability no frontend |
| MVP-16 | [x] | Usuário não autorizado não consegue ver logs | 4, 5 e 6 | F4-30–39, F5-34, F6-21–29, F6-57 | porta não abre após negação; scan/UI permanecem fail-closed |
| MVP-17 | [x] | O scan de logs possui limites de segurança | 5 | F5-33–43, F5-61 | budgets de janela/linha/pod/container/bytes/concorrência/cancelamento testados |
| MVP-18 | [x] | Metrics API indisponível não quebra o dashboard | 5 | F5-44–48, F5-56, F5-59 | serviço, adapter, UI e Playwright comprovam degradação opcional |
| MVP-19 | [x] | Ações não autorizadas ficam ocultas ou desabilitadas | 4 e 7 | F4-38–39, F7-01–09, F7-43–45 | `ResourceActions.test.tsx` mantém `denied`/`unknown` desabilitados |
| MVP-20 | [x] | O backend valida novamente toda ação | 7 | F7-01–03, F7-10, F7-15, F7-20, F7-25, F7-33, F7-44 | testes de actions e exec repetem SAR imediatamente antes de mutação/upgrade |
| MVP-21 | [ ] | O produto funciona sem `cluster-admin` | 4 e 8 | F4-48–50, F8-20–25 | RBAC estático passou; falta E2E com Role/RoleBinding restritos no Kind |
| MVP-22 | [x] | SQLite não armazena credenciais | 3 e 4 | F3-22–26, F3-53, F4-03, F4-43, F4-54 | scanner da Fase 3 e testes F4/F7 de não persistência inspecionam DB/sidecars/backups |
| MVP-23 | [ ] | Os testes principais passam | Todas | F3-43, F4-55, F5-60, F6-60, F7-47, F8-47 | gates locais passaram; faltam Kind e pipeline nativo do estado atual |
| MVP-24 | [x] | `ginger doctor` não apresenta problema não documentado | 3 a 8 | F3-43, F4-55, F5-60, F6-60, F7-47, F8-47 | Ginger v1.4.4 `inspect`/`doctor` executados; diagnósticos heurísticos documentados |
| MVP-25 | [x] | O binário é gerado pelo GoReleaser | 8 | F8-07–13, F8-47 | dois snapshots GoReleaser v2.17.1 com Go 1.25.13 produziram seis archives e checksums idênticos |
| MVP-26 | [ ] | Instaladores validam checksum | 8 | F8-12, F8-27–34, F8-42 | Unix positivo/negativo passou; falta executar PowerShell/Windows e candidate real |
| MVP-27 | [x] | Linux, macOS e Windows estão na configuração de release | 8 | F8-08–13, F8-41 | `.goreleaser.yaml` validado e seis cross-builds passaram; smoke nativo segue complementar |

## Evidências complementares obrigatórias

Antes de considerar a matriz completa:

- [x] o commit/tag exato analisado do DWYT está registrado;
- [x] a dependência `github.com/fvmoraes/ginger v1.4.4` está fixada no `go.mod`;
- [x] os seis documentos da Fase 2 estão atualizados;
- [x] as decisões arquiteturais relevantes possuem ADR;
- [x] nenhum teste usa credencial real;
- [x] nenhum fixture contém Secret, token ou kubeconfig real;
- [x] o cenário E2E inclui namespace permitido e negado;
- [ ] os artefatos finais foram executados, não apenas compilados;
- [x] pendências e exceções conhecidas estão documentadas antes da release nos relatórios das Fases 4–8.

## Gates técnicos complementares

Os 27 critérios acima são necessários, mas não bastam para fechar o MVP. Também é obrigatório comprovar:

- [ ] `start`, `stop`, `status`, `doctor` e `update` nos sistemas suportados;
- [x] bind dinâmico sem corrida, prontidão e cleanup mesmo após timeout de shutdown;
- [x] `/health` separando aplicação, SQLite, kubeconfig, contexto e cluster;
- [x] cursor multi-namespace/kind, expiração, 410 e resultado truncado;
- [x] LIST/watch com RBAC distinto, relist e fallback HTTP;
- [x] limites de bytes e backpressure em scan/follow de logs;
- [ ] restart, scale, delete, port-forward e exec nos caminhos permitido e negado;
- [x] cleanup de watch, SSE, WebSocket, port-forward e exec;
- [x] ConfigMap sob demanda e Secret metadata-only sem valores em memória;
- [ ] ausência de dados proibidos em DB, WAL/journal, backups, logs e archives;
- [x] Settings/preferências limitados à allowlist;
- [x] OpenTelemetry desativado e sem tráfego/exporter por padrão;
- [x] `Cache-Control: no-store` e ausência de persistência browser para dados do cluster;
- [x] uso ou justificativa em ADR dos pacotes Ginger obrigatórios;
- [ ] smoke tests nativos dos archives e instaladores reais.

## Pendências para fechar a matriz

1. Disponibilizar Docker e executar `create`, `validate`, `kubeconfigs` e
   `app-e2e` do harness Kind.
2. Executar instalador/update e lifecycle dos archives em Linux, macOS e
   Windows, incluindo PowerShell e helper de update nativo.
3. Executar o workflow `verify.yml` no commit final e registrar o run real.
4. Repetir a varredura sensível sobre o conjunto exatamente staged e os
   archives finais; então fechar os dois itens de fixtures/credenciais.

## Como atualizar

1. Trocar `[ ]` por `[x]` somente depois que a evidência existir.
2. Substituir a descrição genérica da evidência pelo caminho do teste, workflow ou relatório executado.
3. Se um critério falhar em uma plataforma, mantê-lo pendente para o MVP inteiro.
4. Uma mudança posterior que invalide a evidência reabre o item.
