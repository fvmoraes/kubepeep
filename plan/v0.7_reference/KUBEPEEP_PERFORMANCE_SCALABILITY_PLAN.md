# KubePeep — Plano de Performance, Paginação, Cache e Escalabilidade

> Documento técnico para evolução da camada de leitura de recursos Kubernetes do KubePeep.
>
> **Objetivo principal:** reduzir drasticamente o tempo para exibir recursos, eliminar `LIMIT_EXCEEDED` causado pelo cursor composto, diminuir o número de chamadas ao `kube-apiserver`, manter a interface responsiva em clusters grandes e preservar o comportamento correto em ambientes com RBAC restritivo.
>
> Repositório analisado: `https://github.com/fvmoraes/kubepeep`
>
> Pesquisa e revisão: setembro de 2026.

---

## 1. Resumo executivo

O KubePeep já possui algumas decisões corretas de arquitetura:

- usa `limit` + `continue` nas operações Kubernetes;
- possui fan-out concorrente limitado;
- possui tentativa de `LIST` global quando o RBAC permite;
- utiliza `resourceVersion`;
- já possui `WATCH`;
- já possui backoff exponencial com jitter no fluxo de watch;
- usa React Query no frontend;
- possui limites explícitos de payload e timeout.

O principal problema observado atualmente não é simplesmente “falta de goroutines”.

O gargalo está principalmente na combinação de:

1. **over-fetch por namespace**;
2. **fan-out para muitas origens**;
3. **buffers de DTOs armazenados dentro do cursor HTTP**;
4. **cursor composto serializado no cliente**;
5. **paginação global feita depois que cada origem já trouxe seu próprio lote**;
6. **dependência de múltiplas checagens/autorização e chamadas quando o `LIST` global não está disponível**;
7. **repetição de snapshots quando `WATCH` não pode ser utilizado**;
8. **renderização de listas grandes sem virtualização dedicada**.

A principal evolução recomendada é transformar a arquitetura de:

```text
Frontend
   |
   v
LIST em várias origens
   |
   v
buscar N itens de cada origem
   |
   v
merge
   |
   v
guardar sobras dentro do cursor
   |
   v
devolver cursor enorme
```

para:

```text
Frontend
   |
   v
Resource Query Service
   |
   +--> fast path: LIST global paginado
   |
   +--> fallback: lazy namespace paginator
   |
   +--> cache/watch local sob demanda
   |
   v
Cursor Store local
   |
   v
token opaco pequeno
```

A recomendação é executar as mudanças em quatro níveis:

- **P0 — corrigir arquitetura do cursor e over-fetch**;
- **P1 — melhorar paginação e estratégia de merge**;
- **P2 — introduzir cache/watch eficiente e UX progressiva**;
- **P3 — otimizações avançadas de protocolo, métricas e adaptação dinâmica**.

---

# 2. Diagnóstico do código atual

## 2.1. Cursor composto contém objetos completos

Arquivo relevante:

```text
internal/services/resources/cursor.go
```

Hoje cada origem possui estado semelhante a:

```go
type OriginCursor[T ListItem] struct {
    Origin          Origin
    Continue        string
    ResourceVersion string
    Exhausted       bool
    Buffered        []T
}
```

O campo mais problemático é:

```go
Buffered []T
```

Isso significa que recursos já buscados do Kubernetes, mas ainda não enviados à UI, permanecem serializados no cursor.

Depois o cursor inteiro é convertido para JSON.

Existe ainda o limite:

```go
if len(encoded) > 12<<10 {
    return domainError(
        CodeLimitExceeded,
        "The composed cursor exceeded its safe size.",
        nil,
    )
}
```

Portanto o erro:

```text
LIMIT_EXCEEDED
The composed cursor exceeded its safe size.
```

é consequência direta do modelo atual.

### Conclusão

**Não aumentar o limite de 12 KiB como solução principal.**

Aumentar para 32 KiB, 64 KiB ou 256 KiB apenas adiaria a falha e manteria:

- over-fetch;
- mais CPU para JSON/base64/HMAC;
- mais tráfego backend → frontend;
- mais memória;
- mais latência;
- mais risco de ultrapassar limites futuros.

---

# 3. Como o over-fetch acontece

Arquivo:

```text
internal/services/resources/list.go
```

O KubePeep já possui fan-out limitado:

```go
semaphore := make(chan struct{}, MaximumFanout)
```

e cada origem pode executar:

```go
ListPage(
    ctx,
    PageRequest{
        Origin:   state.Origin,
        Limit:    int64(request.Options.Limit),
        Continue: state.Continue,
    },
)
```

O problema é que o `limit` da página final também é utilizado como `limit` de cada origem.

Exemplo:

```text
50 namespaces
page size da UI = 100
```

O comportamento pode chegar próximo de:

```text
namespace-a -> 100
namespace-b -> 100
namespace-c -> 100
...
namespace-50 -> 100
```

Mesmo com apenas quatro chamadas simultâneas, o sistema eventualmente pode consumir milhares de objetos para produzir apenas 100 itens da página.

Exemplo extremo:

```text
50 namespaces × 100 objetos = 5.000 objetos buscados

UI precisava: 100
```

Os demais precisam ficar em algum lugar.

Hoje ficam principalmente no:

```text
OriginCursor.Buffered
```

Isso explica simultaneamente:

- cursor grande;
- lentidão;
- alto uso de memória;
- muitas chamadas ao API Server;
- maior volume de JSON;
- maior custo de merge/sort;
- maior chance de timeout.

---

# 4. Concorrência: goroutines ajudam, mas não são a solução principal

O KubePeep já usa goroutines.

Portanto:

> aumentar goroutines sem mudar o algoritmo não resolve o problema estrutural.

Pode até piorar.

Mais concorrência significa:

- mais pressão sobre o `kube-apiserver`;
- maior chance de HTTP 429;
- maior disputa no client-side rate limiter;
- mais CPU local;
- mais respostas simultâneas para decodificar;
- mais memória temporária;
- maior probabilidade de bursts.

A regra deve ser:

```text
primeiro reduzir trabalho
depois paralelizar o trabalho restante
```

e não:

```text
executar mais rapidamente um volume desnecessariamente alto de trabalho
```

---

# 5. Fast path: priorizar LIST global sempre que autorizado

O código atual já possui uma excelente base para isso em:

```text
internal/integration/kubernetesruntime/resources_backend.go
```

Existe lógica com:

```go
resolution.PreferGlobal
```

e:

```go
globalListDecision(...)
```

Quando o usuário tem:

```text
list <resource> --all-namespaces
```

o KubePeep pode utilizar apenas uma origem global.

Esse deve continuar sendo o caminho preferencial.

## Comportamento desejado

```text
RBAC permite LIST global?
        |
       sim
        |
        v
GET /api/v1/pods?limit=50
        |
        v
continue token nativo
```

Isso é muito melhor do que:

```text
namespace-a
namespace-b
namespace-c
...
namespace-n
```

## Adicionar observabilidade

Registrar em métricas:

