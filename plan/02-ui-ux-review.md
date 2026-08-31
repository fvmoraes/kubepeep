# Revisão de UI/UX do KubePeep

## 1. Objetivo

Avaliar todas as telas, componentes e estados visuais do KubePeep, identificar inconsistências e propor melhorias que aproximem a experiência de referência do Aptakube sem copiar sua identidade.

## 2. Inventário de telas e componentes

### 2.1 Telas (rotas)

| Rota | Componente | Responsabilidade |
| --- | --- | --- |
| `/` | `Dashboard` | Visão geral com blocos independentes. |
| `/workloads` | `ResourcePages` | Lista e detalhe de workloads. |
| `/pods` | `ResourcePages` | Lista e detalhe de pods. |
| `/logs` | `LogsPage` | Visualizador de logs. |
| `/events` | `ResourcePages` | Lista de eventos. |
| `/network` | `ResourcePages` | Services, Ingresses, EndpointSlices. |
| `/config` | `ResourcePages` | ConfigMaps e Secrets metadata-only. |
| `/namespaces` | `NamespaceScopeEditor` | Gerenciamento de escopos. |
| `/permissions` | `PermissionsMatrix` | Matriz RBAC. |
| `/settings` | `SettingsPage` | Preferências. |

### 2.2 Componentes principais

- `App`: shell, sidebar, topbar, command center, context selector.
- `CommandCenter`: paleta de navegação (`Ctrl/Cmd+K`).
- `ContextSelector`: seleção de profile/contexto.
- `ResourceListControls`: busca, sort, filtros ativos.
- `SavedFilterControls`: salvar/aplicar filtros.
- `ResourceActions`: ações em workloads/pods.
- `ResourceLiveUpdates`: SSE para atualizações.
- `StatePanel`: estados vazio/loading/erro/offline.

## 3. Pontos fortes

- **Tema escuro coeso:** paleta Catppuccin aplicada consistentemente no fundo e nos estados principais.
- **Densidade adequada:** informações técnicas são exibidas de forma compacta.
- **Command Center:** navegação por teclado bem implementada.
- **Acessibilidade parcial:** skip link, roles, aria-labels, aria-live em pontos importantes.
- **Estados de erro:** várias telas tratam loading/vazio/offline/proibido.

## 4. Problemas identificados

### 4.1 Design system inexistente

**Evidência:**
- `web/src/styles.css` é um único arquivo de 471 linhas com todo o CSS da aplicação.
- Tailwind CSS 4 é importado (`@import "tailwindcss";`) mas **nenhuma classe utilitária** é usada nos componentes.
- Espaçamentos são literais variados: `7px`, `8px`, `9px`, `10px`, `11px`, `12px`, `13px`, `14px`, `16px`, `18px`, `20px`, etc.
- Cores de borda/fundo além dos tokens base aparecem como literais (`#201a30`, `#251d35`, `#261e3a`, `#413556`, `#514363`, `#65404c`).

**Impacto:** inconsistência visual, dificuldade de manutenção, regressões visuais frequentes.

**Prioridade:** P1. **Complexidade:** L.

**Recomendação:** criar tokens de espaçamento, raio, sombra, tipografia e cores semânticas; decidir entre Tailwind ou CSS Modules; extrair componentes atômicos.

---

### 4.2 Componentes monolíticos

**Evidência:**
- `ResourcePages.tsx`: 638 linhas, 5 páginas + helpers + views de detalhe.
- `Dashboard.tsx`: 531 linhas, lógica de dados + tabelas + scan de logs.
- `ResourceActions.tsx`: 480 linhas, ações de workload + pod + terminal WebSocket.
- `LogsPage.tsx`: 367 linhas, múltiplos estados + SSE + catálogo + visualização.

**Impacto:** baixa testabilidade, alto risco de regressão, dificuldade de revisão.

**Prioridade:** P1. **Complexidade:** L.

**Recomendação:** dividir em componentes menores por domínio e extrair subcomponentes (`LogControls`, `LogOutput`, `PodActions`, `WorkloadActions`, etc.).

---

### 4.3 Identidade visual subaproveitada

**Evidência:**
- `src/kubepeep-logo.svg` e `src/kubepeep-name.svg` existem mas **não são usados** no frontend.
- A sidebar exibe apenas um quadrado com as letras `"kp"` e o wordmark textual `"kubePeep"`.
- Marca não aparece em tela de carregamento nem em empty states.

**Impacto:** identidade fraca, descolamento dos assets oficiais.

**Prioridade:** P1. **Complexidade:** XS.

**Recomendação:**
- Criar componente `BrandLogo` e `BrandWordmark` reutilizáveis.
- Substituir `"kp"` pelo SVG do logo.
- Usar o SVG do nome no cabeçalho.
- Garantir proporção, alinhamento e legibilidade.

**Critério de aceite:**
- SVGs oficiais aparecem em sidebar, cabeçalho e tela inicial.
- Não há distorção ou fundo indesejado.

---

### 4.4 Inconsistência de tabelas

**Evidência:**
- `.dashboard-table`, `.resource-table`, `.permissions-table` têm estilos similares mas divergem em padding, fonte, bordas e captions.
- Cada página implementa suas próprias colunas e células.

