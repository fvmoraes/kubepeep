# Fase 5 — Investigação: problems, diagnóstico e logs agregados

**Prioridade:** P1. **Entrada:** F2 (cache/índices) e F4 (workspace). **Matriz:** I01–I07, D04 (consumo), T03, T05.

Camada que responde "o que está errado no meu cluster?" usando cache e índices locais — sem transformar o KubePeep em suíte pesada de observabilidade. Baixo consumo de CPU/memória, streams limitados e cancelamento imediato.

Contratos transversais de execução e aceite: [C01–C06](05-contratos-e-aceite.md).

## Tarefas

- [ ] **F5-01 — Índices locais.** No resource cache da F2: Pods por owner/status/label, Services por selector, Events por involvedObject, PVCs por Pod, ConfigMaps por workload. Habilitam investigation, busca e relações sem novas chamadas ao cluster; explicitar cobertura ainda não sincronizada e reutilizar diagnósticos existentes (C06).
- [ ] **F5-02 — Problems Engine.** `internal/problems/` com detectores: CrashLoopBackOff, ImagePullBackOff/ErrImagePull, Pending, OOMKilled, restarts acima de threshold, Jobs failed, replicas unavailable, PVC Pending, Node NotReady, probes falhando, Warning Events. Severidades critical/warning/info conforme a referência (§37). Derivado de estado observado — nunca inventa saúde nem infere permissão.
- [ ] **F5-03 — Interface de Problems.** Contadores por severidade + lista com recurso, namespace, causa resumida e ações [Inspect]/[Logs]; integrada ao Overview e ao Workspace.
- [ ] **F5-04 — Investigation View.** Ao abrir um recurso problemático: owner chain (Pod → ReplicaSet → Deployment), Service/EndpointSlice, ConfigMaps, PVCs, Events e Logs navegáveis pelo Resource Workspace, entrando no histórico.
- [ ] **F5-05 — Logs agregados por workload.** Ao selecionar um workload (Deployment/ReplicaSet/StatefulSet/DaemonSet/Job), descobrir os Pods relacionados e exibir um fluxo único com identificação de Pod/container/timestamp; follow, previous, busca texto/regex, seleção de containers; **limite de streams simultâneos**, cancelamento imediato ao trocar de tela, buffering/batching (50–100 ms) e nada em memória além do necessário.
- [ ] **F5-06 — Command Palette avançada.** Buscar recursos no cache local por termo (Deployment/Pod/Service/ConfigMap/Ingress), ações contextuais do catálogo e navegação; Ctrl/Cmd+K; busca local < 100 ms sem consultar o cluster; sem full-cluster scan por tecla.
- [ ] **F5-07 — Performance Diagnostics.** Tela Settings → Diagnostics → Performance: API latency, p50/p95/p99, cache hit ratio, watches ativas, requests/min, 429, tempo de sync por recurso e namespaces mais lentos — consumindo a instrumentação da F0.
- [ ] **F5-08 — Namespace/Cluster Diagnostics.** Contagens por recurso, problemas por severidade, latências de LIST e restarts por namespace; visão de cluster com versão, totais e estado do próprio KubePeep (cache, watches).

## Cenários obrigatórios de aceite

| Cenário | Resultado exigido |
| --- | --- |
| Pod em CrashLoopBackOff | aparece em Problems (critical); Investigation mostra owner/events/logs em um clique |
| workload com 5 Pods | logs agregados com streams limitados; trocar de tela cancela tudo; busca/regex funcional |
| buscar "portal" na paleta com recursos previamente carregados | resultados locais < 100 ms, sem chamada ao cluster; ausência no cache não é ausência no cluster (C06) |
| diagnostics aberta | métricas da F0 coerentes com o baseline |
| RBAC restritivo | seções sem permissão ficam ausentes/unknown — nunca "zero problemas" falso |
| Cenários de 200 namespaces de C02 sob carga | índices e problems dentro do orçamento de memória |

**Saída:** posicionamento problem-oriented consolidado (FAST + SAFE + PROBLEM-ORIENTED + DEVELOPER-FIRST). **Rollback:** cada detector/tela é independente e desligável sem afetar a leitura.
