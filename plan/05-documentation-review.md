# Revisão de Documentação do KubePeep

## 1. Objetivo

Validar a documentação existente contra o estado real do projeto, identificar desatualizações, omissões e inconsistências, e propor criação/atualização de documentos.

## 2. Documentação existente

### 2.1 Documentos normativos (docs/)

| Documento | Estado | Observações |
| --- | --- | --- |
| `README.md` | Atualizado | Reflete build desktop Wails e comandos atuais. |
| `docs/product-spec.md` | Atualizado | Personas, objetivos, jornadas, estados de interface. |
| `docs/architecture.md` | Parcialmente desatualizado | Ports listados não existem como interfaces Go centralizadas. |
| `docs/api.md` | Atualizado | Contratos, DTOs, códigos de erro bem documentados. |
| `docs/data-model.md` | Atualizado | Schema SQLite, preferências, migrations. |
| `docs/security.md` | Atualizado | Threat model, políticas, classificação de dados. |
| `docs/implementation-plan.md` | Atualizado | Fases, gates, matriz MVP, estratégia de testes. |
| `docs/desktop-architecture.md` | Novo | Descreve Wails v2, bridge, loopback, riscos. |

### 2.2 ADRs (docs/decisions/)

| ADR | Tema | Estado |
| --- | --- | --- |
| `0001-cli-service-bootstrap.md` | Cobra + Ginger | OK |
| `0002-health-and-degraded-state.md` | Health degradado | OK |
| `0003-streaming-protocols.md` | SSE/WebSocket | OK |
| `0004-local-runtime-and-process-lifecycle.md` | Lifecycle local | OK |

### 2.3 Pesquisa (docs/research/)

| Documento | Conteúdo |
| --- | --- |
| `aptakube-ux-benchmark.md` | Benchmark de experiência operacional. |
| `phase1-evidence.md` a `phase9-evidence.md` | Evidências por fase. |
| `compatibility-matrix.md` | Matriz de compatibilidade. |
| `dwyt.md` | Análise do DWYT. |
| `ginger-v1.4.4.md` | Análise do Ginger. |

### 2.4 Plano (plan/)

| Documento | Estado |
| --- | --- |
| `README.md` | Índice do plano de desenvolvimento original. |
| `01-descoberta.md` a `09-experiencia-operacional.md` | Planos por fase. |
| `matriz-aceite-mvp.md` | Matriz de aceite do MVP. |
| `matriz-aceite-ux.md` | Matriz de aceite da experiência operacional. |
| `estado-execucao-2026-08-30.md` | Estado consolidado do projeto. |

## 3. Gaps encontrados

### 3.1 Documentação arquitetural

**Problema:** `docs/architecture.md` §5 lista ports (`KubeconfigLoader`, `ContextService`, `NamespaceService`, etc.) que não existem como interfaces Go centralizadas em `internal/ports/`.

**Impacto:** desalinhamento entre documentação e código.

**Prioridade:** P1. **Complexidade:** S.

**Recomendação:**
- Atualizar `docs/architecture.md` para refletir a estrutura real de ports OU criar as interfaces centralizadas.
- Adicionar diagrama atualizado da organização de ports.

---

### 3.2 Documentação da Fase 9

**Problema:** a Fase 9 está em execução (15/84), mas não há documento consolidado dos facilitadores implementados (paleta, atalhos, filtros draft/applied, ordenação natural).

**Impacto:** dificuldade de rastrear o que já foi feito e o que falta.

**Prioridade:** P2. **Complexidade:** M.

**Recomendação:**
- Atualizar `plan/09-experiencia-operacional.md` com o estado atual.
- Documentar decisões de UX (draft/applied, natural sort, allowlist de refresh).

---

### 3.3 Documentação de UI/UX

**Problema:** não existe documento descrevendo o design system, tokens visuais, componentes reutilizáveis ou guia de contribuição de frontend.

**Impacto:** inconsistência visual e dificuldade para novos contribuidores.

**Prioridade:** P1. **Complexidade:** M.

**Recomendação:**
- Criar `docs/design-system.md` com tokens, componentes e padrões.
- Manter `plan/03-design-system.md` como roteiro de adoção.

---

### 3.4 Documentação de build desktop

**Problema:** `docs/desktop-architecture.md` é novo e bem detalhado, mas não há guia passo a passo de build desktop local nem troubleshooting específico.

**Impacto:** dificuldade para desenvolvedores testarem o desktop.

**Prioridade:** P2. **Complexidade:** S.

**Recomendação:**
- Adicionar seção "Build desktop local" em `README.md` ou `docs/desktop-architecture.md`.
- Documentar dependências por SO e comandos `make dev-desktop`, `make build-desktop-*`.

---

### 3.5 Documentação de logs e observabilidade

**Problema:** `docs/security.md` e `docs/architecture.md` mencionam logs operacionais e OpenTelemetry, mas não há guia de observabilidade prático.

**Impacto:** dificuldade para operar e diagnosticar o KubePeep.

**Prioridade:** P2. **Complexidade:** M.

**Recomendação:**
- Criar `docs/observability.md` com exemplos de configuração OTel, interpretação de logs e métricas.

---

### 3.6 Documentação de ações e permissões

**Problema:** não há documento prático listando as permissões Kubernetes necessárias por funcionalidade.

**Impacto:** usuários não sabem quais RBAC conceder.

**Prioridade:** P2. **Complexidade:** S.

**Recomendação:**
- Criar `docs/rbac-requirements.md` com a matriz de capabilities e permissões por recurso/verbo.

---

### 3.7 Inconsistências de nomenclatura

**Problema:** alguns trechos da documentação antiga usam `kubePeep` enquanto o executável atual é `kubepeep`. A maioria já foi corrigida, mas rastros existem.

**Impacto:** confusão para usuários.

**Prioridade:** P3. **Complexidade:** XS.

**Recomendação:**
- Revisar todos os `.md` e normalizar para `kubepeep` (executável) e `Kube Peep` (produto).

---

## 4. Documentos propostos

| Documento | Finalidade | Prioridade |
| --- | --- | --- |
| `docs/design-system.md` | Tokens, componentes e padrões de UI. | P1 |
| `docs/observability.md` | Logs, métricas, traces e OTel. | P2 |
| `docs/rbac-requirements.md` | Permissões Kubernetes por funcionalidade. | P2 |
| `docs/desktop-build.md` | Guia de build desktop por SO. | P2 |
| `docs/contributing-frontend.md` | Convenções de componentes e estilo. | P3 |

## 5. Recomendações

1. **P1:** atualizar `docs/architecture.md` para refletir ports reais ou centralizá-los.
2. **P1:** criar `docs/design-system.md`.
3. **P2:** atualizar `plan/09-experiencia-operacional.md` com estado atual.
4. **P2:** criar `docs/observability.md` e `docs/rbac-requirements.md`.
5. **P2:** adicionar guia de build desktop.
6. **P3:** normalizar nomenclatura `kubepeep`/`Kube Peep`.

## 6. Critérios de aceite

- [ ] Toda documentação normativa alinhada com o código.
- [ ] Documentos propostos criados e revisados.
- [ ] Links internos validados (sem quebrados).
- [ ] Nomenclatura consistente.
- [ ] Documentação de build desktop testada em pelo menos Linux.
