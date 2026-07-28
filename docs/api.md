# Contrato HTTP e streaming

> **Status:** revisado com as evidências e ADRs da Fase 1
>
> **Base URL:** origem loopback efetivamente publicada pelo processo
>
> **Versão:** `/api/v1`
>
> **Importante:** o envelope público é um requisito do Kube Peep.
> `pkg/response` será reutilizado nos casos comuns; cursores, agregações
> parciais, health e streams usam DTOs próprios compatíveis com este contrato.

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
      "truncated": true
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

O cursor é JSON canônico opaco autenticado por HMAC-SHA-256 com segredo
efêmero do processo. Ele inclui versão, expiração, hash da query, contexto,
escopo, geração e, em fan-out, o estado composto por namespace/kind e merge
determinístico. Alteração ou token de uma instância anterior retornam
`CURSOR_INVALID`; mudança de query/geração retorna `CURSOR_MISMATCH`; expiração
por TTL retorna `CURSOR_EXPIRED`.

Ordenação global só é exposta quando pode ser cumprida dentro de limites. Caso um endpoint só ordene a página atual, seu campo `sort` não é anunciado como global.

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

`decision`: `allowed`, `denied`, `unknown`.

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
sanitização e semântica de dependência degradada pertencem ao Kube Peep,
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
      "context": "development",
      "cluster": "dev-cluster",
      "scopeId": 7,
      "scopeName": "Finance",
      "namespaceCount": 3,
      "generation": "gen_42"
    }
  }
}
```

Ausência de build metadata usa `unknown`, não dado fictício.
As seis chaves de `components` são obrigatórias. Metrics API indisponível ou
desconhecida nunca muda o estado de aplicação/SQLite nem o código HTTP.

## 9. Contextos e profile

| Método/rota | Classe | Request | Response | Autorização | Erros específicos |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/cluster/profiles` | MVP | vazio | `ClusterProfileDTO[]` | Host/origin local; sem RBAC; paths somente como display sanitizado | — |
| `GET /api/v1/contexts` | MVP | `clusterProfileId` | `ContextDTO[]` | Host/origin local; leitura do kubeconfig do profile | `NOT_FOUND`, `KUBECONFIG_NOT_FOUND`, `KUBECONFIG_INVALID` |
| `POST /api/v1/contexts/select` | MVP | `SelectContextRequest` | `SelectionDTO` | CSRF; profile/contexto devem existir | `CONTEXT_NOT_FOUND`, `KUBECONFIG_NOT_FOUND`, `KUBECONFIG_INVALID`, `GENERATION_CHANGED` |
| `GET /api/v1/cluster/profile` | MVP | vazio | `ClusterProfileDTO` ativo | Host/origin local; sem RBAC | `NOT_FOUND` |

