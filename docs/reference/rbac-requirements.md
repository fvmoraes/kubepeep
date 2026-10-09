# Requisitos RBAC do KubePeep

> **Fontes de contrato:** `internal/services/authorization/allowlist.go` e `internal/services/resourcecatalog/` para operações nativas; descoberta validada para as leituras dinâmicas. A decisão final é sempre do Kubernetes.

## 1. Modelo

- O KubePeep **nunca** usa credenciais próprias: toda chamada usa a identidade do kubeconfig selecionado (com `SelfSubjectRulesReview`/`SelfSubjectAccessReview` para descobrir capacidades).
- Toda capacidade é revalidada no servidor em cada operação mutável (guard), nunca apenas cacheada no frontend.
- Mutações nativas usam capabilities do catálogo fechado. Visões dinâmicas
  aceitam somente GVRs descobertos com list/get e não oferecem mutação genérica.

## 2. Perfis de uso

### 2.1 Somente leitura (dashboard/visualização)

Requisitos mínimos por namespace:

```text
namespaces.list                                    (cluster)
pods.list, pods.get, pods.watch                    (namespace)
pods.logs.get                                      (namespace, resourceName)
events.list, events.watch                          (namespace)
deployments.list/get/watch                         (namespace)
statefulsets.list/get/watch                        (namespace)
daemonsets.list/get/watch                          (namespace)
jobs.list/get/watch                                (namespace)
cronjobs.list/get/watch                            (namespace)
services/ingresses/endpointslices/configmaps/secrets
  (list/get/watch — ver allowlist para grupos/verbos exatos)
```

Com esse perfil o usuário vê dashboard, listas, detalhes (Secret mantém metadados por padrão; Reveal data faz get explícito), logs e métricas (quando a Metrics API permitir).

Follow e download completo/anterior de logs exigem o mesmo `get` de `pods/log`
para namespace e Pod exatos. O download usa SSE finito com CSRF/Origin, geração,
escopo e reautorização periódica iguais aos do follow; não exige escrita no
cluster. Exportar a sessão ou linhas visíveis apenas salva o conteúdo já
autorizado no painel. Revogação ou troca de geração encerra streams e descarta
a captura local.

### 2.2 Ações mutáveis (opcionais, por namespace/recurso)

| Capacidade | Verbo Kubernetes | Uso na UI |
| --- | --- | --- |
| `yaml.{collection}.update` | `update` no grupo/plural real do catálogo | editar YAML de objeto do catálogo nativo, incluindo Gateway API, com nome e escopo exatos; exclui visões dinâmicas |
| `deployments.update` | `update` deployments | compatibilidade com a rota anterior de edição |
| `deployments.restart` | `patch` deployments | botão Restart |
| `deployments.scale` | `update` deployments/scale | campo Scale |
| `statefulsets.scale` | `update` statefulsets/scale | campo Scale |
| `pods.delete` | `delete` pods | Delete pod |
| `pods.exec.create` | `create` pods/exec | Terminal exec |
| `pods.portforward.create` | `create` pods/portforward | Port-forward |

Sem a capacidade, o controle correspondente aparece desabilitado com o motivo — nunca escondido silenciosamente.

## 3. Regras de exibição

