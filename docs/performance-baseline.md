# Baseline de performance — Fase 0 v0.7

Esta é a baseline anterior às mudanças de estratégia das Fases 1–6. O código e
o binário medidos correspondem ao commit funcional
`6a8eead83e01a42ef0ceb963a270eede5d61f664`; a coleta representativa foi
feita com worktree limpa. Os JSONs completos ficam em `test/kind/.state/`,
são locais, sanitizados e ignorados pelo Git. Este documento preserva os
agregados necessários para comparação.

## 1. Ambiente e protocolo

| Campo | Valor |
| --- | --- |
| Coleta representativa | `2026-09-12T07:21:15Z` |
| Coleta da matriz | `2026-09-12T07:21:32Z` |
| Commit sob teste | `6a8eead83e01a42ef0ceb963a270eede5d61f664` |
| Estado da árvore nas duas coletas | `clean` |
| Modo | sintético, schema `kubepeep-performance-baseline/v1` |
| Ambiente | Linux/amd64, Go `go1.26.7`, 12 CPUs, `GOMAXPROCS=12` |
| Memória do host | 41.716.428.800 bytes |
| Aquecimento e amostras | 1 execução descartada + 5 repetições por cenário |
| Estatística | p50/p95; latências em milissegundos e memória em bytes |

O runner usa os caminhos reais `resources.Collect` e `api.CursorStore`. Lister,
autorização, latência, falhas e o probe de scheduler/watch são sintéticos. TTFB
é a primeira resposta LIST do upstream sintético; `first_row` é o instante em
que `Collect` devolve uma página não vazia; `full_page` é o término de
`Collect`. Portanto, `first_row` desta suíte **não** mede commit React, pintura
do browser ou WebView.

Todas as cinco repetições precisam produzir o mesmo outcome esperado. Uma
divergência vira `unstable`; um outcome unânime inesperado vira
`unexpected_error`; ambos fazem runner e harness terminarem com status não
zero, preservando o relatório para diagnóstico. O relatório contém apenas
contagens, códigos fechados e recursos do ambiente — nunca nomes de recursos,
namespaces, UIDs, tokens ou payloads.

Contratos preservados:

- **C01:** `filterScope=page` continua honesto; estes números não afirmam
  ordenação global de uma coleção ainda não lida.
- **C02:** o teto público de fan-out restrito permanece 100. O cenário
  `global-200` usa uma única origem all-namespaces autorizada;
  `restricted-200-rejected` comprova a rejeição legítima; 200 origens diretas
  existem apenas no stress interno `internal-200-origins`.
- **C04:** list, merge, cursor, watch e caches existentes são medidos na F0. O
  resource cache da F2 tem contrato definido, mas não emite atividade
  simulada nesta baseline.

## 2. Suíte sintética representativa

### 2.1 Cenários e outcomes

| Cenário | Perfil/escopo | Dataset | Página | Latência/falha/watch | Status e outcomes (5 repetições) |
| --- | --- | ---: | ---: | --- | --- |
| `global-200` | global público autorizado | 200 × 100 | 100 | 0 ms / nenhuma / on | `ok`; `success:5` |
| `namespace-100` | namespace-only público restrito | 100 × 100 | 100 | 20 ms / nenhuma / off | `ok`; `success:5` |
| `mixed-100` | misto público restrito | 100 × 100 | 50 | 50 ms / nenhuma / on | `ok`; `success:5` |
| `authorization-unavailable` | autorização indisponível | 10 × 10 | 25 | 0 ms / nenhuma / off | `expected_error`; `AUTHORIZATION_UNAVAILABLE:5` |
| `restricted-200-rejected` | namespace-only, limite público | 200 × 10 | 100 | 0 ms / nenhuma / off | `expected_error`; `VALIDATION_FAILED:5` |
| `internal-200-origins` | stress interno sintético | 200 × 10 | 100 | 0 ms / nenhuma / off | `ok`; `success:5` |
| `global-429` | global público autorizado | 25 × 500 | 100 | 100 ms / 429 / off | `expected_error`; `CLUSTER_UNAVAILABLE:5` |
| `global-410` | global público autorizado | 25 × 500 | 100 | 200 ms / 410 / on | `expected_error`; `CURSOR_EXPIRED:5` |
| `global-timeout` | global público autorizado | 10 × 100 | 50 | 50 ms / timeout / off | `expected_error`; `UPSTREAM_TIMEOUT:5` |
| `global-reset` | global público autorizado | 10 × 100 | 50 | 20 ms / reset / off | `expected_error`; `CLUSTER_UNAVAILABLE:5` |

`Dataset` usa `namespaces × Pods por namespace`. Falhas esperadas são parte do
contrato e não regressões do runner.

