# Plano de Melhorias — Revisão Completa do KubePeep

> **Status:** revisão técnica, funcional e visual concluída em 2026-08-31.
> **Base:** HEAD `c7e291d` (`feat: desktop support and security hardening`).
> **Objetivo:** transformar os achados da revisão em um plano executável de melhorias, preservando a identidade minimalista do KubePeep e aproximando a experiência de referência do Aptakube sem copiá-la.

## 1. Propósito deste plano

Este diretório contém o resultado de uma revisão profunda do KubePeep, cobrindo:

- funcionamento real da aplicação;
- arquitetura e qualidade técnica;
- experiência do usuário e interface;
- consistência visual e design system;
- documentação e alinhamento com o código;
- observabilidade, confiabilidade e desempenho;
- evolução do produto.

O plano original de desenvolvimento (`01-descoberta.md` a `09-experiencia-operacional.md`, `matriz-aceite-mvp.md`, `matriz-aceite-ux.md`) continua vigente e deve ser consultado para rastreabilidade das fases de implementação. Este novo plano complementa o anterior com a visão do estado atual e o roteiro de melhorias priorizadas.

## 2. Índice

| Arquivo | Conteúdo |
| --- | --- |
| [00-current-state.md](00-current-state.md) | Estado atual do projeto, validações executadas e evidências coletadas. |
| [01-functional-review.md](01-functional-review.md) | Revisão funcional completa: fluxos, estados, gaps e recomendações. |
| [02-ui-ux-review.md](02-ui-ux-review.md) | Avaliação de interface, navegação, telas e experiência do usuário. |
| [03-design-system.md](03-design-system.md) | Proposta de design system, tokens visuais e componentes reutilizáveis. |
| [04-architecture-review.md](04-architecture-review.md) | Análise da arquitetura Go, qualidade técnica e débitos. |
| [05-documentation-review.md](05-documentation-review.md) | Revisão da documentação existente e gaps encontrados. |
| [06-improvement-roadmap.md](06-improvement-roadmap.md) | Roadmap por fases executáveis, com itens priorizados. |
| [07-testing-strategy.md](07-testing-strategy.md) | Estratégia de testes para as melhorias propostas. |
| [08-observability-plan.md](08-observability-plan.md) | Plano de logs internos, métricas, traces e OpenTelemetry. |
| [09-risks-and-migrations.md](09-risks-and-migrations.md) | Riscos, estratégias de migração e rollback. |
| [10-acceptance-checklist.md](10-acceptance-checklist.md) | Checklist objetivo de aceite por fase. |

## 3. Resumo executivo dos achados

### 3.1 O que funciona bem

- **Base técnica sólida:** Go 1.26, Ginger v1.4.4, Cobra, SQLite sem CGO, client-go v0.35.7, React 19, Vite 8, React Router 8.
- **Segurança bem fundamentada:** bind loopback, Host/Origin/CSRF, tokens de controle, Secret metadata-only, RBAC revalidado, redaction de logs.
- **Testes verdes:** 730 testes Go em 36 pacotes, 79 testes Vitest, lint, typecheck, build, `govulncheck` e `npm audit` sem vulnerabilidades.
- **Arquitetura de cancelamento:** geração monotônica, cancelamento hierárquico, invalidação de caches.
- **MVP funcional:** dashboard, recursos somente leitura, logs, ações autorizadas, settings e preferências implementados.
- **Desktop:** suporte inicial Wails v2.15.0 com bridge in-process e loopback interno para streams.

### 3.2 Principais problemas e oportunidades

1. **Frontend monolítico e sem design system:** CSS global único, componentes gigantes, tokens visuais ausentes, Tailwind importado mas não usado, SVGs da marca não integrados.
2. **Inconsistência visual entre telas:** tabelas, formulários, estados de erro e espaçamentos variam arbitrariamente.
3. **Backend com ports dispersos:** `internal/ports` está vazio; interfaces de port espalhadas por handlers e serviços.
4. **Duplicação dashboard/resources:** classificação de pods/workloads e lógicas de listagem existem em duas camadas.
5. **Testes flaky e races:** alguns testes falham com `-race`, indicando problemas reais de concorrência.
6. **Fase 9 incompleta:** favoritos/recentes, diff, gerenciador de port-forward, multi-contexto e parser composto de filtros ainda não implementados.
7. **Documentação fragmentada:** partes da Fase 9 e detalhes de UI/UX carecem de atualização.

### 3.3 Decisões transversais

- Manter a identidade visual própria; não copiar marca, cores ou layout do Aptakube.
- Priorizar componentes reutilizáveis e tokens centralizados sobre correções isoladas.
- Preservar a segurança como premissa inegociável em todas as melhorias.
- Não introduzir edição/aplicação genérica de YAML durante este plano.
- Continuar usando Kind canônico para validação de caminhos reais.

## 4. Próximos passos recomendados

1. Executar a **Fase 0** (correções críticas) antes de qualquer refatoração ampla.
2. Fechar **F4-49** (matriz exaustiva de RBAC) e criar uma **release candidate** para desbloquear F8-42/F8-46/F8-48.
3. Implementar o **design system** e integrar os SVGs oficiais da marca.
4. Refatorar componentes monolíticos do frontend em componentes menores e reutilizáveis.
5. Centralizar ports no backend e consolidar dashboard/resources.
6. Avançar a **Fase 9** com parser composto de filtros, YAML highlight, terminal profissional e gerenciador de port-forward.
7. Atualizar a documentação em conjunto com cada mudança.

## 5. Referências

- Plano de desenvolvimento original: `01-descoberta.md` a `09-experiencia-operacional.md`.
- Matrizes de aceite: `matriz-aceite-mvp.md`, `matriz-aceite-ux.md`.
- Documentação normativa: `docs/product-spec.md`, `docs/architecture.md`, `docs/api.md`, `docs/security.md`, `docs/data-model.md`, `docs/implementation-plan.md`.
- Benchmark de experiência: `docs/research/aptakube-ux-benchmark.md`.
- Evidências de execução: `docs/research/phase1-evidence.md` a `phase9-evidence.md`.
- Screenshots da revisão: `docs/research/screenshots-review/`.
