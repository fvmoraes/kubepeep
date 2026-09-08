# Plano de execução — KubePeep v0.7 (estado, performance e investigação)

Este plano transforma os três documentos de [`v0.7_reference/`](v0.7_reference/) — [avaliação e evolução](v0.7_reference/KUBEPEEP_AVALIACAO_E_PLANO_DE_EVOLUCAO.md), [performance e escalabilidade](v0.7_reference/KUBEPEEP_PERFORMANCE_SCALABILITY_PLAN.md) e [refinamento UI/UX](v0.7_reference/KUBEPEEP_UI_UX_REFINEMENT_PLAN.md) — em entregas faseadas executáveis. **Todo o conteúdo é tratado como trabalho de ajuste, melhoria e revisão a executar nas fases**, independentemente do que já exista na base: nada é assumido como pronto, e cada entrega exige evidência própria.

Comece pelo [estado e limites](v0.7/00-estado-e-escopo.md), depois consulte a [matriz de entregas e aceite](v0.7/01-matriz-de-entregas.md). O plano anterior (expansão de recursos) está executado e arquivado em [`v0/`](v0/), com evidências em [`v0/03-evidencias-execucao.md`](v0/03-evidencias-execucao.md).

**Direção central:** transformar o KubePeep de aplicação orientada a consultas em aplicação **orientada a estado** — `LIST → snapshot → cache → watch → UI incremental` — com performance percebida de IDE Kubernetes local, refinamento UI/UX sem redesign e experiência de investigação (FAST + SAFE + PROBLEM-ORIENTED + DEVELOPER-FIRST).

**Regras obrigatórias do produto:**

1. **Revisão funcional total (P0, bloqueante, Fase 0):** corrigir o bug de visualizar dados de **Pods e a tela ficar sem nada** e revisar **todos os cliques que não funcionam** na interface. Nenhum clique morto sobrevive à release.
2. **Scope default obrigatório por contexto:** todo contexto com namespaces cadastrados tem um scope marcado como **default**, carregado **obrigatoriamente** ao abrir o KubePeep e ao selecionar/alternar contexto. Sem default definido, a UI conduz à marcação. Nunca amplia RBAC nem exige `list/get namespaces`. ([Fase 4](v0.7/phase-04-refinamento-ui-ux.md), R03)

## Ordem de execução

| Fase | Entrega | Dependências | Estado |
| --- | --- | --- | --- |
| 0 | [Correção funcional P0, baseline e instrumentação](v0.7/phase-00-baseline-instrumentacao.md) | base | planejado; R01/R02 bloqueantes |
| 1 | [Paginação estratégica e uso responsável do API Server](v0.7/phase-01-paginacao-e-api-server.md) | F0 | planejado |
| 2 | [Estado orientado a snapshot: cache sob demanda + WATCH](v0.7/phase-02-cache-watch-snapshot.md) | F1 | planejado |
| 3 | [Frontend progressivo, virtualizado e inicialização instantânea](v0.7/phase-03-frontend-instantaneo.md) | F1; efeito completo com F2 | planejado |
| 4 | [Fechamento do refinamento UI/UX e scope default](v0.7/phase-04-refinamento-ui-ux.md) | F0 (inventário); paralelizável com F1–F3 | planejado |
| 5 | [Investigação: problems, diagnóstico e logs agregados](v0.7/phase-05-investigacao-diagnostico.md) | F2, F4 | planejado |
| 6 | [Protocolo e tuning avançado (condicional a benchmark)](v0.7/phase-06-protocolo-avancado.md) | F0, F2 | planejado; cada item só ativa com ganho medido |
| 7 | [Validação comparativa e preparação da release](v0.7/phase-07-validacao-release.md) | F0–F6 | planejado |

F4 pode avançar em paralelo a F1–F3 coordenando os arquivos compartilhados. A evidência de execução é registrada em [`v0.7/03-evidencias-execucao.md`](v0.7/03-evidencias-execucao.md).

## Regras de trabalho

