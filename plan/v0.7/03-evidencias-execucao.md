# Evidências de execução — plano v0.7

Registro de fechamento das fases F0–F7. Cada linha segue a regra da [matriz](01-matriz-de-entregas.md):
`ID → commit → comando/cenário → resultado`. A coluna **Commit** identifica o
código/binário sob teste; o commit que contém esta tabela versiona o resumo
documental. Evidências contêm apenas comando, resultado resumido e commit —
nenhum dado privado, log cru ou screenshot.

Classificação usada neste registro:

- **IMPLEMENTADO:** entrega e aceite da fase possuem evidência compatível;
- **ALTERADO:** entrega funcional aceita como fechada por decisão explícita,
  com validação complementar transferida e rastreada em fase posterior;
- **PENDENTE:** entrega ainda não executada na fase à qual foi atribuída.

## Revisões documentais (não fecham fases funcionais)

- `f7cd651`: avaliação inicial, cinco links corrigidos; 34 links relativos válidos, diff-check e gate de segurança aprovados.
- Revisão C01–C06: contratos incorporados à matriz e F0–F7; 47 links relativos válidos, 74 tarefas anteriores e todos os IDs da matriz preservados; F2-11/F2-12 explicitam integrações dos requisitos existentes. Diff-check aprovado. O commit que introduz esta entrada registra a revisão; nenhuma tarefa funcional foi marcada como concluída.

## Fase 0 — Correção funcional P0, baseline e instrumentação

**Status: FINALIZADA em 2026-09-12.** Por decisão explícita do usuário, as
validações complementares de F0-01, F0-05 e F0-09 foram transferidas para
F1-11, F1-12 e F1-13, sem alegação de que já foram executadas.

Baseline numérica e protocolo: [`docs/performance-baseline.md`](../../docs/performance-baseline.md).

| ID | Classificação | Commit | Evidência | Resultado |
| --- | --- | --- | --- | --- |
| F0-01 | **ALTERADO** | `6a8eead` | `rtk make verify`; `./test/kind/harness.sh app-e2e ./dist/kubePeep` | correção e estados honestos passaram em componentes, Playwright web, transporte `desktop-bridge` simulado e black-box Kind; Wails/WebView nativo foi transferido para F1-11. |
| F0-02 | **IMPLEMENTADO** | `6a8eead` | inventário por tela em `docs/ui-ux-refinement.md`; `rtk make verify` | defeitos encontrados foram corrigidos; todo clique inventariado executa função ou fica desabilitado com motivo; nenhum clique morto conhecido permanece. |
| F0-03 | **IMPLEMENTADO** | `6a8eead` | auditoria estática e geométrica em `docs/ui-ux-refinement.md`; `rtk make verify` | proporção filtros/conteúdo medida em três viewports, exceções tipográficas inventariadas e filtros ligados ao efeito real; riscos visuais de largura estreita ficaram explícitos para F4. |
| F0-04 | **IMPLEMENTADO** | `6a8eead` | `rtk make verify`; `rtk make test-race`; inspeção de `/metrics` nos testes | estratégia LIST, cursor, watch, 429, bytes e throttle possuem séries fechadas e testadas; resource cache permanece reservado para integração real na F2 (C04). |
| F0-05 | **ALTERADO** | `6a8eead` | `web/src/observability/uxMetrics.test.ts`; `rtk make verify` | métricas por tentativa e budgets foram implementados/testados; coleta browser/desktop real foi transferida para F1-12. |
| F0-06 | **IMPLEMENTADO** | `6a8eead` | testes de `internal/observability`; `rtk make verify`; `rtk make test-race` | spans dos componentes existentes aprovados com allowlist sem identidade sensível; contrato F2 definido sem telemetria simulada. |
| F0-07 | **IMPLEMENTADO** | `6a8eead` | `rtk make benchmark`; `rtk make benchmark-matrix`; `rtk test/kind/harness.sh static` | 10 cenários representativos e 47 de matriz concluíram sem `unstable`/`unexpected_error`; dataset explícito cobre 1–200 namespaces × 10–500 Pods e não é aplicado pelos gates. |
| F0-08 | **IMPLEMENTADO** | `6a8eead` | JSONs sanitizados + `docs/performance-baseline.md` | baseline registra SHA limpo, ambiente, p50/p95, requests, bytes, over-fetch, alocação, heap, delta de goroutines, cursor e watch, separando sintético, Kind e Wails. |
| F0-09 | **ALTERADO** | `6a8eead` | benchmarks CursorStore/merge; matriz sintética C02/410 | TTL, LRU, budgets, binding, purge, 410 e cenários sintéticos passaram; cenário Kind real de 200 namespaces foi transferido para F1-13. |

### Gates de fechamento executados

| Gate | Resultado |
| --- | --- |
| `rtk make format-check` | aprovado; nove warnings ESLint conhecidos, sem erro |
| `rtk make verify` | aprovado após restaurar `node_modules` pelo lockfile; lint, tipos, unidades/integração, Playwright, build e smoke verdes |
| `rtk make test-race` | aprovado |
| `rtk make build smoke` | aprovado |
| `rtk scripts/security_check.sh HEAD` | aprovado |
| `rtk npm audit --omit=dev` em `web/` | aprovado; zero vulnerabilidades de produção |
| `rtk git diff --check` | aprovado |
| `rtk test/kind/harness.sh validate` | matriz Kubernetes/RBAC real aprovada |
| `rtk test/kind/harness.sh app-e2e ./dist/kubePeep` | black-box real aprovado; cluster dedicado preservado |

## Fase 1 — Paginação estratégica e uso responsável do API Server

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| F1-11 | — | transferência aprovada da F0 | **PENDENTE na F1:** regressão de Pods em Wails/WebView nativo |
| F1-12 | — | transferência aprovada da F0 | **PENDENTE na F1:** medição UX real contra os budgets |
| F1-13 | — | transferência aprovada da F0 | **PENDENTE na F1:** cenário Kind real C02 com 200 namespaces |

## Fase 2 — Estado orientado a snapshot: cache sob demanda + WATCH

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
|  |  |  |  |

## Fase 3 — Frontend progressivo, virtualizado e inicialização instantânea

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
|  |  |  |  |

## Fase 4 — Fechamento do refinamento UI/UX e scope default

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
|  |  |  |  |

## Fase 5 — Investigação: problems, diagnóstico e logs agregados

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
|  |  |  |  |

## Fase 6 — Protocolo e tuning avançado (condicional a benchmark)

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
|  |  |  |  |

## Fase 7 — Validação comparativa e preparação da release

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
|  |  |  |  |

## Pendências futuras (não reabrem a Fase 0)

- F1-11, F1-12 e F1-13 preservam integralmente as validações transferidas.
- Métricas e spans do novo resource cache continuam atribuídos à F2 por C04;
  não são simulados para fechar D01/D04 antecipadamente.
