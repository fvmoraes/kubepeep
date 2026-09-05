# ADR 0005 — Design System v2, shell de navegação e resource framework

- Status: aceito
- Data: 2026-09-04
- Commit: `5ac7320` (implementação integral)
- Especificação: [`../../plan/reference/KubePeep_UI_UX_Design_System_e_Recursos_Kubernetes.md`](../../plan/reference/KubePeep_UI_UX_Design_System_e_Recursos_Kubernetes.md)

## Contexto

O frontend tinha dois sistemas de estilo convivendo (classes legadas em
`styles.css` com 473 linhas + componentes Tailwind meio adotados), fonte
monoespaçada global, roxo dominante em toda ação e 4 estilos de tabela. Cada
tela parecia um produto diferente e a navegação era uma lista plana de 10
itens, sem estrutura de recursos Kubernetes.

## Decisão

### Tokens únicos (`web/src/tokens.css`)

Superfícies neutras escuras (`#111016` base, `#18161E` cards), texto branco-
primeiro (`#F4F1F7` primário → `#686271` desabilitado), roxo de marca
(`#A78BFA`) restrito a seleção, navegação e foco, e cores semânticas para
significado: azul `#3B82F6` (ação normal), verde `#22C55E` (sucesso),
vermelho `#EF4444` (destrutivo), âmbar `#F59E0B` (aviso). `Degraded` é âmbar
(avisos acionáveis), não vermelho. Escala tipográfica 10–32px com 13px como
densidade padrão de tabela/menu; Inter Variable como família única, com
monospace reservado a logs, YAML, código e terminal.

### Componentes e framework

`web/src/components/ui/` fornece os átomos (Button com variantes semânticas,
Badge/StatusBadge, inputs unificados h-8, Card, DataTable, Drawer, Tabs,
Banner×4, PageHeader, EmptyState); `web/src/components/resource/` fornece o
esqueleto de páginas de recursos (gates de seleção/erro, paginação por
geração, filter bar, fact grid, mapeamento status→cor). Página nova de
recurso **não** cria estrutura nem CSS próprios.

### Shell e navegação

`web/src/navigation/tree.tsx` é a fonte única da árvore Kubernetes (grupos
Cluster, Workloads, Helm, Network, Configuration, Storage, Access Control,
Observability, Administration + Settings fixo). Recursos sem endpoint backend
renderizam como itens desabilitados ("available in a future release") em vez
de páginas mortas; habilitar = preencher `path` + criar a página com o
framework. Topbar de 56px com contexto auto-aplicado on-change (sem botão de
confirmação) e chip de namespace scope. A versão exibida no sidebar vem da
bridge desktop (`buildinfo` via ldflags) com fallback em `/api/v1/status` —
nenhuma versão hardcoded na UI.

### Fronteira de estado

`web/src/security.test.ts` proíbe `localStorage`/`sessionStorage` em produção;
o estado de shell (sidebar compacta, grupos colapsados) permanece em memória
até que o allowlist de `/api/v1/preferences` seja estendido (Fase 6 do plano
v1). Favoritos e filtros salvos continuam no backend, como já eram.

## Consequências

- Novas telas exigem apenas: definição de colunas, endpoint backend e filtros
  específicos do kind.
- O catálogo de navegação é contrato dos testes e2e (catalog assertion em
  `web/e2e/app.spec.ts`); mudanças na árvore atualizam os testes no mesmo
  commit.
- Recursos cluster-scoped não dependem de namespace scope — o framework
  receive o escopo por família (Fase 1 do plano v1 formaliza o contrato).