1. **Negado ≠ zero.** Blocos parcialmente negados exibem estado `denied` distinto, sem inferir valor.
2. `resourceName` só é usado quando a política da capacidade é `ResourceNameTarget`.
3. Escopo da consulta é sempre a interseção escopo ativo ∩ filtros; a query jamais expande o escopo.
4. Toda mutação exige: geração atual válida, `ExpectedResourceVersion`/`ExpectedUID` quando aplicável, confirmação explícita; idempotency key nas ações cujo contrato a exige. O PUT de YAML usa concorrência otimista por UID/resourceVersion. A leitura exige `get` separado e explícito; ver [contrato do editor](api.md#164-editor-yaml-para-todos-os-objetos-disponíveis). As capabilities `yaml.*.update` não ampliam o RBAC no cluster: o editor fica somente leitura quando o usuário não pode atualizar o alvo.

## 4. Auditoria

Cada ação registra evento sanitizado (`internal/services/actions/audit.go`): timestamp, operação, contexto, namespace, recurso, duração e código de erro — nunca corpo, comando, saída ou ticket.

### Diagnóstico de autorização indisponível

`503/AUTHORIZATION_UNAVAILABLE` indica que a revisão de acesso não conseguiu
concluir uma decisão. Não equivale a `403/FORBIDDEN`. Timeout, cancelamento,
falha de autenticação/transporte ou resposta incompleta do
`SelfSubjectAccessReview` podem produzir esse estado. O indicador `healthy`
confirma conectividade do cluster, não as permissões para cada recurso.

Para verificar a identidade selecionada, use o mesmo kubeconfig, contexto e
namespace da aplicação:

```sh
kubectl --kubeconfig /caminho/config --context CONTEXTO -n NAMESPACE auth can-i list pods
kubectl --kubeconfig /caminho/config --context CONTEXTO -n NAMESPACE auth can-i watch pods
```

A rota `GET /api/v1/permissions?namespace=NAMESPACE&capability=pods.list&refresh=true`
consulta novamente essa capacidade e retorna um `reasonCode` sanitizado
(`SAR_TIMEOUT`, `SAR_INCOMPLETE`, `SAR_AUTHENTICATION_UNAVAILABLE`,
`SAR_UNAVAILABLE` ou `REQUEST_CANCELED`). A consulta deve ocorrer durante a
falha: uma resposta posterior `SAR_ALLOWED` confirma recuperação, mas não
identifica a causa original. Consulte o [contrato da API](api.md) para os
formatos e limites.

`SAR_INCOMPLETE` também inclui a resposta sem opinião (`allowed=false`,
`denied=false`), mesmo sem erro de avaliação. Portanto, esse código não
prova uma falha transitória: a identidade pode estar sem permissão naquele
namespace. O KubePeep preserva `unknown` nesse caso, conforme seu contrato
de segurança. Para confirmar uma negação efetiva, uma consulta de leitura
direta ao mesmo recurso pode retornar `403/Forbidden`. A
[documentação do Kubernetes](https://kubernetes.io/docs/reference/access-authn-authz/authorization/)
explica que a requisição é negada quando nenhum autorizador concede acesso.
Se isso ocorrer apenas em parte do escopo, revisar as permissões desses
namespaces ou a seleção do escopo; retentativas não concedem acesso.

Na visão de todos os namespaces, uma origem sem acesso não deve invalidar
as páginas dos namespaces autorizados. A coleta encerra sem novo cursor
quando só restam origens com falha e informa cobertura parcial. Isso evita
uma página adicional que retornaria 403/503 e ocultaria a tabela inteira.
As falhas permanecem visíveis no rodapé; cada nova coleta reavalia o escopo.

O cache de revisão dura 45 s por padrão (configurável entre 30 e 60 s), exceto
para requisições canceladas. Uma revisão `unknown` pode ser resolvida por uma
leitura real limitada do mesmo recurso e escopo; isso não autoriza mutações.
Uma decisão negada explicitamente não recebe esse fallback. Falhas transitórias
têm tentativas limitadas com backoff, e listas/dashboard voltam a consultar a
cada 10 s enquanto Auto estiver ligado. Autenticação expirada pode recuperar
a identidade e tentar novamente; negação 403 e mudança de geração interrompem
a consulta antiga. Dados sem leitura autorizada confirmada permanecem ocultos.
Mensagens distinguem autenticação/plugin, DNS, TLS, conexão, timeout e
throttling sem expor erro bruto ou credenciais.

### Helm, Gateway API e recursos dinâmicos

- Gateway API exige list/get por plural do grupo `gateway.networking.k8s.io`;
  GatewayClass é cluster-scoped. O editor exige update no objeto exato.
- Helm lê metadata de Secrets ou ConfigMaps com `owner=helm` para o inventário;
  detalhe, values e manifest exigem get do registro correspondente. Upgrade e
  rollback precisam das permissões sobre o armazenamento e sobre os recursos e
  hooks realmente executados pelo SDK Helm. Permissão de leitura não concede
  essas operações, e não há retry automático de escrita.
- A descoberta de APIs não concede list/get dos objetos. Visões dinâmicas
  revalidam GVR, namespace/escopo e nome e reutilizam a recuperação de leitura.
  O YAML é somente leitura; não são aceitos URLs arbitrários ou subresources.
- Seleção em massa é uma interação local. Cada alvo de ação exige permissão
  própria; somente Pods oferecem delete em massa. Reinício de Pods usa delete
  e exige controlador que possa recriá-los.

### Timeouts e cancelamentos no dashboard

`UPSTREAM_TIMEOUT` informa que a coleta atingiu seu prazo; `CLIENT_CANCELED`
indica cancelamento da requisição ou da seleção. Esses estados não equivalem
a falta de permissão. Um escopo extenso exige listas paginadas e revisões de
acesso por namespace/recurso; o limitador do cliente pode consumir parte do
prazo mesmo com o cluster acessível. A primeira coleta, sem capacidades em
cache, pode ser parcial. O prazo continua limitado por `dashboard.blockTimeout`.

O dashboard compartilha leituras simultâneas da mesma página entre blocos,
isoladas por perfil, contexto, geração, recurso, namespace e cursor. Não guarda
páginas concluídas nesse mecanismo. Cancelar um bloco não cancela outro que
ainda aguarda a mesma leitura; sem consumidores, o trabalho é cancelado.
Pods e eventos são coletados em paralelo dentro do prazo do bloco. Contadores
não consultam o controlador de cada Pod; o ranking de reinícios resolve os
controladores apenas dos Pods presentes no resultado limitado.

As falhas permanecem disponíveis em **Collection issues**, com contagens e
detalhes recolhíveis. Mensagens idênticas são agrupadas na apresentação; a API
continua retornando os erros e a cobertura. `FORBIDDEN` persistente exige
revisar RBAC ou o escopo, e não é resolvido reiniciando a aplicação.

## 5. Recursos cluster-scoped (ADR 0006)

Capacidades cluster-scoped são próprias e nunca derivam de um scope
namespaced. `nodes.list`/`nodes.get` são os primeiros exemplos: exigem apenas
contexto válido e o verbo Kubernetes correspondente; `get` forma produto com
`resourceName` (policy `target`). Um operador sem `list nodes` continua com o
restante do produto útil, e negação de Nodes não bloqueia o fluxo namespaced.

## 6. Checklist de uma família nova

Toda família nova (namespaced ou cluster-scoped) segue o mesmo caminho —
usar Nodes (`cb86d6b`) como fonte do guia, não abstrações de planos antigos:

1. **Catálogo** — coleção/GVR em `internal/services/resources` (namespaced:
   `service.go`; cluster-scoped: `cluster.go`) + regras de sort/status em
   `options.go` + capabilities `list`/`get` em
   `internal/services/authorization/allowlist.go` (ID ≠ plural Kubernetes).
2. **DTO/porta** — DTOs de lista/detalhe fechados com limites explícitos e
   conversores testados; política de YAML definida antes de expor rota
   (`MarshalReadOnlyYAML` ou documento curado via `MarshalYAMLDocument`).
3. **Runtime** — lister/getter typed em
   `internal/integration/kubernetesruntime` (`resources_backend.go` +
   `resources_adapter.go`), reutilizando `Collect`/`clusterCollect` com o
   budget de janela configurável.
4. **Handler/wiring** — métodos em `internal/api/handlers/resources.go`
   (`handleList`/`handleClusterList`, `detail`/`clusterDetail`,
   `yaml`/`clusterYAML`), rotas em `routes.go` e `allowedMethods` completos
   (GET/HEAD; POST/PUT/DELETE separados).
5. **Cliente** — tipos em `web/src/api/types.ts` e funções em `client.ts`
   com geração esperada e AbortSignal.
6. **Página/rota/nav** — página com o resource framework (`ResourcePage`,
   `ResourceListControls`, `DataTable`, `InfiniteCollectionFooter`, `ResourceWorkspacePanel`,
   `YamlViewer`), rota em `App.tsx`, item com `path` em
   `navigation/tree.tsx` e ajuste do teste de catálogo da paleta.
7. **Testes** — unitários de DTO/contrato, integração handler/runtime com
   cliente fake (403/unknown/vazio distintos) e E2E mockado da jornada
   lista → detalhe → YAML autorizado.
