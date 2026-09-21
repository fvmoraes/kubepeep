# Observabilidade do KubePeep

## 1. Princípios

- Nunca registrar payloads, credenciais, valores de Secret, logs de containers,
  comandos/saída de `exec`, nomes/UIDs de objetos ou identidade do usuário.
- Observabilidade nunca altera autorização, fallback nem resultado funcional;
  falha de exportação é sanitizada e não interrompe o core.
- Métricas e OTel são opt-in. Sem configuração explícita, nenhum tráfego de
  observabilidade sai do processo.
- Nomes de série, chaves de label, spans e atributos usam vocabulário fechado.
  O registry limita cada família a 1024 combinações.

## 2. Logs internos

### 2.1 Schema fechado

O handler JSONL (`internal/logging`) aceita somente:

```text
timestamp, level, component, operation, request_id,
context, namespace, resource, duration, duration_ms, error_code
```

`duration` é legível para humanos e `duration_ms` é numérico. Todo texto passa
por sanitização de credenciais, remoção de controles e truncamento em 1024
bytes. Esse schema de log não deve ser confundido com labels de métricas: os
campos `context` e `namespace` nunca se tornam séries Prometheus nem atributos
OTel.

### 2.2 Retenção

Rotação em 10 MiB, cinco backups, remoção após 14 dias e modo `0600`.

## 3. Métricas internas

### 3.1 Registro e labels

`internal/observability.Registry` mantém counters e gauges concorrentes e
renderiza Prometheus text format deterministicamente. Métrica ou label fora da
allowlist é ignorada. As únicas chaves aceitas são:

```text
method, route, status, resource, strategy, traffic
```

- `route`: padrão estático do `http.ServeMux`, ou `unmatched`;
- `resource`: coleção canônica do produto, nunca nome/GVR fornecido pelo usuário;
- `strategy`: nas métricas de LIST, somente `global` ou `fanout`; spans também
  aceitam `global-native`, `namespace-sequential` e `lazy-merge`;
- `traffic`: somente `unary` ou `streaming` no client-go;
- `status`: status HTTP decimal ou `transport_error`.

### 3.2 Séries expostas a partir das Fases 0 e 2

| Série | Tipo | Labels | Interpretação |
| --- | --- | --- | --- |
| `kubepeep_requests_total` | counter | `method`, `route`, `status` | requests HTTP locais |
| `kubepeep_resource_lists_total` | counter | `resource`, `strategy` | operações LIST por estratégia |
| `kubepeep_resource_list_duration_nanoseconds_total` | counter | `resource`, `strategy` | soma de duração; dividir pelo contador de LIST para média |
| `kubepeep_resource_list_items_received_total` | counter | `resource` | DTOs recebidos do Kubernetes |
| `kubepeep_resource_list_items_returned_total` | counter | `resource` | DTOs devolvidos à UI |
| `kubepeep_cursor_entries` | gauge | — | estados vivos no store |
| `kubepeep_cursor_bytes` | gauge | — | bytes serializados no store |
| `kubepeep_cursor_hits_total` | counter | — | referências encontradas |
| `kubepeep_cursor_misses_total` | counter | — | referências ausentes/expiradas |
| `kubepeep_cursor_expired_total` | counter | — | entradas purgadas por TTL |
| `kubepeep_cursor_evicted_total` | counter | — | entradas removidas por LRU/budget |
| `kubepeep_resource_cache_entries` | gauge | — | snapshots vivos no cache sob demanda |
| `kubepeep_resource_cache_bytes` | gauge | — | bytes serializados dos snapshots vivos |
| `kubepeep_resource_cache_hits_total` | counter | `resource` | snapshots encontrados pela identidade completa |
| `kubepeep_resource_cache_misses_total` | counter | `resource` | snapshots ausentes ou incompatíveis |
| `kubepeep_resource_cache_evictions_total` | counter | `resource` | snapshots inativos removidos por LRU/budget |
| `kubepeep_resource_cache_invalidations_total` | counter | `resource` | snapshots removidos por geração, RBAC ou reset explícito |
| `kubepeep_collection_cache_entries` | gauge | — | páginas de coleção retidas em memória |
| `kubepeep_collection_cache_bytes` | gauge | — | bytes serializados dessas páginas |
| `kubepeep_collection_cache_hits_total` | counter | `resource` | páginas autorizadas encontradas e cobertas por watch |
| `kubepeep_collection_cache_misses_total` | counter | `resource` | páginas ausentes, expiradas ou sem cobertura de watch |
| `kubepeep_collection_cache_evictions_total` | counter | `resource` | páginas removidas por LRU/budget |
| `kubepeep_collection_cache_invalidations_total` | counter | `resource` | páginas removidas por geração ou alteração do watch |
| `kubepeep_watch_active` | gauge | `resource` | workers de watch ativos |
| `kubepeep_watch_reconnects_total` | counter | `resource` | novas conexões após a inicial |
| `kubepeep_watch_expired_total` | counter | `resource` | 410/RV expirado observado |
| `kubepeep_watch_events_total` | counter | `resource` | eventos aceitos pelo worker |
| `kubepeep_watch_lag_milliseconds` | gauge | `resource` | lag da última mudança recebida |
| `kubepeep_kubernetes_requests_total` | counter | `traffic`, `status` | chamadas client-go reais |
| `kubepeep_kubernetes_request_duration_nanoseconds_total` | counter | `traffic`, `status` | soma da latência de transporte |
| `kubepeep_kubernetes_response_bytes_total` | counter | `traffic`, `status` | bytes lidos do body real |
| `kubepeep_kubernetes_429_total` | counter | `traffic` | respostas HTTP 429 |
| `kubepeep_client_throttle_total` | counter | `traffic` | esperas no rate limiter client-side |
| `kubepeep_client_throttle_nanoseconds_total` | counter | `traffic` | tempo total dessas esperas |
| `kubepeep_trace_export_errors_total` | counter | — | falhas sanitizadas do exporter OTel |

