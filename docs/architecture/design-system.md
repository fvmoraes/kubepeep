# Design System do KubePeep (v2)

> **Status:** vigente. A fonte da verdade é o código: `web/src/tokens.css`, `web/src/components/ui/` e `web/src/components/resource/`.
> **Direção visual:** superfícies escuras e roxo KubePeep preservados; menus sincronizados, filtros comuns e escala tipográfica ampliada. Acesso restrito e cadastro de namespaces em lote continuam premissas do produto.

## 1. Fundamentos

- Tailwind CSS 4 com tokens em `web/src/tokens.css` (`@theme`); novos valores visuais devem usar os tokens.
- **Uma família tipográfica**: Inter Variable (`@fontsource-variable/inter`). Monospace (`--font-mono`, classe `.mono`) **somente** em logs, YAML/JSON, código e terminal.
- Superfícies neutras escuras; o roxo (`#A78BFA`) é marca de identidade restrita a **seleção, navegação, foco e pequenos destaques** — nunca cor dominante de texto.
- Cores semânticas para significado: azul (ação normal), verde (sucesso/healthy), vermelho (destrutivo/erro), âmbar (warning/pending).

## 2. Tokens essenciais

| Grupo | Tokens |
| --- | --- |
| Superfícies | `kp-crust #0E0D13` (inputs/logs) · `kp-mantle` (sidebar) · `kp-base #111016` · `kp-surface-0 #18161E` (cards) · `kp-surface-1 #1D1A24` (elevado) · `kp-surface-3 #25212D` (hover) |
| Bordas | `kp-overlay-0 #302A3A` (padrão) · `kp-overlay-1 #3A3346` (forte) · `kp-divider #24202E` (linhas de tabela) |
| Texto | `kp-text #F4F1F7` · `kp-subtext #C8C2D0` · `kp-overlay-text #918A9E` · `kp-text-disabled #686271` |
| Marca | `kp-mauve #A78BFA` · `kp-mauve-hover #C4B5FD` · `kp-accent-bg #221E33` · `kp-accent-border #4A3D6E` |
| Ação (azul) | `kp-blue #3B82F6` · `kp-blue-hover #60A5FA` · `kp-blue-bg/border` (info) |
| Sucesso | `kp-green #4ADE80` (texto) · `kp-green-solid #22C55E` (botão) · `kp-green-bg/border` |
| Destrutivo | `kp-red #F87171` (texto) · `kp-red-solid #EF4444` (botão) · `kp-red-bg/border` |
| Aviso | `kp-yellow #FBBF24` (texto) · `kp-amber #F59E0B` (botão) · `kp-yellow-bg/border` · `kp-peach #FB923C` (attention) |
| Tipografia | `--text-2xs 12` · `xs 13` · `sm 14` · `base 15` (menus/tabelas/inputs) · `lg 16` · `xl 18` · `2xl 22` (títulos) · `3xl 28` (números) · `4xl 34` |
| Controles | `h-7` 28px · `h-8` 32px (padrão) · `h-9` 36px; radius 5/6/8/10/12 |
| Layout | `--sidebar-width 240px` · `--sidebar-width-compact 56px` · `--header-height 56px` · `--content-max-width 1400px` |

## 3. Componentes (`web/src/components/ui/`)

| Componente | Variantes / notas |
| --- | --- |
| `Button` | `primary` (azul) · `secondary` · `success` · `danger` · `warning` · `ghost` · `icon`; tamanhos `sm/md/lg` (28/32/36px) |
| `Badge` / `StatusBadge` | `default/healthy/warning/danger/info/unknown`; StatusBadge inclui dot semântico |
| `Input` / `Select` / `Checkbox` / `Field` / `SearchInput` | h-8, bg crust, foco borda mauve + ring; `Field` = label + help + error |
| `Card` (+Header/Title/Content) | surface-0, borda overlay-0, rounded-xl, sem sombra pesada |
| `DataTable<T>` | células 15px, header 12px uppercase, divisores sutis, hover, `compact` |
| `Drawer` | detalhe de recurso (dialog, Esc, backdrop) |
| `Tabs` / `ResourceTabStrip` | underline com accent no ativo |
| `Banner` (+Error/Warning/Info/Success) | título humano + mensagem; detalhes técnicos em `<details>` |
| `PageHeader` | título 22px + descrição + ações — toda página começa por ele (ou `ResourcePage`) |
| `EmptyState` / `Skeleton` | vazio compacto centrado; placeholder de carregamento |

## 4. Resource framework (`web/src/components/resource/`)

Páginas de recursos **não** duplicam estrutura: usam `ResourcePage` (scaffold + PageHeader), `SelectionGate`/`QueryState`/`CollectionFooter` (estados e paginação por geração), `ResourceListControls` (search/filtros/sort/order), `TableLink`, `Facts`, `ResourceTabStrip` e os helpers `format.ts` (age/dateTime), `status.ts` (status→cor semântica), `errors.ts`.

Adicionar um recurso Kubernetes (checklist):

1. Backend: adapter → service (DTO allowlisted) → handler/rotas → capabilities → testes.
2. Cliente TS em `web/src/api/client.ts` + tipos.
3. Página com o framework; colunas e ações específicas do kind.
4. Habilitar item em `web/src/navigation/tree.tsx` + rota em `App.tsx` + command palette/favoritos.
5. Documentar em `docs/reference/api.md` e `docs/reference/rbac-requirements.md`.

## 5. Semântica de status Kubernetes

| Estado | Cor |
| --- | --- |
| healthy, running, succeeded, active, bound, available | verde |
| pending, progressing, suspended, degraded, terminating | âmbar |
| failed, error, CrashLoopBackOff, evicted | vermelho |
| unknown | cinza |
| informativo (bloco opcional) | azul |

`Degraded` é âmbar por decisão de produto (aviso acionável), não vermelho.

## 6. Regras de adoção

1. Novos componentes/telas usam os tokens e os atômicos — CSS próprio em página é rejeitado em review.
2. Toda tabela usa `DataTable`; todo estado vazio usa `EmptyState`; todo erro de usuário usa `Banner`.
3. Nada de browser storage: `web/src/security.test.ts` bloqueia localStorage/sessionStorage; estado de shell vai para `/api/v1/preferences` (allowlist).
4. Densidade primeiro: 13px é o padrão de tabela/menu; tamanhos maiores apenas em títulos e números-herói.

## 7. Validação

- `make lint typecheck`, `npm --prefix web test` (Vitest) e `make test-e2e` (Playwright) são os gates de validação.
- Validar a UI em 1366×768, 1440×900, 1920×1080 e 2560×1440;
  screenshots de evidência ficam **fora do Git**.
- Navegável em todas essas resoluções sem scroll horizontal global.

## Navegação e leitura operacional (2026-10)

O menu horizontal `ResourceFamilyNav` e a sidebar usam a mesma árvore e resolvem a seleção pela rota, inclusive em detalhes. Filtros comuns: busca, ordenação, direção, aplicar, atualizar e limpar. Campos específicos ficam em **More filters**. A escala tipográfica subiu 2px, preservando Inter e a paleta existente; não reduzir a raiz nem texto monospace por fatores adicionais.

CPU/memória mostram uso atual / orçamento configurado e percentual. HPA de utilização usa requests; sem alvo compatível usa limit ou request e alerta a 80%. Amarelo inicia no alvo (limitado a 90%); acima de 90% é vermelho. Ausência de amostra/orçamento não significa zero.