```text
kubepeep_resource_list_strategy_total{
    strategy="global"
}

kubepeep_resource_list_strategy_total{
    strategy="namespace_fanout"
}
```

Assim será possível saber quanto do uso real está caindo no caminho caro.

---

# 6. P0 — substituir o cursor pesado por cursor opaco server-side

Esta é a mudança de maior prioridade.

## 6.1. Arquitetura proposta

Frontend recebe:

```json
{
  "items": [],
  "nextCursor": "kp_01JXYZ..."
}
```

Em vez de receber estado completo.

Backend mantém:

```text
kp_01JXYZ...
    |
    +--> generation
    +--> context
    +--> scope
    +--> resource/GVR
    +--> filters
    +--> ordering
    +--> per-origin continuation
    +--> pequenos buffers
    +--> expiresAt
```

## 6.2. Cursor Store local

Como KubePeep é uma aplicação local, não é necessário Redis.

Usar algo simples:

```go
type CursorStore interface {
    Put(state CursorState, ttl time.Duration) (string, error)
    Get(token string) (CursorState, bool)
    Delete(token string)
    PurgeExpired()
}
```

Implementação inicial:

```text
map[string]*entry
sync.RWMutex
TTL
LRU
limite de entradas
limite total estimado de memória
```

Ou usar uma pequena estrutura LRU já madura.

## 6.3. Requisitos de segurança

O cursor opaco deve:

- ser aleatório e impraticável de adivinhar;
- usar pelo menos 128 bits de entropia;
- expirar;
- estar vinculado à geração atual;
- estar vinculado ao contexto;
- estar vinculado ao scope;
- estar vinculado à consulta/filtro/sort;
- ser invalidado ao trocar cluster/contexto;
- nunca armazenar secrets;
- não ser persistido em disco;
- ser apagado quando a geração for cancelada;
- possuir limite global de memória.

Sugestão:

```text
TTL padrão: 5 minutos
idle TTL: 2–5 minutos
máximo inicial: 512 ou 1.024 cursores
```

A configuração final deve ser baseada em benchmark.

---

# 7. P0 — corrigir o timeout inconsistente

Hoje existe:

```go
DefaultListWindowTimeout = 30 * time.Second
```

mas dentro de `Collect()` ainda existe fallback equivalente a:

```go
if timeout <= 0 {
    timeout = 10 * time.Second
}
```

Isso gera inconsistência entre contrato e execução.

Substituir por:

```go
timeout := NormalizeListWindowTimeout(request.Timeout)

requestContext, cancel := context.WithTimeout(ctx, timeout)
defer cancel()
```

Isso não resolve o problema de performance, mas elimina comportamento inesperado e simplifica tuning.

---

# 8. P0 — separar `page size da UI` de `chunk size por origem`

Hoje os dois conceitos acabam próximos demais.

Eles devem ser independentes.

Exemplo:

```text
UI page size     = 100
origin chunk     = 10
```

ou:

```text
UI page size     = 50
origin chunk     = 10
```

## Sugestão inicial

```go
const (
    DefaultUIPageSize       = 50
    DefaultOriginChunkSize  = 10
    MaxOriginChunkSize      = 50
)
```

Não tratar esses números como definitivos.

Eles devem ser validados por benchmark.

---

# 9. P1 — algoritmo especializado para ordenação Namespace + Name

O screenshot analisado usa:

```text
Namespace and name
Ascending
```

Esse caso possui uma otimização muito importante.

Se a ordenação principal é namespace, não é necessário consultar todos os namespaces ao mesmo tempo.

## Exemplo

Namespaces ordenados:

```text
alpha
backend
default
monitoring
production
```

A paginação pode funcionar assim:

```text
alpha
  chunk 1
  chunk 2
  acabou

backend
  chunk 1

atingiu 50 itens

STOP
```

O cursor precisa de muito pouco estado:

```json
{
  "namespaceIndex": 1,
  "continue": "TOKEN_K8S"
}
```

Com cursor server-side, nem isso precisa sair para a UI.

## Benefícios

- número muito menor de requests;
- praticamente nenhum over-fetch;
- cursor minúsculo;
- algoritmo simples;
- comportamento previsível;
- excelente para RBAC restritivo;
- primeira página aparece rapidamente.

## Descendente

Para descending por namespace, simplesmente inverter a ordem da lista de namespaces.

---

# 10. P1 — lazy k-way merge para ordenações globais

Para ordenações como:

```text
Age
Created
Restarts
CPU
Memory
Status
```

não é possível simplesmente consumir namespace por namespace.

Nesse caso usar **lazy k-way merge**.

## Ideia

Cada namespace/origem fornece apenas um pequeno chunk:

```text
ns-a -> 10
ns-b -> 10
ns-c -> 10
```

Cada lista está ordenada de acordo com o mesmo comparator.

O backend mantém o primeiro candidato de cada origem em um heap.

```text
              Heap
         /      |      \
      ns-a     ns-b    ns-c
```

Ao consumir o item de `ns-b`, o próximo item de `ns-b` entra no heap.

Só buscar uma nova página Kubernetes para `ns-b` quando o buffer daquele namespace estiver esgotando.

## Complexidade

Em vez de procurar linearmente entre todas as origens para cada item:

```text
O(pageSize × origins)
```

usar um heap:

```text
O(pageSize × log(origins))
```

Para poucas origens a diferença é pequena.

Para dezenas/centenas, melhora.

## Estrutura conceitual

```go
type originState[T any] struct {
    origin          Origin
    items           []T
    continueToken   string
    resourceVersion string
    exhausted       bool
}

type heapItem[T any] struct {
    originIndex int
    value       T
}
```

O importante é que:

> apenas origens que precisam ser reabastecidas fazem nova chamada.

---

# 11. P1 — worker pool bounded em vez de “mais goroutines”

Manter concorrência limitada.

O modelo pode evoluir para:

```text
jobs
 |
 v
bounded worker pool
 |
 +--- worker
 +--- worker
 +--- worker
 +--- worker
 |
 v
results
```

## Recomendações

Começar com:

```text
4 workers
```

e medir:

```text
4
6
8
```

Não assumir que 16 ou 32 será melhor.

## Nunca usar concorrência ilimitada

Evitar:

```go
for _, namespace := range namespaces {
    go fetch(namespace)
}
```

em scopes grandes.

---

# 12. Rate limiting do client-go

O `client-go` possui por padrão:

```text
DefaultQPS   = 5
DefaultBurst = 10
```

se `QPS/Burst` não forem configurados.

O KubePeep deve observar se está sendo limitado pelo próprio client.

## Adicionar métricas

Medir:

```text
client-side throttle duration
HTTP 429
requests/s
requests in flight
queue wait
```

## Tuning

Não alterar QPS/Burst no escuro.

Procedimento:

1. medir defaults;
2. medir cluster local pequeno;
3. medir cluster remoto;
4. medir cluster com 50+ namespaces;
5. observar 429 e latência;
6. aumentar moderadamente apenas se necessário.