A duração de LIST é observada exatamente uma vez por operação no backend,
inclusive em erro. `received ÷ returned` mede over-fetch; quando `returned=0`,
a razão deve ser tratada como indefinida, não como zero. Percentis não são
inferidos desses counters: p50/p95 vêm do runner reproduzível da baseline.

As séries `kubepeep_resource_cache_*` e `kubepeep_collection_cache_*`
começaram na F2; não fazem parte da medição histórica da F0. O cache de
páginas só responde se o watch ativo cobre todas as origens da página e cada
origem passa novamente pela autorização. Uma primeira página inteira que cabe
no limite pode ser reconstruída diretamente do snapshot do watch, sem LIST;
páginas maiores preservam o LIST paginado. Sem watch, Refresh executa LIST.

### 3.3 Endpoint `/metrics`

O endpoint é desabilitado por padrão. Quando configurado, o servidor continua
exclusivo em loopback:

```yaml
version: 1
observability:
  metrics:
    enabled: true
```

## 4. Métricas UX e budgets

### 4.1 Política de freshness da Fase 2

| Fonte | Revalidação | Limite/condição |
| --- | --- | --- |
| recursos core com stream ativo | WATCH após snapshot inicial | compartilhamento por identidade completa; idle de 45 s; sem polling implícito se WATCH indisponível |
| páginas de coleção cobertas por WATCH | cache em memória por até 30 s | reautorização por origem em cada hit; delta, 403 e troca de geração invalidam |
| CPU/memória do overview | 8 s | somente após Tier 1; suspenso em background |
| capabilities/RBAC | TTL de 30–60 s (45 s padrão) | 403/revogação e generation exigem revalidação/invalidação |
| discovery da Metrics API | 10 min | generation troca o cliente e seu cache |
| análise de logs | sob ação explícita | nunca bloqueia Tier 1 nem inicia varredura global |

O botão Refresh consulta o Kubernetes quando não há watch cobrindo
integralmente as origens. A UI agrupa deltas em janelas de 2 s antes de
revalidar a página HTTP; o watch e sua fila fazem coalescing por objeto para
os tópicos level-driven, preservando Events cronológicos. Páginas completas e
pequenas são reconstruídas do snapshot local após cada delta; páginas maiores
ainda usam LIST paginado.

### 4.2 Métricas no cliente

`web/src/observability/uxMetrics.ts` mantém no máximo 256 amostras apenas em
memória do WebView/browser. Não há POST, persistência ou exporter. A inspeção
manual usa `window.__KUBEPEEP_UX_METRICS__.snapshot()` e o reset usa
`.reset()`.

Cada tentativa de LIST recebe uma identidade interna efêmera. A conclusão da
request e o array exato retornado ficam associados apenas em memória; o
`DataTable` registra a primeira linha no commit desse mesmo array. A identidade
não entra na amostra. Assim, cache hit não duplica medição, refetch com o mesmo
número de linhas permanece distinto, retry descarta a tentativa falha e
requests concorrentes na mesma view não sobrescrevem estado entre si.

