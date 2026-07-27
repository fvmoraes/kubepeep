# Matriz de aceite do MVP

Esta matriz liga os 27 critérios do prompt às fases responsáveis e à evidência que deverá existir. O status inicial de todos os itens é pendente.

| ID | Status | Critério | Fase | Tarefas responsáveis | Evidência mínima esperada |
| --- | --- | --- | --- | --- | --- |
| MVP-01 | [ ] | `kubePeep` inicia a aplicação local | 3 | F3-06, F3-12–15, F3-49 | Teste E2E do comando raiz e processo saudável |
| MVP-02 | [ ] | O navegador abre automaticamente | 3 | F3-14, F3-41 | Teste do adapter de browser e smoke test por plataforma |
| MVP-03 | [ ] | O frontend está embutido no binário | 3 | F3-36–37, F3-41 | Teste do asset embutido e rota SPA no binário compilado |
| MVP-04 | [ ] | Não há Node.js em runtime | 3 e 8 | F3-41, F8-41 | Execução do artefato em ambiente sem Node.js |
| MVP-05 | [ ] | O usuário pode selecionar um contexto | 4 | F4-10–14, F4-51 | Testes de API, serviço e interface de seleção |
| MVP-06 | [ ] | O usuário pode cadastrar um namespace | 4 | F4-19–23, F4-53 | Teste de escopo `single` persistido |
| MVP-07 | [ ] | O usuário pode cadastrar vários namespaces de uma vez | 4 | F4-15–23, F4-53 | Teste de importação `list` em uma única requisição/transação |
| MVP-08 | [ ] | Duplicados são removidos corretamente | 4 | F4-16, F4-20–22, F4-53 | Teste unitário do parser, UI e índice único |
| MVP-09 | [ ] | Entradas inválidas são informadas | 4 | F4-17–18, F4-21, F4-53 | Testes de validação DNS, API e interface |
| MVP-10 | [ ] | “Todos os namespaces” funciona respeitando RBAC | 4 | F4-24–29, F4-49, F4-56 | Cenários com/sem `list namespaces`, recursos negados e banco sem `*` |
| MVP-11 | [ ] | O dashboard mostra pods problemáticos | 5 | F5-19–23, F5-56, F5-59 | Testes de classificação e cenário E2E |
| MVP-12 | [ ] | O dashboard mostra pods com mais restarts | 5 | F5-14–18, F5-56, F5-59 | Testes de cálculo, ordenação e cenário E2E |
| MVP-13 | [ ] | O dashboard mostra workloads degradados | 5 | F5-24–28, F5-56, F5-59 | Testes por tipo de workload e cenário E2E |
| MVP-14 | [ ] | O dashboard mostra eventos `Warning` | 5 | F5-29–32, F5-56, F5-59 | Teste de classificação/agregação e cenário E2E |
| MVP-15 | [ ] | Usuário autorizado consegue ver logs | 6 | F6-21–29, F6-57 | Integração positiva e teste de stream |
| MVP-16 | [ ] | Usuário não autorizado não consegue ver logs | 4, 5 e 6 | F4-30–39, F5-34, F6-21–29, F6-57 | UI sem ação, scan limitado e API com `FORBIDDEN` real |
| MVP-17 | [ ] | O scan de logs possui limites de segurança | 5 | F5-33–43, F5-61 | Testes de janela, linhas, bytes, pods, containers, concorrência e cancelamento |
| MVP-18 | [ ] | Metrics API indisponível não quebra o dashboard | 5 | F5-44–48, F5-56, F5-59 | Integração sem `metrics.k8s.io` e interface degradada utilizável |
| MVP-19 | [ ] | Ações não autorizadas ficam ocultas ou desabilitadas | 4 e 7 | F4-38–39, F7-01–09, F7-43–45 | Testes de capabilities na interface para cada ação |
| MVP-20 | [ ] | O backend valida novamente toda ação | 7 | F7-01–03, F7-10, F7-15, F7-20, F7-25, F7-33 | SAR e negação imediatamente antes de cada mutação/upgrade |
| MVP-21 | [ ] | O produto funciona sem `cluster-admin` | 4 e 8 | F4-48–50, F8-20–25 | E2E com Role/RoleBinding restritos em Kind/K3d |
| MVP-22 | [ ] | SQLite não armazena credenciais | 3 e 4 | F3-22–26, F3-53, F4-03, F4-43, F4-54 | Inspeção do DB/journal/backup e testes de persistência |
| MVP-23 | [ ] | Os testes principais passam | Todas | F3-43, F4-55, F5-60, F6-60, F7-47, F8-47 | Pipeline verde com unitários, integração, frontend e E2E |
| MVP-24 | [ ] | `ginger doctor` não apresenta problema não documentado | 3 a 8 | F3-43, F4-55, F5-60, F6-60, F7-47, F8-47 | Saída anexada ao gate de cada fase |
| MVP-25 | [ ] | O binário é gerado pelo GoReleaser | 8 | F8-07–13, F8-47 | Build snapshot e release candidate bem-sucedidos |
| MVP-26 | [ ] | Instaladores validam checksum | 8 | F8-12, F8-27–34, F8-42 | Testes positivo e negativo de SHA-256 nos dois instaladores |
| MVP-27 | [ ] | Linux, macOS e Windows estão na configuração de release | 8 | F8-08–13, F8-41 | Matriz GoReleaser validada; amd64/arm64 conforme compatibilidade |

