# Fase 5 — Dashboard

**Estado inicial:** pendente

**Dependências:** conexão, escopos, DTOs e autorização da Fase 4
**Gate seguinte:** a Fase 6 estende os mesmos ports de consulta, sem criar adapters paralelos.

## Objetivo

Entregar um overview rápido e orientado a problemas. Cada bloco deve carregar independentemente, respeitar RBAC e continuar útil quando cluster, eventos, logs ou Metrics API estiverem parcial ou totalmente indisponíveis.

## Entregáveis

- endpoints independentes de summary, problems, restarts, events, log scan e métricas;
- classificadores puros e testáveis para pods, workloads, eventos e possíveis erros em logs;
- scan de logs limitado, sanitizado, cancelável e não persistente;
- integração opcional com `metrics.k8s.io`;
- interface progressiva com cards, tabelas, filtros e erros parciais;
- testes unitários, integração, frontend e cenário E2E.

## Tarefas ordenadas

### Orquestração do dashboard

- [ ] **F5-01** Implementar a fundação mínima compartilhada de `PodService`, `WorkloadService`, `EventService`, `LogService` e `MetricsService` atrás de ports; o `DashboardService` apenas orquestra essas consultas, que a Fase 6 estenderá.
- [ ] **F5-02** Definir budgets independentes de timeout, bytes, paginação e concorrência por bloco e retornar `complete/truncated`, cobertura e erros parciais.
- [ ] **F5-03** Implementar `GET /api/v1/dashboard/summary`.
- [ ] **F5-04** Implementar `GET /api/v1/dashboard/problems`.
- [ ] **F5-05** Implementar `GET /api/v1/dashboard/restarts`.
- [ ] **F5-06** Implementar `GET /api/v1/dashboard/events`.
- [ ] **F5-07** Implementar `POST /api/v1/dashboard/log-scan`.
- [ ] **F5-08** Implementar métricas no contrato definido para `GET /api/v1/metrics`.
- [ ] **F5-09** Retornar falhas por bloco sem transformar uma negação ou API opcional ausente em falha global.

### Resumo

- [ ] **F5-10** Contar namespaces do escopo, pods totais/saudáveis/problemáticos, workloads degradados, restarts, eventos `Warning` e possíveis correspondências de logs quando um scan tiver sido executado.
- [ ] **F5-11** Diferenciar zero real, acesso negado, dado indisponível, resultado truncado e coleta ainda em andamento.
- [ ] **F5-12** Evitar carregar YAML ou objetos completos para produzir contagens.
- [ ] **F5-13** Incluir contexto, cluster, escopo e instante da coleta em DTO próprio.

### Pods com restarts

- [ ] **F5-14** Calcular restarts de containers normais, init containers e containers efêmeros separadamente.
- [ ] **F5-15** Resolver workload proprietário sem inventar relação quando o owner não estiver disponível.
- [ ] **F5-16** Ordenar de forma determinística por total decrescente e retornar top 10 por padrão, ou todos quando existirem menos de dez.
- [ ] **F5-17** Implementar níveis `0`, `1–2`, `3–9` e `10+` como configuração de apresentação.
- [ ] **F5-18** Expor namespace, pod, owner, container, total, status, último motivo, idade e ações para abrir detalhe e logs.

### Pods problemáticos

- [ ] **F5-19** Detectar CrashLoopBackOff, ImagePullBackOff, ErrImagePull, CreateContainerConfigError e RunContainerError.
- [ ] **F5-20** Detectar OOMKilled, Evicted, Failed, containers não prontos e Pending prolongado.
- [ ] **F5-21** Correlacionar falhas de probe e scheduling apenas quando eventos/condições reais fornecerem o motivo.
- [ ] **F5-22** Preservar reason/message reais do Kubernetes e diferenciar ausência de diagnóstico.
- [ ] **F5-23** Testar prioridade quando um pod possui múltiplos problemas.

### Workloads degradados

- [ ] **F5-24** Classificar Deployment com réplicas indisponíveis.
- [ ] **F5-25** Classificar StatefulSet incompleto e DaemonSet indisponível.
- [ ] **F5-26** Classificar Job falhando e CronJob com falhas recentes segundo regra documentada.
- [ ] **F5-27** Produzir DTO uniforme com namespace, kind, nome, ready, desired, available, updated, status e idade.
- [ ] **F5-28** Não marcar recurso como saudável quando campos necessários foram negados ou não coletados.

### Eventos Warning

- [ ] **F5-29** Carregar eventos somente nos namespaces e escopos permitidos.
- [ ] **F5-30** Priorizar `type=Warning`, preservar contador e timestamps originais.
- [ ] **F5-31** Agrupar repetições somente com chave documentada, sem perder `count`.
- [ ] **F5-32** Expor timestamp, namespace, tipo do objeto, nome do objeto, reason, message, count e source.

### Scan limitado de logs

