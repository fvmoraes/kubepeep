# Plano KubePeep v1 — Design System, Recursos Kubernetes e Experiência Operacional

> **Estado:** UI/UX, Design System, shell de navegação e resource framework **entregues** (`5ac7320`, 2026-09-04).
> **Objetivo da v1:** expandir os recursos Kubernetes suportados (backend + páginas), fechar a experiência operacional (Fase 9) e cortar a release 1.0.0.
> **Base:** [reference/KubePeep_UI_UX_Design_System_e_Recursos_Kubernetes.md](reference/KubePeep_UI_UX_Design_System_e_Recursos_Kubernetes.md) — especificação original da reorganização.
> **Histórico:** o plano de desenvolvimento original (`01-descoberta`…`09-experiencia-operacional`) e a revisão de 2026-08 (`00-current-state`…`10-acceptance-checklist`) permanecem no histórico Git (commit `5ac7320^`); foram arquivados por estarem ≥99% executados.

## Regras de execução (inegociáveis)

1. **Apenas commit; nunca push.** O push é sempre decisão explícita do usuário.
2. Antes de **todo** commit: `scripts/security_check.sh HEAD` (gitleaks, identidades noreply, paths, mensagens).
3. Nenhum dado sensível no repositório: kubeconfig, credenciais, tokens, chaves, conteúdo de Secret, logs de aplicação, paths de máquina, resultados crus de teste.
4. Funcionalidades existentes não podem regredir: `make verify` (ou os alvos equivalentes) verde antes de cada commit de fase.
5. Novo recurso Kubernetes = backend (adapter → service → handler → capabilities) **+** página via resource framework **+** nav em `web/src/navigation/tree.tsx` **+** testes. Nunca página avulsa com estilos próprios.
6. Segurança constante: Secrets metadata-only; RBAC revalidado no backend; geração/abort em toda query; nada de storage de navegador no frontend (`web/src/security.test.ts`).

## Como validar

| Alvo | Comando |
| --- | --- |
| Unit + integração | `make test` (`go test ./...` + `npm test` no `web/`) |
| E2E | `make test-e2e` |
| Qualidade | `make lint typecheck format-check` |
| Build | `make build` (CLI) · `make build-desktop` (Wails) |
| Segurança | `scripts/security_check.sh HEAD` |

## Fases

| Fase | Documento | Escopo | Estado |
| --- | --- | --- | --- |
| 0 | — | UI/UX, Design System, shell, navegação em árvore, resource framework, migração das 10 páginas | ✅ `5ac7320` |
| 1 | [v1/phase-01-backend-recursos.md](v1/phase-01-backend-recursos.md) | Fundação backend para coleções novas (adapters → services → handlers → capabilities) com Nodes como piloto | ☐ |
| 2 | [v1/phase-02-cluster-storage.md](v1/phase-02-cluster-storage.md) | Cluster scoped: Nodes, Leases, PV, PVC, StorageClasses, CSI | ☐ |
| 3 | [v1/phase-03-workloads-configuracao.md](v1/phase-03-workloads-configuracao.md) | ReplicaSets, HPA, PDB, ResourceQuotas, LimitRanges, ServiceAccounts | ☐ |
| 4 | [v1/phase-04-acesso-administracao.md](v1/phase-04-acesso-administracao.md) | RBAC objects, CRDs, Priority/Runtime Classes, Admission Webhooks, IngressClasses, NetworkPolicies, Endpoints | ☐ |
| 5 | [v1/phase-05-experiencia-operacional.md](v1/phase-05-experiencia-operacional.md) | Port-forward panel, ações contextuais, colunas, favoritos/recentes, YAML/logs refinements | ☐ |
| 6 | [v1/phase-06-preferencias-multi-contexto.md](v1/phase-06-preferencias-multi-contexto.md) | Persistência de preferências de shell, ordenação/filtros avançados, multi-contexto readonly | ☐ |
| 7 | [v1/phase-07-release-v1.md](v1/phase-07-release-v1.md) | Documentação atualizada, RC imutável, gates de distribuição, tag 1.0.0 | ☐ |

Itens **fora da v1** (backlog pós-1.0): Helm Releases, Gateway API (Gateways/HTTPRoutes/GRPCRoutes) — a navegação já reserva os grupos; VolumeAttributesClasses e ValidatingAdmissionPolicies quando a versão mínima de cluster suportar.

## Definição de pronto (por fase)

- Tarefas do documento marcadas com evidência (commit + teste).
- `go test ./...`, `npm test`, `test:e2e`, lint, typecheck e build verdes.
- E2E cobre o fluxo novo nas resoluções 1280×720 e 1920×1080 (screenshots em evidência local, nunca versionados).
- Documentação afetada (`docs/api.md`, `docs/architecture.md`, `docs/design-system.md`, `docs/rbac-requirements.md`) atualizada no mesmo commit da feature.
- Sem push; usuário decide quando publicar.
