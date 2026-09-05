# Contrato HTTP e streaming

> **Escopo:** contrato da base atual; rotas explicitamente reservadas não são funcionalidades disponíveis. A execução corrente está no [plano v1](../plan/README.md).
>
> **Transporte:** origem loopback no modo web; bridge JSON e loopback de streams no [desktop](desktop-architecture.md).
>
> **Versão:** `/api/v1`
>
> **Importante:** o envelope público é um requisito do KubePeep.
> Cursores, agregações parciais, health e streams usam DTOs próprios
> compatíveis com este contrato.

## 1. Princípios

- JSON UTF-8, exceto assets, YAML, SSE e upgrade bidirecional.
- DTOs próprios; nenhum objeto client-go é serializado diretamente.
- Leitura Kubernetes e permissões usam `Cache-Control: no-store`.
- Requests mutáveis usam JSON estrito, proteção de Host/Origin/CSRF e limite de body.
- Autorização da UI é informativa; ações são revalidadas no backend.
- Listas potencialmente grandes são paginadas.
- Resultados agregados declaram completude, truncamento e cobertura.
- Cancelamento e geração impedem que resposta antiga substitua a atual.

## 2. Convenções

Salvo override explícito na tabela da rota, GET/PUT JSON bem-sucedido retorna
200, criação de recurso/sessão retorna 201, ação assíncrona aceita retorna 202 e
DELETE sem payload retorna 204. Toda referência a `DTO[]` usa o envelope de
sucesso e paginação aplicável; YAML bem-sucedido retorna 200 `application/yaml`.

### 2.1 Headers de request

| Header | Uso |
| --- | --- |
| `Accept: application/json` | padrão |
| `Content-Type: application/json` | obrigatório em body JSON |
| `X-KubePeep-CSRF` | obrigatório em mutações JSON e streams SSE via `fetch` |
| `Idempotency-Key` | obrigatório nas ações indicadas |
| `Last-Event-ID` | retomada SSE apenas quando o contrato da rota permitir |

O cliente não envia contexto ou autorização em headers customizados; seleção e RBAC são resolvidos no servidor.

### 2.2 Headers de response

| Header | Regra |
| --- | --- |
| `Content-Type` | tipo exato da resposta |
| `X-Request-ID` | presente em toda resposta possível |
| `Cache-Control: no-store` | API, YAML, logs, permissões e sessão |
| `Retry-After` | opcional em 429/503 |

### 2.3 Envelope de sucesso

```json
{
  "data": {},
  "meta": {
    "requestId": "req_...",
    "generation": "gen_...",
    "collectedAt": "2026-07-27T12:00:00Z"
  }
}
```

`meta` contém somente campos pertinentes. Resposta `204 No Content` não usa envelope.

### 2.4 Envelope de erro

```json
{
  "code": "FORBIDDEN",
  "message": "You do not have permission to access pod logs in this namespace.",
  "requestId": "req_...",
  "details": {
    "resource": "pods/log",
    "verb": "get",
    "namespace": "payments"
  }
}
```

`details` é allowlisted por código. Nunca contém erro bruto, credencial, path desnecessário, payload Kubernetes ou saída de plugin.

## 3. Decoding e limites comuns

- Campo JSON desconhecido: rejeitar com `UNKNOWN_FIELD`.
- Conteúdo após o primeiro valor JSON: `INVALID_JSON`.
- Body vazio quando obrigatório: `INVALID_JSON`.
- Tipo incompatível: `VALIDATION_FAILED`.
- `Content-Type` incorreto: HTTP 415 `UNSUPPORTED_MEDIA_TYPE`.
- Body JSON máximo geral: 1 MiB.
- Requests de escopo em massa: máximo 1 MiB e 10.000 entradas antes de deduplicação.
- Strings textuais de busca: máximo 256 bytes.
- IDs devem ter formato/tamanho validado antes de consultar storage.
- Nomes Kubernetes usam a validação oficial adequada ao recurso.

Limites de bytes de logs/frames têm configuração própria e não herdam o body geral.

## 4. Códigos de erro

| HTTP | Código | Significado |
| --- | --- | --- |
| 400 | `INVALID_JSON` | JSON malformado ou trailing content |
| 400 | `UNKNOWN_FIELD` | campo não reconhecido |
| 400 | `VALIDATION_FAILED` | valor inválido; details por campo |
| 400 | `KUBECONFIG_INVALID` | kubeconfig não pôde ser interpretado com segurança |
| 400 | `CONTEXT_INVALID` | nome/formato de contexto não aceito |
| 400 | `PREFERENCE_SENSITIVE_VALUE` | preferência parece conter dado proibido |
| 400 | `CURSOR_INVALID` | cursor malformado, adulterado ou de outra instância |
| 400 | `CURSOR_MISMATCH` | cursor pertence a outra query/geração |
| 403 | `FORBIDDEN` | negação explícita do Kubernetes/operação |
| 403 | `CSRF_REJECTED` | Host/Origin/token não aceito |
| 404 | `NOT_FOUND` | entidade/recurso não encontrado |
| 404 | `KUBECONFIG_NOT_FOUND` | nenhum arquivo resolvido existe |
| 404 | `CONTEXT_NOT_FOUND` | contexto solicitado não existe |
| 409 | `CONFLICT` | versão, precondition, instância ou estado concorrente |
| 409 | `IDEMPOTENCY_CONFLICT` | mesma chave com body diferente |
| 409 | `GENERATION_CHANGED` | seleção mudou antes da operação |
| 409 | `SELECTION_MISMATCH` | entidade pertence a outro profile/contexto ativo |
| 410 | `CURSOR_EXPIRED` | TTL do cursor expirou; nova lista necessária |
| 410 | `SESSION_GONE` | sessão já encerrada |
| 413 | `BODY_TOO_LARGE` | body excedeu limite |
| 415 | `UNSUPPORTED_MEDIA_TYPE` | tipo de conteúdo não aceito |
| 429 | `LIMIT_EXCEEDED` | concorrência/sessões/budget excedido |
| 500 | `INTERNAL` | falha local inesperada sanitizada |
| 503 | `CLUSTER_UNAVAILABLE` | API Kubernetes inacessível |
| 503 | `AUTHENTICATION_UNAVAILABLE` | kubeconfig/plugin não concluiu autenticação |
| 503 | `AUTHORIZATION_UNAVAILABLE` | revisão não pôde decidir; não é negação |
| 503 | `FEATURE_UNAVAILABLE` | API opcional/protocolo não disponível |
| 504 | `UPSTREAM_TIMEOUT` | timeout da dependência |

Se o cliente fechar a conexão, pode não existir resposta. O servidor registra apenas `CLIENT_CANCELED` como código operacional, sem tratá-lo como erro do produto.

## 5. Paginação e filtros

### 5.1 Query comum

| Parâmetro | Default | Máximo/regra |
| --- | --- | --- |
| `limit` | 100 | 500 |
| `continue` | ausente | token opaco até 16 KiB |
| `search` | vazio | 256 bytes; sem regex arbitrária |
| `namespace` | escopo ativo | repetível somente onde documentado |
| `status` | todos | enum por recurso |
| `sort` | chave estável do endpoint | allowlist |
| `order` | `asc` | `asc` ou `desc` |

Parâmetros são case-sensitive, URL-decoded uma vez e únicos, exceto
`namespace`, `status` e `kind` quando a tabela abaixo os declara repetíveis.
Repetição não declarada, valor vazio e query desconhecida retornam
`VALIDATION_FAILED`. `namespace` aceita no máximo 100 valores distintos, todos
dentro do scope ativo; funciona como interseção, nunca amplia o scope.
`status` e `kind` preservam a ordem canônica da tabela, não a ordem recebida.

### 5.2 Meta de página

```json
{
  "data": [],
  "meta": {
    "requestId": "req_...",
    "generation": "gen_42",
    "page": {
      "limit": 100,
      "next": "opaque-token-or-empty",
      "complete": false,
      "truncated": true,
      "filterScope": "page"
    },
    "coverage": {
      "requestedNamespaces": 12,
      "completedNamespaces": 10,
      "deniedNamespaces": ["restricted"],
      "failed": [
        {
          "namespace": "unstable",
          "code": "UPSTREAM_TIMEOUT",
          "message": "Collection timed out."
        }
      ]
    }
  }
}
```

`filterScope` é `page` ou `collection` conforme §5.3 e sempre existe em
resposta paginada.

O cursor é JSON canônico opaco autenticado por HMAC-SHA-256 com segredo
efêmero do processo. Ele inclui versão, expiração, hash da query, contexto,
escopo, geração e, em fan-out, o estado composto por namespace/kind e merge
determinístico. Cada token expira exatamente 5 minutos após ser emitido, sem
sliding TTL; uma página válida emite um novo token com sua própria janela de 5
minutos. Alteração ou token de uma instância anterior retornam
`CURSOR_INVALID`; mudança de query/geração retorna `CURSOR_MISMATCH`; expiração
por TTL retorna `CURSOR_EXPIRED`.

Ordenação global só é exposta quando pode ser cumprida dentro de limites. Caso um endpoint só ordene a página atual, seu campo `sort` não é anunciado como global.

### 5.3 Filtros e merge por coleção

Nas coleções de recursos, `search` aceita termos, frases entre aspas e
exclusões com `-termo` ou `!termo` (incluindo `-"frase exata"`). Todos os
termos positivos precisam corresponder e nenhuma exclusão pode corresponder.
Cada termo é comparado como substring usando Unicode simple case folding
nos campos enumerados abaixo, unidos por espaço; não há regex nem operador `+`.
O parser é `internal/services/resources.ParseSearch`. Coleções locais e
blocos do dashboard mantêm seus filtros específicos; não presumir a mesma
gramática sem integração com esse parser. Como a
API Kubernetes não oferece substring global, search/sorts marcados `page`
operam somente sobre a janela coletada para aquela página; `meta.page` inclui
`filterScope: "page"`. A UI nunca os chama de resultado global quando
`complete=false`. `identity` é a ordenação determinística do merge, não uma
promessa de snapshot integral.

| Coleção | Filtros/enums além de namespace | Campos de `search` | `sort` allowlist (default; escopo) | Tupla final de merge/desempate |
| --- | --- | --- | --- | --- |
| `/namespaces` | `status`: `Active`, `Terminating`, `Unknown` | name | `name` (default; page) | name, uid |
| `/namespace-scopes` | nenhum namespace/status | name, context | `name` (default; collection), `updatedAt` (collection) | name, id |
| `/dashboard/problems` | `status`: `info`, `warning`, `critical` | pod, container, reason, message | `severity` (default desc; page), `age` (page), `identity` (page) | severity rank desc, namespace, pod, container |
| `/dashboard/restarts` | somente `namespace` e `limit` 1–50 | não aceita `search` | fixa por `restarts` desc; não aceita sort/order | restarts desc, namespace, pod, containerType, container |
| `/dashboard/events` | `status`: `Normal`, `Warning`, `Unknown` | objectKind, objectName, reason, message | `timestamp` (default desc; page), `count` (page), `identity` (page) | timestamp desc, namespace, uid |
| `/metrics` | nenhum status | pod, container | `cpu` (default desc; page), `memory` (page), `identity` (page) | medida desc, namespace, pod |
| `/workloads` | `kind`: os cinco plurais de §14, 1–5, default todos; `status`: enum de `WorkloadDTO` | namespace, name, kind | `identity` (default; page), `name` (page), `age` (page), `status` (page) | kind canônico, namespace, name, uid |
| `/pods` | `status`: `Running`, `Pending`, `Succeeded`, `Failed`, `Unknown`; `workload`, `node`; `restarts`: `any`, `gt0`, `gte3`, `gte10`; `problematic`: `true`/`false` | namespace, name, node, owner name | `identity` (default; page), `name` (page), `age` (page), `restarts` (page), `status` (page) | namespace, name, uid |
| `/events` | `status`: `Normal`, `Warning`, `Unknown`; `objectKind`, `reason` | namespace, objectKind, objectName, reason, message | `timestamp` (default desc; page), `count` (page), `identity` (page) | timestamp desc, namespace, uid |
| `/services` | nenhum status | namespace, name, type, clusterIPs | `identity` (default; page), `name` (page), `type` (page) | namespace, name, uid |
| `/ingresses` | nenhum status | namespace, name, className, hosts | `identity` (default; page), `name` (page) | namespace, name, uid |
| `/endpoint-slices` | `addressType`: `IPv4`, `IPv6`, `FQDN`, `Unknown` | namespace, name, addresses | `identity` (default; page), `name` (page), `addressType` (page) | namespace, name, uid |
| `/configmaps` | nenhum status | namespace, name | `identity` (default; page), `name` (page), `createdAt` (page) | namespace, name, uid |
| `/secrets` | nenhum status | namespace, name | `identity` (default; page), `name` (page), `createdAt` (page) | namespace, name, uid |
| `/nodes` | `status`: `Ready`, `NotReady`, `Unknown`; sem `namespace` (cluster-scoped) | name, roles, kubeletVersion | `identity` (default; page), `name` (page), `age` (page), `status` (page) | name |