Exemplo de faixa a testar:

```text
QPS 5 / Burst 10
QPS 10 / Burst 20
QPS 20 / Burst 40
```

**Não definir 100/200 apenas para “ficar mais rápido”.**

---

# 13. Respeitar pressão do kube-apiserver

Kubernetes utiliza API Priority and Fairness.

Em carga elevada o cliente pode observar:

```text
429 Too Many Requests
latência maior
requests enfileiradas
```

KubePeep deve:

- respeitar `Retry-After`;
- aplicar exponential backoff;
- adicionar jitter;
- evitar retry storm;
- cancelar retries quando a tela/contexto mudou;
- reduzir fan-out após 429 repetidos;
- nunca transformar timeout em loop agressivo de refresh.

---

# 14. P1 — Adaptive Concurrency

Depois da versão básica funcionar, pode ser criada uma política adaptativa.

Exemplo:

```text
começa com concurrency = 4

latência boa e sem 429:
    pode subir gradualmente até 6/8

429 ou timeout crescente:
    reduzir para 2/4

cluster recuperou:
    aumentar lentamente
```

Isso é similar a um controle AIMD:

```text
Additive Increase
Multiplicative Decrease
```

Exemplo conceitual:

```text
success stable -> +1
429            -> /2
timeout burst  -> /2
```

Limites:

```text
min = 2
default = 4
max = 8
```

Deve ser opcional e validado por benchmark antes de virar default.

---

# 15. P1 — Request coalescing

Se duas partes da UI pedirem ao mesmo tempo:

```text
pods / cluster X / scope Y / filtros Z
```

não executar duas consultas idênticas.

Criar um mecanismo `singleflight`.

Em Go:

```text
golang.org/x/sync/singleflight
```

Key:

```text
generation
context
scope
GVR
namespace filter
label selector
field selector
sort
page/cursor
```

## Ganho

Especialmente útil em:

- refresh;
- navegação rápida;
- vários componentes consumindo o mesmo recurso;
- dashboard;
- React re-mount;
- reconexão.

---

# 16. P1 — cancelar trabalho obsoleto imediatamente

Se o usuário:

- troca contexto;
- troca namespace;
- troca recurso;
- muda filtros;
- muda sort;
- navega para outra tela;

as consultas anteriores devem ser canceladas.

Não deixar goroutines antigas continuarem buscando recursos que ninguém mais verá.

Usar:

```go
context.Context
```

em toda a cadeia.

Frontend também deve abortar requests antigos usando o `AbortSignal` fornecido pelo React Query.

---

# 17. P1 — debounce de busca e filtros

Campo de search não deve fazer uma nova consulta Kubernetes a cada tecla.

Sugestão:

```text
debounce: 200–300 ms
```

Para filtros locais já carregados, o debounce pode ser ainda menor.

Quando filtro depende do backend:

```text
digitando "deploy"
d
de
dep
depl
deplo
deploy
```

deve resultar idealmente em 1–2 consultas, não 6.

---

# 18. P1 — enviar filtros ao Kubernetes quando possível

Sempre que a semântica permitir, usar:

```text
labelSelector
fieldSelector
```

no servidor.

Evitar:

```text
buscar 5.000
filtrar localmente para obter 20
```

quando o API Server consegue devolver os 20 diretamente.

## Atenção

Nem todos os campos são suportados por `fieldSelector`.

Criar uma matriz por recurso.

Exemplo conceitual:

| Recurso | Server-side label selector | Server-side field selector |
|---|---:|---:|
| Pods | sim | parcial |
| Services | sim | parcial |
| Deployments | sim | parcial |
| Events | sim | útil |
| CRDs | depende | depende |

Fallback para filtro local apenas quando necessário.

---

# 19. P2 — LIST + WATCH + cache local

Esta é provavelmente a evolução arquitetural mais importante depois do cursor.

Kubernetes foi desenhado para:

```text
LIST inicial
+
WATCH incremental
```

O KubePeep não deveria executar snapshots completos repetidamente enquanto a tela permanece aberta e o watch está autorizado.

## Fluxo ideal

```text
           Kubernetes API
                |
          initial LIST
                |
                v
         local resource cache
                |
                v
              React
                ^
                |
        ADDED / MODIFIED /
             DELETED
                ^
                |
              WATCH
```

## Resultado

Após sincronização inicial:

- criação de pod = 1 delta;
- mudança de status = 1 delta;
- delete = 1 delta;

em vez de:

```text
listar tudo novamente
```

---

# 20. SharedInformer: utilizar com cuidado

`client-go/tools/cache` fornece `SharedInformer` / `SharedIndexInformer`.

Ele mantém cache local eventualmente consistente e é exatamente o padrão comum do ecossistema Kubernetes.

Entretanto, para o KubePeep há uma consideração importante:

> KubePeep é um dashboard desktop interativo, não um controller que precisa observar tudo o tempo inteiro.

Portanto não iniciar informers de todos os GVRs de todos os clusters automaticamente.

Isso poderia causar:

- alto consumo de memória;
- muitos watches;
- maior pressão no API Server;
- trabalho desnecessário.

## Modelo recomendado

**Demand-driven shared watch/cache.**

Chave:

```text
generation
cluster/context
scope
GVR
selector
```

Abrir watch apenas quando uma tela precisa dele.

Usar:

```text
reference count
+
idle timeout
```

Exemplo:

```text
Pods page abre
    refCount = 1
    iniciar list/watch

Dashboard também precisa de pods
    refCount = 2
    reutilizar mesmo cache/watch

Pods page fecha
    refCount = 1

Dashboard fecha
    refCount = 0

espera 30–60 s
encerra watch/cache
```

Isso evita thrashing durante navegação.

---

# 21. Watch manager atual: preservar boas decisões

O KubePeep já possui comportamento relevante em:

```text
internal/services/resources/watch.go
```

Há:

```text
backoff inicial próximo de 250 ms
crescimento exponencial
máximo próximo de 10 s
jitter
reset após estabilidade
tratamento de ResourceExpired
```

Isso deve ser preservado.

A evolução deve focar em:

- compartilhamento de watches;
- bookmarks;
- cache;
- reconexão;
- fallback;
- métricas;
- capability detection.

Não reescrever essa parte sem necessidade.

---

# 22. WATCH bookmarks

Usar:

```text
allowWatchBookmarks=true
```

quando suportado.

Um `BOOKMARK` informa um `resourceVersion` até o qual os eventos já foram sincronizados.

Benefícios:

- checkpoints melhores;
- reconexão;
- redução do risco de depender de RV muito antigo;
- melhor observabilidade da sincronização.

Importante:

> o servidor não é obrigado a enviar bookmarks em intervalo fixo.

Portanto nunca implementar lógica que dependa de “um bookmark a cada X segundos”.

---

# 23. Streaming lists — otimização moderna do Kubernetes

Kubernetes moderno possui suporte a:

```text
sendInitialEvents=true
```

em WATCH.

Estado atual na documentação oficial pesquisada:

```text
Streaming Lists
Beta desde Kubernetes v1.34
habilitado por padrão
```

Fluxo:

```text
WATCH
sendInitialEvents=true
allowWatchBookmarks=true
resourceVersionMatch=NotOlderThan
```

O servidor envia:

```text
ADDED
ADDED
ADDED
...
BOOKMARK
...
eventos normais do watch
```

Isso permite unificar:

```text
LIST inicial + WATCH
```

em uma única stream.

## Recomendação para o KubePeep

Implementar como **fast path opcional**.

Nunca depender exclusivamente disso porque o KubePeep precisa funcionar com clusters Kubernetes mais antigos.

Estratégia:

```text
cluster suporta streaming list?
        |
       sim
        |
        v
sendInitialEvents
        |
       não
        |
        v
LIST + WATCH clássico
```

---

# 24. Sharded List and Watch

A documentação atual do Kubernetes registra:

```text
Sharded List and Watch
Alpha desde Kubernetes v1.36
desabilitado por padrão
```

Pode ser interessante futuramente para clusters extremamente grandes.

Para o KubePeep atual:

**não tornar dependência.**

Registrar apenas como futura otimização experimental.

---

# 25. P2 — metadata-only fetch

Kubernetes suporta:

```text
PartialObjectMetadata
PartialObjectMetadataList
```

Isso pode reduzir significativamente o payload quando a UI precisa apenas de:

```text
name
namespace
labels
annotations
creationTimestamp
resourceVersion
UID
```

Header:

```text
Accept:
application/json;as=PartialObjectMetadataList;g=meta.k8s.io;v=v1
```

Também existe versão Protobuf.

## Usar quando

- catálogo genérico;
- autocomplete;
- relações entre objetos;
- seleção;
- lookup;
- telas onde spec/status não são necessários.

## Não usar quando

a tabela precisa de:

```text
Pod status
restarts
container state
Deployment replicas
Service ports
etc.
```

Nesse caso o objeto completo ainda é necessário.

---

# 26. P2 — Kubernetes Protobuf para recursos built-in

Para APIs built-in suportadas, preferir:

```text
application/vnd.kubernetes.protobuf
```

com fallback:

```text
application/json
```

Exemplo de `Accept`:

```text
application/vnd.kubernetes.protobuf, application/json
```

Benefícios potenciais:

- menos bytes;
- menor custo de serialização/deserialização;
- menor CPU.

## Importante

Protobuf Kubernetes não está disponível para todos os tipos.

Em especial:

- CRDs não têm suporte ao Kubernetes Protobuf tradicional;
- alguns aggregated APIs podem não suportar.

Portanto sempre manter fallback JSON.

---

# 27. Compressão HTTP

Verificar se:

```text
APIResponseCompression
```

está sendo utilizada pelo servidor.

Para respostas JSON grandes, gzip pode reduzir bastante o tráfego.

Não assumir que compressão sempre melhora tudo.

Em conexões localhost:

```text
CPU de compressão pode custar mais que rede
```

Em cluster remoto:

```text
compressão tende a ser mais útil
```

Medir.

---

# 28. Não buscar detalhes completos durante a listagem

Padrão:

```text
lista = dados mínimos para linha
detalhes = fetch quando usuário abre
```

Evitar:

```text
listar 100 pods
+
100 GET de detalhes
+
100 GET de logs/status auxiliar
```

Isso é N+1.

## Regra

A tela de lista deve funcionar com os dados da própria operação de listagem.

Detalhes caros devem ser lazy.

---

# 29. Métricas Kubernetes devem ser tratadas separadamente

CPU e memória normalmente vêm do Metrics API, não diretamente do objeto Pod.

Não deixar uma lista de Pods bloquear esperando métricas.

Melhor UX:

```text
1. carregar pods
2. renderizar linhas
3. buscar métricas em paralelo
4. preencher CPU/RAM progressivamente
```

Se Metrics API não estiver disponível:

```text
pods continuam visíveis
CPU/RAM = unavailable
```

Isso segue o princípio já presente no KubePeep de tratar capabilities independentes.

---

# 30. P2 — frontend: usar `useInfiniteQuery`

O KubePeep já possui:

```text
@tanstack/react-query
```

Portanto aproveitar `useInfiniteQuery`.

Modelo:

```text
page 1
    |
nextCursor
    |
page 2
    |
nextCursor
```

Recursos úteis:

```text
fetchNextPage
hasNextPage
isFetchingNextPage
maxPages
```

## `maxPages`

Muito importante para evitar crescimento infinito de memória no frontend.

Exemplo:

```text
maxPages = 5
```

Se cada página tiver 50 itens:

```text
máximo aproximado no query cache da tela = 250 itens
```

Ajustar conforme UX.

---

# 31. P2 — virtualização da lista

O KubePeep atualmente não possui `@tanstack/react-virtual` entre as dependências analisadas.

Adicionar:

```text
@tanstack/react-virtual
```

A ideia é:

> renderizar apenas as linhas visíveis e um pequeno overscan.

Exemplo:

```text
5.000 recursos carregados
viewport mostra 25
DOM mantém talvez 35–50
```

Isso reduz:

- custo de layout;
- custo de React reconciliation;
- memória de DOM;
- travamentos durante scroll.

Virtualização não substitui paginação.

Usar as duas:

```text
backend pagination
+
frontend infinite query
+
DOM virtualization
```

---

# 32. UX progressiva

Evitar spinner global até todos os namespaces terminarem.

Melhor:

```text
Loading resources...

12 resources
28 resources
50 resources

Complete
```

Ou:

```text
Loaded 37 / 50
```

Quando tecnicamente possível.

A primeira linha deve aparecer o mais cedo possível.

## Performance percebida importa

Duas implementações podem levar 2 segundos no total.

A:

```text
0 ms -------- 2000 ms -> tudo aparece
```

B:

```text
300 ms -> primeiras linhas
700 ms -> mais linhas
1200 ms -> página
2000 ms -> dados auxiliares
```

A segunda parece muito mais rápida.

---

# 33. Preservar dados enquanto refresh acontece

React Query permite manter dados anteriores enquanto nova página/filtro carrega.

Usar:

```text
placeholderData / previousData
```

em vez de apagar a tabela e mostrar spinner sempre que:

- sort muda;
- página muda;
- refresh acontece.

UI:

```text
dados anteriores continuam visíveis
+
indicador sutil "Refreshing..."
```

---

# 34. Prefetch da próxima página

Quando o usuário chega perto do final da página atual:

```text
prefetch next page
```

Por exemplo:

```text
scroll chegou a 70–80%
```

ou:

```text
mouse/teclado indica navegação adiante
```

Não buscar páginas indefinidamente.

Manter prefetch de no máximo uma página futura.

---

# 35. Evitar polling quando WATCH está saudável

Regra:

```text
WATCH healthy
    -> não fazer polling completo

WATCH unavailable
    -> refresh manual / polling controlado

WATCH caiu temporariamente
    -> reconnect + backoff
```

Se fallback automático por polling for introduzido futuramente:

```text
intervalo mínimo razoável
jitter
pause quando app estiver em background
pause quando tela não estiver ativa
```

Nunca:

```text
LIST completo a cada 1–2 segundos
```

---

# 36. Cache em duas camadas

Modelo recomendado:

```text
L1 - cache local de objetos/watch
L2 - React Query cache
```

## L1 Backend

Responsável por:

```text
Kubernetes state
resourceVersion
watch
deduplicação
```

## L2 Frontend

Responsável por:

```text
UX
paginação
reuso rápido de tela
placeholder
staleTime
```

Não duplicar objetos desnecessariamente em muitas estruturas internas.

---

# 37. Cache com orçamento de memória

Não criar cache ilimitado.

Adicionar orçamento.

Exemplo inicial:

```text
resource cache budget: 128–256 MiB
cursor store: 32–64 MiB
```

Esses números são apenas ponto de teste.

O KubePeep deve funcionar em máquinas modestas.

## Estratégia de eviction

```text
inactive watches primeiro
cursor expirado
cache de tela não utilizada
LRU por resource scope
```

---

# 38. Backpressure entre WATCH e frontend

O backend pode receber eventos mais rápido do que a UI consegue processar.

Nunca deixar channel crescer sem limite.

Modelo:

```text
bounded queue
+
coalescing por resource key
```

Exemplo:

```text
Pod A modified RV 100
Pod A modified RV 101
Pod A modified RV 102
```

Se a UI ainda não consumiu nenhum deles, para uma interface de estado atual pode ser suficiente entregar:

```text
Pod A RV 102
```

em vez dos três estados intermediários.

Isso deve ser feito somente em telas **level-driven**, onde interessa o estado atual.

Logs e eventos cronológicos têm semântica diferente e não devem sofrer esse tipo de coalescing indiscriminadamente.

---

# 39. Level-driven em vez de edge-driven

Para a UI principal:

```text
o que importa é o estado atual
```

Não é necessário preservar cada transição intermediária de um Pod se a tela ficou momentaneamente lenta.

Isso simplifica:

- reconexão;
- relist;
- 410 Gone;
- cache recovery.

Para:

```text
events
logs
audit-like streams
```

preservar semântica apropriada.

---

# 40. ResourceVersion e 410 Gone

Continue tokens podem expirar.

A documentação Kubernetes informa que, por padrão, tokens de paginação podem expirar após um período curto, frequentemente em torno de 5 minutos.

Quando ocorrer:

```text
410 Gone
```

não tentar usar indefinidamente o mesmo token.

Fluxo:

```text
cursor expired
   |
   v
invalidar cursor local
   |
   v
reiniciar LIST consistente
   |
   v
reconstruir cache/página
```

A UI deve mostrar algo discreto como:

```text
Resource view refreshed because the Kubernetes snapshot expired.
```

não um erro fatal quando for possível recuperar automaticamente.

---

# 41. Consistência de paginação

Um benefício importante do `limit + continue` Kubernetes é que as páginas da mesma sequência representam um snapshot consistente, com o mesmo `resourceVersion`.

Não misturar arbitrariamente páginas de snapshots diferentes durante uma mesma navegação.

Guardar por origem:

```text
resourceVersion
continue token
```

e invalidar a sequência se a coerência não puder ser garantida.

---

# 42. Cursor local deve ter geração

KubePeep já possui conceito de `Generation`.

Isso deve ser usado como fence.

Cursor criado em:

```text
generation A
```

não pode continuar válido depois de:

```text
context switch -> generation B
```

Ao trocar generation:

```text
cancel goroutines
stop watches
invalidate cursor store entries
clear resource caches associated
cancel frontend requests
```

---

# 43. Melhorar autorização sem explodir chamadas

No fan-out atual existe checagem de autorização por origem.

Isso é correto do ponto de vista de segurança.

Mas pode ficar caro.

## Estratégia

Cachear resultados de autorização por:

```text
generation
namespace
apiGroup
resource
verb
```

com TTL curto e invalidação por generation.

Nunca compartilhar autorização entre:

```text
clusters
contexts
identidades
generations
```

## Resultado

Evita repetir dezenas de:

```text
SelfSubjectAccessReview
```

para a mesma capability durante navegação.

---

# 44. Tratar `AUTHORIZATION_UNAVAILABLE` separadamente de `Forbidden`

O screenshot apresenta:

```text
AUTHORIZATION_UNAVAILABLE
```

Esse estado significa algo diferente de:

```text
403 Forbidden
```

A UI deve continuar distinguindo:

```text
Forbidden:
RBAC respondeu não.

Authorization unavailable:
não foi possível determinar a capability.
```

No segundo caso:

- não fazer retry agressivo;
- permitir refresh manual;
- mostrar diagnóstico;
- não presumir autorização.

---

# 45. Cache de descoberta Kubernetes

API discovery também pode gerar tráfego.

Cachear:

```text
API groups
versions
resources
namespaced flag
verbs
```

por contexto/generation.

Não fazer discovery completo ao abrir cada tela.

Invalidar quando:

- contexto muda;
- geração muda;
- usuário pede refresh de discovery;
- erro indicar recurso mudou.

---

# 46. Transport reuse

Garantir que clientes Kubernetes reutilizem:

```text
http.Transport
TCP connections
TLS sessions
HTTP/2 quando aplicável
```

Evitar criar um novo client/transport para cada request.

O KubePeep já possui `ClientCache`; preservar esse conceito.

Medir:

```text
connection creation
TLS handshake
idle connection reuse
```

principalmente em clusters remotos.

---

# 47. Lazy loading de recursos caros

Nem toda informação precisa carregar junto com a tela.

Prioridade:

```text
P0 name/namespace/status
P1 basic metadata
P2 metrics
P3 expensive relations/details
```

Exemplo Pod:

```text
primeiro:
name
namespace
phase
ready
restarts
age

depois:
CPU
memory

ao abrir:
containers detalhados
volumes
conditions
owner chain
yaml
logs
```

---

# 48. Evitar recalcular sort repetidamente

Hoje o fluxo pode ordenar buffers por origem e depois executar merge.

Com o novo modelo:

- manter cada chunk previamente ordenado;
- usar heap;
- não ordenar novamente todo buffer a cada refill;
- evitar `sort.SliceStable` de arrays grandes repetidamente.

Para listas no cache:

- manter índice quando fizer sentido;
- ou ordenar apenas o conjunto solicitado.

---

# 49. Search

Existem dois tipos de search:

## Search local

Se a página já contém dados suficientes:

```text
filtrar no frontend
```

É instantâneo e não toca Kubernetes.

## Search server-side

Se o universo é muito grande:

```text
backend search
```

Mas deve usar:

- selectors quando possível;
- índices do cache local;
- debounce.

Evitar full-cluster scan a cada tecla.

---

# 50. Observabilidade de performance

O projeto já possui diretório:

```text
internal/observability
```

A performance da nova arquitetura deve ser mensurável.

## Métricas essenciais

### Kubernetes requests