### 2.2 Latência, tráfego e over-fetch

| Cenário | TTFB p50/p95 (ms) | First row p50/p95 (ms) | Full page p50/p95 (ms) | Requests p50 | Bytes p50 | Over-fetch p50 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `global-200` | 0,123 / 0,150 | 0,143 / 0,170 | 0,143 / 0,170 | 1 | 18.601 | 1 |
| `namespace-100` | 20,373 / 20,460 | 507,930 / 508,069 | 507,930 / 508,069 | 100 | 186.100 | 10 |
| `mixed-100` | 50,625 / 50,938 | 758,426 / 760,736 | 758,426 / 760,736 | 60 | 111.660 | 12 |
| `authorization-unavailable` | — | — | 0,041 / 0,048 | 0 | 0 | — |
| `restricted-200-rejected` | — | — | 0,022 / 0,025 | 0 | 0 | — |
| `internal-200-origins` | 0,194 / 0,217 | 1,341 / 1,503 | 1,341 / 1,503 | 200 | 372.200 | 20 |
| `global-429` | 100,337 / 100,772 | — | 100,356 / 100,791 | 1 | 4 | — |
| `global-410` | 200,367 / 200,629 | — | 200,406 / 200,647 | 1 | 4 | — |
| `global-timeout` | 50,265 / 50,333 | — | 50,280 / 50,350 | 1 | 4 | — |
| `global-reset` | 20,201 / 20,217 | — | 20,220 / 20,239 | 1 | 4 | — |

`—` significa que não houve amostra aplicável, e não latência zero. Over-fetch
é `items_received / items_returned`; quando nenhum item é devolvido, a razão é
indefinida.

### 2.3 Memória, concorrência, cursor e watch

Todos os valores abaixo são p50. Alocação total e delta de heap são snapshots
do processo Go durante o cenário, não RSS nem limite de memória do host.

| Cenário | Alocação total (B) | Delta heap (B) | Delta do pico de goroutines | Cursor (B) | Falhas parciais | Watch lag p50/p95 (ms) |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `global-200` | 138.528 | 138.528 | 2 | 168 | 0 | 0 / 0 |
| `namespace-100` | 6.196.168 | 3.105.272 | 100 | 184.095 | 0 | — |
| `mixed-100` | 3.736.240 | 2.858.840 | 98 | 116.580 | 40 | 1,062 / 1,082 |
| `authorization-unavailable` | 18.536 | 18.536 | 0 | 0 | 10 | — |
| `restricted-200-rejected` | 8.840 | 8.840 | 0 | 0 | 0 | — |
| `internal-200-origins` | 12.567.712 | 3.023.128 | 200 | 383.495 | 0 | — |
| `global-429` | 4.688 | 4.688 | 1 | 0 | 1 | — |
| `global-410` | 8.240 | 8.240 | 2 | 0 | 0 | 1,033 / 1,066 |
| `global-timeout` | 4.840 | 4.840 | 1 | 0 | 1 | — |
| `global-reset` | 4.848 | 4.848 | 1 | 0 | 1 | — |

Os quatro cenários bem-sucedidos que precisam de continuação mantiveram uma
entrada de cursor. O caminho global de 200 namespaces armazenou apenas 168 B;
o stress de 200 origens armazenou 383.495 B, abaixo do budget individual de
1 MiB. O store continua limitado a 1.024 entradas e 32 MiB totais, com TTL,
LRU e purge.

## 3. Matriz sintética completa

A matriz contém 47 cenários selecionados (não o produto cartesiano de todas as
dimensões):

- namespaces: 1, 10, 25, 50, 100 e 200;
- Pods por namespace: 10, 50, 100, 250 e 500;
- páginas: 25, 50 e 100;
- latência: 0, 20, 50, 100 e 200 ms;
- falhas: nenhuma, 429, 410, timeout e reset;
- perfis: global, namespace-only, mixed, authorization-unavailable e stress
  interno de origens;
- watch: on e off; escopos contratuais público global, público restrito,
  rejeição de limite e interno sintético.

| Resultado agregado | Valor |
| --- | ---: |
| Cenários `ok` | 29 |
| Cenários `expected_error` | 18 |
| Cenários `unstable` | 0 |
| Cenários `unexpected_error` | 0 |
| Outcomes `success` | 145 |
| `AUTHORIZATION_UNAVAILABLE` | 30 |
| `CLUSTER_UNAVAILABLE` | 20 |
| `CURSOR_EXPIRED` | 10 |
| `UPSTREAM_TIMEOUT` | 10 |
| `VALIDATION_FAILED` | 20 |

## 4. Microbenchmarks Go