`order` default é o indicado por `desc`; nos demais casos é `asc`. UID faz
parte do modelo interno/list metadata mesmo quando um DTO compacto não o
mostra. O cursor guarda continuations por namespace/GVR, a janela já coletada
e a tupla final emitida; a página seguinte nunca reconstrói um snapshot global
nem mistura geração/resourceVersion incompatível. `410 ResourceExpired`
descarta a página inteira e retorna 410 para recomeço, sem combinar dados.

Em `SavedFilterSet.query`, somente `namespace`, `search`, `status`, `sort`,
`order` e os extras da linha correspondente podem ser salvos; `namespace`,
`status` e `kind` são arrays de strings, boolean/integer preservam seu tipo e
os demais são strings. `limit`, `continue` e cursor nunca são persistidos.

## 6. Recursos de referência

### 6.1 `ResourceRef`

```json
{
  "apiGroup": "apps",
  "kind": "Deployment",
  "namespace": "payments",
  "name": "api",
  "uid": "..."
}
```

### 6.2 `ComponentState`

```json
{
  "status": "healthy",
  "code": "OK",
  "message": "SQLite is available.",
  "checkedAt": "2026-07-27T12:00:00Z"
}
```

`status` é `healthy`, `degraded`, `unhealthy` ou `unknown`; `code` e `message`
são strings públicas allowlisted. `checkedAt` é timestamp RFC 3339 ou null
quando o componente ainda não foi consultado. As quatro chaves sempre existem.

### 6.3 `Capability`

```json
{
  "capabilityId": "pods.logs.get",
  "namespace": "payments",
  "apiGroup": "",
  "resource": "pods",
  "subresource": "log",
  "verb": "get",
  "resourceName": "",
  "decision": "allowed",
  "reasonCode": "SAR_ALLOWED",
  "expiresAt": "2026-07-27T12:00:45Z"
}
```

`decision`: `allowed`, `denied`, `unknown`. `capabilityId` é sempre um ID da
allowlist de §11; nunca é texto fornecido pelo Kubernetes.

### 6.4 Primitivos de detalhe

Os DTOs de detalhe reutilizam apenas estes primitivos, nunca `ObjectMeta` ou
tipos do client-go:

| DTO | Campos e regras |
| --- | --- |
| `ResourceMetadataDTO` | `namespace`, `name`, `uid`, `resourceVersion` e `creationTimestamp` RFC 3339; `labels` é map string/string limitado a 64 pares e 16 KiB totais, ordenado por chave na serialização de testes |
| `ConditionDTO` | `type`, `status` (`True`, `False` ou `Unknown`), `reason: string|null`, `message: string|null` sanitizada até 4 KiB e `lastTransitionTime: RFC3339|null` |
| `ContainerSpecDTO` | `name`, `image` e `ports: ContainerPortDTO[]`; omite command, args, env, envFrom, mounts e valores de Secret |
| `ContainerPortDTO` | `name: string|null`, `containerPort` 1–65535, `protocol` (`TCP`, `UDP` ou `SCTP`) |
| `PodContainerDTO` | `spec: ContainerSpecDTO`, `type` (`regular`, `init` ou `ephemeral`), `ready: boolean|null`, `restartCount` não negativo, `state` (`waiting`, `running`, `terminated` ou `unknown`) e `reason: string|null` |

`labels` acima é a única metadata arbitrária exposta nesses detalhes.
Annotations, managed fields, ownerReferences crus e qualquer mapa não
allowlisted são omitidos. Arrays preservam a ordem Kubernetes, salvo quando o
contrato da rota declara ranking/ordenação.

## 7. Classificação de rotas

- **MVP:** necessária aos critérios/fluxos do MVP.
- **Interna:** suporta o frontend local; não é API pública de integração.
- **Opcional em runtime:** contrato MVP cuja dependência Kubernetes pode não
  existir, caso em que retorna `FEATURE_UNAVAILABLE`.
- **Pós-MVP:** reservada, não implementar agora.

Não há rota de Secret YAML, edição YAML, impersonation ou credenciais.

## 8. Status, sessão e health

| Método/rota | Classe | Request | Response | Autorização | Erros específicos |
| --- | --- | --- | --- | --- | --- |
| `GET /health` | MVP/probe | vazio | `HealthDTO` | proteção Host; sem RBAC | 503 somente por falha local crítica; 500 se a própria resposta não puder ser construída |
| `GET /api/v1/status` | MVP | vazio | `StatusDTO` | Host allowlisted; Origin, se presente, deve ser same-origin; sem RBAC | — |
| `GET /api/v1/session` | Interna | vazio | `SessionDTO` | Host allowlisted; Origin, se presente, deve ser same-origin; CORS desabilitado; `no-store` | `CSRF_REJECTED` |
| `GET /metrics` | Opt-in (Fase 5) | vazio | texto Prometheus | loopback exclusivo; sem RBAC (rede local confiável) | rota não registrada (404) quando `observability.metrics.enabled != true` |

`GET /metrics` expõe apenas o contador allowlisted
`kubepeep_requests_total{method,route,status}` — o label `route` usa o padrão
de rota do `http.ServeMux`, nunca caminhos do usuário. Detalhes do contrato de
observabilidade (schema de logs com `duration_ms`, opt-in do OTel, sanidade)
estão em [observability.md](observability.md).

`SessionDTO`:

```json
{
  "data": {
    "csrfToken": "base64url-ephemeral-nonce",
    "origin": "http://127.0.0.1:2748",
    "generation": "gen_42",
    "expiresAt": "2026-07-27T20:00:00Z"
  }
}
```

O nonce tem TTL máximo inicial de 8 horas, é rotacionado no restart e em todo
commit que cria uma nova geração de seleção. Depois de receber uma geração
diferente em `SelectionDTO` ou em `meta.generation`, o frontend deve concluir a
resposta corrente, descartar o nonce anterior e executar imediatamente
`GET /api/v1/session` antes da próxima mutação ou abertura de SSE. Outras abas
recuperam `CSRF_REJECTED` da mesma forma, sem repetir automaticamente a mutação
rejeitada. O frontend também refaz o bootstrap após expiração. CSRF não
autentica GET e não se confunde com o token de controle do processo, que nunca
entra em uma rota `/api/v1`.

Mesmo sem seleção Kubernetes, o processo cria uma generation ID inicial opaca
e não nula. Assim, `SessionDTO.generation` permanece sempre string e protege
preferences, criação local de scopes e outros fluxos disponíveis no shell
degradado. A primeira seleção válida substitui essa geração e rotaciona o nonce.

### 8.1 `HealthDTO`

```json
{
  "data": {
    "status": "degraded",
    "components": {
      "application": {"status": "healthy", "code": "OK", "message": "Application is ready.", "checkedAt": "2026-07-27T12:00:00Z"},
      "sqlite": {"status": "healthy", "code": "OK", "message": "SQLite is available.", "checkedAt": "2026-07-27T12:00:00Z"},
      "kubeconfig": {"status": "degraded", "code": "NOT_FOUND", "message": "No kubeconfig is available.", "checkedAt": "2026-07-27T12:00:00Z"},
      "context": {"status": "unknown", "code": "NOT_SELECTED", "message": "No context is selected.", "checkedAt": null},
      "cluster": {"status": "unknown", "code": "NOT_CHECKED", "message": "The cluster has not been checked.", "checkedAt": null}
    }
  }
}
```

HTTP:

- 200 se aplicação e SQLite estão saudáveis, mesmo com cluster/kubeconfig/contexto degradado;
- 503 se aplicação não consegue servir corretamente, SQLite obrigatório falha
  ou shutdown já começou;
- 500 somente se ocorrer uma falha inesperada ao construir a própria resposta.

O mux externo dá precedência a este handler sobre o `/health` registrado por
`app.New`. `pkg/health.Checker` permanece como contrato dos checks; deadline,
sanitização e semântica de dependência degradada pertencem ao KubePeep,
conforme ADR 0002. Tanto `/health` quanto `/api/v1/status` retornam
`Cache-Control: no-store`.

### 8.2 `StatusDTO`

```json
{
  "data": {
    "version": "0.1.0",
    "commit": "unknown",
    "buildDate": "unknown",
    "port": 2748,
    "components": {
      "application": {"status": "healthy", "code": "OK", "message": "Application is ready.", "checkedAt": "2026-07-27T12:00:00Z"},
      "sqlite": {"status": "healthy", "code": "OK", "message": "SQLite is available.", "checkedAt": "2026-07-27T12:00:00Z"},
      "kubeconfig": {"status": "healthy", "code": "OK", "message": "Kubeconfig is loaded.", "checkedAt": "2026-07-27T12:00:00Z"},
      "context": {"status": "healthy", "code": "OK", "message": "Context is selected.", "checkedAt": "2026-07-27T12:00:00Z"},
      "cluster": {"status": "degraded", "code": "CLUSTER_UNAVAILABLE", "message": "The cluster is temporarily unavailable.", "checkedAt": "2026-07-27T12:00:00Z"},
      "metrics": {"status": "unknown", "code": "NOT_CHECKED", "message": "Metrics API has not been checked.", "checkedAt": null}
    },
    "selection": {
      "clusterProfileId": 1,
      "context": "development",
      "cluster": "dev-cluster",
      "scopeId": 7,
      "scopeName": "Finance",
      "scopeMode": "list",
      "scopeSource": "saved",
      "defaultNamespace": "payments",
      "namespaceCount": 3,
      "generation": "gen_42"
    }
  }
}
```

Ausência de build metadata usa `unknown`, não dado fictício.
As seis chaves de `components` são obrigatórias. Metrics API indisponível ou
desconhecida nunca muda o estado de aplicação/SQLite nem o código HTTP.

`selection` é `SelectionSummaryDTO | null`. Ela é null quando não há
profile/contexto ativo e validado. Nesse estado, `context`, `cluster`
e `metrics` permanecem `unknown` com códigos públicos `NOT_SELECTED` ou
`NOT_CHECKED`; kubeconfig disponível pode continuar saudável. Quando há
profile/contexto selecionado, mas nenhum scope ativo, `scopeId` e `scopeName`
são null, `scopeMode`/`defaultNamespace` são null, `scopeSource` é `none`,
`namespaceCount` é 0 e a geração continua presente.

### 8.3 Canal interno de controle

Este canal não pertence a `/api/v1`, não é acessado pelo browser e preserva o
contrato nativo comprovado na Fase 1:

| Método/rota | Request | Response | Uso |
| --- | --- | --- | --- |
| `GET /_kubepeep/control/v1/status` | vazio | `ControlIdentityDTO`, 200 | provar que a instância publicada está ativa |
| `POST /_kubepeep/control/v1/stop` | vazio | `ControlIdentityDTO`, 200 | provar identidade e cancelar o contexto raiz uma vez |

Ambos exigem peer loopback, Host exatamente `127.0.0.1:<porta-publicada>`,
Origin ausente e `X-KubePeep-Control-Token` comparado em tempo constante. Query
e body são proibidos. O cliente usa timeout total de 2 segundos. Respostas têm
`Content-Type: application/json`, `Cache-Control: no-store` e
`X-Content-Type-Options: nosniff`; decoder estrito e limite de 64 KiB também
valem para a prova recebida.

```json
{
  "schema": 1,
  "instance_id": "inst_...",
  "pid": 12345,
  "fingerprint": "platform-start-fingerprint",
  "port": 2748,
  "protocol": "kubepeep-control/v1"
}
```

O token nunca aparece na resposta. O cliente compara os seis campos com
`instance.json` antes de considerar `status` válido ou `stop` aceito. Falhas:
400 para query/body, 401 para token, 403 para peer/Host/Origin, 404/405 para
rota/método e 500 sanitizado. `stop` escreve e faz flush da prova antes de um
`sync.Once` cancelar o contexto. Estado ausente ou comprovadamente obsoleto é
sucesso idempotente do comando CLI; identidade divergente nunca sinaliza nem
encerra outro PID.

## 9. Contextos e profile