Não existe rota web para criar profile ou enviar path/conteúdo de kubeconfig no
MVP. Antes de servir a API, o bootstrap resolve o conjunto ordenado pela
precedência canônica, normaliza os paths e, sob transação, reutiliza o profile
com conjunto exatamente igual ou cria um novo. O primeiro recebe `isDefault`;
profiles posteriores só se tornam default por seleção explícita. Fingerprints
de arquivo permanecem em memória e não participam da identidade persistida.
`GET /api/v1/cluster/profiles` é a superfície sanitizada para descobrir os IDs
que podem ser usados no seletor de contextos.

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
    "scopeId": 7,
    "generation": "gen_42",
    "components": {
      "cluster": {"status": "degraded", "code": "CLUSTER_UNAVAILABLE"}
    }
  }
}
```

`scopeId` pode ser null imediatamente após trocar para um profile/contexto sem
escopo selecionado.

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
| `GET /api/v1/namespace-scopes` | MVP | `limit`, `continue`, `search` | `NamespaceScopeDTO[]` | Host/origin local; storage local | cursor local |
| `POST /api/v1/namespace-scopes` | MVP | `NamespaceScopeWriteRequest` | scope criado, 201 | CSRF; `all` exige `list namespaces` | 403 real, validação/conflito |
| `GET /api/v1/namespace-scopes/{id}` | MVP | vazio | scope | Host/origin local; storage local | 404 |
| `PUT /api/v1/namespace-scopes/{id}` | MVP | body + `version` | scope atualizado; `meta.generation` muda se ativo | CSRF; `all` exige `list namespaces` | 409 versão; profile/contexto imutáveis |
| `DELETE /api/v1/namespace-scopes/{id}` | MVP | `NamespaceScopeDeleteRequest` | 204 se inativo; `SelectionDTO`, 200, se ativo | CSRF; substituto `all` revalida `list namespaces` | 404; 409 se ativo sem substituto |
| `POST /api/v1/namespace-scopes/validate` | MVP | `NamespaceScopeValidateRequest` | relatório | CSRF; existência só se permitida | parcial |
| `POST /api/v1/namespace-scopes/{id}/select` | MVP | `SelectNamespaceScopeRequest` | `SelectionDTO` | CSRF; `all` revalida `list namespaces` | 403 real, 404, `GENERATION_CHANGED` |

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

`NamespaceScopeValidateRequest` permite os mesmos campos, com `name` opcional.
O `clusterProfileId` elimina ambiguidade quando profiles diferentes contêm um
contexto com o mesmo nome.

No `PUT`, o mesmo objeto inclui `"version": 3`; mismatch retorna 409 sem
alteração. `clusterProfileId` e `context` precisam coincidir com o aggregate
existente e são imutáveis; mover um scope exige criar outro. Atualizar um scope
ativo cria nova geração depois do commit, cancela a anterior, invalida caches e
rotaciona o nonce, mesmo quando somente o nome mudou. Atualizar para `all`
revalida `list namespaces` antes do commit.

Exclusão usa:

```json
{
  "confirmed": true,
  "version": 3,
  "replacementScopeId": 8
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
| `GET /api/v1/permissions` | MVP | `namespace`, `refresh`, lista limitada de capabilities | matriz/capabilities | SAR/SSRR | `AUTHORIZATION_UNAVAILABLE` |

`refresh=true` ignora cache para as chaves solicitadas. A rota limita quantidade de reviews por request e retorna decisões parciais. Um review incompleto nunca se transforma em `denied`.

## 12. Preferências

| Método/rota | Classe | Request | Response | Autorização | Erros |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/preferences` | MVP | vazio | snapshot allowlisted | Host/origin local; sem RBAC | — |
| `PUT /api/v1/preferences` | MVP | objeto versionado | snapshot atualizado | CSRF | `UNKNOWN_FIELD`, `VALIDATION_FAILED` |

Exemplo:

```json
{
  "version": 1,
  "logs": {
    "wrap": false,
    "timestamps": true,
    "tailLines": 200
  },
  "dashboard": {
    "logScanWindow": "15m",
    "sectionOrder": ["summary", "problems", "restarts", "events", "logScan", "metrics"]
  }
}
```

O endpoint não é key/value arbitrário; o schema está em [data-model.md](data-model.md).

## 13. Dashboard

| Método/rota | Classe | Response | Autorização | Erros/completude |
| --- | --- | --- | --- | --- |
| `GET /api/v1/dashboard/summary` | MVP | contagens + possível log count | por bloco/recurso | parcial |
| `GET /api/v1/dashboard/problems` | MVP | `ProblemPodDTO[]` | `list/get pods`, events se usados | parcial/cursor limitado |
| `GET /api/v1/dashboard/restarts` | MVP | `RestartDTO[]` | `list pods` | limite default 10 |
| `GET /api/v1/dashboard/events` | MVP | `EventDTO[]` | `list events` | parcial |
| `POST /api/v1/dashboard/log-scan` | MVP | `LogMatchDTO[]` | CSRF + `get pods/log` por namespace | budgets/429 |
| `GET /api/v1/metrics` | MVP opcional em runtime | summary/top CPU/memória | discovery + `list pods.metrics.k8s.io` | `FEATURE_UNAVAILABLE` isolado |

Todos retornam `complete`, `truncated`, coverage e erros parciais. Falha de Metrics API nunca muda o status dos demais endpoints.

`LogScanRequest`:

```json
{
  "window": "15m",
  "tailLines": 200,
  "maxPods": 20,
  "maxConcurrentContainers": 4
}
```

O backend reduz valores acima do contrato somente se a resposta informar os limites efetivos; preferencialmente rejeita valor inválido. Conteúdo nunca é persistido.

Contadores distinguem zero de ausência:

```json
{
  "data": {
    "namespaces": {"state": "available", "value": 3},
    "podsTotal": {"state": "available", "value": 18},
    "podsHealthy": {"state": "available", "value": 15},
    "podsProblematic": {"state": "available", "value": 3},
    "workloadsDegraded": {"state": "available", "value": 1},
    "restarts": {"state": "available", "value": 12},
    "warningEvents": {"state": "denied", "value": null},
    "possibleLogMatches": {"state": "notCollected", "value": null}
  }
}
```

Estados de contador: `available`, `denied`, `unavailable`, `notCollected`, `collecting`, `truncated`.

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

DTO de evento separa `objectKind` e `objectName` e preserva `count`.

`ProblemPodDTO` inclui namespace, pod, owner opcional, container opcional, status, `reason`, `message`, origem do diagnóstico (`status`, `condition` ou `event`), severidade e idade. Campo ausente permanece null; o backend não fabrica owner ou causa.

## 14. Workloads

Kinds aceitos: `deployments`, `statefulsets`, `daemonsets`, `jobs`, `cronjobs`.

| Método/rota | Classe | Request/response | Autorização | Erros |
| --- | --- | --- | --- | --- |
| `GET /api/v1/workloads` | MVP | query comum + `kind`; `WorkloadDTO[]` | `list` de cada kind | parcial/cursor |
| `GET /api/v1/workloads/{kind}/{namespace}/{name}` | MVP | `WorkloadDetailDTO` | `get` kind/alvo | 403/404 |
| `GET /api/v1/workloads/{kind}/{namespace}/{name}/yaml` | MVP | YAML | `get` kind/alvo | 403/404/413 |
| `POST /api/v1/workloads/{kind}/{namespace}/{name}/restart` | MVP | confirmação | `patch apps/deployments` com resourceName | kind não suportado |
| `PUT /api/v1/workloads/{kind}/{namespace}/{name}/scale` | MVP | réplica/confirmação/precondition | `update` ou verbo aprovado em `*/scale` | conflito |

Restart aceita apenas Deployment no MVP e exige `Idempotency-Key`.

```json
{
  "confirmed": true,
  "target": {
    "context": "development",
    "namespace": "payments",
    "kind": "Deployment",
    "name": "api"
  },
  "expectedResourceVersion": "12345"
}
```

Scale:

```json
{
  "replicas": 3,
  "confirmed": true,
  "expectedResourceVersion": "12345"
}
```

Resposta informa `accepted`, não “rollout concluído”.

`WorkloadDTO`:

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

O detalhe acrescenta labels/annotations allowlisted, selector, condições, containers e referências relacionadas necessárias à tela. O YAML continua separado e sob demanda.

## 15. Pods e logs

| Método/rota | Classe | Request/response | Autorização | Erros |
| --- | --- | --- | --- | --- |
| `GET /api/v1/pods` | MVP | query comum; `PodDTO[]` | `list pods` | parcial/cursor |
| `GET /api/v1/pods/{namespace}/{name}` | MVP | `PodDetailDTO` | `get pods` | 403/404 |
| `GET /api/v1/pods/{namespace}/{name}/yaml` | MVP | YAML | `get pods` | 403/404/413 |
| `DELETE /api/v1/pods/{namespace}/{name}` | MVP | confirmação/precondition | `delete pods` resourceName | 403/404/409 |
| `GET /api/v1/pods/{namespace}/{name}/logs` | MVP | `container`, `previous`, `timestamps`, `tailLines`, `since` | `get pods/log` | limits/404 |
| `GET /api/v1/pods/{namespace}/{name}/logs/stream` | MVP | SSE via `fetch` | CSRF/Origin + `get pods/log` | stream events |
| `POST /api/v1/pods/{namespace}/{name}/exec` | MVP | cria ticket one-shot `ExecInit` | CSRF + `create pods/exec` | Origin/limits |
| `POST /api/v1/pods/{namespace}/{name}/port-forward` | MVP | `PortForwardCreate` | `create pods/portforward` | limits/port conflict |

Delete:

```json
{
  "confirmed": true,
  "expectedUid": "...",
  "expectedResourceVersion": "12345"
}
```

Logs comuns retornam texto sanitizado ou envelope com linhas, conforme `Accept`; o frontend usa JSON por padrão:

```json
{
  "data": {
    "container": "api",
    "previous": false,
    "lines": [
      {"timestamp": "2026-07-27T12:00:00Z", "text": "redacted text"}
    ],
    "truncated": false
  }
}
```

`PodDTO`:

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

O detalhe acrescenta containers regulares/init/efêmeros, estados/últimos estados, condições e referências de eventos permitidos. Env vars, mounts ou campos capazes de expor Secret são omitidos ou reduzidos a referências sem valor.

## 16. Events, Network e Config

| Método/rota | Classe | Paginação | Autorização |
| --- | --- | --- | --- |
| `GET /api/v1/events` | MVP | query comum | `list events` |
| `GET /api/v1/services` | MVP | query comum | `list services` |
| `GET /api/v1/services/{namespace}/{name}` | MVP | não | `get services` |
| `GET /api/v1/services/{namespace}/{name}/yaml` | MVP | não | `get services` |
| `GET /api/v1/ingresses` | MVP | query comum | `list networking.k8s.io/ingresses` |
| `GET /api/v1/ingresses/{namespace}/{name}` | MVP | não | `get networking.k8s.io/ingresses` |
| `GET /api/v1/ingresses/{namespace}/{name}/yaml` | MVP | não | `get networking.k8s.io/ingresses` |
| `GET /api/v1/endpoint-slices` | MVP | query comum | `list discovery.k8s.io/endpointslices` |
| `GET /api/v1/endpoint-slices/{namespace}/{name}` | MVP | não | `get discovery.k8s.io/endpointslices` |
| `GET /api/v1/endpoint-slices/{namespace}/{name}/yaml` | MVP | não | `get discovery.k8s.io/endpointslices` |
| `GET /api/v1/configmaps` | MVP | query comum metadata-first | `list configmaps` |
| `GET /api/v1/configmaps/{namespace}/{name}` | MVP | não | `get configmaps` |
| `GET /api/v1/configmaps/{namespace}/{name}/yaml` | MVP | não | `get configmaps` |
| `GET /api/v1/secrets` | MVP | query comum | `list secrets`; PartialObjectMetadata |
| `GET /api/v1/secrets/{namespace}/{name}` | MVP | não | `get secrets`; PartialObjectMetadata |

Erros comuns: 403, 404, 410, 503, 504. Listas multi-namespace podem ser parciais.

ConfigMap list usa PartialObjectMetadata quando possível para não carregar conteúdo; conteúdo só entra no detalhe autorizado. Secrets exigem `PartialObjectMetadata`; se o servidor não suportar resposta metadata-only, retornar `FEATURE_UNAVAILABLE` em vez de buscar o objeto completo. Secret não possui rota YAML.

Campos mínimos:

| DTO | Campos |
| --- | --- |
| `EventDTO` | timestamp, namespace, objectKind, objectName, reason, message, count, source, type |
| `ServiceDTO` | namespace, name, type, clusterIPs, ports, selector allowlisted, external endpoints |
| `IngressDTO` | namespace, name, className, hosts, paths, backend refs, TLS presente sem conteúdo |
| `EndpointSliceDTO` | namespace, name, addressType, ports, endpoints/conditions dentro do limite |
| `ConfigMapListDTO` | namespace, name, creationTimestamp |
| `ConfigMapDetailDTO` | metadata allowlisted, keys e valores somente após `get`; tamanho/truncamento |
| `SecretMetadataDTO` | somente PartialObjectMetadata allowlisted |

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

## 17. Port-forwards

| Método/rota | Classe | Request | Response | Autorização | Erros |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/port-forwards` | MVP | vazio | sessões da geração atual, sem tráfego | Host/origin local; sem RBAC adicional | — |
| `DELETE /api/v1/port-forwards/{id}` | MVP | `PortForwardDeleteRequest` | 204 | CSRF; owner/generation | 404, `SESSION_GONE`, `GENERATION_CHANGED` |

Criação pelo endpoint do Pod:

```json
{
  "remotePort": 8080,
  "localPort": null,
  "confirmed": true
}
```

Exige `Idempotency-Key`. O backend escolhe porta quando null e retorna somente após listener local adquirido:

```json
{
  "data": {
    "id": "pf_...",
    "context": "development",
    "namespace": "payments",
    "pod": "api-abc",
    "remotePort": 8080,
    "localAddress": "127.0.0.1",
    "localPort": 49152,
    "status": "active"
  }
}
```

`PortForwardDeleteRequest`:

```json
{
  "confirmed": true,
  "expectedGeneration": "gen_42"
}
```

Encerrar sessão é idempotente dentro da mesma tentativa do frontend: uma
sessão já encerrada retorna `SESSION_GONE`, que a UI reconcilia como estado
final sem repetir contra outro ID.

## 18. SSE e watch

### 18.1 Logs follow

`GET /api/v1/pods/{namespace}/{name}/logs/stream` usa `text/event-stream`, heartbeat e eventos:

```text
event: meta
data: {"requestId":"...","generation":"gen_42"}

event: line
data: {"timestamp":"...","text":"sanitized"}

event: error
data: {"code":"UPSTREAM_TIMEOUT","message":"..."}

event: end
data: {"reason":"context_changed"}
```

O servidor não envia linha acima do limite; marca truncamento ou encerra conforme contrato de F2-55.

O frontend consome SSE por `fetch` com parser incremental, não por `EventSource`, para enviar `X-KubePeep-CSRF` sem colocar token na URL. Reconexão é explícita e respeita generation ID.

### 18.2 Atualizações de recursos

Rota interna do MVP:

```text
GET /api/v1/stream?topic=pods&topic=events
```

| Request | Response | Guards | Erros/eventos finais |
| --- | --- | --- | --- |
| tópicos allowlisted repetíveis, `Last-Event-ID` opcional | `text/event-stream` | Host e Origin exatos, CSRF via `fetch`, generation e capability por tópico | `CSRF_REJECTED`, `GENERATION_CHANGED`, `AUTHORIZATION_UNAVAILABLE`, evento `reset` ou `error` |

Ela multiplexa somente tópicos allowlisted já autorizados. Eventos:

```text
snapshot, added, modified, deleted, reset, error, heartbeat
```

`reset` exige refetch HTTP. Retomada por `Last-Event-ID` só existe se o manager ainda possuir o evento dentro de buffer curto; não há persistência.

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
  "expectedGeneration": "gen_42"
}
```

Após JSON estrito, limites, confirmação, alvo, geração e SAR, o servidor
canonicaliza método, path, namespace, pod e `ExecInit` e liga seu hash ao
ticket. Alterar qualquer parâmetro exige outro POST e outra confirmação.

Resposta de criação do ticket:

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

O upgrade não aceita `ExecInit` nem substituição de command/container/TTY. O
primeiro frame de aplicação já pertence ao protocolo tipado e só pode ser
`stdin`, `resize`, `heartbeat` ou `close`, conforme as capacidades fixadas no
POST.

Regras:

- `command` é argv, nunca string concatenada;
- tamanho/quantidade de args limitados;
- revalidar `create pods/exec` no POST e imediatamente antes do upgrade GET;
- frames de controle cobrem resize, heartbeat e close;
- stdout/stderr nunca são logados;
- desconexão não é retomável;
- troca de geração encerra com razão segura.

O transporte browser-backend usa `github.com/coder/websocket v1.8.15`, que
implementa masking e fragmentação; o Kube Peep impõe os tipos, limites,
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
- [x] 403 representa somente negação real.
- [x] Secret não possui rota YAML nem fallback para objeto completo.
- [x] SSE e `exec` têm protocolo, biblioteca e cadeia HTTP decididos.
- [x] Os helpers Ginger foram avaliados; extensões próprias estão delimitadas.
- [ ] Exemplos passam por validação de schema gerado quando o harness existir.