**Impacto:** aparência amadora, dificuldade de manter padrões.

**Prioridade:** P1. **Complexidade:** M.

**Recomendação:** criar componente `DataTable` com variantes (`compact`, `default`) e sistema de colunas reutilizável.

---

### 4.5 Estados visuais não padronizados

**Evidência:**
- Dashboard usa `ResultBody`.
- ResourcePages usa `QueryState` e `SelectionGate`.
- LogsPage e NamespaceScopeEditor usam renderização inline (`permission-notice`, `field-error`, `field-help`).

**Impacto:** experiência inconsistente para o usuário.

**Prioridade:** P2. **Complexidade:** M.

**Recomendação:** estender `StatePanel` para cobrir todos os estados (`loading`, `empty`, `offline`, `denied`, `unknown`, `partial`, `truncated`, `canceled`, `stale`) e criar `QueryBoundary` padronizado.

---

### 4.6 Formulários e inputs não padronizados

**Evidência:**
- Labels, inputs, selects e checkboxes repetidos inline com variações sutis.
- Não existe `<FormField>`, `<CheckboxField>`, `<SelectField>`.

**Impacto:** inconsistência, repetição de lógica de validação visual.

**Prioridade:** P2. **Complexidade:** M.

**Recomendação:** criar componentes de formulário reutilizáveis com estados de erro, focus e disabled consistentes.

---

### 4.7 Responsividade frágil

**Evidência:**
- Breakpoints manuais em `760px` e `520px` com muitas regras específicas.
- Em telas pequenas a sidebar vira barra inferior com ícones apenas; a topbar fica sobrecarregada.

**Impacto:** experiência ruim em tablets e telas pequenas.

**Prioridade:** P2. **Complexidade:** M.

**Recomendação:** revisar breakpoints, adotar grid flexível, garantir que ações essenciais permaneçam acessíveis em telas pequenas.

---

### 4.8 YAML sem syntax highlighting

**Evidência:**
- YAML é exibido em `<pre>` sem highlight (`ResourcePages.tsx`).

**Impacto:** dificuldade de leitura de YAML denso.

**Prioridade:** P1. **Complexidade:** S.

**Recomendação:** integrar `react-syntax-highlighter` ou Shiki para YAML, garantindo que Secret nunca passe pelo highlight (pois não possui rota YAML).

---

### 4.9 Terminal exec rudimentar

**Evidência:**
- Usa `<pre>` simples com spans coloridos.
- Sem resize automático, scrollback configurável, temas ou clipboard integrado.

**Impacto:** experiência inferior à de referências como Aptakube/k9s.

**Prioridade:** P2. **Complexidade:** L.

**Recomendação:** avaliar `xterm.js` para terminal exec, preservando segurança (não persistir comandos/saída).

---

### 4.10 Navegação e contexto

**Evidência:**
- Detalhe de recurso ocupa tela inteira; volta para lista perde scroll/filtros.
- Não há painel lateral persistente.

**Impacto:** perda de contexto, navegação lenta.

**Prioridade:** P1. **Complexidade:** L.

**Recomendação:** adicionar painel lateral de detalhes que mantém a lista visível e preserva filtros/scroll.

---

## 5. Comparação com Aptakube (referência)

| Capacidade Aptakube | Status KubePeep | Prioridade |
| --- | --- | --- |
| Navegação em árvore/painel lateral persistente | Não existe | P1 |
| Busca global de recursos | Apenas busca de páginas | P1 |
| Painel de detalhes lateral | Detail em tela cheia | P1 |
| YAML com syntax highlight | `<pre>` sem highlight | P1 |
| Terminal exec profissional | `<pre>` simples | P2 |
| Ações rápidas em massa | Apenas individuais | P3 |
| Timeline de eventos em detalhes | Eventos em página separada | P2 |
| Comparador de YAML/revisões | Não existe | P3 |
| Filtros contextuais avançados | Básico | P2 |
| Integração de marca/logo | SVGs não usados | P1 |
| Temas e personalização | Escuro único | P3 |
| Atalhos de teclado globais | Bons | — |

## 6. Recomendações por prioridade

### P1 — Consistência e identidade
1. Criar design system com tokens centralizados.
2. Integrar SVGs oficiais da marca.
3. Criar componentes atômicos (Button, Input, Select, Table, Card, Badge, Modal, Tabs, EmptyState).
4. Quebrar componentes monolíticos.
5. Criar `DataTable` reutilizável.
6. Adicionar painel lateral de detalhes.
7. Adicionar syntax highlighting de YAML.

### P2 — UX avançada
8. Padronizar `QueryBoundary` e estados visuais.
9. Criar componentes de formulário padronizados.
10. Melhorar responsividade.
11. Adicionar terminal exec com `xterm.js`.
12. Melhorar logs com pause/highlight/filtro por nível.
13. Criar gerenciador de port-forwards.

### P3 — Refinamentos
14. Temas claro/escuro.
15. Favoritos/recentes.
16. Empty states ilustrados.
17. Animações e transições refinadas.