| Método/rota | Classe | Request | Response | Autorização | Erros específicos |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/cluster/profiles` | MVP | vazio | `ClusterProfileDTO[]` | Host/origin local; sem RBAC; paths somente como display sanitizado | — |
| `GET /api/v1/contexts` | MVP | query `clusterProfileId` positiva e obrigatória | `ContextDTO[]`, 200 | Host/origin local; leitura do kubeconfig do profile | `VALIDATION_FAILED`, `NOT_FOUND`, `KUBECONFIG_NOT_FOUND`, `KUBECONFIG_INVALID` |
| `POST /api/v1/contexts/select` | MVP | `SelectContextRequest` | `SelectionDTO`, 200 | CSRF; profile/contexto devem existir | `CONTEXT_NOT_FOUND`, `KUBECONFIG_NOT_FOUND`, `KUBECONFIG_INVALID`, `GENERATION_CHANGED` |
| `GET /api/v1/cluster/profile` | MVP | vazio | `ClusterProfileDTO` ativo | Host/origin local; sem RBAC | `NOT_FOUND` |

Não existe rota web para criar profile ou enviar path/conteúdo de kubeconfig no
MVP. Antes de servir a API, o bootstrap resolve o conjunto ordenado pela
precedência canônica, normaliza os paths e, sob transação, reutiliza o profile
com conjunto exatamente igual ou cria um novo. O primeiro recebe `isDefault`;
profiles posteriores só se tornam default por seleção explícita. Fingerprints
de arquivo permanecem em memória e não participam da identidade persistida.
`GET /api/v1/cluster/profiles` é a superfície sanitizada para descobrir os IDs
que podem ser usados no seletor de contextos.

A precedência de source no startup é: `--kubeconfig` explícito,
`KUBECONFIG`, profile persistido com `isDefault=true` e, somente se nenhum
desses existir, o path recomendado da plataforma. Para o profile resolvido, a
precedência do contexto é `--context`, `context_name` persistido e
`current-context` somente no primeiro reconcile de um profile ainda sem
seleção. Uma fonte ou contexto escolhido e inválido nunca cai silenciosamente
para o próximo; o processo mantém o shell local em HTTP 200 degradado e expõe
erro sanitizado. Sem profile persistido e sem arquivo recomendado existente,
não se cria profile vazio.

`--namespace` aceita exatamente um nome, rejeita `*` e falha como uso inválido
antes do startup se a sintaxe Kubernetes estiver errada. Ele cria um scope
`single` efêmero, somente em memória, aplicado uma vez ao primeiro contexto
válido do processo; não exige `list namespaces`, não cria linha no SQLite e não
sobrescreve scope salvo. No DTO ele aparece com `scopeId`/`scopeName` null,
`scopeMode: "single"`, `scopeSource: "cli"`, o nome em `defaultNamespace` e
`namespaceCount: 1`. Depois de qualquer seleção explícita de scope, o flag já
consumido não volta a substituir a intenção da UI.

`SelectContextRequest`:

```json
{
  "clusterProfileId": 1,
  "context": "development",
  "setDefault": true,
  "expectedGeneration": "gen_41"
}
```

A rota não aceita token, certificado ou conteúdo/path arbitrário. O conjunto de
kubeconfigs vem do profile explícito e validado. Depois de validar localmente
profile e contexto, a mesma transação atualiza `context_name` e, quando
`setDefault=true`, troca o único profile default. Só depois do commit o serviço
cria a nova geração, cancela a anterior, rotaciona o nonce CSRF e tenta a
conexão. Cluster offline ou autenticação temporariamente indisponível não
desfazem a seleção: a resposta é 200 com `components.cluster` degradado ou
desconhecido. Falha de parse/path/contexto ocorre antes do commit e preserva
banco, geração e nonce.

`SelectionDTO`:

```json
{
  "data": {
    "clusterProfileId": 1,
    "context": "development",
    "cluster": "dev-cluster",
    "scopeId": 7,
    "scopeName": "Finance",
    "scopeMode": "list",
    "scopeSource": "saved",
    "defaultNamespace": "payments",
    "namespaceCount": 3,
    "generation": "gen_42",
    "components": {
      "cluster": {"status": "degraded", "code": "CLUSTER_UNAVAILABLE", "message": "The cluster is temporarily unavailable.", "checkedAt": "2026-07-27T12:00:00Z"}
    }
  }
}
```

`scopeId` e `scopeName` podem ser null imediatamente após trocar para um
profile/contexto sem escopo salvo selecionado. `scopeSource` distingue `saved`,
`cli` e `none`; `scopeMode` é `single`, `list`, `all` ou null. Todo estado em
`components` usa o `ComponentState` completo, inclusive em respostas degradadas
ou desconhecidas.

`ContextDTO`:

```json
{
  "clusterProfileId": 1,
  "name": "development",
  "cluster": "dev-cluster",
  "selected": true
}
```

`ClusterProfileDTO` pode expor paths apenas como display sanitizado:

```json
{
  "id": 1,
  "name": "Development",
  "context": "development",
  "isDefault": true,
  "kubeconfigFiles": [
    {"position": 0, "displayPath": "~/.kube/config"}
  ]
}
```

`context` é string ou null quando o kubeconfig do profile ainda não possui
contexto selecionado. A rota plural retorna o mesmo DTO, ordenado por
`isDefault DESC, name ASC, id ASC`; nunca retorna o path normalizado completo,
fingerprint, conteúdo ou credencial.

## 10. Namespaces e escopos

| Método/rota | Classe | Request | Response | Autorização | Paginação/erros |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/namespaces` | MVP | query comum | `NamespaceDTO[]` | `list namespaces` | cursor; 403 real |
| `GET /api/v1/namespace-scopes` | MVP | `limit`, `continue`, `search` | `NamespaceScopeDTO[]`, 200 | Host/origin local; storage local | cursor local |
| `POST /api/v1/namespace-scopes` | MVP | `NamespaceScopeWriteRequest` | `NamespaceScopeDTO`, 201 | CSRF; `all` exige `list namespaces` | 403 real, validação/conflito |
| `GET /api/v1/namespace-scopes/{id}` | MVP | vazio | `NamespaceScopeDTO`, 200 | Host/origin local; storage local | 404 |
| `PUT /api/v1/namespace-scopes/{id}` | MVP | body + `version` + `expectedGeneration` | `NamespaceScopeDTO`, 200; `meta.generation` muda se ativo | CSRF; `all` exige `list namespaces` | 404; 409 versão/geração; profile/contexto imutáveis |
| `DELETE /api/v1/namespace-scopes/{id}` | MVP | `NamespaceScopeDeleteRequest` | 204 se inativo; `SelectionDTO`, 200, se ativo | CSRF; substituto `all` revalida `list namespaces` | 404; 409 versão/geração ou ativo sem substituto |
| `POST /api/v1/namespace-scopes/validate` | MVP | `NamespaceScopeValidateRequest` | `NamespaceScopeValidationDTO`, 200 | CSRF; existência só se permitida | parcial |
| `POST /api/v1/namespace-scopes/{id}/select` | MVP | `SelectNamespaceScopeRequest` | `SelectionDTO`, 200 | CSRF; `all` revalida `list namespaces` | 403 real, 404, `GENERATION_CHANGED`, `SELECTION_MISMATCH` |

`NamespaceScopeWriteRequest`:

```json
{
  "clusterProfileId": 1,
  "name": "Aplicações Financeiras",
  "context": "development",
  "mode": "list",
  "namespaces": ["payments", "billing", "invoices"],
  "defaultNamespace": "payments"
}
```

Também é aceito `rawInput` em vez de `namespaces`, nunca ambos:

```json
{
  "clusterProfileId": 1,
  "name": "Aplicações Financeiras",
  "context": "development",
  "mode": "list",
  "rawInput": "payments,billing\ninvoices",
  "defaultNamespace": "payments"
}
```

Gramática canônica de `rawInput`:

- após remover BOM e whitespace externo, entrada iniciada por `[` ou `{` é
  JSON estrito: array de strings ou objeto com a única chave `namespaces`
  contendo esse array;
- entrada cujo primeiro conteúdo é `---`, `namespaces:` ou `- ` é YAML simples:
  sequência top-level de scalars string ou mapping apenas com `namespaces` e
  essa sequência; aliases, anchors, tags, múltiplos documentos, objetos
  aninhados e valores não string são rejeitados;
- qualquer outra entrada é texto de tokens bare separados por newline, vírgula,
  ponto e vírgula ou uma sequência de espaço/tab; nomes Kubernetes não contêm
  whitespace, portanto esse separador é não ambíguo;
- formato que parece JSON/YAML e falha no parse não recebe fallback de texto;
  retorna `VALIDATION_FAILED` com detail code `INVALID_NAMESPACE_INPUT`;
- cada item é trimado; vazios, inclusive strings vazias em JSON/YAML, contam em
  `discardedEmptyCount`; a primeira ocorrência preserva a ordem, ocorrências
  posteriores contam em `duplicateCount`; inválidos permanecem no relatório,
  e `*` é sempre inválido.

`NamespaceScopeValidateRequest` permite os mesmos campos, com `name` opcional.
O `clusterProfileId` elimina ambiguidade quando profiles diferentes contêm um
contexto com o mesmo nome.

No `PUT`, o mesmo objeto inclui `"version": 3` e
`"expectedGeneration": "gen_41"`; mismatch de qualquer precondition retorna
409 sem alteração. `clusterProfileId` e `context` precisam coincidir com o aggregate
existente e são imutáveis; mover um scope exige criar outro. Atualizar um scope
ativo cria nova geração depois do commit, cancela a anterior, invalida caches e
rotaciona o nonce, mesmo quando somente o nome mudou. Atualizar para `all`
revalida `list namespaces` antes do commit.

Exclusão usa:

```json
{
  "confirmed": true,
  "version": 3,
  "replacementScopeId": 8,
  "expectedGeneration": "gen_41"
}
```

`replacementScopeId` é obrigatório somente se o scope removido está ativo e
precisa pertencer ao mesmo profile/contexto. Excluir scope inativo não muda a
geração e retorna 204. Excluir o ativo valida o substituto e suas permissões,
remove o aggregate, ativa o substituto, cria uma nova geração e retorna
`SelectionDTO`; qualquer falha preserva scope e seleção anteriores.

`SelectNamespaceScopeRequest`:

```json
{
  "expectedGeneration": "gen_41"
}
```

Selecionar um scope cria uma nova geração e cancela a anterior. Em `all`, a
operação revalida `list namespaces`, usa exatamente a coleção retornada e nunca
materializa `*`. Se a permissão foi removida, a seleção falha com 403 e a
geração anterior permanece ativa.

A rota seleciona somente scope pertencente ao profile/contexto já ativo. Um ID
de outra origem retorna `SELECTION_MISMATCH` sem trocar profile, default,
contexto, banco, geração ou nonce; a UI precisa selecionar primeiro o
profile/contexto correspondente. Isso impede que um ID local cause troca
implícita de credencial/contexto.

Toda resposta acima que publica uma nova geração exige o rebootstrap de sessão
descrito em §8 antes da próxima mutação/SSE.

Relatório:

```json
{
  "data": {
    "valid": ["payments", "billing"],
    "validCount": 2,
    "duplicateCount": 1,
    "discardedEmptyCount": 1,
    "invalid": [
      {"input": "Bad_Name", "code": "INVALID_NAMESPACE_NAME"}
    ],
    "invalidCount": 1,
    "existence": {
      "checked": false,
      "reasonCode": "NAMESPACE_LIST_FORBIDDEN"
    }
  }
}
```

Falta de permissão para listar não invalida lista manual. O backend processa toda a entrada em uma requisição/transação.

`NamespaceDTO`:

```json
{
  "name": "payments",
  "phase": "Active",
  "selected": true
}
```

`NamespaceScopeDTO`:

```json
{
  "id": 7,
  "clusterProfileId": 1,
  "name": "Aplicações Financeiras",
  "context": "development",
  "mode": "list",
  "namespaces": ["payments", "billing", "invoices"],
  "defaultNamespace": "payments",
  "version": 3,
  "createdAt": "2026-07-27T12:00:00Z",
  "updatedAt": "2026-07-27T12:30:00Z"
}
```

## 11. Permissions

