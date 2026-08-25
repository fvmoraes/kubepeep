# Evidências da Fase 6 — Recursos somente leitura

**Data da validação local:** 2026-08-24
**Plataforma principal:** Linux amd64
**Estado:** implementação local concluída; 66 de 67 tarefas comprovadas; E2E real no Kind pendente

## Resultado

As áreas Workloads, Pods, Logs, Events, Network, Config e Settings estão
implementadas com DTOs compactos, autorização, paginação/cursor composto,
YAML sob demanda e respostas `no-store`. Logs e os sete tópicos de atualização
possuem streams limitados e canceláveis. Secret permanece metadata-only e não
oferece YAML.

## Rastreabilidade da implementação

| Área | Implementação | Evidência automatizada |
| --- | --- | --- |
| Opções, fan-out e cursor | `internal/services/resources/options.go`, `list.go`, `cursor.go` | testes cobrem gramática fechada, limites, namespaces autorizados/negados, concorrência 4, cursor opaco ligado à query/generation, TTL e `ResourceExpired` |
| Workloads e Pods | `workloads.go`, `pods.go`, adapters/runtime e handlers | conversores de cinco kinds, detalhe, YAML e relações autorizadas por UID; CronJob usa histórico real de Jobs na janela de 24 h |
| Logs atuais/anteriores/follow | `logs.go`, `resources_stream.go` | autorização antes de abrir, limits por reader/linha/evento/total, redaction, linha sem newline, slow consumer, cancelamento e eventos meta/line/heartbeat/error/end |
| Events e Network | `events.go`, `network.go` | conversores allowlisted para Event, Service, Ingress e EndpointSlice; backend retorna `FEATURE_UNAVAILABLE` sem fallback para Endpoints |
| Config e Secret | `config.go`, `yaml.go`, metadata client no runtime | testes provam ConfigMap sob demanda e Secret via metadata API, sem `data`, `stringData`, annotations, managedFields ou YAML |
| LIST/watch e SSE | `watch.go`, adapter e handlers de stream | testes cobrem compartilhamento, list+watch all-or-nothing, fan-out/HTTP fallback, 410/relist, snapshot transacional, replay, ring/fila, oito sessões e reautorização periódica |
| Preferências e filtros | `preferences.go`, adapter SQLite e handlers | schema fechado, replace transacional e rejeição de chaves arbitrárias/sensíveis; UI de Settings e filtros salvos possui testes |
| Interface | `ResourcePages`, `LogsPage`, `ResourceLiveUpdates`, `SavedFilterControls`, `SettingsPage` | testes cobrem listas/detalhe/YAML, filtros, catálogo de logs por capability, follow/cancelamento, HTTP fallback e ausência de persistência browser |

## Streams e limites comprovados

- `/api/v1/stream` aceita somente a allowlist de sete tópicos e nunca Secret;
- snapshot de cada tópico é uma transação coerente e seus chunks compartilham
  o mesmo ID; exceder 10.000 itens/10 MiB descarta a transação inteira;
- replay usa IDs opacos ligados à sessão e ring limitado; ID expirado ou
  incompatível termina em reset explícito;
- follow de logs rejeita resume, aplica heartbeat e revalida RBAC
  periodicamente sem prometer revogação instantânea;
- generation change cancela watchers, follow, filas e respostas obsoletas;
- cliente lento recebe término explícito em vez de queda silenciosa.

## Gates locais executados

A suíte completa Go 1.25.13, race detector, vet, build e `govulncheck` passaram.
Frontend audit/lint/typecheck/build, 63 testes Vitest e três Playwright também
passaram. Ginger v1.4.4 `inspect` e `doctor` passaram com os diagnósticos
intencionais já documentados. A validação estática do harness passou.

## Pendências exatas

- **F6-57:** executar lista → detalhe → YAML/logs nos caminhos permitido e
  negado pelo black-box:
  `./test/kind/harness.sh validate`,
  `./test/kind/harness.sh kubeconfigs` e
  `./test/kind/harness.sh app-e2e ./kubePeep`.

Os cenários adversariais de protocolo permanecem cobertos localmente por
testes Go/frontend; a pendência acima é exclusivamente a integração com API
Kubernetes real. Não há URL de CI atual.
