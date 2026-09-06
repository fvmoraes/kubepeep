# Fase 3 — Workloads, Configuration e ServiceAccounts

**Prioridade:** P0. **Entrada:** F1. **Matriz:** R07–R10, R19–R22, R28, U03–U05. Pode avançar junto da F2/F4.

As famílias novas desta fase são namespaced. ServiceAccounts é implementado aqui por compartilhar esse contrato, mas aparece em **Access Control** na navegação.

## Tarefas

- [ ] **V3-01 — ReplicaSets.** Ampliar o catálogo Workloads com ReplicaSets (`apps`) sem criar duas coleções divergentes. Definir com o padrão existente `kind=replicasets`, detalhe/YAML e caminho `/workloads/kind/replicasets`; atualizar query allowlist, DTO, capabilities, testes dos kinds e deep links. Exibir desired/ready/available/owner/age.
- [ ] **V3-02 — Relações.** Deployment → ReplicaSets → Pods por `ownerReferences` e UID, não igualdade de nome. Validar comportamento atual do filtro `workload` de Pods antes de estendê-lo; preservar Deployment/StatefulSet/DaemonSet/Job/CronJob. Destino negado/desconhecido não provoca fetch de conteúdo nem link enganoso.
- [ ] **V3-03 — HorizontalPodAutoscalers.** Integração `autoscaling` com versão descoberta/suportada, lista/detalhe com target, min/max, current/desired, condições e métricas disponíveis. Mostrar ausência/unknown sem inventar uso zero; ligar ao workload alvo.
- [ ] **V3-04 — PodDisruptionBudgets.** Integração `policy`, lista/detalhe de minAvailable/maxUnavailable, current/desired healthy, disruptionsAllowed, expectedPods e selector aprovado. Unidades/IntOrString preservadas no contrato.
- [ ] **V3-05 — ResourceQuotas.** Exibir hard/used por recurso com unidades consistentes; aceitar quotas com contagens de objetos sem interpretar os valores como conteúdo desses objetos. Arrays/maps limitados e ausência distinta de zero.
- [ ] **V3-06 — LimitRanges.** Exibir tipo, min/max, default/defaultRequest e ratios quando presentes. Limites de tamanho e renderização previsível para múltiplos itens.
- [ ] **V3-07 — ServiceAccounts.** Lista/detalhe de metadados aprovados e contagens; não buscar tokens, não seguir referências de Secret e não apresentar annotations arbitrárias. Na v1, não oferecer YAML bruto para esta família; documentar eventual documento sanitizado antes de implementá-lo. Metadados não devem se transformar em exceção para ler Secret.
- [ ] **V3-08 — Integração comum.** Todas as famílias recebem DTO/porta/runtime/handler/wiring/cliente/rotas/capabilities `list/get` e página via framework. Atualizar catálogos de filtros, navigation e destinos. Configuration continua com ConfigMaps/Secrets existentes e quatro novas entradas; ServiceAccounts fica em Access Control.
- [ ] **V3-09 — Detalhes e feedback.** Conditions, eventos relacionados e tabs só onde pertinentes/autorizados. Não fabricar aba vazia de logs/métricas para um recurso que não possui essas capacidades. Preservar semânticas visuais do Design System.
- [ ] **V3-10 — Documentação e aceite.** Atualizar API, RBAC e contrato de kinds. O aviso de scale controlado por HPA na F5 consome esta leitura com autorização independente; ausência de `list hpa` não prova inexistência de HPA.

## Aceite

| Cenário | Evidência |
| --- | --- |
| ReplicaSet de Deployment e homônimo sem relação | navegação inclui somente owners corretos; regressão dos cinco kinds anteriores passa |
| Listagem multi-namespace com um namespace negado | coverage e dados permitidos preservados; detalhe negado não vaza |
| HPA sem métricas; PDB sem disruptions disponíveis | UI diferencia unknown/ausente/zero e mostra condições reais |
| Quota/LimitRange com quantidades e campos ausentes | unidade correta e DTO limitado |
| ServiceAccount com annotations/token refs sintéticos | nenhuma resposta ou YAML expõe campos proibidos; nenhum fetch de Secret |
| Novo destino por sidebar/paleta/deep link | mesma seleção, scope e estado ativo; reload/back funcionam |

Testes de contrato e adapter por família; E2E de ReplicaSets e HPA/PDB, mais inspeção negativa de ServiceAccounts. Gate integrado conforme [plano](../README.md).

**Saída:** Workloads/Configuration completos e ServiceAccounts disponível. **Rollback:** commits por família com atualização coordenada de kind map, rota e nav; manter compatibilidade com filtros/favoritos já persistidos.
