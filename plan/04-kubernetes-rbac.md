# Fase 4 — Kubernetes e RBAC

**Estado inicial:** pendente

**Dependências:** fundação executável da Fase 3 e contratos da Fase 2
**Gate seguinte:** dashboard, recursos e ações só usam os ports seguros desta fase.

## Objetivo

Implementar a conexão Kubernetes, seleção de contexto, escopos de namespaces e autorização RBAC. O backend deve mostrar e executar apenas capacidades reais, sem armazenar credenciais nem permitir que handlers acessem o clientset diretamente.

## Entregáveis

- adapters de kubeconfig e Kubernetes isolados por ports;
- cache concorrente de clients cuja chave lógica é o conjunto ordenado de
  paths + contexto; fingerprints ficam somente em memória como versão de
  invalidação, nunca como identidade persistida;
- seleção de contextos e cluster profile;
- CRUD e validação de escopos `single`, `list` e `all`;
- autorização por verbo/recurso/subresource com cache curto;
- matriz de permissões na API e interface;
- DTOs e tradução padronizada de erros Kubernetes;
- testes unitários, integração e frontend da matriz RBAC.

## Tarefas ordenadas

### Kubeconfig e clients

- [ ] **F4-01** Implementar `KubeconfigLoader` com precedência `--kubeconfig` > `KUBECONFIG` > profile persistido default > path recomendado e contexto `--context` > persistido > `current-context` no primeiro reconcile, sem fallback silencioso de fonte escolhida inválida.
- [ ] **F4-02** Suportar múltiplos arquivos, contextos, certificados, tokens referenciados e plugins `exec` por meio das bibliotecas oficiais do Kubernetes.
- [ ] **F4-03** Persistir somente a referência ao conjunto ordenado de paths selecionados e o nome do contexto; nunca serializar kubeconfig ou `rest.Config`.
- [ ] **F4-04** Sanitizar erros de parsing e de plugins de autenticação antes de resposta ou log.
- [ ] **F4-05** Criar factory sem globais mutáveis, separando clientes unary com timeout finito de watch/log follow/exec/port-forward controlados por contexto e idle deadline.
- [ ] **F4-06** Criar cache concorrente pela chave lógica `conjunto ordenado de paths + contexto`, mantendo fingerprints transitórios apenas no estado de invalidação.
- [ ] **F4-07** Invalidar cache em mudança de qualquer path/contexto, symlink/replace/modificação de arquivo ou certificado/token referenciado e erro de autenticação classificado.
- [ ] **F4-08** Cancelar requests, watches e streams vinculados ao contexto anterior.
- [ ] **F4-09** Implementar teste de conectividade sem confundir cluster offline com falha da aplicação local.

### Contextos e cluster profiles

- [ ] **F4-10** Implementar `ContextService` e listagem sanitizada de profiles/contextos com DTO mínimo e sem dados de autenticação ou paths normalizados completos.
- [ ] **F4-11** Implementar seleção de contexto coordenada: validar, atualizar `context_name` em transação, trocar default somente com `setDefault=true`, então criar generation, cancelar a anterior e rotacionar CSRF; cluster/auth offline após commit mantém seleção degradada, enquanto parse/path/contexto inválido preserva banco/generation/nonce.
- [ ] **F4-12** Implementar `GET /api/v1/cluster/profiles`, `GET /api/v1/cluster/profile`, `GET /api/v1/contexts` e `POST /api/v1/contexts/select`.
- [ ] **F4-13** Criar seletor de contexto na interface, com loading, offline, contexto inexistente e cancelamento de requests anteriores.
- [ ] **F4-14** Atualizar status/cabeçalho sem persistir conteúdo do cluster.

### Parser e escopos de namespaces

- [ ] **F4-15** Implementar exatamente a gramática canônica de `rawInput` de `docs/api.md`: texto delimitado, array/object JSON estrito e sequência/mapping YAML simples, sem fallback de formato malformado.
- [ ] **F4-16** Remover vazios e duplicados, preservar ordem estável e contabilizar separadamente válidos, duplicados descartados, vazios descartados e inválidos.
- [ ] **F4-17** Validar nomes com as regras oficiais usadas pelo Kubernetes.
- [ ] **F4-18** Validar existência apenas quando a identidade puder fazê-lo; ausência de `list namespaces` não impede uma lista manual válida.
- [ ] **F4-19** Implementar `NamespaceService`, modelos e serviços para `single`, `list` e `all`, incluindo namespace padrão consistente.
- [ ] **F4-20** Salvar todos os itens de um escopo em uma única transação e uma única chamada da interface.
- [ ] **F4-21** Implementar `GET /api/v1/namespaces`, `GET/POST /api/v1/namespace-scopes`, `GET/PUT/DELETE /api/v1/namespace-scopes/{id}`, `POST /api/v1/namespace-scopes/validate` e `POST /api/v1/namespace-scopes/{id}/select`; PUT/DELETE exigem `expectedGeneration` e entram no mesmo coordenador monotônico das seleções, incluindo nova geração/rebootstrap quando o scope ativo mudar e `SELECTION_MISMATCH` para outra origem.
- [ ] **F4-22** Rejeitar conflito de nome/item com erro útil e preservar o banco se qualquer etapa transacional falhar.
- [ ] **F4-23** Criar interface de escopos com profile explícito, contexto, nome, modo, namespace padrão, textarea, chips, quatro contadores, validar, salvar, remoção individual e limpeza.

