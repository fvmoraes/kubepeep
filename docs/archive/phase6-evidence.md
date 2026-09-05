# Evidências da Fase 6 — Recursos somente leitura

> Registro histórico: resultados e gates referem-se à execução datada abaixo.
> A sequência atual de entrega está no [plano v1](../../plan/README.md).

**Data da validação local:** 2026-08-24
**Data da validação Kind:** 2026-08-30
**Plataforma principal:** Linux amd64
**Estado:** fase concluída; 67 de 67 tarefas comprovadas, incluindo o E2E real no Kind

## Resultado

As áreas Workloads, Pods, Logs, Events, Network, Config e Settings estão
implementadas com DTOs compactos, autorização, paginação/cursor composto,
YAML sob demanda e respostas `no-store`. Logs e os sete tópicos de atualização
possuem streams limitados e canceláveis. Secret permanece metadata-only e não
oferece YAML.

O fechamento dinâmico confirmou listas, detalhes, YAML permitido, logs atuais e
anteriores, streams, replay e revogação contra uma API Kubernetes real.

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
Frontend audit/lint/typecheck/build, 73 testes Vitest e três Playwright também
passaram. Ginger v1.4.4 `inspect` e `doctor` passaram com os diagnósticos
intencionais já documentados. A validação estática do harness passou.

## Evidência Kind canônica

O fechamento local executou:

```bash
rtk ./test/kind/harness.sh create
rtk ./test/kind/harness.sh validate
rtk ./test/kind/harness.sh kubeconfigs
rtk ./test/kind/harness.sh app-e2e ./dist/kubePeep
```

No perfil permitido, o black-box comprovou listas de Workloads, Pods, Events,
Services, Ingresses, EndpointSlices e ConfigMaps; detalhe e YAML; detalhe de
ConfigMap; Secret restrito a metadados e sem YAML; logs atuais e anteriores; e
o ciclo snapshot, update, follow, replay e troca de geração. Os mesmos caminhos
no perfil negado não ofereceram bypass de autorização.

Durante a revogação, a ausência de uma decisão do SSAR tornou a autorização
desconhecida: leituras e streams do produto falharam fechados com
`503/AUTHORIZATION_UNAVAILABLE`. A leitura direta pela identidade revogada foi
negada autoritativamente pela API Kubernetes com `Forbidden`, sem converter
essa evidência em uma negação que o SSAR não retornou. A restauração do grant
também foi comprovada.

O harness recria de forma idempotente o Pod usado para previous-log e o Event
inicial `000-kp-warning`, um por vez, com ownership estrito, precondição de UID
e recuperação canônica armada antes da remoção. A recuperação repete um DELETE
de resposta ambígua com a mesma UID, aguarda qualquer substituto em terminação
e cria o manifesto canônico sem `apply`. Isso evita que idade, contador de
restart ou ordenação de uma execução anterior contaminem a seguinte. Nenhum conteúdo de log,
kubeconfig, token ou payload de Event foi copiado para esta evidência.

Esta é evidência local do Kind; nenhum run adicional de CI é atribuído ao
fechamento.