```text
kubepeep_k8s_requests_total{
  verb,
  resource,
  strategy,
  code
}
```

```text
kubepeep_k8s_request_duration_seconds
```

```text
kubepeep_k8s_response_bytes
```

```text
kubepeep_k8s_items_received_total
```

### Fan-out

```text
kubepeep_list_fanout_origins
kubepeep_list_fanout_concurrency
kubepeep_list_fanout_duration_seconds
```

### Over-fetch

Uma métrica extremamente útil:

```text
kubepeep_list_overfetch_ratio
```

Fórmula:

```text
items_received_from_k8s / items_returned_to_ui
```

Exemplo ruim:

```text
5000 / 100 = 50x
```

Meta:

```text
idealmente próximo de 1–3x
```

dependendo da estratégia.

### Cursor

```text
kubepeep_cursor_store_entries
kubepeep_cursor_store_bytes
kubepeep_cursor_store_hit_total
kubepeep_cursor_store_miss_total
kubepeep_cursor_expired_total
```

### Watch

```text
kubepeep_watch_active
kubepeep_watch_reconnect_total
kubepeep_watch_410_total
kubepeep_watch_events_total
kubepeep_watch_lag_seconds
```

### Cache

```text
kubepeep_resource_cache_objects
kubepeep_resource_cache_bytes
kubepeep_resource_cache_hit_total
kubepeep_resource_cache_evictions_total
```

### Rate limiting

```text
kubepeep_k8s_429_total
kubepeep_client_throttle_seconds
```

---

# 51. Traces OpenTelemetry

Criar spans:

```text
resources.list
resources.list.global
resources.list.fanout
resources.list.origin
resources.merge
cursor.get
cursor.put
watch.connect
watch.reconnect
cache.snapshot
cache.apply_event
```

Atributos seguros:

```text
resource
apiGroup
strategy
namespace_count
page_size
origin_chunk_size
fanout
status_code
items_count
duration
```

Evitar atributos de alta cardinalidade:

```text
pod name
UID
cursor token
raw selector
user identity
```

Não vazar dados sensíveis.

---

# 52. Métricas de UX

Backend rápido não significa UX rápida.

Medir também:

```text
time_to_first_row
time_to_page_complete
filter_interaction_latency
sort_interaction_latency
scroll_frame_drops
rendered_row_count
```

Meta principal:

> primeira informação útil o mais cedo possível.

---

# 53. Performance budgets sugeridos

São metas iniciais, não contratos absolutos.

## Cluster pequeno

```text
< 10 namespaces
< 1.000 pods
```

Meta:

```text
first row < 300 ms
page complete < 800 ms
```

em rede local razoável.

## Cluster médio

```text
25–50 namespaces
1.000–5.000 objetos
```

Meta:

```text
first row < 500 ms
page complete < 1.5 s
```

## Cluster grande

```text
100+ namespaces
10.000+ objetos
```

Meta:

```text
first row < 1 s
page complete < 2–3 s
```

desde que API Server e rede estejam saudáveis.

O objetivo mais importante:

```text
tempo deve crescer sublinearmente em relação ao total
para consultas paginadas
```

e nunca exigir carregar todos os objetos para exibir página 1.

---

# 54. Benchmark obrigatório antes/depois

Criar laboratório com Kind ou cluster sintético.

## Matriz

### Namespaces

```text
1
10
25
50
100
200
```

### Pods por namespace

```text
10
50
100
500
```

### Page size

```text
25
50
100
```

### RBAC

```text
global list allowed
namespace-only list
mixed allowed/denied
authorization unavailable
watch allowed
watch denied
```

### Latência simulada

```text
0 ms
20 ms
50 ms
100 ms
200 ms
```

### Erros

```text
429
410
timeout
connection reset
watch close
```

---

# 55. O que medir no benchmark

Para cada cenário:

```text
TTFB
time to first row
time to full page
Kubernetes request count
bytes received
objects decoded
objects returned
over-fetch ratio
peak memory
allocations
goroutine count
CPU
cursor memory
429 count
cache hit ratio
watch reconnects
```

---

# 56. Go benchmarks

Criar benchmarks específicos:

```text
BenchmarkMergeOriginPages
BenchmarkLazyKWayMerge
BenchmarkCursorEncodeOld
BenchmarkCursorStoreNew
BenchmarkSortPods
BenchmarkWatchApplyEvent
```

Sizes:

```text
10 origins
50 origins
100 origins
200 origins
```

Buffers:

```text
10
50
100
500
```

Rodar:

```text
go test -bench=. -benchmem
```

Guardar resultados comparativos na documentação, mas não versionar dumps gigantes.

---

# 57. Profiling

Usar em desenvolvimento:

```text
pprof CPU
pprof heap
allocs
goroutine
```

Procurar:

```text
encoding/json
sort.SliceStable
DTO conversion
unstructured conversion
deepcopy
reflection
authorization checks
```

Não otimizar por suposição.

---

# 58. Testes de regressão

O novo desenho precisa preservar:

- ordenação determinística;
- ausência de duplicatas;
- ausência de gaps;
- RBAC;
- generation fence;
- cancelamento;
- 410 recovery;
- partial failures;
- context switch;
- list global;
- namespace fan-out;
- secret safety;
- bounded payload;
- bounded memory.

---

# 59. Teste fundamental de paginação

Gerar dataset conhecido:

```text
10 namespaces
100 pods cada
1000 objetos
```

Paginar tudo com:

```text
limit=37
```

Concatenar todas as páginas.

Validar:

```text
count == 1000
duplicates == 0
missing == 0
ordering == expected
```

Repetir para:

```text
namespace/name asc
namespace/name desc
age asc
age desc
```

---

# 60. Teste com mutações durante paginação

Enquanto o cliente pagina:

```text
create
update
delete
```

Validar o contrato escolhido.

Para snapshot baseado em `continue`:

- manter consistência do snapshot;
- mudanças posteriores chegam via watch/relist.

---

# 61. Teste de cursor expirado

Simular:

```text
cursor TTL expired
```

Esperado:

```text
backend não panic
frontend recebe erro recuperável
UI refaz primeira página ou solicita refresh
```

Não retornar stack trace.

---

# 62. Teste de memory bound

Criar:

```text
200 namespaces
grandes coleções
muitos cursores
```

Confirmar que:

```text
cursor store não cresce indefinidamente
watch cache não cresce indefinidamente
goroutines retornam ao baseline
```

---

# 63. Não usar SQLite para cursores transitórios

O KubePeep possui SQLite para preferências/configurações locais.

Não usar SQLite para:

```text
cursor buffer
watch events
resource snapshots temporários
```

Motivos:

- I/O desnecessário;
- lifecycle transitório;
- risco de persistir dados do cluster;
- complexidade;
- segurança.

Manter isso em memória.

---

# 64. Secrets continuam especiais

Mesmo que o cache evolua:

```text
Secret values nunca devem entrar no cache.
```

Apenas metadata autorizada e já prevista pelo KubePeep.

Não armazenar:

```text
data
stringData
decoded values
```

em:

- cursor;
- resource cache;
- telemetry;
- logs;
- traces.

---

# 65. Arquitetura alvo

```text
┌──────────────────────────────────────────────┐
│                  React UI                    │
│                                              │
│  React Query + Infinite Query + Virtualizer  │
└───────────────────────┬──────────────────────┘
                        │
                        │ cursor opaco
                        ▼
┌──────────────────────────────────────────────┐
│              Resource Query API              │
│                                              │
│  request coalescing                          │
│  cancellation                                │
│  query binding                               │
│  pagination strategy                         │
└──────────────┬─────────────────┬─────────────┘
               │                 │
        Global LIST       Namespace fallback
               │                 │
               │          Lazy paginator
               │          bounded workers
               │          k-way heap
               │                 │
               └────────┬────────┘
                        ▼
┌──────────────────────────────────────────────┐
│             Resource Cache Layer             │
│                                              │
│  demand-driven LIST/WATCH                    │
│  resourceVersion                             │
│  reference counting                          │
│  bounded memory                              │
└──────────────┬───────────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────────┐
│               client-go                      │
│                                              │
│ limit / continue                             │
│ selectors                                    │
│ QPS/Burst                                    │
│ protobuf/json                                │
│ bookmarks                                    │
└───────────────────────┬──────────────────────┘
                        ▼
                 kube-apiserver
```

---

# 66. Estratégia por tipo de consulta

## Caso A — Global LIST autorizado

```text
usar global LIST
usar limit/continue nativo
cursor local pequeno
```

Esse é o caminho ideal.

## Caso B — namespace/name sort + global proibido

```text
ordenar namespaces
consumir um namespace por vez
parar assim que completar página
```

Provavelmente o melhor caminho para muitos ambientes restritos.

## Caso C — sort global + global proibido

```text
lazy k-way merge
chunk pequeno por namespace
refill sob demanda
bounded concurrency
```

## Caso D — watch autorizado

```text
cache + watch
lista passa a consultar cache
```

## Caso E — watch proibido

```text
snapshot paginado
refresh manual
opcional polling lento/controlado
```

---

# 67. Planos de implementação

## Fase 0 — Baseline e instrumentação

Antes de alterar algoritmo:

- adicionar métricas básicas;
- criar benchmark Kind;
- registrar:
  - total de requests;
  - itens recebidos;
  - itens retornados;
  - cursor size;
  - duração;
  - memory;
- documentar baseline.

### Saída

```text
docs/performance-baseline.md
```

---

# 68. Fase 1 — eliminar `LIMIT_EXCEEDED`

### Alterar

```text
internal/services/resources/cursor.go
internal/services/resources/list.go
internal/api/... cursor codec
internal/integration/kubernetesruntime/resources_backend.go
```

### Implementar

- `CursorStore`;
- cursor token opaco;
- TTL;
- LRU;
- generation binding;
- query binding;
- memory bound;
- purge;
- testes.

### Remover do transporte

```text
Buffered []T
```

ou garantir que nunca seja serializado para a UI.

### Critério de sucesso

```text
200 namespaces
page=100
não gera cursor > limite HTTP
```

---

# 69. Fase 2 — reduzir over-fetch

Implementar:

```text
UI page size != origin chunk size
```

Começar com:

```text
originChunk=10
```

Adicionar métrica:

```text
overfetch_ratio
```

### Critério

Reduzir drasticamente cenário:

```text
50 namespaces × 100
```

para algo muito menor.

---

# 70. Fase 3 — paginator especializado Namespace+Name

Criar estratégia:

```go
type PaginationStrategy interface {
    Next(context.Context, QueryState) (Page, QueryState, error)
}
```

Implementações:

```text
GlobalNativePaginator
NamespaceSequentialPaginator
LazyMergePaginator
```

### Benefício

Evita colocar todas as regras dentro de `Collect()`.

---

# 71. Fase 4 — lazy k-way merge

Implementar heap.

Usar:

```text
container/heap
```

Evitar dependência externa.

Adicionar testes de:

```text
determinism
duplicates
descending
refill
partial namespace failures
```

---

# 72. Fase 5 — frontend infinite + virtual

Frontend:

```text
web/
```

Já existe:

```text
@tanstack/react-query
```

Adicionar:

```text
@tanstack/react-virtual
```

Implementar:

- `useInfiniteQuery`;
- cursor;
- `maxPages`;
- next page prefetch;
- virtual rows;
- placeholder previous data;
- loading incremental.

---

# 73. Fase 6 — demand-driven resource cache

Criar camada:

```text
internal/services/resourcecache/
```

ou equivalente consistente com arquitetura atual.

Componentes:

```text
Manager
Subscription
Store
Watcher
RefCounter
Eviction
```

Não misturar responsabilidades diretamente em `list.go`.

---

# 74. Fase 7 — streaming list capability

Detectar versão/capability.

Se suportado:

```text
sendInitialEvents=true
```

Fallback:

```text
LIST + WATCH
```

Adicionar feature flag interno até maturar.

---

# 75. Fase 8 — protocolo e payload

Testar:

- Protobuf built-ins;
- JSON fallback;
- PartialObjectMetadata;
- gzip;
- selectors.

Ativar apenas quando benchmarks mostrarem ganho real.

---

# 76. Fase 9 — adaptive concurrency

Somente após:

```text
métricas
benchmarks
429 telemetry
```

Implementar se houver benefício comprovado.

---

# 77. Critérios de aceite P0

A versão não deve ser considerada pronta sem:

- nenhum cursor contendo milhares de DTOs no transporte;
- `LIMIT_EXCEEDED` não reproduzível no cenário original;
- cursor TTL;
- cursor memory bound;
- generation invalidation;
- timeout normalizado;
- testes de concorrência;
- testes com `-race`;
- benchmark antes/depois.

---

# 78. Critérios de aceite P1

- namespace/name não precisa consultar todos os namespaces para produzir página 1;
- global sort usa lazy merge;
- over-fetch ratio mensurado;
- fan-out limitado;
- request cancellation;
- request coalescing;
- 429 não causa retry storm;
- 410 recuperável.

---

# 79. Critérios de aceite P2

- WATCH saudável elimina polling completo;
- watches compartilhados;
- cache tem limite;
- frontend virtualizado;
- infinite query;
- primeira página permanece utilizável durante refresh;
- Metrics API não bloqueia lista;
- UI começa a exibir conteúdo progressivamente.

---

# 80. Anti-patterns a evitar

Não fazer:

```text
aumentar cursor para 1 MB
```

Não fazer:

```text
fan-out 50 apenas porque goroutine é barata
```

Não fazer:

```text
poll completo a cada segundo
```

Não fazer:

```text
um informer para todo recurso conhecido permanentemente
```

Não fazer:

```text
buscar detalhes de cada row durante listagem
```

Não fazer:

```text
recriar Kubernetes client para cada request
```

Não fazer:

```text
persistir snapshots de cluster em SQLite
```

Não fazer:

```text
retry sem jitter
```