Coleta local no mesmo commit e ambiente, com árvore limpa. Estes são resultados
de uma invocação adaptativa do benchmark Go (`ns/op`, `B/op`, `allocs/op`),
não percentis da suíte de cinco repetições.

### 4.1 CursorStore Put/Get

| Origens | ns/op | B/op | allocs/op |
| ---: | ---: | ---: | ---: |
| 10 | 29.610 | 15.843 | 235 |
| 50 | 123.182 | 62.243 | 961 |
| 100 | 232.709 | 127.539 | 1.865 |
| 200 | 445.082 | 259.588 | 3.670 |

### 4.2 Merge determinístico de páginas

| Origens | Chunk | ns/op | B/op | allocs/op |
| ---: | ---: | ---: | ---: | ---: |
| 10 | 10 | 10.012 | 7.864 | 45 |
| 10 | 50 | 19.966 | 15.224 | 45 |
| 50 | 10 | 55.178 | 29.432 | 205 |
| 50 | 50 | 85.549 | 66.232 | 205 |
| 100 | 10 | 105.930 | 57.032 | 405 |
| 100 | 50 | 179.731 | 130.632 | 405 |
| 200 | 10 | 232.277 | 116.456 | 805 |
| 200 | 50 | 379.306 | 263.656 | 805 |

## 5. Kind real

A evidência de cluster real é deliberadamente qualitativa; ela não reutiliza
latências do runner sintético.

| Comando | Resultado observado no commit funcional |
| --- | --- |
| `./test/kind/harness.sh static` | fixtures, imagens pinadas, RBAC e invariantes estáticas aprovadas |
| `./test/kind/harness.sh validate` | matriz Kubernetes/RBAC F4–F7 aprovada no cluster dedicado `kubepeep-f4` |
| `./test/kind/harness.sh app-e2e ./dist/kubePeep` | contextos/scopes, dashboard, SSE de recursos/logs, WebSocket exec, revogação e modo offline aprovados contra API Kubernetes real |

O binário usado carregava o commit `6a8eead`. Nenhum dataset grande foi
aplicado automaticamente; o gerador só produz manifesto mediante comando
explícito. O cluster `kubepeep-f4` foi preservado, conforme o contrato do
harness.

## 6. Desktop/Wails e budgets UX

**Não medido nesta coleta.** Não foi coletado tempo de startup da janela,
WebView, commit React, pintura, uso de GPU nem RSS do processo desktop. O gate
`make build smoke` cobre o binário servidor/CLI, e `make verify` cobre a UI
React compartilhada e o contrato do bridge desktop; nenhum desses resultados
é apresentado como benchmark Wails nativo.

Os budgets de primeira linha, página completa, filtro/sort e linhas
renderizadas estão definidos em `docs/observability.md` para portes pequeno,
médio e grande. Eles são critérios para uma coleta browser/desktop real e
**não** foram aprovados pelos números sintéticos deste documento.

## 7. Reprodução

```sh
make benchmark
make benchmark-matrix

go test -run '^$' -bench '^BenchmarkCursorStorePutGet$' -benchmem ./internal/api
go test -run '^$' -bench '^BenchmarkMergeOriginPages$' -benchmem ./internal/services/resources

./test/kind/harness.sh static
./test/kind/harness.sh validate
./test/kind/harness.sh app-e2e ./dist/kubePeep
```

Para gerar — sem aplicar — um dataset Kind determinístico:

```sh
make benchmark-dataset BENCHMARK_NAMESPACES=10 BENCHMARK_PODS_PER_NAMESPACE=100
```

Comparações futuras devem registrar novo SHA, estado da árvore, ambiente,
dataset, autorização, página, aquecimento e repetições. Se alguma condição
mudar, a diferença deve ser declarada em vez de misturar as séries.

## 8. Limitações e warnings conhecidos

- Latência e watch sintéticos validam contratos e regressões; não modelam
  jitter de rede, etcd, admission webhooks ou contenção real do API Server.
- As coletas não fixaram frequência de CPU nem isolaram o host; diferenças
  pequenas exigem múltiplas execuções antes de qualquer decisão algorítmica.
- O build frontend mantém o warning conhecido `INEFFECTIVE_DYNAMIC_IMPORT` em
  `src/api/client.ts` e um chunk principal de aproximadamente 633 kB; o
  tratamento está fora da F0.
- Na coleta original da F0 havia nove warnings ESLint e duas vulnerabilidades
  moderadas de desenvolvimento. No fechamento geral em `52aa3db`, o lint
  manteve oito warnings Fast Refresh e zero erro; Vitest 4.1.11 e `undici`
  8.11.2 levaram `npm audit` completo a zero vulnerabilidades. O aviso jsdom de
  `HTMLCanvasElement.getContext()` sem o pacote `canvas` continua informativo.

