# Plano de execução — KubePeep 1.0

Este plano transforma a [especificação UI/UX original](reference/KubePeep_UI_UX_Design_System_e_Recursos_Kubernetes.md) em entregas executáveis. O redesign do commit `5ac7320` é a base a preservar. As fases abaixo descrevem trabalho **planejado**, não recursos já disponíveis no aplicativo.

Comece pelo [estado e limites](v1/00-estado-e-escopo.md), depois consulte a [matriz de entregas e aceite](v1/01-matriz-de-entregas.md). A documentação em [docs/](../docs/README.md) descreve o comportamento implementado; este diretório define o próximo trabalho.

## Ordem de execução

| Fase | Entrega | Dependências | Estado |
| --- | --- | --- | --- |
| Base | [Estado atual e escopo](v1/00-estado-e-escopo.md) | redesign `5ac7320` | implementado; regressão revalidada na F7 |
| 1 | [Contrato de recursos e Nodes ponta a ponta](v1/phase-01-backend-recursos.md) | base | planejado |
| 2 | [Cluster e Storage](v1/phase-02-cluster-storage.md) | F1 | planejado |
| 3 | [Workloads e Configuration](v1/phase-03-workloads-configuracao.md) | F1 | planejado |
| 4 | [Network, Access Control e Administration](v1/phase-04-acesso-administracao.md) | F1; integra ServiceAccounts da F3 | planejado |
| 5 | [Experiência operacional](v1/phase-05-experiencia-operacional.md) | F1; integra famílias F2–F4 ao fechá-las | planejado |
| 6 | [Preferências e integração final](v1/phase-06-preferencias-integracao.md) | F2–F5 | planejado |
| 7 | [Validação e preparação da release](v1/phase-07-release-v1.md) | F1–F6 | planejado |

Após F1, as famílias de F2–F4 e o trabalho de UX em F5 podem avançar em paralelo, coordenando os arquivos compartilhados. F5 define colunas e referências em memória; F6 persiste esses contratos. Não há dependência circular. Cada fase admite vários commits pequenos por família ou jornada.

## Regras de trabalho

1. **Apenas commit; nunca push autônomo.** Publicação, tags de release e execução remota com efeitos de publicação dependem de decisão explícita do usuário. Preparar e validar localmente antes dessa decisão.
2. Consultar o grafo do Codebase MCP antes de alterar estrutura; conferir fonte e cobertura. Ler o Project Brain e salvar decisões/contexto no Obsidian. Histórico antigo não substitui evidência do código atual.
3. Prefixar comandos de shell com `rtk`. Antes de cada commit, executar `rtk scripts/security_check.sh HEAD` e usar a identidade GitHub noreply aprovada.
4. Preservar Secrets metadata-only, loopback, RBAC no backend, CSRF das operações que o exigem, geração/cancelamento, paginação limitada e ausência de armazenamento de conteúdo Kubernetes no navegador.
5. Manter no Git código, testes, fixtures sintéticas e documentação útil. Kubeconfigs, credenciais, bancos, logs, relatórios crus, traces, screenshots de execução e binários de release ficam fora do Git. Evidências duráveis contêm somente comando, resultado resumido e commit, sem dados privados.
6. Não extrair abstração nova nem recriar feature existente sem demonstrar o problema. Toda família segue serviço → integração Kubernetes → handler → cliente → componentes compartilhados, com registro de capacidade e navegação coerentes.

## Como executar e concluir uma fase

- Escolher um ID de tarefa; conferir pré-requisitos e contratos existentes. Registrar decisões de API/dados antes de codificar mudanças nesses contratos.
- Implementar a menor fatia completa; atualizar docs no mesmo commit funcional. Não habilitar destino cujo backend/rota ainda não estejam disponíveis.
- Validar os casos de aceite da fase e os testes afetados. Antes de fechar a fase, executar a verificação integrada; a F7 repete o gate no commit final.
- Marcar tarefas apenas com evidência: `ID → commit → comando/cenário → resultado`. “Compilou” não comprova RBAC, navegação ou execução Wails. Registrar bloqueio de ambiente como pendente, nunca como aprovado.
- Atualizar a matriz e salvar contexto no Project Brain; revisar diff/staging e executar o gate de segurança antes do commit.

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

Os alvos podem repetir etapas; na execução diária, usar os equivalentes já documentados para validar uma única vez cada requisito. Build nativo e execução nas demais plataformas são gates da F7, não inferências a partir do cross-build.

## Escopo e histórico

A v1 inclui todos os recursos obrigatórios da matriz, preferências de shell e UX consistente. Helm, Gateway API e os recursos explicitamente condicionais da especificação ficam no [backlog pós-v1](v1/02-backlog-pos-v1.md). Multi-contexto simultâneo também tem escopo próprio ali; **F6 permanece obrigatória** para a v1.

Planos anteriores permanecem no Git (`5ac7320^:plan/`). Seus checkboxes registram diferentes momentos e não representam um percentual global de conclusão. A referência original é preservada; decisões de recorte ficam neste plano, sem reescrever o pedido original.