Não fazer:

```text
ignorar HTTP 429
```

Não fazer:

```text
ordenar 50.000 itens no frontend para mostrar 50
```

Não fazer:

```text
carregar 10.000 DOM rows
```

---

# 81. Ordem recomendada de prioridade

```text
P0
│
├── server-side opaque cursor
├── corrigir timeout
├── separar page size / origin chunk
└── instrumentar over-fetch
    │
    ▼
P1
│
├── sequential paginator para namespace/name
├── lazy k-way merge
├── bounded worker pool
├── request cancellation
├── singleflight
└── selectors
    │
    ▼
P2
│
├── demand-driven LIST/WATCH cache
├── watch bookmarks
├── infinite query
├── virtualização
└── progressive metrics
    │
    ▼
P3
│
├── streaming lists
├── protobuf
├── metadata-only
├── compression tuning
└── adaptive concurrency
```

---

# 82. Resultado esperado

Depois das mudanças, uma página de Pods em cluster grande deve funcionar aproximadamente assim:

```text
User abre Pods
      |
      v
Backend identifica query
      |
      +--> cache hit?
      |        |
      |       sim
      |        |
      |        +----> responde quase imediatamente
      |
      +--> global list autorizado?
               |
              sim
               |
               +--> LIST limit=50
               |
              não
               |
               +--> estratégia lazy
                       |
                       +--> pequenos chunks
                       +--> bounded workers
                       +--> para ao completar página

Frontend recebe 50
      |
      +--> renderiza apenas rows visíveis
      |
      +--> prefetch próxima página
      |
      +--> watch mantém cache atualizado
```

A complexidade percebida pelo usuário passa a depender muito mais de:

```text
tamanho da página
```

do que de:

```text
quantidade total de objetos do cluster
```

Esse deve ser o objetivo arquitetural.

---

# 83. Prompt curto para execução futura por agente de código

```text
Analise profundamente a performance da listagem de recursos do KubePeep usando este documento como especificação. Não resolva o problema apenas aumentando limites, timeouts ou goroutines. Redesenhe a paginação para eliminar over-fetch e cursores com DTOs, implemente cursor opaco server-side com TTL/LRU e binding por generation/query, preserve o fast path de LIST global, crie paginator sequencial para Namespace+Name e lazy k-way merge com pequenos chunks para sorts globais, mantenha concorrência bounded e respeite QPS/Burst/429. Evolua LIST+WATCH para cache local sob demanda, reutilize watches, suporte bookmarks e opcionalmente streaming lists quando compatível. No React use TanStack Query infinite queries, maxPages e virtualização. Adicione métricas OpenTelemetry, benchmarks, testes de 410/429/RBAC/context switch, race tests e compare request count, over-fetch, TTFB, memória e tempo para primeira página antes/depois. Preserve segurança, RBAC e a regra de nunca persistir Secret values.
```

---

# 84. Fontes e boas práticas pesquisadas

## Kubernetes API Concepts

Documentação principal:

`https://kubernetes.io/docs/reference/using-api/api-concepts/`

Pontos relevantes:

- `limit` + `continue`;
- snapshots consistentes;
- `resourceVersion`;
- 410 Gone;
- WATCH;
- bookmarks;
- streaming lists;
- PartialObjectMetadata;
- Protobuf;
- response compression.

---

## Kubernetes API Priority and Fairness

`https://kubernetes.io/docs/concepts/cluster-administration/flow-control/`

Relevante para:

- HTTP 429;
- fairness;
- sobrecarga do API Server;
- comportamento em bursts;
- tuning responsável de concorrência.

---

## client-go SharedInformer

`https://pkg.go.dev/k8s.io/client-go/tools/cache`

Relevante para:

- cache local;
- `SharedInformer`;
- `SharedIndexInformer`;
- sincronização LIST/WATCH;
- eventual consistency.

---

## client-go Watch helpers

`https://pkg.go.dev/k8s.io/client-go/tools/watch`

Relevante para:

- `RetryWatcher`;
- reconexão;
- recovery;
- comportamento diante de RV expirado;
- abordagem level-driven.

---

## client-go REST Config

`https://pkg.go.dev/k8s.io/client-go/rest`

Defaults pesquisados:

```text
DefaultQPS   = 5
DefaultBurst = 10
```

---

## client-go ListPager

`https://github.com/kubernetes/client-go/blob/master/tools/pager/pager.go`

Relevante para:

- paginação;
- buffering de chunks;
- cancelamento;
- expiração da sequência.

---

## Kubernetes metadata client

`https://github.com/kubernetes/client-go/blob/master/metadata/metadata.go`

Relevante para:

- PartialObjectMetadata;
- preferência por Protobuf;
- fallback JSON.

---

## TanStack Query — Infinite Queries

`https://tanstack.com/query/latest/docs/framework/react/guides/infinite-queries`

Relevante para:

- `useInfiniteQuery`;
- `fetchNextPage`;
- `getNextPageParam`;
- `maxPages`.

---

## TanStack Query — Paginated Queries

`https://tanstack.com/query/latest/docs/framework/react/guides/paginated-queries`

Relevante para:

- manter dados anteriores;
- evitar flicker;
- placeholder data.

---

## TanStack Virtual

`https://tanstack.com/virtual/latest/docs/framework/react`

Relevante para:

- virtualização;
- listas grandes;
- redução de DOM;
- scroll eficiente.

---

# 85. Observações específicas do KubePeep analisado

Na revisão do repositório em setembro de 2026 foram observados:

- aplicação desktop Wails + backend Go + React;
- `@tanstack/react-query` já presente;
- React 19;
- paginação composta existente;
- `Buffered []T` no cursor;
- limite de cursor serializado;
- fan-out com goroutines e semaphore;
- `MaximumFanout`;
- `limit` por origem baseado no page limit;
- fast path global com `PreferGlobal`;
- autorização global através de `globalListDecision`;
- timeout de janela configurável;
- fallback antigo de 10s ainda presente em `Collect`;
- WATCH próprio;
- exponential backoff + jitter;
- tratamento de resource version expirado;
- chunking de snapshot SSE;
- geração utilizada para isolamento/cancelamento.

Portanto a recomendação é **evoluir a arquitetura existente**, não substituí-la por uma implementação completamente nova.

---

# 86. Decisão arquitetural final recomendada

A arquitetura mais adequada para o KubePeep é:

```text
GLOBAL LIST quando autorizado
        +
NAMESPACE LAZY PAGINATION quando restrito
        +
OPAQUE SERVER-SIDE CURSOR
        +
DEMAND-DRIVEN LIST/WATCH CACHE
        +
BOUNDED CONCURRENCY
        +
REACT INFINITE QUERY
        +
VIRTUALIZED ROWS
        +
OTEL PERFORMANCE METRICS
```

Em uma frase:

> **buscar somente o necessário, reutilizar o que já foi buscado, transmitir somente o necessário e renderizar somente o que está visível.**

Esse princípio deve orientar toda otimização futura de performance do KubePeep.