### Semântica de `all`

- [ ] **F4-24** Guardar `all` como modo, nunca como item `*`.
- [ ] **F4-25** Verificar `list namespaces` antes de ativar o modo.
- [ ] **F4-26** Quando permitido, usar somente namespaces devolvidos pela API Kubernetes.
- [ ] **F4-27** Quando negado, explicar a limitação e oferecer retorno a uma lista manual.
- [ ] **F4-28** Para operações globais permitidas, preferir a chamada Kubernetes de escopo global em vez de fan-out por namespace.
- [ ] **F4-29** Nunca inferir, inventar ou completar namespaces ausentes da resposta.

### Autorização

- [ ] **F4-30** Implementar `AuthorizationService` baseado em `SelfSubjectAccessReview`.
- [ ] **F4-31** Usar `SelfSubjectRulesReview` somente como otimização de resumo quando disponível.
- [ ] **F4-32** Representar a chave completa: generation ID da seleção, namespace, grupo de API, recurso, subresource, verbo e `resourceName` quando a decisão for específica de um objeto.
- [ ] **F4-33** Criar cache de permissões com TTL inicial de 45 segundos, configurável entre 30 e 60 segundos, e deduplicação de requests concorrentes.
- [ ] **F4-34** Invalidar o cache em troca de contexto/escopo e oferecer refresh manual.
- [ ] **F4-35** Representar capability como `allowed`, `denied` ou `unknown`; falhar fechado para mutações/upgrades, mas permitir que leituras usem a chamada Kubernetes real como autoridade quando a revisão estiver indisponível.
- [ ] **F4-36** Criar middleware/helper de revalidação para handlers; a interface nunca substitui a checagem backend.
- [ ] **F4-37** Mapear negação explícita ou 403 da operação real para `FORBIDDEN`; mapear falha/timeout da revisão para `unknown`/indisponível, nunca para uma negação inventada.
- [ ] **F4-38** Implementar `GET /api/v1/permissions` somente sobre a allowlist completa ID → group/resource/subresource/verb/resourceName de `docs/api.md`, com expansão máxima de 100 decisões, e a matriz textual correspondente na interface.
- [ ] **F4-39** Ocultar/desabilitar ações conforme regra documentada e mostrar `unknown` como “permissão não pôde ser verificada”, nunca como negação confirmada.

### DTOs e erros

- [ ] **F4-40** Criar DTOs próprios para status, contextos, namespaces, escopos e permissões.
- [ ] **F4-41** Traduzir `StatusError`, timeout, cancelamento, offline e autenticação para códigos estáveis.
- [ ] **F4-42** Garantir envelopes Ginger e request ID em respostas de erro.
- [ ] **F4-43** Não incluir `rest.Config`, headers, certificados ou conteúdo do kubeconfig em JSON/logs.
- [ ] **F4-44** Rejeitar campos JSON desconhecidos, trailing content e bodies acima do limite nos endpoints mutáveis.

### Diagnóstico, status e teste real