| Método/rota | Classe | Request | Response | Autorização | Erros |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/permissions` | MVP | `namespace`, `refresh`, `capability`, `resourceName` | `CapabilityMatrixDTO`, 200 | SAR/SSRR | `VALIDATION_FAILED`, `GENERATION_CHANGED`, `AUTHORIZATION_UNAVAILABLE` |

`namespace` é repetível até 20 itens, `capability` até 100 IDs e
`resourceName` até 20 nomes. Sem `namespace`, usam-se os primeiros 20 nomes do
scope ativo em ordem canônica e `truncated=true` se houver mais; sem
`capability`, usam-se todos os IDs abaixo. `resourceName` é opcional e forma o
produto apenas com IDs de policy `target`; sem ele, esses IDs consultam a
permissão geral com resourceName vazio. O total expandido não pode exceder 100
decisões. `refresh` é boolean único, default false, e ignora cache somente para
as chaves solicitadas. A rota não é proxy de SAR arbitrário.

Allowlist completa do MVP (`group=""` significa core):

| Capability ID | Group | Resource/subresource | Verb | Escopo / policy de resourceName |
| --- | --- | --- | --- | --- |
| `namespaces.list` | `""` | namespaces | list | cluster / vazio |
| `pods.list` | `""` | pods | list | namespace / vazio |
| `pods.get` | `""` | pods | get | namespace / target |
| `pods.watch` | `""` | pods | watch | namespace / vazio |
| `pods.logs.get` | `""` | pods/log | get | namespace / target |
| `pods.delete` | `""` | pods | delete | namespace / target |
| `pods.exec.create` | `""` | pods/exec | create | namespace / target |
| `pods.portforward.create` | `""` | pods/portforward | create | namespace / target |
| `events.list` | `""` | events | list | namespace / vazio |
| `events.watch` | `""` | events | watch | namespace / vazio |
| `deployments.list` | `apps` | deployments | list | namespace / vazio |
| `deployments.get` | `apps` | deployments | get | namespace / target |
| `deployments.watch` | `apps` | deployments | watch | namespace / vazio |
| `deployments.restart` | `apps` | deployments | patch | namespace / target |
| `deployments.scale` | `apps` | deployments/scale | update | namespace / target |
| `statefulsets.list` | `apps` | statefulsets | list | namespace / vazio |
| `statefulsets.get` | `apps` | statefulsets | get | namespace / target |
| `statefulsets.watch` | `apps` | statefulsets | watch | namespace / vazio |
| `statefulsets.scale` | `apps` | statefulsets/scale | update | namespace / target |
| `daemonsets.list` | `apps` | daemonsets | list | namespace / vazio |
| `daemonsets.get` | `apps` | daemonsets | get | namespace / target |
| `daemonsets.watch` | `apps` | daemonsets | watch | namespace / vazio |
| `jobs.list` | `batch` | jobs | list | namespace / vazio |
| `jobs.get` | `batch` | jobs | get | namespace / target |
| `jobs.watch` | `batch` | jobs | watch | namespace / vazio |
| `cronjobs.list` | `batch` | cronjobs | list | namespace / vazio |
| `cronjobs.get` | `batch` | cronjobs | get | namespace / target |
| `cronjobs.watch` | `batch` | cronjobs | watch | namespace / vazio |
| `services.list` | `""` | services | list | namespace / vazio |
| `services.get` | `""` | services | get | namespace / target |
| `services.watch` | `""` | services | watch | namespace / vazio |
| `ingresses.list` | `networking.k8s.io` | ingresses | list | namespace / vazio |
| `ingresses.get` | `networking.k8s.io` | ingresses | get | namespace / target |
| `ingresses.watch` | `networking.k8s.io` | ingresses | watch | namespace / vazio |
| `endpoint-slices.list` | `discovery.k8s.io` | endpointslices | list | namespace / vazio |
| `endpoint-slices.get` | `discovery.k8s.io` | endpointslices | get | namespace / target |
| `endpoint-slices.watch` | `discovery.k8s.io` | endpointslices | watch | namespace / vazio |
| `configmaps.list` | `""` | configmaps | list | namespace / vazio |
| `configmaps.get` | `""` | configmaps | get | namespace / target |
| `configmaps.watch` | `""` | configmaps | watch | namespace / vazio |
| `secrets.list` | `""` | secrets | list | namespace / vazio |
| `secrets.get` | `""` | secrets | get | namespace / target |
| `nodes.list` | `""` | nodes | list | cluster / vazio |
| `nodes.get` | `""` | nodes | get | cluster / target |
| `metrics.pods.list` | `metrics.k8s.io` | pods | list | namespace / vazio |

O backend separa `resource` e `subresource` no `Capability`; a barra da tabela é
somente notação compacta. Namespace fora do scope, ID/nome inválido, parâmetro
repetido indevido ou produto acima de 100 retorna `VALIDATION_FAILED`. Resposta:

```json
{
  "data": {
    "generation": "gen_42",
    "decisions": [],
    "complete": false,
    "truncated": false,
    "errors": [
      {"code":"AUTHORIZATION_UNAVAILABLE","message":"Authorization could not be confirmed."}
    ]
  }
}
```

`decisions` contém `Capability` de §6.3. Review incompleto produz decisão
`unknown`/erro parcial em HTTP 200, nunca `denied`; 503 só ocorre quando não é
possível construir nenhuma decisão para a geração ativa.

## 12. Preferências

| Método/rota | Classe | Request | Response | Autorização | Erros |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/preferences` | MVP | vazio | `PreferencesDTO`, 200 | Host/origin local; sem RBAC | — |
| `PUT /api/v1/preferences` | MVP | `PreferencesDTO` versionado | `PreferencesDTO`, 200 | CSRF | `UNKNOWN_FIELD`, `VALIDATION_FAILED`, `PREFERENCE_SENSITIVE_VALUE` |

Exemplo:

```json
{
  "version": 1,
  "ui": {
    "language": "en"
  },
  "logs": {
    "wrap": false,
    "timestamps": true,
    "tailLines": 200
  },
  "dashboard": {
    "logScanWindow": "15m",
    "sectionOrder": ["summary", "problems", "restarts", "workloads", "events", "logScan", "metrics"],
    "hiddenSections": []
  },
  "filters": {
    "workloads": {"version": 1, "items": []},
    "pods": {"version": 1, "items": []},
    "events": {"version": 1, "items": []},
    "logs": {"version": 1, "items": []}
  },
  "favorites": {
    "version": 1,
    "items": [
      {"id": "fav_1a2b3c4d5e6f7a8b", "kind": "pod", "namespace": "payments", "name": "api-abc"}
    ]
  }
}
```

`PreferencesDTO` é um snapshot completo: PUT exige todas as chaves do exemplo,
inclusive filter sets vazios, e rejeita campos ausentes/desconhecidos. GET
sempre materializa defaults. Enums, limites e o schema fechado de cada
`SavedFilterSet` estão em [data-model.md](data-model.md); nomes HTTP usam
camelCase e são mapeados para as chaves internas snake_case de forma fixa. O
endpoint não aceita merge patch, key/value arbitrário nem null.

A seção `favorites` (F7-01) fixa até 50 recursos apenas pela identidade
(`kind`, `namespace`, `name`): kinds permitidos são `pod`, `deployment`,
`statefulset`, `daemonset`, `job`, `cronjob`, `service`, `ingress`,
`endpointslice`, `configmap` e `secret` (Secret favoritável porque o detalhe
é metadata-only); namespace/name seguem o padrão de identificadores DNS e
alvos duplicados são rejeitados. Clientes anteriores podem omitir a seção:
um conjunto zero é tratado como vazio e regravado na forma canônica.

## 13. Dashboard

