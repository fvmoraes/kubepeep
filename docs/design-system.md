# Design System do KubePeep

> **Status:** estabelecido na Fase 1 do plano de melhorias e em adoção incremental.

## 1. Fundamentos

- Tailwind CSS 4 com tokens definidos em `web/src/tokens.css` via `@theme`.
- Paleta base: Catppuccin Mocha adaptada (prefixo `kp-`).
- Componentes atômicos em `web/src/components/ui/`, todos tipados, acessíveis e testados.
- Interface permanece monoespaçada e minimalista; a marca usa os SVGs oficiais.

## 2. Tokens

### 2.1 Cores

| Token | Valor | Uso |
| --- | --- | --- |
| `kp-crust` | `#0e0a16` | fundo de inputs, logs, terminal |
| `kp-mantle` | `#120e1a` | sidebar |
| `kp-base` | `#16121f` | fundo da aplicação |
| `kp-surface-0` | `#1c1730` | cards, painéis |
| `kp-surface-1` | `#191427` | estados vazios, sub-superfícies |
| `kp-surface-2..4` | `#201a30`–`#261e3a` | hover, ativo |
| `kp-overlay-0..5` | `#33294a`–`#6c568a` | bordas em escalas |
| `kp-text` / `kp-subtext` | `#cdd6f4` / `#bac2de` | texto primário/secundário |
| `kp-overlay-text` | `#7f849c` | texto terciário |
| `kp-mauve` (+hover) | `#cba6f7` / `#d9b8ff` | primário, foco |
| `kp-red` (+bg/border) | `#f38ba8` | erro/destrutivo |
| `kp-peach` | `#fab387` | atenção |
| `kp-yellow` (+bg/border) | `#f9e2af` | aviso |
| `kp-green` (+border) | `#a6e3a1` | sucesso |
| `kp-sky` (+bg/border) | `#89dceb` | informação, links |

### 2.2 Espaçamento, raios e tipografia

- Espaçamento: escala 4px (`spacing-0` … `spacing-24`).
- Raios: `sm` 6px, `md` 8px, `lg` 10px, `xl` 12px, `2xl` 16px, `full`.
- Tipografia: `font-mono` padrão da aplicação; tamanhos `2xs` 9px → `4xl` responsivo.
- Sombras: `shadow-card`, `shadow-dialog`, `shadow-focus`.

## 3. Componentes atômicos (`web/src/components/ui/`)

| Componente | Variantes / props principais |
| --- | --- |
| `Button` | `primary`, `secondary`, `danger`, `ghost`; tamanho `default`/`compact` |
| `Input`, `Select` | estilo unificado, foco com `shadow-focus`, suportam `forwardRef` |
| `Badge` | `default`, `healthy`, `warning`, `danger`, `info`, `unknown` |
| `Card` (+`Header`/`Title`/`Content`) | superfície padrão com borda e sombra |
| `DataTable<T>` | colunas declarativas, `compact`, caption, footer, `onRowClick`, `getRowKey` |
| `Drawer` | painel lateral acessível (dialog, Esc, backdrop) |
| `Tabs` | tablist com estado controlado |
| `EmptyState` | título, descrição, ícone, ação |
| `Skeleton` | placeholder de carregamento |

Regras de adoção:

1. Novos componentes de UI **devem** usar os tokens; valores hardcoded só com justificativa em comentário.
2. Novas tabelas **devem** usar `DataTable`; novos botões/inputs/seleções usam os atômicos.
3. Migração de código legado é incremental; classes legadas em `styles.css` permanecem até a migração completa.

## 4. Marca

- `BrandLogo` (`web/src/assets/kubepeep-logo.svg`) e `BrandWordmark` (`kubepeep-name.svg`).
- Uso obrigatório em sidebar; proporção preservada, sem distorção.

## 5. Validação

- `npm --prefix web run lint` / `typecheck` / `test` / `build` precisam passar.
- Regressões visuais são verificadas com screenshots (`docs/research/screenshots-review/`).