| Métrica | Unidade | Momento |
| --- | --- | --- |
| `time_to_first_row` | ms | primeiro commit React de tabela não vazia |
| `time_to_page_complete` | ms | conclusão da request da página |
| `filter_interaction_latency` | ms | Apply até conclusão da página |
| `sort_interaction_latency` | ms | Apply de sort/order até conclusão |
| `rendered_row_count` | linhas | linhas realmente passadas à tabela |

A dimensão `view` usa somente `overview`, `pods`, `workloads`, `events`,
`network`, `config`, `configuration`, `storage`, `nodes`, `leases`, `access`,
`administration`, `service-accounts` ou `other`; pathname completo,
namespace, nome, UID e identidade não entram na amostra.

Budgets definidos para comparação F0→F7 (p95, máquina do laboratório sem
throttle artificial de CPU):

| Porte | Dataset de referência | primeira linha | página completa | filtro/sort | linhas renderizadas antes da F3 |
| --- | --- | ---: | ---: | ---: | ---: |
| pequeno | ≤10 namespaces, ≤5.000 Pods | <250 ms | <750 ms | <200 ms | ≤100 |
| médio | ≤50 namespaces, ≤25.000 Pods | **<500 ms** | **<1.500 ms** | <300 ms | ≤100 |
| grande | ≤200 namespaces, ≤100.000 Pods, LIST global autorizado | <1.000 ms | <3.000 ms | <500 ms | ≤100 |

O budget médio é o contrato v0.7. O porte grande não amplia o fan-out público:
segue a separação C02. A baseline sintética mede o backend; os budgets UX só
podem ser julgados por browser/desktop real e não são “aprovados” por números
do runner sintético.

## 5. Traces OpenTelemetry

OTel é opt-in (`observability.otel.enabled`, default `false`). Quando desligado,
não cria exporter, socket nem goroutine. Quando ligado, usa provider local,
endpoint validado, sem proxy, sem headers arbitrários, sem retry, timeout de 2
segundos e fila limitada; erro de exportação incrementa a série sanitizada.

Spans emitidos por componentes existentes:

```text
resources.list
resources.list.global | resources.list.fanout
resources.list.origin
resources.merge
cursor.get | cursor.put
watch.connect | watch.reconnect
cache.snapshot | cache.apply_event
cache.authorization | cache.clients
```

Atributos aceitos são somente `strategy`
(`global|fanout|global-native|namespace-sequential|lazy-merge`) e inteiros
positivos `namespace_count`, `page_size`, `origin_chunk_size`, `fanout`,
`items_count`.
Caches podem acrescentar apenas `cache.outcome` em
`hit|miss|coalesced|refresh`; a chave/valor do cache nunca é exportada. Erros
marcam status genérico `operation failed`, sem copiar mensagem upstream.

### 5.1 Contrato do resource cache da Fase 2 (C04)

`cache.snapshot` é emitido ao assinar e armazenar um snapshot real;
`cache.apply_event` é emitido quando o worker aplica um delta ao snapshot local.
Nenhum dos dois inclui identidade de objeto ou conteúdo no span. A integração
da F2 deve:

- usar `cache.snapshot` para construir/obter snapshot e `cache.apply_event`
  para aplicar evento autorizado;
- limitar atributos aos inteiros acima e ao `cache.outcome` fechado;
- nunca anexar `WatchKey`, selector, context, scope, namespace, RV, nome ou UID;
- emitir atividade somente em operação real, com testes de hit/miss,
  invalidação, erro e ausência total quando OTel estiver desabilitado.

## 6. Protocolo de inspeção

```sh
make benchmark              # sintético representativo, JSON em .state/
make benchmark-matrix       # matriz sintética completa
./test/kind/harness.sh static
./test/kind/harness.sh validate   # somente com Kind disponível
```

O protocolo, dimensões, SHA, ambiente, warmup e repetições ficam em
`docs/performance-baseline.md` e no próprio JSON. Kind real, sintético e Wails
nativo são resultados separados.

## 7. Testes

| Garantia | Cobertura |
| --- | --- |
| schema/sanitização/rotação dos logs | `internal/logging` |
| allowlists, gauges concorrentes, limite e render determinístico | `internal/observability` |
| status, bytes, 429 e throttle reais do client-go | `internal/adapters/kubernetes` |
| estratégia e uma duração por LIST | `internal/integration/kubernetesruntime` |
| TTL/LRU/budget/hit/miss/expired do cursor | `internal/api` |
| active/reconnect/410/event/lag de watch | `internal/services/resources` |
| spans e atributos sem dados sensíveis | `internal/observability/tracing_test.go` |
| métricas UX, vocabulário e limite de amostras | `web/src/observability/uxMetrics.test.ts` |
| matriz sintética, C02 e sanitização do relatório | `test/kind/benchmark` |