- [ ] **F5-33** Selecionar primeiro pods problemáticos, pods com restarts e containers terminados recentemente.
- [ ] **F5-34** Verificar `get pods/log` para cada namespace/subrecurso necessário.
- [ ] **F5-35** Aplicar defaults: 15 minutos, 200 linhas, até 20 pods, concorrência 4 e timeout de 8 segundos, além dos limites de bytes por linha/container/scan definidos na Fase 2.
- [ ] **F5-36** Permitir apenas janelas configuradas de 15 min, 30 min, 1 h e 4 h, com limites máximos backend.
- [ ] **F5-37** Buscar padrões case-insensitive e reconhecer logs JSON nos campos definidos na especificação.
- [ ] **F5-38** Classificar o motivo do match sem afirmar que o trecho é um erro confirmado.
- [ ] **F5-39** Mascarar bearer tokens, JWTs longos, senhas, chaves, authorization headers e connection strings.
- [ ] **F5-40** Nunca persistir linhas, resultados ou downloads do scan.
- [ ] **F5-41** Interromper imediatamente em troca de contexto/escopo, saída da tela ou novo scan.
- [ ] **F5-42** Implementar o botão explícito `Scan logs now`; qualquer scan inicial usa o budget mínimo.
- [ ] **F5-43** Expor timestamp detectável, namespace, pod, container, workload, trecho sanitizado, motivo, link para logs, copiar e aplicar filtro.

### Metrics API opcional

- [ ] **F5-44** Descobrir se `metrics.k8s.io` está disponível.
- [ ] **F5-45** Verificar permissão antes de consultar pod metrics.
- [ ] **F5-46** Expor CPU, memória e top pods somente quando dados estiverem disponíveis.
- [ ] **F5-47** Tratar API ausente, proibida ou temporariamente offline como estado opcional, não erro global.
- [ ] **F5-48** Ocultar cards e mostrar indicação discreta quando não houver métricas.

### Interface

- [ ] **F5-49** Construir cabeçalho com contexto, cluster, escopo, namespaces, conexão, última atualização e atalhos.
- [ ] **F5-50** Criar cards compactos clicáveis que preservam o filtro ao navegar.
- [ ] **F5-51** Criar tabelas de restarts, problemas, workloads, eventos `Warning` e possíveis logs com densidade e acessibilidade.
- [ ] **F5-52** Carregar blocos com queries/cancelamentos independentes.
- [ ] **F5-53** Mostrar erro específico no bloco afetado e manter os demais utilizáveis.
- [ ] **F5-54** Atualizar dados manualmente sem polling agressivo nem duplicação de requests.
- [ ] **F5-55** Persistir somente preferências allowlisted do dashboard, como janela de scan, sem persistir resultados coletados.
- [ ] **F5-56** Estender o harness restrito com pod em restart, workload degradado, evento `Warning`, logs sintéticos e Metrics API ausente.
- [ ] **F5-57** Testar parsing e ordenação de `resource.Quantity` para CPU/memória, incluindo unidades distintas.
- [ ] **F5-58** Distinguir visualmente “scan ainda não executado”, zero correspondências, acesso negado e falha parcial.
- [ ] **F5-59** Executar no harness o dashboard permitido, parcialmente negado, sem métricas e com cluster temporariamente offline.
- [ ] **F5-60** Executar e registrar `ginger inspect`, `ginger doctor`, testes, lint e build no fechamento da fase.

### Limites finais

- [ ] **F5-61** Limitar também o total de containers considerados e usar reader limitado antes de classificar/redigir cada trecho.
- [ ] **F5-62** Nunca apresentar total, zero ou top 10 como completo quando um cursor, namespace, permissão ou budget ficou pendente.

## Testes obrigatórios

- cálculo de restarts para os três tipos de containers;
- ordenação determinística e níveis visuais;
- cada motivo de pod problemático e caso sem diagnóstico;
- cada kind de workload degradado;
- agrupamento e contador de eventos;
- scan respeitando pods, linhas, janela, concorrência e timeout;
- scan respeitando bytes por linha/container/scan e total de containers;
- cancelamento antes e durante leitura de logs;
- detecção de texto e JSON;
- redaction de cada classe sensível;
- ausência de persistência de logs no SQLite;
- acesso negado a logs;
- Metrics API ausente, proibida e disponível;
- cluster offline e erro parcial;
- agregação truncada com cobertura/limitação visível;
- troca de contexto descartando respostas antigas;
- interface loading, vazia, parcial e filtrada;
- E2E com pod em restart, workload degradado, Warning e log sintético.

## Riscos específicos

| Risco | Mitigação |
| --- | --- |
| Dashboard gerar carga excessiva | Endpoints separados, budgets e concorrência limitada |
| Falso positivo em logs | Linguagem “possível erro” e motivo explícito |
| Trecho de log expor segredo | Redaction antes do DTO e testes adversariais |
| Resultado antigo aparecer após troca de contexto | Cancelamento + generation ID da seleção |
| Falha de Metrics API derrubar o overview | Adapter opcional e erro isolado |

## Fora de escopo

- Scan ilimitado ou contínuo de todo o cluster.
- Persistência de snapshots ou logs.
- Gráficos decorativos.
- Ações mutáveis em workloads/pods.

## Critério de saída

O overview responde às perguntas centrais do prompt com blocos independentes; exibe problemas, restarts, workloads e eventos `Warning`; o scan obedece todos os limites e sanitizações; a ausência de logs, eventos ou métricas não impede o uso das demais áreas.