- [ ] **F4-45** Completar `/health` e `/api/v1/status` com estados separados de aplicação, SQLite, kubeconfig, contexto e cluster; somente falha local crítica altera a disponibilidade HTTP global.
- [ ] **F4-46** Estender `kubePeep doctor` com checagens somente leitura de kubeconfig, contexto, plugin `exec`, conectividade e capabilities básicas.
- [ ] **F4-47** Tratar plugin `exec` ausente, não executável, interativo/incompatível ou com saída sensível por diagnóstico sanitizado.
- [ ] **F4-48** Criar o harness mínimo canônico em Kind, com recursos permitidos em um namespace, recursos negados em outro e identidade sem `cluster-admin`; K3d é apenas alternativa local equivalente.
- [ ] **F4-49** Executar no harness os fluxos de contexto, `single`, `list`, `all`, acesso negado e refresh de permissões; testar a gramática de `/permissions` (limites 20/100/20, target/resourceName, produto ≤100, refresh, ID inválido, parcial unknown e 503 total), nonce antigo/rebootstrap, offline pós-commit sem rollback, erro pré-commit preservando banco/generation e corridas mistas entre context select, scope select, PUT e DELETE, inclusive scope inativo que vira ativo antes do commit.
- [ ] **F4-50** Testar mudança de RBAC entre capability exibida e operação real, garantindo que a API Kubernetes continue sendo a autoridade final.
- [ ] **F4-51** Conectar `--kubeconfig`, `--context` e `--namespace` ao bootstrap real, profile/escopo ativo e cache de clients; `--namespace` cria um `single` efêmero aplicado uma vez, sem SQLite. Testar precedência, `*`/valor inválido, contexto inicialmente ausente, consumo após seleção explícita e efeito observado.
- [ ] **F4-52** Fixar `k8s.io/client-go`, `k8s.io/api`, `k8s.io/apimachinery` e `k8s.io/metrics` exatamente em v0.35.7; manter Metrics no adapter opcional e validar version skew e `go list -m`.
- [ ] **F4-53** Criar testes frontend de importação por todos os delimitadores, JSON/YAML simples, vazios, duplicados, inválidos, quatro contadores, `single`, `list`, `all` e cancelamento ao trocar contexto.
- [ ] **F4-54** Provar por teste/inspeção que `rest.Config.Impersonate` permanece vazio e que não existe fluxo de credenciais próprio.
- [ ] **F4-55** Executar e registrar `ginger inspect`, `ginger doctor`, testes, lint e build no fechamento da fase.
- [ ] **F4-56** No modo `all`, tentar lista global de cada recurso somente quando autorizada; caso contrário, fazer SAR + fan-out limitado e distinguir namespaces `allowed`, `denied` e `unknown`.
- [ ] **F4-57** Testar fingerprints com múltiplos arquivos, symlink, replace atômico, certificado referenciado alterado e troca de ordem em `KUBECONFIG`.
- [ ] **F4-58** Testar separação entre requests unary e operações longas, comprovando que timeout global não encerra watch/log/exec/port-forward prematuramente.
- [ ] **F4-59** Aplicar timeout interno e erro público sanitizado a cada checker usado por `/health`.

## Matriz mínima de testes

- usuário com acesso a um namespace;
- usuário com lista manual de vários namespaces;
- usuário que pode listar todos os namespaces;
- usuário que não pode listar namespaces;
- usuário que pode listar pods mas não acessar `pods/log`;
- usuário que pode acessar logs mas não `pods/exec`;
- usuário que pode alterar `scale` mas não deletar;
- contexto inexistente;
- cluster offline;
- plugin `exec` falhando com texto sensível;
- cache válido, expirado, invalidado e refresh manual;
- capability `allowed`, `denied` e `unknown`, sem converter indisponibilidade em 403;
- arquivo kubeconfig modificado;
- conjunto `KUBECONFIG` reordenado ou com um arquivo/certificado alterado;
- duas seleções rápidas de contexto cancelando a primeira;
- rollback de transação de escopo;
- inspeção do banco e logs para ausência de credenciais;
- `/health` com cluster offline/contexto ausente sem derrubar a aplicação local.

Usar fake clientset para comportamento simples, servidor HTTP de teste para respostas controladas, SQLite temporário e um teste com API Kubernetes real/fake server onde o fake clientset não representar autorização.

## Riscos específicos

| Risco | Mitigação |
| --- | --- |
| `SelfSubjectRulesReview` omitir regras | SAR e operação real permanecem autoridade |
| Fan-out em muitos namespaces | Chamadas globais quando permitidas, paginação e limites |
| Mudança de contexto misturar respostas | Geração/epoch de contexto e cancelamento hierárquico |
| Erro de plugin expor token | Sanitização antes de log e resposta |
| Divergência parser frontend/backend | Backend canônico; frontend apenas antecipa o mesmo contrato |
| Cache conceder ação após mudança de RBAC | TTL curto e revalidação imediatamente antes da ação |

## Fora de escopo

- Coleta do dashboard.
- Listas detalhadas de workloads e pods.
- Execução de restart, scale, delete, port-forward ou exec.
- Conteúdo de Secrets.

## Critério de saída

Contextos e escopos funcionam nos modos `single`, `list` e `all`; o banco não contém credenciais nem `*`; capabilities permitidas, negadas e desconhecidas são representadas corretamente; mutações falham fechado, leituras respeitam a resposta real da API; troca de contexto cancela trabalho anterior; e os cenários RBAC principais passam.