## Evidências complementares obrigatórias

Antes de considerar a matriz completa:

- [ ] o commit/tag exato analisado do DWYT está registrado;
- [ ] a dependência `github.com/fvmoraes/ginger v1.4.4` está fixada no `go.mod`;
- [ ] os seis documentos da Fase 2 estão atualizados;
- [ ] as decisões arquiteturais relevantes possuem ADR;
- [ ] nenhum teste usa credencial real;
- [ ] nenhum fixture contém Secret, token ou kubeconfig real;
- [ ] o cenário E2E inclui namespace permitido e negado;
- [ ] os artefatos finais foram executados, não apenas compilados;
- [ ] pendências e exceções conhecidas estão documentadas antes da release.

## Gates técnicos complementares

Os 27 critérios acima são necessários, mas não bastam para fechar o MVP. Também é obrigatório comprovar:

- [ ] `start`, `stop`, `status`, `doctor` e `update` nos sistemas suportados;
- [ ] bind dinâmico sem corrida, prontidão e cleanup mesmo após timeout de shutdown;
- [ ] `/health` separando aplicação, SQLite, kubeconfig, contexto e cluster;
- [ ] cursor multi-namespace/kind, expiração, 410 e resultado truncado;
- [ ] LIST/watch com RBAC distinto, relist e fallback HTTP;
- [ ] limites de bytes e backpressure em scan/follow de logs;
- [ ] restart, scale, delete, port-forward e exec nos caminhos permitido e negado;
- [ ] cleanup de watch, SSE, WebSocket, port-forward e exec;
- [ ] ConfigMap sob demanda e Secret metadata-only sem valores em memória;
- [ ] ausência de dados proibidos em DB, WAL/journal, backups, logs e archives;
- [ ] Settings/preferências limitados à allowlist;
- [ ] OpenTelemetry desativado e sem tráfego/exporter por padrão;
- [ ] `Cache-Control: no-store` e ausência de persistência browser para dados do cluster;
- [ ] uso ou justificativa em ADR dos pacotes Ginger obrigatórios;
- [ ] smoke tests nativos dos archives e instaladores reais.

## Como atualizar

1. Trocar `[ ]` por `[x]` somente depois que a evidência existir.
2. Substituir a descrição genérica da evidência pelo caminho do teste, workflow ou relatório executado.
3. Se um critério falhar em uma plataforma, mantê-lo pendente para o MVP inteiro.
4. Uma mudança posterior que invalide a evidência reabre o item.