## 9. Comparativo final da Fase 7

A coleta posterior às Fases 1–6 foi executada em 29 de setembro de 2026 no
commit `b148a6028dd800e30b9cc54a1cd14a8c862f1fac`, com árvore limpa, no mesmo
host, versão de Go, arquitetura, quantidade de CPUs, `GOMAXPROCS`, memória,
aquecimento e cinco repetições da baseline. O schema passou para
`kubepeep-performance-baseline/v2` apenas para explicitar `http_429`,
`cache_hits` e `watch_reconnects`; os demais campos e o protocolo de medição
permaneceram comparáveis.

### 9.1 Antes e depois nos caminhos bem-sucedidos

| Cenário | Full page p95 antes → depois | Requests p50 | Bytes p50 | Over-fetch p50 | Alocação p50 | Pico de goroutines p50 | Cursor p50 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `global-200` | 0,170 → 0,144 ms (-15,3%) | 1 → 1 | 18.601 → 18.601 | 1 → 1 | 138.528 → 175.064 B | 2 → 1 | 168 → 168 B |
| `namespace-100` | 508,069 → 61,363 ms (-87,9%) | 100 → 12 | 186.100 → 22.332 | 10 → 1,2 | 6.196.168 → 636.312 B | 100 → 5 | 184.095 → 14.959 B |
| `mixed-100` | 760,736 → 101,773 ms (-86,6%) | 60 → 6 | 111.660 → 11.166 | 12 → 1,2 | 3.736.240 → 486.384 B | 98 → 6 | 116.580 → 12.792 B |
| `internal-200-origins` | 1,503 → 0,535 ms (-64,4%) | 200 → 12 | 372.200 → 22.332 | 20 → 1,2 | 12.567.712 → 987.256 B | 200 → 5 | 383.495 → 25.355 B |

O caminho global já era unitário; a pequena variação de alocação é ruído de
uma suíte sintética sem isolamento de CPU. Nos três caminhos de fan-out, tempo,
requests, bytes, over-fetch, alocação, concorrência e cursor caíram juntos. O
delta final de goroutines foi zero em todos os dez cenários.

### 9.2 Falhas, watch e contadores explícitos

| Cenário | Full page p95 | Requests p50 | HTTP 429 p50 | Falhas parciais p50 | Watch lag p50/p95 |
| --- | ---: | ---: | ---: | ---: | ---: |
| `authorization-unavailable` | 0,087 ms | 0 | 0 | 10 | — |
| `restricted-200-rejected` | 0,026 ms | 0 | 0 | 0 | — |
| `global-429` | 101,055 ms | 1 | 1 | 1 | — |
| `global-410` | 201,043 ms | 1 | 0 | 0 | 1,022 / 1,040 ms |
| `global-timeout` | 50,211 ms | 1 | 0 | 1 | — |
| `global-reset` | 20,275 ms | 1 | 0 | 1 | — |

Cada falha continuou com um único request, sem amplificação. `cache_hits=0` e
`watch_reconnects=0` são resultados explícitos: este laboratório chama
`resources.Collect` diretamente, não possui o cache de produção, e o probe de
watch saudável não reconecta. Cache hit, reconexão e eviction continuam
cobertos nas suítes dos componentes e nos Diagnostics; atribuir atividade
artificial ao runner violaria C04.

### 9.3 Matriz, protocolo e aceite real

A matriz final repetiu os 47 cenários: 29 `ok`, 18 `expected_error`, zero
instável e zero erro inesperado. Foram 145 outcomes `success`, 30
`AUTHORIZATION_UNAVAILABLE`, 20 `CLUSTER_UNAVAILABLE`, 10 `CURSOR_EXPIRED`,
10 `UPSTREAM_TIMEOUT` e 20 `VALIDATION_FAILED`.

O protocolo Kind real no mesmo SHA e árvore limpa observou 23 Pods. Partial
metadata usou 69.590 bytes contra 142.634 do JSON; Protobuf, 95.503 bytes; gzip,
21.037 bytes. No ensaio determinístico de 429, concorrência fixa produziu 9
respostas e AIMD produziu 2, com duas reduções e quatro recuperações. As
decisões F6 permanecem válidas.

O harness Kind completo passou com contexto/scope, dashboard, recursos,
SSE/log, WebSocket exec, revogação/restauração de RBAC e modo offline. No
binário Wails atual, uma execução nativa isolada registrou shell pronto em
431,9 ms; após restaurar o default persistido, a árvore AT-SPI da tela Pods
expôs uma tabela autorizada com nove linhas reais do namespace `kp-allowed`.
Esta inspeção comprova R01 no desktop, mas a amostra única de shell não
substitui a série de startup registrada na F3.
