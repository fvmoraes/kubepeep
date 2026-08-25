# Matriz de aceite da experiência operacional

Esta matriz complementa, sem renumerar ou substituir, os 27 critérios do
[MVP original](matriz-aceite-mvp.md). Os facilitadores são inspirados em
comportamentos documentados oficialmente pelo Aptakube, mas possuem contratos,
design e implementação próprios do Kube Peep.

**Estado inicial em 2026-08-25:** requisitos aprovados; evidências herdáveis e
lacunas ainda serão auditadas. Um item só muda para `[x]` quando a evidência do
estado atual do código tiver sido executada e registrada.

| ID | Status | Critério | Fase/tarefas | Evidência mínima |
| --- | --- | --- | --- | --- |
| UX-M01 | [ ] | O produto descobre contextos do kubeconfig sem modificar arquivos nem expor credenciais | F9-01–08, F9-66–75 | hash/mtime antes/depois, contextos parciais, plugin `exec` sanitizado, inspeção negativa |
| UX-M02 | [x] | Paleta e atalhos permitem navegar somente por teclado, sem oferecer mutações ou persistir dados remotos | F9-09–18 | `CommandCenter.test.tsx`, `App.test.tsx` e Playwright; foco/teclado, zero fetch próprio e storages vazios; [evidência F9](../docs/research/phase9-evidence.md) |
| UX-M03 | [ ] | Listas oferecem busca, filtros visíveis/resetáveis e ordenação natural determinística | F9-19–25, F9-29 | parser/ordenação unitários, paginação, alta cardinalidade e E2E |
| UX-M04 | [ ] | Favoritos e recentes armazenam apenas referências allowlisted, limitadas e removíveis | F9-26–29 | migration/DTO/API/UI, limpeza, limites e inspeção de browser/SQLite |
| UX-M05 | [ ] | Listas e detalhes apresentam status e relações de forma humana, acessível e ligada à evidência real | F9-30–33, F9-77–78 | componentes por kind, relações autorizadas, fixtures degradadas e acessibilidade |
| UX-M06 | [ ] | Ações rápidas aparecem apenas para alvo/capability pertinente e toda mutação é reautorizada | F9-34–40 | allowed/denied/unknown na UI, SAR imediato, 403/409/cancelamento no Kind |
| UX-M07 | [ ] | YAML somente leitura oferece busca/recolhimento sem persistência e nunca revela Secret | F9-41–45, F9-79–80 | get permitido/negado, objeto grande, Secret metadata-only, sentinela ausente |
| UX-M08 | [ ] | Diff somente leitura mantém as duas origens explícitas, normalização opt-in e recusa Secret | F9-46–49 | ausência/vazio, versões diferentes, 403/offline, objeto grande e sentinela |
| UX-M09 | [ ] | Logs atuais/anteriores/follow podem ser filtrados e agregados com proveniência, budgets e cancelamento | F9-50–56, F9-59 | múltiplos pods/containers, denied parcial, backpressure, leak/storage scan |
| UX-M10 | [ ] | Métricas opcionais aparecem com unidades consistentes e degradam isoladamente | F9-57–59 | Metrics API presente/ausente/proibida/parcial e refresh sem duplicação |
| UX-M11 | [ ] | Port-forwards possuem gerenciador de sessões, bind loopback e encerramento individual/coletivo | F9-60–65 | RBAC, colisão de porta, limite, stop/stop-all, shutdown e leak check |
| UX-M12 | [ ] | Um conjunto de contextos pode ser consultado somente para leitura com origem em cada resultado | F9-66–71 | dois clusters/contextos, merge determinístico, provenance e nenhuma mutação em massa |
| UX-M13 | [ ] | Falha, autorização e reconexão são isoladas por contexto, sem mistura de capability ou resposta obsoleta | F9-70–75 | um cluster offline, RBAC divergente, revogação, atraso, retry/backoff e generation fencing |
| UX-M14 | [ ] | Todas as superfícies novas distinguem loading, vazio, offline, proibido, unknown, parcial, cancelado, stale e truncado | F9-74, F9-77–78 | matriz de estados em componentes + E2E, sem depender apenas de cor |
| UX-M15 | [ ] | Nenhuma facilidade nova persiste ou publica kubeconfig, credencial, Secret, log, YAML, objeto remoto ou diagnóstico bruto | F9-06, F9-42–43, F9-56, F9-76, F9-79–84 | gitleaks/history gate, sentinelas, storage/DB/log/archive scan e CI final |

## Requisitos futuros, fora do gate F9

Os itens abaixo ficam registrados para evitar que uma conveniência seja
introduzida informalmente, sem threat model ou critério de aceite. Eles não são
prometidos pela interface e não impedem a conclusão da Fase 9.

| ID | Capacidade | Pré-condições obrigatórias |
| --- | --- | --- |
| UX-P01 | edição/criação de YAML | schema, server-side dry-run, preview de diff, `resourceVersion`, RBAC, confirmação reforçada, recusa de Secret e teste de admission/409 |
| UX-P02 | quick actions de CronJob | contrato de trigger/suspend, verbs/subresources explícitos, idempotência, GitOps e Kind restritivo |
| UX-P03 | adaptadores especializados de CRD | versionamento, discovery, fallback genérico, fixtures por versão e nenhuma elevação de permissão |
| UX-P04 | exportação avançada de sessões | consentimento explícito, streaming direto, limites, redaction e zero cópia interna |

## Matriz de cobertura obrigatória por capacidade

Cada linha `UX-M` deve manter uma ficha de evidência com:

| Campo | Conteúdo |
| --- | --- |
| requisito | ID, prioridade, tarefas e estado |
| proveniência | fonte oficial do benchmark e data da consulta |
| superfície | grid, detalhe, painel, menu, paleta, viewer ou sessão |
| origem | profile, contexto, cluster, namespace e geração aplicáveis |
| Kubernetes | API group, resource, subresource, verb e resourceName quando aplicável |
| RBAC | sem acesso, leitura, logs, operação restrita, unknown e revogação durante a operação |
| affordance | oculta, desabilitada com motivo ou disponível |
| dados | classificação, memória, persistência, cópia, download, redaction e tamanho |
| confirmação | nenhuma, simples ou reforçada |
| degradação | loading, vazio, offline, forbidden, unknown, partial, cancelled, stale e truncated |
| evidência | unitário, integração, frontend, Kind, E2E, native e inspeção negativa |
| rastreabilidade | commit, workflow/run e relatório versionado sem dados sensíveis |

## Cenários mínimos

- um contexto e um namespace;
- um contexto e múltiplos namespaces;
- dois contextos com um offline;
- permissões diferentes entre origens;
- permissão revogada entre capability e operação;
- Metrics API ausente, proibida e parcial;
- watch/stream encerrado, reconectado e cancelado por troca de geração;
- Secret, log, YAML e erro com sentinela sintética;
- mutação com sucesso, `403`, `409`, admission rejection e cancelamento;
- navegação completa somente por teclado;
- alta cardinalidade, resposta atrasada e troca rápida de escopo;
- browser storage, SQLite, sidecars, backups, logs e archives sem dados proibidos.

## Regra de atualização

1. Não inferir conclusão por existência de componente ou teste antigo.
2. Executar a evidência contra o commit que contém o requisito.
3. Registrar falhas parciais e plataformas separadamente.
4. Reabrir o item quando contrato, dependência ou superfície mudar.
5. Nunca colar em evidência token, certificado, kubeconfig, conteúdo remoto,
   mensagem bruta, path local ou dado pessoal.