| Método/rota | Classe | Request | Response | Autorização | Erros/completude |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/dashboard/summary` | MVP | seleção ativa | `DashboardBlockDTO<SummaryDTO>`, 200 | por contador/recurso | parcial; 409/503/504 |
| `GET /api/v1/dashboard/problems` | MVP | query comum | `DashboardBlockDTO<ProblemPodDTO[]>`, 200 | `list/get pods`, events se usados | parcial/cursor; 409/503/504 |
| `GET /api/v1/dashboard/restarts` | MVP | `limit` default 10, máximo 50 | `DashboardBlockDTO<RestartDTO[]>`, 200 | `list pods` | parcial; 409/503/504 |
| `GET /api/v1/dashboard/events` | MVP | query comum | `DashboardBlockDTO<EventDTO[]>`, 200 | `list events` | parcial/cursor; 409/503/504 |
| `GET /api/v1/dashboard/namespace-health` | MVP | vazio (sem query) | `DashboardBlockDTO<NamespaceHealthDTO[]>`, 200 | herda pods/workloads | parcial; 409/503/504 |
| `POST /api/v1/dashboard/log-scan` | MVP | `LogScanRequest` | `DashboardBlockDTO<LogMatchDTO[]>`, 200 | CSRF + `get pods/log` por namespace | 400/403/409/429/503/504 |
| `GET /api/v1/metrics` | MVP opcional em runtime | query comum | `DashboardBlockDTO<MetricsDTO>`, 200 | discovery + `list pods.metrics.k8s.io` | 403/409/503 `FEATURE_UNAVAILABLE`/504 |

`DashboardBlockDTO<T>` usa o envelope comum e contém, dentro de `data`, os cinco
campos obrigatórios abaixo. `coverage` usa exatamente o schema de §5.2 e pode
ser null somente quando a consulta não é fan-out; `errors` nunca é null.

| Campo | Tipo/regra |
| --- | --- |
| `value` | `T`; coleção vazia/contadores explícitos quando o resultado autoritativo é zero |
| `complete` | boolean; true somente se toda a cobertura solicitada terminou |
| `truncated` | boolean; true quando qualquer budget/limite cortou o resultado |
| `coverage` | `CoverageDTO` ou null |
| `errors` | `PartialErrorDTO[]`; cada item contém `namespace` opcional, `code` e `message` públicos |

`meta` mantém `requestId`, `generation` e `collectedAt`. Falha de um
namespace/bloco retorna 200 parcial quando existe valor honesto; falha total
usa o erro autoritativo da tabela. Metrics API nunca muda o status dos demais
endpoints.

`LogScanRequest`:

```json
{
  "window": "15m",
  "tailLines": 200,
  "maxPods": 20,
  "maxConcurrentContainers": 4
}
```

Todos os quatro campos de `LogScanRequest` são opcionais e, quando ausentes,
usam respectivamente `15m`, 200, 20 e 4. `window` é somente `15m`, `30m`, `1h`
ou `4h`; `tailLines` fica entre 1 e 2.000, `maxPods` entre 1 e 50 e
`maxConcurrentContainers` entre 1 e 8. Tipo errado, zero, negativo, campo
desconhecido ou valor fora desses conjuntos retorna `VALIDATION_FAILED`; o
backend não reduz silenciosamente. Conteúdo nunca é persistido.

Contadores distinguem zero de ausência:

```json
{
  "data": {
    "value": {
      "namespaces": {"state": "available", "value": 3},
      "podsTotal": {"state": "available", "value": 18},
      "podsHealthy": {"state": "available", "value": 15},
      "podsProblematic": {"state": "available", "value": 3},
      "workloadsDegraded": {"state": "available", "value": 1},
      "restarts": {"state": "available", "value": 12},
      "warningEvents": {"state": "denied", "value": null},
      "possibleLogMatches": {"state": "notCollected", "value": null}
    },
    "complete": false,
    "truncated": false,
    "coverage": null,
    "errors": []
  }
}
```

`SummaryDTO` contém exatamente os oito contadores mostrados. Cada
`CounterDTO` contém `state` e `value`; `value` é inteiro não negativo somente
em `available`/`truncated` e null nos demais estados. Estados:
`available`, `denied`, `unavailable`, `notCollected`, `collecting`,
`truncated`.

DTO de restart:

```json
{
  "namespace": "payments",
  "pod": "api-abc",
  "owner": {"kind": "Deployment", "name": "api"},
  "container": "api",
  "containerType": "regular",
  "restarts": 12,
  "severity": "critical",
  "status": "CrashLoopBackOff",
  "lastReason": "Error",
  "ageSeconds": 840
}
```

`RestartDTO.severity` é derivado somente de `restarts`: 0 → `healthy`, 1–2 →
`attention`, 3–9 → `warning` e 10 ou mais → `critical`. Containers regulares,
init e efêmeros são contados separadamente; o ranking ordena por restarts
decrescente e desempata por namespace, pod, `containerType` e container. Esses
thresholds são contrato do MVP e só se tornam configuráveis em versão futura.

DTO de evento separa `objectKind` e `objectName` e preserva `count`.

`ProblemPodDTO` contém exatamente `namespace`, `pod`,
`owner: ResourceRef|null`, `container: string|null`,
`containerType: "regular"|"init"|"ephemeral"|null`, `status`,
`reason: string|null`, `message: string|null`, `source`, `severity` e
`ageSeconds`. `source` é `podStatus`, `containerWaiting`,
`containerTerminated`, `containerStatus`, `condition` ou `event`; `severity` é `warning` ou
`critical`; idade é inteira não negativa. Status/reason têm até 256 bytes e
message sanitizada até 4 KiB. Campo ausente permanece null; o backend não
fabrica owner ou causa.

Há no máximo um `ProblemPodDTO` por Pod. `PodDTO.problematic=true` se e somente
se alguma linha abaixo casar. Múltiplos motivos usam severidade e depois a
prioridade numérica menor; empate de container usa `regular`, `init`,
`ephemeral`, nome ascendente. Reason/message são os valores reais da fonte:

| Prioridade | Match real | Severidade/source |
| --- | --- | --- |
| 1 | phase `Failed` ou status reason `Evicted` | critical / `podStatus` |
| 2 | last/current terminated reason `OOMKilled` | critical / `containerTerminated` |
| 3 | waiting reason `CrashLoopBackOff`, `CreateContainerConfigError` ou `RunContainerError` | critical / `containerWaiting` |
| 4 | Event Warning reason `Unhealthy` cuja message começa, case-insensitive, por `Liveness probe failed`, nos últimos 15 min | critical / `event` |
| 5 | waiting reason `ImagePullBackOff` ou `ErrImagePull` | warning / `containerWaiting` |
| 6 | condition `PodScheduled=False` com reason `Unschedulable` | warning / `condition` |
| 7 | Event Warning `FailedScheduling` nos últimos 15 min | warning / `event` |
| 8 | Event Warning `Unhealthy` cuja message começa por `Readiness probe failed`, nos últimos 15 min | warning / `event` |
| 9 | container `ready=false` em Pod `Running` com idade mínima de 2 min, sem match anterior | warning / `containerStatus` |
| 10 | phase `Pending` por pelo menos 5 min, sem match anterior | warning / `podStatus` |

O adapter normaliza `events.k8s.io/v1.regarding` e
`core/v1.involvedObject`. Um Event só pode casar se kind=`Pod`, namespace,
name e UID forem exatamente os do Pod; UID ausente não recebe fallback por
nome. Seu instante normalizado usa, na ordem, `eventTime`,
`series.lastObservedTime`, `lastTimestamp`, `metadata.creationTimestamp`; sem
instante, ou fora do intervalo inclusivo `[capturedNow-15m,
capturedNow+1m]`, ele não participa.

Entre Events do mesmo match vence timestamp mais novo, depois `count` maior,
reason lexical e UID lexical. Condition sempre vence Event porque ocupa
prioridade 6 antes da 7. Matches vindos de Event/condition deixam
`container`/`containerType` null; status é sempre a phase real do Pod e
reason/message são copiados da fonte vencedora. Evento ausente/negado não é
inventado e não impede matches de status. O relógio é capturado uma vez por
request e injetável em teste; idade negativa por clock skew vira zero.

Classificação fechada de `WorkloadDTO.status`: Deployment, StatefulSet e
DaemonSet exigem `status.observedGeneration >= metadata.generation`; Job e
CronJob usam os campos batch abaixo, que não dependem desse marcador. Se uma
prova/campo necessário faltar, o resultado é `Unknown`:

| Kind | Regras em ordem |
| --- | --- |
| Deployment | `Degraded` se condition `Progressing=False` com reason `ProgressDeadlineExceeded` ou `available < desired`; `Progressing` se `updated < desired` ou `ready < desired`; `Healthy` se ready/available/updated = desired |
| StatefulSet | `Degraded` se `ready < desired`; `Progressing` se `updated < desired`; senão `Healthy` |
| DaemonSet | `Degraded` se `numberUnavailable > 0` ou `ready < desired`; `Progressing` se `updated < desired`; senão `Healthy` |
| Job | `Failed` se condition `Failed=True` ou `failed > 0`; `Suspended` se `spec.suspend=true`; `Progressing` se `active > 0`; `Completed` se condition `Complete=True` ou completions desejadas foram atingidas; senão `Unknown` |
| CronJob | `Failed` se o Job pertencente mais recente terminou `Failed` nas últimas 24 h; `Suspended` se `spec.suspend=true`; `Progressing` se há active refs; `Healthy` se há `lastScheduleTime` e nenhum caso anterior; senão `Unknown` |

Para CronJob, “recente” significa `(capturedNow - completion/condition time) <=
24h`; Job sem owner UID correspondente não participa. Prioridade é
`Failed`/`Degraded`, `Suspended`, `Progressing`, `Completed`, `Healthy`,
`Unknown`; valor negado, truncado ou não coletado nunca vira saudável.

Mapeamento dos contadores uniformes:

| Kind | ready / desired / available / updated |
| --- | --- |
| Deployment | `readyReplicas` / `spec.replicas` (default 1) / `availableReplicas` / `updatedReplicas` |
| StatefulSet | `readyReplicas` / `spec.replicas` (default 1) / `availableReplicas` ou null se ausente / `updatedReplicas` |
| DaemonSet | `numberReady` / `desiredNumberScheduled` / `numberAvailable` / `updatedNumberScheduled` |
| Job | `status.ready` ou null / `spec.completions` (default 1) / `succeeded` / null |
| CronJob | tamanho de `status.active` / null / null / null |

`LogMatchDTO` contém exatamente: `namespace`, `pod`, `container`,
`workload: ResourceRef | null`, `timestamp: RFC3339 | null`, `excerpt` já
sanitizado (máximo 4 KiB), `reasonCode`, `redacted` e `truncated`. `reasonCode`
é `ERROR_KEYWORD`, `JSON_ERROR_LEVEL`, `JSON_ERROR_FIELD`, `STACK_TRACE`,
`OOM` ou `PANIC`; ele descreve o match, não confirma causa. Os três primeiros
identificadores são strings não vazias; os dois últimos campos são boolean.

O detector trabalha somente na janela/bytes já limitados, antes da redaction
do excerpt, e produz no máximo um match por linha. Para texto, faz comparação
ASCII case-insensitive com boundary não alfanumérico para palavras e reconhece
exatamente, nesta ordem de prioridade: `panic`; `oom`; `segmentation fault`;
`exception`; `fatal`; `error`; `failed`; `failure`; `timeout`; `refused`;
`unavailable`; `killed`. `panic` mapeia a `PANIC`, `oom` a `OOM` e os demais a
`ERROR_KEYWORD`.

Quando a linha inteira é um único objeto JSON válido, com profundidade máxima
8 e sem trailing content, somente estes campos top-level são examinados:
`level`, `severity`, `message`, `msg`, `error`, `stack`, `timestamp`, `time`.
`level`/`severity` string igual a `error`, `fatal`, `panic` ou `critical`
produz `JSON_ERROR_LEVEL`; `error` produz `JSON_ERROR_FIELD` somente se for
string com trim não vazio, boolean true, número diferente de zero, array não
vazio ou objeto não vazio. Null, false, zero, string vazia/whitespace e
array/objeto vazio não casam. `stack` casa somente quando é string com trim não
vazio e produz `STACK_TRACE`;
`message`/`msg` também passam pela lista textual. A prioridade final é
`PANIC`, `OOM`, `STACK_TRACE`, `JSON_ERROR_LEVEL`, `JSON_ERROR_FIELD`,
`ERROR_KEYWORD`.

Primeiro escolhe-se o reason pela prioridade acima. Empate usa a ordem de campo
`message`, `msg`, `error`, `stack`, `level`, `severity`, e depois a linha
textual. O excerpt é o valor string do campo vencedor; boolean/número usa sua
representação JSON, e array/objeto de `error` usa JSON canônico compacto com
chaves lexicais. Para `JSON_ERROR_LEVEL`, `level` vence `severity`. Todo valor
é limitado pelos mesmos depth/bytes, passa pela redaction central e só então é
truncado a 4 KiB em boundary UTF-8. Timestamp usa primeiro
`timestamp`/`time` string RFC 3339,
depois o timestamp do stream Kubernetes, senão null. JSON inválido é tratado
como texto; nenhum erro de parse aparece para o usuário e nenhum conteúdo é
logado ou persistido.

`MetricsDTO` contém `collectedAt` RFC 3339, `windowSeconds` inteiro positivo,
`pods: PodMetricDTO[]`, `topCPU: MetricRankDTO[]` e
`topMemory: MetricRankDTO[]`. `PodMetricDTO` contém `namespace`, `pod`,
`cpuMillicores`, `memoryBytes` e `containers`; cada container repete `name`,
`cpuMillicores` e `memoryBytes`. Os valores são inteiros não negativos já
normalizados de `resource.Quantity`. `MetricRankDTO` contém `namespace`,
`pod`, `cpuMillicores` e `memoryBytes`; cada ranking tem no máximo dez itens.
Ausência da Metrics API usa `FEATURE_UNAVAILABLE`, nunca números fabricados.

## 14. Workloads

Kinds aceitos para leitura: `deployments`, `statefulsets`, `daemonsets`, `jobs`,
`cronjobs`. Restart aceita somente `deployments`; scale aceita somente
`deployments` e `statefulsets`.

| Método/rota | Classe | Request/response | Autorização | Erros |
| --- | --- | --- | --- | --- |
| `GET /api/v1/workloads` | MVP | query de §5.3; `WorkloadDTO[]`, 200 | `list` de cada kind | 403/409/410/503/504; parcial em 200 |
| `GET /api/v1/workloads/{kind}/{namespace}/{name}` | MVP | vazio; `WorkloadDetailDTO`, 200 | `get` kind/alvo | 403/404/409/503/504 |
| `GET /api/v1/workloads/{kind}/{namespace}/{name}/yaml` | MVP | vazio; YAML, 200 | `get` kind/alvo | 403/404/409/413/503/504 |
| `POST /api/v1/workloads/{kind}/{namespace}/{name}/restart` | MVP | `RestartRequest`; `ActionAcceptedDTO`, 202 | CSRF + `patch apps/deployments` com resourceName | 403/404/409; kind não suportado |
| `PUT /api/v1/workloads/{kind}/{namespace}/{name}/scale` | MVP | `ScaleRequest`; `ScaleResultDTO`, 200 | CSRF + `update apps/{deployments|statefulsets}/scale` com resourceName | 403/404/409; kind não suportado |

Restart aceita apenas Deployment no MVP e exige `Idempotency-Key`.

O backend envia um strategic merge patch mínimo que altera somente
`spec.template.metadata.annotations["kubectl.kubernetes.io/restartedAt"]`, com
timestamp UTC RFC 3339 gerado pelo servidor, preservando as demais annotations
e todo campo não relacionado. `expectedResourceVersion` é incluído como
precondition otimista; divergência retorna 409. A semântica da chave idempotente
é a de §20.

`ActionTargetDTO`, repetido em toda ação, precisa coincidir canonicamente com o
path e a seleção ativa. O path usa plural lowercase e o DTO usa Kind Kubernetes:
`deployments` ↔ `Deployment`, `statefulsets` ↔ `StatefulSet` e Pod path ↔
`Pod`; qualquer outra combinação retorna `VALIDATION_FAILED` antes de SAR.

```json
{
  "clusterProfileId": 1,
  "context": "development",
  "namespace": "payments",
  "kind": "Deployment",
  "name": "api"
}
```

Os campos `action` e `consequenceCode` são enums estáveis e validados pelo
servidor; não são descrições livres fornecidas pelo browser.

```json
{
  "confirmed": true,
  "action": "restart",
  "consequenceCode": "RECREATE_WORKLOAD_PODS",
  "target": {
    "clusterProfileId": 1,
    "context": "development",
    "namespace": "payments",
    "kind": "Deployment",
    "name": "api"
  },
  "expectedGeneration": "gen_42",
  "expectedResourceVersion": "12345"
}
```

Scale:

```json
{
  "replicas": 3,
  "confirmed": true,
  "action": "scale",
  "consequenceCode": "CHANGE_REPLICA_COUNT",
  "target": {
    "clusterProfileId": 1,
    "context": "development",
    "namespace": "payments",
    "kind": "StatefulSet",
    "name": "ledger"
  },
  "expectedGeneration": "gen_42",
  "expectedResourceVersion": "12345"
}
```

Scale usa `UpdateScale`/verbo Kubernetes `update` no subresource `scale`; não
usa `patch`. Kinds diferentes dos dois allowlisted retornam
`VALIDATION_FAILED` antes de SAR.

`ActionAcceptedDTO` de restart/delete informa aceitação, não “rollout/remoção
concluído”:

```json
{
  "data": {
    "accepted": true,
    "action": "restart",
    "target": {"clusterProfileId": 1, "context": "development", "namespace": "payments", "kind": "Deployment", "name": "api"},
    "generation": "gen_42",
    "resourceVersion": "12346"
  }
}
```

`ScaleResultDTO` usa o mesmo núcleo, acrescenta `replicas` e confirma somente a
atualização aceita pelo subresource, não disponibilidade futura dos Pods.
Em `ActionAcceptedDTO`, `resourceVersion` é string ou null: restart retorna a
versão observada no patch; delete pode retornar null quando a resposta
Kubernetes não publica uma nova versão do objeto.

`WorkloadDTO` contém exatamente os campos do exemplo:

```json
{
  "namespace": "payments",
  "kind": "Deployment",
  "name": "api",
  "ready": 2,
  "desired": 3,
  "available": 2,
  "updated": 3,
  "status": "Degraded",
  "ageSeconds": 86400
}
```

`kind` é `Deployment`, `StatefulSet`, `DaemonSet`, `Job` ou `CronJob`;
`ready`, `desired`, `available` e `updated` são inteiros não negativos ou null
quando o kind não possui aquela medida. `status` é `Healthy`, `Progressing`,
`Degraded`, `Suspended`, `Completed`, `Failed` ou `Unknown`.

`WorkloadDetailDTO` contém exatamente:

| Campo | Tipo/regra |
| --- | --- |
| `metadata` | `ResourceMetadataDTO` |
| `kind` | mesmo enum de `WorkloadDTO` |
| `ready`, `desired`, `available`, `updated` | inteiros não negativos ou null, com a mesma semântica da lista |
| `status` | mesmo enum da lista |
| `selector` | map string/string ou null, máximo 64 pares/16 KiB |
| `restartAt` | valor allowlisted da annotation de restart, RFC 3339 ou null |
| `conditions` | `ConditionDTO[]`, máximo 64 |
| `containers` | `ContainerSpecDTO[]`, máximo 128 |
| `related` | `ResourceRef[]`, máximo 256; somente relações necessárias à tela e autorizadas |

Nenhuma outra annotation é exposta. O YAML continua separado e sob demanda.

## 15. Pods e logs

| Método/rota | Classe | Request/response | Autorização | Erros |
| --- | --- | --- | --- | --- |
| `GET /api/v1/pods` | MVP | query de §5.3; `PodDTO[]`, 200 | `list pods` | 403/409/410/503/504; parcial em 200 |
| `GET /api/v1/pods/{namespace}/{name}` | MVP | vazio; `PodDetailDTO`, 200 | `get pods` | 403/404/409/503/504 |
| `GET /api/v1/pods/{namespace}/{name}/yaml` | MVP | vazio; YAML, 200 | `get pods` | 403/404/409/413/503/504 |
| `DELETE /api/v1/pods/{namespace}/{name}` | MVP | `PodDeleteRequest`; `ActionAcceptedDTO`, 202 | CSRF + `delete pods` resourceName | 403/404/409 |
| `GET /api/v1/pods/{namespace}/{name}/logs` | MVP | `LogReadQuery`; `LogReadDTO`, 200 JSON | `get pods/log` | 400/403/404/409/503/504 |
| `GET /api/v1/pods/{namespace}/{name}/logs/stream` | MVP | `LogFollowQuery`; SSE via `fetch` | CSRF/Origin + `get pods/log` | 400/403/404/409/429/503/504 ou stream events |
| `POST /api/v1/pods/{namespace}/{name}/exec` | MVP | `ExecInit`; `ExecTicketDTO`, 201 | CSRF + `create pods/exec` | Origin/limits |
| `POST /api/v1/pods/{namespace}/{name}/port-forward` | MVP | `PortForwardCreateRequest`; `PortForwardDTO`, 201 | CSRF + `create pods/portforward` | 400/403/404/409/429/503/504 |

Delete:

```json
{
  "confirmed": true,
  "action": "deletePod",
  "consequenceCode": "DELETE_POD",
  "target": {
    "clusterProfileId": 1,
    "context": "development",
    "namespace": "payments",
    "kind": "Pod",
    "name": "api-abc"
  },
  "expectedGeneration": "gen_42",
  "expectedUid": "...",
  "expectedResourceVersion": "12345"
}
```

Logs comuns retornam somente `LogReadDTO` JSON no MVP; a ação explícita de
download do frontend cria um Blob em memória a partir desse DTO e não ativa um
segundo formato/arquivo no servidor:

```json
{
  "data": {
    "container": "api",
    "previous": false,
    "lines": [
      {"timestamp": "2026-07-27T12:00:00Z", "text": "redacted text", "truncated": false}
    ],
    "truncated": false
  }
}
```

`LogReadQuery` e `LogFollowQuery` usam a mesma gramática fechada:

| Query | Obrigatório/default | Regra |
| --- | --- | --- |
| `container` | obrigatório | um DNS label Kubernetes, 1–63 bytes |
| `previous` | default `false` | somente `true` ou `false`; `true` é proibido em follow |
| `timestamps` | default `true` | somente `true` ou `false` |
| `tailLines` | default 200 | inteiro decimal 1–2.000 |
| `since` | ausente | `^[1-9][0-9]*(s\|m\|h)$`, sem composição, máximo 4 h |

Parâmetro repetido, vazio, encoding inválido ou campo não reconhecido retorna
`VALIDATION_FAILED`. `LogReadDTO.lines` contém no máximo 10 MiB serializados;
cada item contém `timestamp: RFC3339|null`, `text` sanitizado e `truncated`.
Timestamp é null quando `timestamps=false`. O DTO de topo contém ainda
`container`, `previous` e `truncated`; o último fica true quando qualquer
limite de linha/container/resposta foi atingido.

`PodDTO` contém exatamente os campos do exemplo:

```json
{
  "namespace": "payments",
  "name": "api-abc",
  "status": "Running",
  "ready": {"current": 1, "desired": 2},
  "restarts": 4,
  "node": "worker-1",
  "ip": "10.0.0.12",
  "owner": {"kind": "ReplicaSet", "name": "api-xyz"},
  "ageSeconds": 840,
  "problematic": true
}
```

`node`, `ip` e `owner` podem ser null; os demais campos sempre existem.
`ready.current`, `ready.desired`, `restarts` e `ageSeconds` são inteiros não
negativos.

`PodDetailDTO` contém exatamente:

| Campo | Tipo/regra |
| --- | --- |
| `metadata` | `ResourceMetadataDTO` |
| `summary` | `PodDTO` |
| `conditions` | `ConditionDTO[]`, máximo 64 |
| `containers` | `PodContainerDTO[]` com `type=regular`, máximo 128 |
| `initContainers` | `PodContainerDTO[]` com `type=init`, máximo 128 |
| `ephemeralContainers` | `PodContainerDTO[]` com `type=ephemeral`, máximo 128 |
| `relatedEvents` | `ResourceRef[]`, máximo 100 e somente eventos que o usuário pode obter |

Env vars, commands, args, mounts e campos capazes de expor Secret são
omitidos, não reduzidos a objetos genéricos.

## 16. Events, Network e Config

| Método/rota | Request | Response | Autorização | Paginação/erros |
| --- | --- | --- | --- | --- |
| `GET /api/v1/events` | query comum | `EventDTO[]`, 200 | `list events` | cursor/parcial; 403/409/410/503/504 |
| `GET /api/v1/services` | query comum | `ServiceDTO[]`, 200 | `list services` | cursor/parcial; 403/409/410/503/504 |
| `GET /api/v1/services/{namespace}/{name}` | vazio | `ServiceDetailDTO`, 200 | `get services` com resourceName | 403/404/409/503/504 |
| `GET /api/v1/services/{namespace}/{name}/yaml` | vazio | YAML, 200 | `get services` com resourceName | 403/404/409/413/503/504 |
| `GET /api/v1/ingresses` | query comum | `IngressDTO[]`, 200 | `list networking.k8s.io/ingresses` | cursor/parcial; 403/409/410/503/504 |
| `GET /api/v1/ingresses/{namespace}/{name}` | vazio | `IngressDetailDTO`, 200 | `get networking.k8s.io/ingresses` com resourceName | 403/404/409/503/504 |
| `GET /api/v1/ingresses/{namespace}/{name}/yaml` | vazio | YAML, 200 | `get networking.k8s.io/ingresses` com resourceName | 403/404/409/413/503/504 |
| `GET /api/v1/endpoint-slices` | query comum | `EndpointSliceDTO[]`, 200 | `list discovery.k8s.io/endpointslices` | cursor/parcial; 403/409/410/503/504 |
| `GET /api/v1/endpoint-slices/{namespace}/{name}` | vazio | `EndpointSliceDetailDTO`, 200 | `get discovery.k8s.io/endpointslices` com resourceName | 403/404/409/503/504 |
| `GET /api/v1/endpoint-slices/{namespace}/{name}/yaml` | vazio | YAML, 200 | `get discovery.k8s.io/endpointslices` com resourceName | 403/404/409/413/503/504 |
| `GET /api/v1/configmaps` | query comum | `ConfigMapListDTO[]`, 200 | `list configmaps` metadata-first | cursor/parcial; 403/409/410/503/504 |
| `GET /api/v1/configmaps/{namespace}/{name}` | vazio | `ConfigMapDetailDTO`, 200 | `get configmaps` com resourceName | 403/404/409/413/503/504 |
| `GET /api/v1/configmaps/{namespace}/{name}/yaml` | vazio | YAML, 200 | `get configmaps` com resourceName | 403/404/409/413/503/504 |
| `GET /api/v1/resources/{collection}/{namespace}/{name}/yaml-diff` | vazio | `LastAppliedDiffDTO`, 200 JSON | `get` do recurso (mesma autorização da rota YAML) | 404 coleção inválida; 403/404/409/413/503/504 |

`yaml-diff` (F7-02) compara o documento vivo contra o baseline
`kubectl.kubernetes.io/last-applied-configuration`. Coleções elegíveis:
`pods`, `deployments`, `statefulsets`, `daemonsets`, `jobs`, `cronjobs`,
`services`, `ingresses`, `endpoint-slices`, `configmaps` — **Secrets são
recusados** (404 de coleção) antes de qualquer fetch. O DTO contém
`absent` (sem baseline; nenhum valor zero é implícito), `truncated` e
`lines[{kind: same|added|removed, text}]`; o diff é limitado a 6.000 linhas
por lado e 4.000 linhas renderizadas, e a anotação last-applied é removida
do documento corrente para não poluir o resultado.
| `GET /api/v1/secrets` | query comum | `SecretMetadataDTO[]`, 200 | `list secrets`; PartialObjectMetadata | cursor/parcial; 403/409/410/503/504 |
| `GET /api/v1/secrets/{namespace}/{name}` | vazio | `SecretMetadataDTO`, 200 | `get secrets` com resourceName; PartialObjectMetadata | 403/404/409/503/504 |

Listas multi-namespace podem retornar 200 parcial com coverage/erros; ausência de
qualquer resultado autoritativo usa o erro HTTP da tabela. `GENERATION_CHANGED`
é 409 e cursor expirado é 410 conforme §4.

ConfigMap list usa PartialObjectMetadata quando possível para não carregar conteúdo; conteúdo só entra no detalhe autorizado. Secrets exigem `PartialObjectMetadata`; se o servidor não suportar resposta metadata-only, retornar `FEATURE_UNAVAILABLE` em vez de buscar o objeto completo. Secret não possui rota YAML.

Schemas fechados de lista:

| DTO | Campos e regras |
| --- | --- |
| `EventDTO` | `timestamp: RFC3339|null`, `namespace`, `objectKind`, `objectName`, `reason`, `message` sanitizada até 4 KiB, `count` não negativo, `source: string|null`, `type` (`Normal`, `Warning` ou `Unknown`) |
| `ServiceDTO` | `namespace`, `name`, `type` (`ClusterIP`, `NodePort`, `LoadBalancer`, `ExternalName` ou `Unknown`), `clusterIPs: string[]`, `ports: ServicePortDTO[]`, `selector` map limitado ou null e `externalEndpoints: ExternalEndpointDTO[]` |
| `IngressDTO` | `namespace`, `name`, `className: string|null`, `hosts: string[]`, `paths: IngressPathDTO[]` e `tlsHosts: string[]`; nenhum conteúdo ou nome de Secret TLS |
| `EndpointSliceDTO` | `namespace`, `name`, `addressType` (`IPv4`, `IPv6`, `FQDN` ou `Unknown`), `ports: EndpointSlicePortDTO[]` e `endpoints: EndpointDTO[]` |
| `ConfigMapListDTO` | `namespace`, `name`, `uid` e `creationTimestamp` RFC 3339 |
| `SecretMetadataDTO` | somente PartialObjectMetadata allowlisted, no formato abaixo |

Tipos de rede:

| DTO | Campos e regras |
| --- | --- |
| `ServicePortDTO` | `name: string|null`, `protocol` (`TCP`, `UDP` ou `SCTP`), `port` 1–65535, `targetPort: {type, value}` com `type` igual a `number` ou `name`, `nodePort: integer|null`, `appProtocol: string|null` |
| `ExternalEndpointDTO` | `address`, `port` 1–65535 e `protocol` (`TCP`, `UDP` ou `SCTP`), máximo 256 por Service |
| `IngressBackendDTO` | `serviceName` e `servicePort: {type, value}` com `type` igual a `number` ou `name`; outros tipos de backend retornam `FEATURE_UNAVAILABLE` para o detalhe, sem objeto cru |
| `IngressPathDTO` | `host`, `path`, `pathType` (`Exact`, `Prefix` ou `ImplementationSpecific`) e `backend: IngressBackendDTO` |
| `EndpointSlicePortDTO` | `name: string|null`, `protocol` igual a `TCP`, `UDP`, `SCTP` ou null, `port: integer|null`, `appProtocol: string|null` |
| `EndpointDTO` | `addresses: string[]`, `hostname: string|null`, `nodeName: string|null`, `zone: string|null`, `conditions: {ready:boolean|null, serving:boolean|null, terminating:boolean|null}` e `targetRef: ResourceRef|null`; máximo 1.000 por slice |

DTOs de detalhe:

| DTO | Campos e regras |
| --- | --- |
| `ServiceDetailDTO` | `metadata: ResourceMetadataDTO`, `summary: ServiceDTO`, `sessionAffinity` (`None` ou `ClientIP`), `externalTrafficPolicy` igual a `Cluster`, `Local` ou null, `ipFamilies` contendo `IPv4`/`IPv6` e `healthCheckNodePort: integer|null` |
| `IngressDetailDTO` | `metadata: ResourceMetadataDTO`, `summary: IngressDTO`, `defaultBackend: IngressBackendDTO|null` e `loadBalancerAddresses: string[]` |
| `EndpointSliceDetailDTO` | `metadata: ResourceMetadataDTO` e `summary: EndpointSliceDTO`; não acrescenta annotations ou payload Kubernetes cru |
| `ConfigMapDetailDTO` | `metadata: ResourceMetadataDTO`, `entries: ConfigMapEntryDTO[]`, `totalBytes` não negativo e `truncated` boolean |

`ConfigMapEntryDTO` contém `key`, `encoding` (`utf-8` ou `base64`), `value` e
`truncated`. Entradas são ordenadas por chave; `binaryData` usa base64, sem
conversão lossy. O conjunto serializado respeita o limite de 10 MiB e marca
truncamento, embora o limite nativo do Kubernetes normalmente seja menor.

`SecretMetadataDTO`:

```json
{
  "apiVersion": "v1",
  "kind": "Secret",
  "metadata": {
    "name": "registry",
    "namespace": "payments",
    "uid": "...",
    "creationTimestamp": "2026-07-27T12:00:00Z"
  }
}
```

## 16.1 Recursos cluster-scoped (ADR 0006)

Leitura cluster-scoped exige contexto e geração válidos; **não** exige
namespace scope. Filtro `namespace` retorna `VALIDATION_FAILED`. A cobertura
nunca reporta contagens de namespaces: `requestedNamespaces`,
`completedNamespaces` e `deniedNamespaces` são `0`/`[]`, e falhas parciais
chegam em `coverage.failed` com `namespace` ausente (nível cluster).

| Método/rota | Request | Response | Autorização | Paginação/erros |
| --- | --- | --- | --- | --- |
| `GET /api/v1/nodes` | query comum sem `namespace`; `status`: `Ready`, `NotReady`, `Unknown` | `NodeDTO[]`, 200 | `list nodes` (cluster) | cursor/parcial; 403/409/410/503/504 |
| `GET /api/v1/nodes/{name}` | vazio | `NodeDetailDTO`, 200 | `get nodes` com resourceName | 403/404/409/503/504 |
| `GET /api/v1/nodes/{name}/yaml` | vazio | YAML curado, 200 | `get nodes` com resourceName | 403/404/409/413/503/504 |

Schemas da família Nodes:

| DTO | Campos e regras |
| --- | --- |
| `NodeDTO` | `name`, `status` (`Ready`, `NotReady` ou `Unknown`), `ready` boolean, `roles` apenas da família de labels `node-role.kubernetes.io/*` (máx. 8), `kubeletVersion`, `internalIP: string|null`, `ageSeconds` |
| `NodeDetailDTO` | `metadata` (sem namespace), campos de `NodeDTO`, `conditions` (máx. 20), `capacity`/`allocatable` (máx. 32 entradas cada), `taints` (máx. 32; `key`/`value`/`effect`) e `truncated` boolean |
| YAML de Node | documento curado: apiVersion/kind/metadata (nome, uid, creationTimestamp, roles)/status (ready, conditions, internalIP, capacity, allocatable, taints, nodeInfo) + `x-kubepeep-omitted` listando annotations, managedFields, finalizers, spec, config, images e volumes. O objeto cru nunca é serializado |

## 17. Port-forwards

| Método/rota | Classe | Request | Response | Autorização | Erros |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/port-forwards` | MVP | vazio | `PortForwardDTO[]`, 200; geração atual, sem payload/contadores de tráfego | Host/origin local; sem RBAC adicional | `GENERATION_CHANGED` |
| `DELETE /api/v1/port-forwards/{id}` | MVP | `PortForwardDeleteRequest` | 204 | CSRF; geração atual | 404, `SESSION_GONE`, `GENERATION_CHANGED` |

Criação pelo endpoint do Pod:

```json
{
  "remotePort": 8080,
  "localPort": null,
  "confirmed": true,
  "action": "portForward",
  "consequenceCode": "EXPOSE_POD_PORT_LOCALLY",
  "target": {
    "clusterProfileId": 1,
    "context": "development",
    "namespace": "payments",
    "kind": "Pod",
    "name": "api-abc"
  },
  "expectedGeneration": "gen_42"
}
```

Exige `Idempotency-Key`. O backend escolhe porta quando null e retorna somente após listener local adquirido:

```json
{
  "data": {
    "id": "pf_...",
    "clusterProfileId": 1,
    "context": "development",
    "generation": "gen_42",
    "namespace": "payments",
    "pod": "api-abc",
    "remotePort": 8080,
    "localAddress": "127.0.0.1",
    "localPort": 49152,
    "status": "active",
    "createdAt": "2026-07-27T12:00:00Z",
    "expiresAt": "2026-07-27T20:00:00Z",
    "endedAt": null,
    "endReason": null
  }
}
```

`remotePort` é inteiro 1–65535. `localPort` é null ou inteiro 1024–65535. Com
null, o backend chama `net.Listen("tcp", "127.0.0.1:0")`, lê a porta atribuída
do listener já mantido aberto e não executa probe separado; explícita tenta
somente a porta pedida. Porta ocupada retorna 409 `CONFLICT`, limite de oito
sessões ativas retorna 429 `LIMIT_EXCEEDED`, input inválido retorna
`VALIDATION_FAILED`, negação real retorna `FORBIDDEN` e falhas Kubernetes usam
`CLUSTER_UNAVAILABLE`, `AUTHENTICATION_UNAVAILABLE`,
`AUTHORIZATION_UNAVAILABLE` ou `UPSTREAM_TIMEOUT` conforme a causa.

`PortForwardDTO.status` é `active`, `closed`, `expired`, `podGone` ou `failed`.
Toda sessão tem duração absoluta de 8 h; `expiresAt = createdAt + 8h`. Ao
expirar, o serviço fecha listener/conexões e publica `expired`. Se o canal
upstream retornar NotFound/EOF que prove término do Pod, fecha e publica
`podGone`; sem essa evidência não inventa detecção instantânea. Erro terminal
restante usa `failed`. Estados terminais definem `endedAt` e `endReason` com a
mesma literal pública de status e ficam visíveis no GET por 10 min, somente em
memória; depois somem. Troca de geração fecha e remove todas as sessões da
geração anterior.

`PortForwardDeleteRequest`:

```json
{
  "confirmed": true,
  "expectedGeneration": "gen_42"
}
```

Não existe owner por aba no app local single-user. Qualquer cliente same-origin
com CSRF válido pode encerrar uma sessão da geração atual; ID de outra geração
é rejeitado. O owner interno do registry é o contexto cancelável da geração,
não uma identidade web.

Encerrar sessão é idempotente dentro da mesma tentativa do frontend: uma
sessão já encerrada retorna `SESSION_GONE`, que a UI reconcilia como estado
final sem repetir contra outro ID.

## 18. SSE e watch

### 18.1 Logs follow

`GET /api/v1/pods/{namespace}/{name}/logs/stream` usa `text/event-stream` via
`fetch`. Aceita `container`, `timestamps`, `tailLines` e `since` com os mesmos
limites da leitura comum; `previous=true` é inválido para follow. A rota não
suporta retomada: nenhum evento recebe `id`, e `Last-Event-ID` retorna 400
`VALIDATION_FAILED` antes dos headers. Reconexão é uma nova autorização/request
com `since`; a UI sinaliza que pode existir duplicata ou lacuna e não afirma
continuidade perfeita.

`meta` é sempre o primeiro evento; heartbeat ocorre a cada 15 segundos:

```text
event: meta
data: {"requestId":"req_...","generation":"gen_42","container":"api","startedAt":"2026-07-27T12:00:00Z"}

event: line
data: {"timestamp":"2026-07-27T12:00:01Z","text":"sanitized","truncated":false}

event: heartbeat
data: {"generation":"gen_42","sentAt":"2026-07-27T12:00:15Z"}

event: error
data: {"code":"UPSTREAM_TIMEOUT","message":"The log stream timed out.","retryable":true,"retryAfterMs":500}
```

O `error` acima é um terminal. Em um encerramento normal, o stream envia em vez
dele um único terminal `end`, por exemplo:

```text
event: end
data: {"reason":"generation_changed","generation":"gen_42","truncated":false}
```

`end.reason` é `upstream_eof`, `container_terminated`, `generation_changed`,
`limit_reached`, `duration_reached` ou `server_shutdown`. O reader aceita no
máximo 64 KiB de uma linha; antes de emitir, serializa o DTO e, se escaping JSON
fizer a linha `data:` exceder 68 KiB, trunca `text` novamente em boundary UTF-8
até a linha serializada inteira caber e marca `truncated=true`. Fila
cheia/cliente lento envia `error` `LIMIT_EXCEEDED`
quando possível e encerra; nenhum payload é descartado silenciosamente. Follow
dura no máximo 4 horas, emite no máximo 10 MiB cumulativos de JSON serializado
em eventos `line` e usa fila limitada ao menor entre 1 MiB e 1.000 eventos. Se
a próxima linha ultrapassaria o cumulativo, ela não é emitida e o terminal é
`end` com `reason=limit_reached` e `truncated=true`.

Guards/erros antes de `200 text/event-stream` usam envelope HTTP normal. Depois
dos headers, `error` ou `end` é o último evento, seguido de flush/close. O
frontend usa parser incremental para enviar `X-KubePeep-CSRF` sem token na URL;
reconexão é explícita e respeita generation ID.

### 18.2 Atualizações de recursos

Rota interna do MVP:

```text
GET /api/v1/stream?topic=pods&topic=events
```

| Request | Response | Guards | Erros/eventos finais |
| --- | --- | --- | --- |
| tópicos allowlisted repetíveis, `Last-Event-ID` opcional | `text/event-stream` | Host e Origin exatos, CSRF via `fetch`, generation e capability por tópico | `CSRF_REJECTED`, `FORBIDDEN`, `GENERATION_CHANGED`, `AUTHORIZATION_UNAVAILABLE`, evento `reset` ou `error` |

Ela multiplexa somente tópicos allowlisted já autorizados. A query exige de um
a sete `topic`, rejeita duplicata, vazio, campo extra e valor desconhecido e
canonicaliza a ordem antes de criar o binding:

| `topic` | GVRs observados | DTO |
| --- | --- | --- |
| `pods` | core/v1 pods | `PodDTO` |
| `events` | core/v1 events | `EventDTO` |
| `workloads` | apps/v1 deployments/statefulsets/daemonsets e batch/v1 jobs/cronjobs | `WorkloadDTO` |
| `services` | core/v1 services | `ServiceDTO` |
| `ingresses` | networking.k8s.io/v1 ingresses | `IngressDTO` |
| `endpoint-slices` | discovery.k8s.io/v1 endpointslices | `EndpointSliceDTO` |
| `configmaps` | core/v1 configmaps, metadata somente no snapshot/evento | `ConfigMapListDTO` |

Secrets e conteúdo de ConfigMap nunca são tópicos. Antes dos headers, cada GVR
do tópico exige `list` e `watch` para toda a cobertura selecionada; negação
explícita retorna 403, decisão desconhecida retorna 503 e a UI usa refresh HTTP.
Cada watch usa `timeoutSeconds=300`, bookmarks e o backoff de
[architecture.md](architecture.md#11-watches-e-atualização-em-tempo-real).
Eventos:

```text
snapshot, added, modified, deleted, reset, error, heartbeat
```

`reset` exige refetch HTTP. Retomada por `Last-Event-ID` só existe se o manager ainda possuir o evento dentro de buffer curto; não há persistência.

Cada evento usa `event`, `id` quando retomável e uma única linha JSON compacta
em `data`. O ID `kpse1.<epoch-base64url-128-bit>.<sequence-base36>` é opaco e
fica ligado em memória a instance ID, generation e conjunto canônico de
tópicos; a sequência é global ao stream multiplexado. Evento individual tem no
máximo 64 KiB já serializado, incluindo a linha `data:`; objeto que não couber
produz `reset` terminal `event_too_large`, nunca truncamento estrutural. Sem
`Last-Event-ID`, o manager faz LIST e envia snapshot; com ID
ainda no ring, reproduz somente eventos posteriores. ID malformado retorna 400
`VALIDATION_FAILED` antes dos headers. ID válido fora do ring ou de outro
binding envia `reset` terminal.

Snapshot pode ser dividido e só é publicado no cliente após `final:true`:

```text
event: snapshot
id: kpse1.ZXBvY2gtMTI4LWJpdA.1
data: {"streamId":"str_...","topic":"pods","generation":"gen_42","sequence":1,"observedAt":"2026-07-27T12:00:00Z","snapshotId":"snap_...","chunk":0,"final":true,"resourceVersion":"123","items":[]}
```

O snapshot inicial inteiro é limitado ao primeiro valor atingido entre 10.000
items e 10 MiB de JSON serializado, além do limite de 64 KiB por chunk. O
servidor não marca `final:true` antes de saber que todo o snapshot cabe; se não
couber, envia `reset` `snapshot_too_large`, e o frontend descarta todos os
chunks daquele `snapshotId`.

`added`/`modified` carregam o DTO allowlisted do tópico; `deleted` carrega
somente `ResourceRef`:

```text
event: modified
id: kpse1.ZXBvY2gtMTI4LWJpdA.2
data: {"streamId":"str_...","topic":"pods","generation":"gen_42","sequence":2,"observedAt":"2026-07-27T12:00:01Z","resourceVersion":"124","object":{}}
```

Heartbeat ocorre a cada 15 segundos, não recebe `id` e não entra no ring:

```text
event: heartbeat
data: {"streamId":"str_...","generation":"gen_42","sentAt":"2026-07-27T12:00:15Z"}
```

`reset` e `error` são terminais alternativos, sem `id`; depois de exatamente um
deles e do flush o servidor fecha. Exemplo de reset:

```text
event: reset
data: {"streamId":"str_...","topic":"pods","generation":"gen_42","reason":"resume_unavailable","message":"State continuity was lost.","refetchRequired":true}
```

Exemplo alternativo de erro:

```text
event: error
data: {"streamId":"str_...","topic":"pods","generation":"gen_42","requestId":"req_...","code":"AUTHORIZATION_UNAVAILABLE","message":"Authorization could not be confirmed.","retryable":true,"retryAfterMs":500}
```

Razões de reset allowlisted: `resume_unavailable`,
`resource_version_expired`, `generation_changed`, `slow_consumer`,
`snapshot_too_large`, `event_too_large` e `server_shutdown`. Guard que falha
antes de `200 text/event-stream` usa erro HTTP normal; falha posterior usa evento
terminal. Ring/fila permanece limitado ao menor entre 1 MiB e 1.000 eventos e
nunca é persistido.

Se um watch ativo recebe 410, todos os subscribers afetados recebem `reset`
terminal `resource_version_expired`; o manager descarta o RV, executa novo LIST
e só então aceita um novo stream. Ele nunca mistura o relist com o snapshot já
publicado. A autorização de tópicos é all-or-nothing: qualquer GVR/namespace
negado impede o stream inteiro antes dos headers, sem subset silencioso.

SSE usa o encoder `pkg/sse` em `HandleRaw`, sem o wrapper de
`http.ResponseWriter` da cadeia padrão, com fila, heartbeat, budgets e
cancelamento próprios conforme ADR 0003.

## 19. Exec

`POST /api/v1/pods/{namespace}/{name}/exec` recebe e valida todo o `ExecInit`
antes de criar um ticket one-shot, curto e somente em memória. Ele não tenta
fazer upgrade por POST, operação que a API WebSocket do browser não oferece.
Não existe variante em que command/container/TTY sejam escolhidos no primeiro
frame.

Body completo do POST:

```json
{
  "container": "api",
  "command": ["/bin/sh"],
  "tty": true,
  "stdin": true,
  "confirmed": true,
  "action": "exec",
  "consequenceCode": "OPEN_INTERACTIVE_PROCESS",
  "target": {
    "clusterProfileId": 1,
    "context": "development",
    "namespace": "payments",
    "kind": "Pod",
    "name": "api-abc"
  },
  "expectedGeneration": "gen_42"
}
```

Após JSON estrito, limites, confirmação, alvo, geração e SAR, o servidor
canonicaliza método, path, namespace, pod e `ExecInit` e liga seu hash ao
ticket. Alterar qualquer parâmetro exige outro POST e outra confirmação.

Resposta 201 de criação do ticket, cujo TTL é exatamente 10 segundos:

```json
{
  "data": {
    "sessionId": "exec_...",
    "websocketUrl": "/api/v1/exec/exec_.../stream",
    "protocols": [
      "kubepeep.exec.v1",
      "kp-ticket.eyJ..."
    ],
    "expiresAt": "2026-07-27T12:00:10Z"
  }
}
```

`GET /api/v1/exec/{sessionId}/stream` é a rota interna de upgrade. O browser
passa exatamente o array `protocols` ao construtor WebSocket. O servidor aceita
`kubepeep.exec.v1`, extrai e consome o token `kp-ticket.*` oferecido em
`Sec-WebSocket-Protocol`, repete Origin/generation/SAR e então abre a sessão
remota. O token nunca aparece na URL, log ou protocolo selecionado na resposta.
Ticket vencido, reutilizado ou de outra geração retorna
`SESSION_GONE`/`GENERATION_CHANGED`.

Antes do `101`, o backend valida Host, Origin, ticket/binding one-shot,
generation, SAR e o limite de duas sessões. Falhas permanecem HTTP:
`FORBIDDEN`, `GENERATION_CHANGED`, `SESSION_GONE`, `LIMIT_EXCEEDED` ou
`AUTHORIZATION_UNAVAILABLE`. A resposta seleciona somente
`kubepeep.exec.v1`, nunca `kp-ticket.*`, e compressão WebSocket fica
desabilitada.

### 19.1 Encoding e schemas de frames

O upgrade não aceita `ExecInit` nem substituição de command/container/TTY.
Limites valem para a mensagem remontada depois de fragmentação. Dados usam
mensagem binária de até 65.536 bytes: o primeiro byte identifica o stream e os
65.535 restantes são payload opaco.

| Primeiro byte | Direção | Conteúdo |
| --- | --- | --- |
| `0x00` | browser → backend | stdin, somente se `ExecInit.stdin=true` |
| `0x01` | backend → browser | stdout |
| `0x02` | backend → browser | stderr; nunca emitido quando `tty=true` |

Controles usam uma mensagem texto UTF-8 por objeto JSON estrito, sem campos
desconhecidos/trailing data e com no máximo 4 KiB. Browser → backend:

```jsonl
{"type":"resize","columns":120,"rows":40}
{"type":"heartbeat","nonce":"hb_base64url"}
{"type":"close","stream":"stdin"}
{"type":"close","stream":"session"}
```

`resize` exige TTY e dimensões entre 1 e 4096; nonce base64url tem no máximo 64
caracteres. Backend → browser:

```jsonl
{"type":"ready","sessionId":"exec_...","generation":"gen_42","tty":true,"stdin":true}
{"type":"heartbeat","nonce":"hb_base64url"}
{"type":"exit","exitCode":0,"reason":"completed"}
{"type":"error","code":"GENERATION_CHANGED","message":"The active selection changed.","retryable":false}
```

Em `exit`, `exitCode` é inteiro 0–255 ou null quando o executor remoto não
fornece código; `reason` é `completed`, `remote_error`, `signal` ou `canceled`.
Saída normal 0 produz `completed`/0; saída normal não zero produz
`remote_error`/código; término reportado por signal produz `signal`/null;
fechamento explícito do usuário produz `canceled`/null. Encerramento local por
geração, timeout, policy, backpressure ou shutdown envia `error` allowlisted
quando possível e fecha sem fabricar `exitCode`. `exit` e `error` terminal são
mutuamente exclusivos, e nenhum dado vem depois deles.

Sequência obrigatória: `101`; abertura do stream remoto; `ready` como primeira
mensagem de aplicação; dados/controles; exatamente um `exit` quando o processo
remoto termina; close WebSocket. O browser aguarda `ready`; mensagem precoce ou
em direção/capability inválida encerra por policy violation. Nenhum dado é
enviado depois de `exit`.

### 19.2 Heartbeat, backpressure e close

- Ping de transporte a cada 15 segundos e Pong em até 10 segundos;
- depois de `ready`, o backend também inicia heartbeat de aplicação a cada 15
  segundos com nonce novo; o browser ecoa exatamente o mesmo objeto em até 10
  segundos, não inicia nonce próprio e nonce ausente/repetido/divergente é
  protocol violation;
- Ping/Pong e heartbeat de aplicação não renovam idle de 30 minutos;
- qualquer outro frame válido ou dado remoto renova idle; nenhuma atividade
  de aplicação por 30 minutos encerra a sessão;
- duração absoluta é 4 horas;
- fila de saída é o menor entre 1 MiB e 64 mensagens; stdout/stderr nunca é
  descartado silenciosamente, e saturação cancela o stream;
- desconexão abrupta cancela o exec e nunca permite retomada.

| Causa pós-101 | Terminal de aplicação quando possível | Close code |
| --- | --- | --- |
| processo remoto saiu 0 | `exit`, `exitCode:0`, `reason:completed` | 1000 |
| processo remoto saiu 1–255 | `exit`, código recebido, `reason:remote_error` | 1000 |
| processo remoto terminou por signal | `exit`, `exitCode:null`, `reason:signal` | 1000 |
| usuário enviou `close session` | `exit`, `exitCode:null`, `reason:canceled` | 1000 |
| generation mudou | `error` `GENERATION_CHANGED`, retryable false | 1001 |
| idle de 30 min | `error` `EXEC_IDLE_TIMEOUT`, retryable false | 1001 |
| duração de 4 h | `error` `EXEC_DURATION_LIMIT`, retryable false | 1001 |
| shutdown local | `error` `SERVER_SHUTDOWN`, retryable true | 1001 |
| autorização não pôde ser confirmada no recheck | `error` `AUTHORIZATION_UNAVAILABLE`, retryable true | 1008 |
| autorização foi negada no recheck | `error` `FORBIDDEN`, retryable false | 1008 |
| schema, direção, capability ou heartbeat inválido | `error` `PROTOCOL_VIOLATION`, retryable false | 1008 |
| fila/backpressure excedida | `error` `LIMIT_EXCEEDED`, retryable true | 1008 |
| mensagem remontada acima do limite | `error` `LIMIT_EXCEEDED`, retryable false | 1009 |
| alvo/container desapareceu após upgrade | `error` `EXEC_TARGET_GONE`, retryable false | 1008 |
| upstream retornou rede/429/5xx/timeout | `error` `CLUSTER_UNAVAILABLE`, retryable true | 1011 |
| autenticação upstream ficou indisponível | `error` `AUTHENTICATION_UNAVAILABLE`, retryable true | 1011 |
| setup/protocolo upstream falhou por outra causa | `error` `EXEC_UPSTREAM_ERROR`, retryable false | 1011 |
| falha interna sanitizada | `error` `INTERNAL`, retryable false | 1011 |

O backend envia no máximo um terminal, inicia o close handshake imediatamente
depois e usa como close reason somente o código público, truncado ao limite do
WebSocket. Se a conexão já não aceitar escrita, fecha com o mesmo close code
sem prometer que o terminal chegou.

Regras:

- `command` é argv de 1–64 strings, nunca string concatenada;
- `command[0]` tem 1–4.096 bytes UTF-8; cada argumento seguinte tem até 4.096
  bytes, o total é no máximo 32 KiB e nenhum item contém NUL;
- revalidar `create pods/exec` no POST e imediatamente antes do upgrade GET;
- frames de controle cobrem ready, resize, heartbeat, close, error e exit;
- stdout/stderr nunca são logados;
- desconexão não é retomável;
- troca de geração encerra com razão segura.

O transporte browser-backend usa `github.com/coder/websocket v1.8.15`, que
implementa masking e fragmentação; o KubePeep impõe os tipos, limites,
heartbeat, backpressure e encerramento descritos no ADR 0003. `pkg/ws` do
Ginger não é usado no caminho de `exec`.

## 20. Idempotência e cancelamento

| Operação | Semântica |
| --- | --- |
| GET | idempotente; cursor vinculado à geração |
| PUT scope/preferences/scale | mesmo body/precondition produz mesmo estado ou conflito explícito |
| DELETE | alvo já ausente retorna 404; UI pode tratar como estado final conhecido |
| POST context select | novo request substitui o anterior; antigo não publica seleção |
| POST log-scan | novo scan da mesma tela cancela o anterior; não persiste |
| POST restart | exige `Idempotency-Key`; repetição idêntica retorna resultado original no TTL |
| POST port-forward | exige `Idempotency-Key`; evita sessão duplicada |
| POST exec | nunca é repetido automaticamente |

`Idempotency-Key` possui 16–128 caracteres de `[A-Za-z0-9._~-]`; o servidor
valida somente formato e tamanho. O cliente deve gerar a chave com pelo menos
128 bits de aleatoriedade. O registry em memória usa a chave como índice e liga
a entrada a esta identidade completa:

```text
método HTTP
+ path/alvo canônico
+ clusterProfileId
+ generation ID
+ SHA-256 do JSON canônico do body
```

Reusar a chave alterando qualquer parte retorna `IDEMPOTENCY_CONFLICT`, mesmo
que o body e as portas sejam iguais em outro Pod. Requests concorrentes com a
mesma identidade aguardam a única execução bounded e recebem exatamente o
status/body original, com `Idempotency-Replayed: true`; não há segundo patch,
listener ou sessão. A entrada terminal permanece 10 minutos, contados da
primeira resposta, e não é persistida entre processos. Depois que uma mutação
foi enviada ao Kubernetes, a execução bounded pertence ao registry e termina
mesmo se o transporte HTTP desconectar; antes desse ponto, cancelamento impede
o envio. Em restart de processo ou resposta incerta fora do TTL, o frontend
reconcilia por GET e nunca repete cegamente.

Seleção de contexto/scope e todo PUT/DELETE de scope compartilham um único
coordenador monotônico de intenção. Cada request validado registra uma sequência
e cancela o predecessor; somente a sequência mais nova pode iniciar o commit
local e publicar geração/nonce. Sob o mesmo lock lógico, o serviço reconsulta
scope/seleção e compara `expectedGeneration` imediatamente antes da transação;
assim, um PUT/DELETE que era inativo mas se tornou ativo não escapa do
coordenador. Um predecessor já commitado antes da chegada do novo request é
causalmente válido, mas a intenção nova cria a geração seguinte quando
aplicável. Falha de parse/path/contexto/precondition antes do commit preserva
banco, geração e nonce; cluster/auth offline após o commit mantém a seleção nova
com componente degradado, sem rollback.

Se a conexão cair depois de uma mutação, o cliente não afirma falha nem repete cegamente; refaz GET do recurso/sessão e apresenta estado desconhecido até reconciliar.

## 21. Rotas pós-MVP/proibidas

Pós-MVP somente após nova especificação:

- edição/aplicação de YAML;
- CRUD genérico Kubernetes;
- autenticação própria;
- API remota;
- histórico de sessões;
- in-cluster config como prioridade;
- valores de Secret.

Não reservar uma rota funcional que sugira suporte atual.

## 22. Rastreabilidade F2

| Tarefa | Cobertura |
| --- | --- |
| F2-29 | classificação de rotas |
| F2-30 | DTOs próprios |
| F2-31 | envelopes e cursor |
| F2-32 | filtros/limites |
| F2-33 | códigos e JSON estrito |
| F2-34 | contratos por área |
| F2-35 | SSE e exec |
| F2-36 | idempotência/cancelamento |
| F2-45 | cache HTTP |
| F2-48–49 | resourceName e cursor composto |
| F2-50 | health |
| F2-52 | rotas YAML |
| F2-53 | EndpointSlices e Secrets |
| F2-54–55 | completude e limites |
| F2-57 | LIST/watch |
| F2-59 | lifecycle de port-forward |
| F2-60 | preferences |

Critérios cobertos pelo contrato: **MVP-01**, **MVP-05–20**, **MVP-22**.

## 23. Checklist de revisão do contrato

- [x] Cada rota MVP tem método, request, response, autorização e erros.
- [x] Nenhuma rota retorna objetos client-go crus.
- [x] Todos os bodies mutáveis são estritos e limitados.
- [x] Paginação multi-namespace declara cobertura e truncamento.
- [x] `FORBIDDEN` representa somente negação autoritativa; rejeição local usa
  `CSRF_REJECTED`, ainda que ambas tenham HTTP 403.
- [x] Secret não possui rota YAML nem fallback para objeto completo.
- [x] SSE e `exec` têm protocolo, biblioteca e cadeia HTTP decididos.
- [x] Os helpers Ginger foram avaliados; extensões próprias estão delimitadas.
- [ ] Exemplos passam por validação de schema gerado quando o harness existir.
