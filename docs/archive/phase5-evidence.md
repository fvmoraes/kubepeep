# Evidências da Fase 5 — Dashboard

> Registro histórico: resultados e gates referem-se à execução datada abaixo.
> A sequência atual de entrega está no [plano v1](../../plan/README.md).

**Data da validação local:** 2026-08-24
**Data da validação Kind:** 2026-08-30
**Plataforma principal:** Linux amd64
**Estado:** fase concluída; 62 de 62 tarefas comprovadas, incluindo o cenário dinâmico no Kind

## Resultado

O dashboard foi implementado com seis consultas independentes, cobertura e
erros parciais explícitos. Pods problemáticos, restarts, workloads, eventos,
scan limitado de logs e Metrics API opcional possuem classificadores e budgets
próprios. A interface mantém os demais blocos utilizáveis quando uma fonte é
negada ou indisponível.

O fechamento dinâmico confirmou os quatro estados exigidos: dashboard completo,
dashboard parcialmente negado, Metrics API ausente sem falha global e perfil
offline com término limitado.

## Rastreabilidade da implementação

| Área | Implementação | Evidência automatizada |
| --- | --- | --- |
| Orquestração e DTOs | `internal/services/dashboard/summary.go`, `ports.go`, `dto.go`, `budgets.go` | `services_test.go` prova estados de contador, cobertura, truncamento e isolamento entre blocos |
| Endpoints e generation fence | `internal/api/handlers/dashboard.go` | `dashboard_test.go` cobre summary, problems, restarts, events, log-scan, metrics, cursor, query fechada e descarte de geração obsoleta |
| Restarts e problemas de Pod | `pods.go` | `pods_test.go` cobre três tipos de container, thresholds, tabela fechada, UID, prioridades e boundaries de 2/5/15 minutos |
| Workloads | `workloads.go`, `workload_service.go` | `workloads_test.go` e `workload_service_test.go` cobrem os cinco kinds, campos ausentes/stale e CronJob na janela exata de 24 h |
| Eventos | `events.go` | `events_test.go` cobre timestamp canônico, agrupamento fechado, contador e ordenação |
| Scan de logs | `logs.go`, `log_selection.go`, `log_detection.go`, `log_redaction.go` | testes cobrem alvos prioritários, janelas/linhas/containers/bytes, concorrência, timeout, cancelamento, texto/JSON, excerpt determinístico e todas as classes de redaction |
| Metrics API | `metrics.go` e adapter Kubernetes | `metrics_test.go` e `dashboard_test.go` cobrem quantities, ranking e estados ausente/proibido/disponível sem falha global |
| Interface | `web/src/components/Dashboard.tsx` | `Dashboard.test.tsx` e Playwright cobrem carga independente, estado parcial, scan explícito, conteúdo redigido e ausência de Metrics API |

## Invariantes comprovados

- cada bloco distingue zero real, indisponibilidade, negação e truncamento;
- o scan usa limites agregados e por fonte e cancela leitores pendentes ao
  esgotar o budget;
- linhas/resultados de logs não são persistidos; somente um contador agregado
  ligado à geração permanece em memória;
- owner/reason/message não são inventados quando faltam evidências;
- a interface denomina os matches como possíveis erros e mantém resultados
  apenas em memória;
- a ausência de `metrics.k8s.io` não altera a saúde da aplicação local.

## Gates locais executados

Os gates Go 1.25.13 (`test`, race, vet, build e `govulncheck`), os gates de
frontend (audit sem vulnerabilidades, lint, typecheck, build, 73 Vitest e três
Playwright) e Ginger v1.4.4 (`inspect`/`doctor`) passaram no fechamento local.
O segundo cenário Playwright prova o dashboard parcial e o scan explícito.

## Evidência Kind canônica

O fechamento local executou:

```bash
rtk ./test/kind/harness.sh create
rtk ./test/kind/harness.sh validate
rtk ./test/kind/harness.sh kubeconfigs
rtk ./test/kind/harness.sh app-e2e ./dist/kubePeep
```

O black-box comprovou o dashboard completo no perfil permitido, a degradação
parcial com `FORBIDDEN` sem bloquear os demais cartões, a ausência opcional da
Metrics API e o encerramento limitado do perfil offline. A fixture também
comprovou um Pod com log anterior após restart e o Event inicial
`000-kp-warning`, necessário para exercitar paginação e ordenação reais.

Para tornar execuções repetidas determinísticas, o harness primeiro valida a
propriedade e reaplica o conjunto completo; depois substitui, uma por vez,
somente as fixtures sensíveis ao tempo. Cada delete usa a UID observada como
precondição, e cada substituição arma a recuperação antes do DELETE. Sinais
encerram o fluxo pelo trap de saída; resposta perdida converge repetindo o
DELETE precondicionado, e a restauração é feita por criação canônica. Assim, o
restart volta a produzir previous-log e o Event recebe timestamp e posição de
ordenação novos. A evidência registra apenas os
estados e identificadores sintéticos necessários: nenhum conteúdo de log,
kubeconfig, token ou payload de Event foi copiado para este documento.

Esta é evidência local do Kind; nenhum run adicional de CI é atribuído ao
fechamento.
