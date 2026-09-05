# Fase 7 — Release 1.0.0

> **Objetivo:** fechar a v1: documentação atualizada, gates de distribuição pendentes (F8-42/46/48), release candidate imutável e tag 1.0.0.
> **Prioridade:** P0. **Dependências:** Fases 1–5 concluídas (6 pode ter sido adiada para 1.1).

## Tarefas

### Documentação

- [ ] **V7-01** `docs/api.md`: todas as coleções novas da v1 (rotas, envelope, filtros, capabilities).
- [ ] **V7-02** `docs/architecture.md`: famílias novas, contrato cluster-scoped, padrão "adicionar uma família" (referência à Fase 1/ADR).
- [ ] **V7-03** `docs/product-spec.md`: grupos de navegação §36 completos, ações contextuais, port-forward panel.
- [ ] **V7-04** `docs/design-system.md`: revisão final contra o código (tokens/components/framework).
- [ ] **V7-05** `README.md`: paridade de recursos da v1; screenshots novos do UI redesenhado (locais, sanitizados; só entram no repo se o usuário decidir).
- [ ] **V7-06** `CHANGELOG.md`: entrada 1.0.0 via ferramenta de release existente.

### Distribuição (gates F8)

- [ ] **V7-07** F8-42/46/48 (herdadas da fase 8 original): fechar os itens restantes de distribuição listados em `git show 5ac7320^:plan/09-experiencia-operacional.md` e `git show 5ac7320^:plan/08-distribuicao.md` — reavaliar cada um contra a CI atual antes de executar.
- [ ] **V7-08** Release candidate imutável: tag `v1.0.0-rc.1`, artefatos de todos os runners nativos verdes, checksums; smoke dos archives (`make smoke` equivalente) e do desktop.
- [ ] **V7-09** E2E de sanidade contra o RC empacotado (harness kind já usado pela CI).
- [ ] **V7-10** Tag `v1.0.0` somente após RC aprovado pelo usuário — **commit ok, push decisão do usuário**; a publicação da release no GitHub também é decisão dele.

### Verificação final da v1

- [ ] **V7-11** Checklist §38 da especificação de referência, reavaliado item a item (fonte única, escala tipográfica, semânticas, sidebar completa, cluster-scoped sem escopo, layout único, sem scroll horizontal em 1280×720, build/testes verdes).
- [ ] **V7-12** Revisão de higiene: `scripts/security_check.sh HEAD`, `govulncheck`, `npm audit`, nenhum artefato novo versionado por engano (`git status` limpo).

## Critérios de aceite

- Tag `v1.0.0-rc.1` e `v1.0.0` criadas localmente com CI verde no commit apontado (executar CI via push é decisão do usuário).
- Documentação descreve exatamente o binário da tag (nenhum recurso "planejado" documentado como pronto).
- Zero pendências abertas das Fases 1–5 sem item correspondente em backlog pós-1.0.

## Rollback

- Tag local é descartável (`git tag -d`); releases publicadas seguem o processo de rollback da CI existente.
