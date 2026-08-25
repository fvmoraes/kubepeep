# Evidências da Fase 5 — Dashboard

**Data da validação local:** 2026-08-24
**Plataforma principal:** Linux amd64
**Estado:** implementação local concluída; 61 de 62 tarefas comprovadas; cenário dinâmico no Kind pendente

## Resultado

O dashboard foi implementado com seis consultas independentes, cobertura e
erros parciais explícitos. Pods problemáticos, restarts, workloads, eventos,
scan limitado de logs e Metrics API opcional possuem classificadores e budgets
próprios. A interface mantém os demais blocos utilizáveis quando uma fonte é
negada ou indisponível.

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
frontend (audit sem vulnerabilidades, lint, typecheck, build, 63 Vitest e três
Playwright) e Ginger v1.4.4 (`inspect`/`doctor`) passaram no fechamento local.
O segundo cenário Playwright prova o dashboard parcial e o scan explícito.

## Pendências exatas

- **F5-59:** executar o dashboard contra a API Kubernetes real nos caminhos
  permitido, parcialmente negado, sem métricas e offline:
  `./test/kind/harness.sh validate` e
  `./test/kind/harness.sh app-e2e ./kubePeep`.

O harness e suas fixtures passaram na validação estática, mas os comandos
dinâmicos aguardam Docker. Não existe run atual de CI a citar.