1. **Apenas commit; nunca push autônomo.** Publicação, tags de release e candidatas dependem de decisão explícita do usuário. Preparar e validar localmente antes dessa decisão.
2. Consultar o grafo do Codebase MCP antes de alterar estrutura; conferir fonte e cobertura. Ler o Project Brain e salvar decisões/contexto no Obsidian. Histórico antigo não substitui evidência do código atual.
3. Prefixar comandos de shell com `rtk`. Antes de cada commit, executar `rtk scripts/security_check.sh HEAD` e usar a identidade GitHub noreply aprovada.
4. Preservar Secrets metadata-only, loopback, RBAC no backend, CSRF, geração/cancelamento, paginação limitada e ausência de conteúdo Kubernetes no navegador. Cursor e cache de recursos apenas em memória — nunca em disco nem no SQLite.
5. Manter no Git código, testes, fixtures sintéticas e documentação útil. Kubeconfigs, credenciais, bancos, logs, relatórios crus, traces, screenshots e binários ficam fora do Git. Evidências contêm somente comando, resultado resumido e commit.
6. Não extrair abstração nova nem recriar feature existente sem demonstrar o problema. Anti-patterns da referência de performance (§80) são proibições explícitas: aumentar cursor/goroutines/QPS "no escuro", polling agressivo, informer global, SQLite para recursos, retry sem jitter.
7. Medir antes de otimizar: baseline da F0 precede qualquer mudança de estratégia; F6 só ativa otimização com ganho comprovado.

## Como executar e concluir uma fase

- Escolher um ID de tarefa; conferir pré-requisitos e contratos existentes. Registrar decisões de API/dados antes de codificar mudanças nesses contratos.
- Implementar a menor fatia completa; atualizar docs no mesmo commit funcional. Não habilitar destino cujo backend/rota ainda não estejam disponíveis.
- Validar os cenários de aceite da fase e os testes afetados. Antes de fechar a fase, executar a verificação integrada; a F7 repete o gate no commit final.
- Marcar tarefas apenas com evidência: `ID → commit → comando/cenário → resultado`. "Compilou" não comprova RBAC, navegação, execução Wails ou ganho de performance. Bloqueio de ambiente é pendente registrado, nunca aprovado.
- Atualizar a matriz e as evidências; salvar contexto no Project Brain; revisar diff/staging e executar o gate de segurança antes do commit.

| Verificação | Comando existente |
| --- | --- |
| Gate integrado local | `rtk make verify` |
| Testes Go/integração + frontend | `rtk make test` |
| E2E Playwright | `rtk make test-e2e` |
| Formatação, lint e tipos | `rtk make format-check lint typecheck` |
| Race detector | `rtk make test-race` |
| CLI embutida e smoke | `rtk make build smoke` |
| Desktop Wails, dependências nativas instaladas | `rtk make build-desktop` |
| Segurança pré-commit | `rtk scripts/security_check.sh HEAD` |
| Benchmark de performance | laboratório da [Fase 0](v0.7/phase-00-baseline-instrumentacao.md) sobre `test/kind/` |

Os alvos podem repetir etapas; na execução diária, validar cada requisito uma única vez. Build nativo e execução nas demais plataformas são gates da F7, não inferências a partir do cross-build.

## Escopo e histórico

A v0.7 entrega: correção funcional P0, arquitetura de leitura orientada a estado (cursor opaco, estratégias de paginação, cache+watch), frontend progressivo/virtualizado/instantâneo, refinamento UI/UX fechado com scope default obrigatório, camada de investigação e otimizações de protocolo condicionais a benchmark. Multi-contexto simultâneo, Prometheus, Resource Diff entre origens, Helm, Gateway API, CR genérico, plugins e supply chain extra ficam no [backlog pós-v0.7](v0.7/02-backlog-pos-v1.md).

Planos anteriores permanecem no Git (`plan/v0/` executado; `5ac7320^:plan/` histórico). As referências em `v0.7_reference/` são preservadas como fonte; decisões de recorte ficam neste plano.

## Avaliação antes da execução

Consulte a [avaliação técnica do plano](v0.7/04-avaliacao-do-plano.md). A revisão documental não fecha F0 nem substitui seus testes e benchmarks. Nesta execução, cada fase termina com validação e commit; a próxima exige autorização explícita do usuário.
