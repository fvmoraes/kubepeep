# Design System do KubePeep

## 1. Objetivo

Definir um design system leve, consistente e escalável para o KubePeep, eliminando inconsistências visuais e reduzindo a duplicação de estilos. O design system deve preservar a identidade minimalista do produto e aproximá-lo da fluidez de referências como Aptakube sem copiá-las.

## 2. Decisão estratégica: Tailwind ou CSS Modules?

**Diagnóstico atual:**
- Tailwind CSS 4 já está no projeto (`web/package.json`) e importado em `styles.css`.
- Nenhuma classe utilitária Tailwind é usada nos componentes.
- Todo CSS está em `styles.css` com seletores globais.

**Recomendação:** adotar **Tailwind CSS 4** como base, pois já é dependência do projeto, e migrar gradualmente os estilos customizados para:
- Tokens no `@theme` do Tailwind 4.
- Componentes reutilizáveis que encapsulam padrões.
- Classes utilitárias para layout e espaçamento.

Estilos muito específicos de componentes podem permanecer em CSS Modules ou em arquivos colocados junto aos componentes.

## 3. Tokens de design

### 3.1 Cores

Manter a paleta Catppuccin como base, mas adicionar tokens semânticos:

```css
:root {
  /* Brand */
  --color-brand-primary: #cba6f7;
  --color-brand-secondary: #89dceb;

  /* Semânticas */
  --color-success: #a6e3a1;
  --color-warning: #f9e2af;
  --color-error: #f38ba8;
  --color-info: #89dceb;
  --color-accent: #cba6f7;

  /* Superfícies */
  --color-bg-base: #16121f;
  --color-bg-mantle: #120e1a;
  --color-bg-crust: #0e0a16;
  --color-surface-0: #2d2540;
  --color-surface-1: #201a30;
  --color-surface-2: #251d35;

  /* Texto */
  --color-text-primary: #cdd6f4;
  --color-text-secondary: #bac2de;
  --color-text-muted: #7f849c;

  /* Bordas */
  --color-border-default: #413556;
  --color-border-strong: #514363;
  --color-border-error: #65404c;
}
```

### 3.2 Espaçamento

Adotar escala de 4px:

```css
--space-1: 4px;
--space-2: 8px;
--space-3: 12px;
--space-4: 16px;
--space-5: 20px;
--space-6: 24px;
--space-8: 32px;
--space-10: 40px;
--space-12: 48px;
```

### 3.3 Raios de borda

```css
--radius-sm: 6px;
--radius-md: 8px;
--radius-lg: 10px;
--radius-xl: 12px;
--radius-full: 999px;
```

### 3.4 Sombras

```css
--shadow-card: 0 14px 40px rgba(0, 0, 0, 0.16);
--shadow-modal: 0 28px 80px rgba(0, 0, 0, 0.52);
--shadow-dropdown: 0 8px 24px rgba(0, 0, 0, 0.24);
```

### 3.5 Tipografia

```css
--font-sans: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
--font-mono: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;

--text-xs: 11px;
--text-sm: 12px;
--text-md: 13px;
--text-lg: 14px;
--text-xl: 16px;
--text-2xl: 18px;
--text-3xl: 22px;

--font-regular: 400;
--font-medium: 500;
--font-semibold: 600;
--font-bold: 700;
```

**Recomendação:** usar fonte sans-serif para corpo da interface e mono apenas para dados técnicos (nomes de recursos, logs, YAML).

## 4. Componentes atômicos

Criar em `web/src/components/ui/`:

| Componente | Variantes/Props |
| --- | --- |
| `Button` | `variant: primary \| secondary \| danger \| ghost \| link`, `size: sm \| md \| compact`, `loading`, `disabled` |
| `IconButton` | Botão com ícone apenas, com tooltip acessível. |
| `Input` | `size`, `error`, `disabled`, ícone prefix/suffix. |
| `Select` | Nativo customizado com estados. |
| `TextArea` | Para YAML e logs. |
| `Checkbox` | Com label integrada. |
| `Switch` | Toggle. |
| `Badge` | `variant: default \| success \| warning \| error \| info \| accent`. |
| `Card` | Com header, body, footer opcionais. |
| `Table` | `DataTable` com colunas configuráveis, ordenação, seleção. |
| `Modal` | Com foco trap, overlay, ações. |
| `Drawer` | Painel lateral deslizante. |
| `Tabs` | Abas com estado controlado. |
| `EmptyState` | Ícone, título, descrição, ação. |
| `Skeleton` | Placeholder de carregamento. |
| `Spinner` | Indicador de progresso. |
| `Tooltip` | Acessível por teclado. |
| `Toast` | Notificações não bloqueantes. |

## 5. Padrões de componentes

### 5.1 Tabelas

- Cabeçalho fixo ao rolar verticalmente.
- Colunas com padding padronizado (`12px 16px`).
- Hover sutil nas linhas.
- Bordas entre linhas com `--color-border-default`.
- Células de status com `Badge`.
- Ações em linha com `IconButton` + tooltip.
- Paginação ou cursor no rodapé.

### 5.2 Formulários

- Labels acima dos inputs, fonte `--text-sm`, cor `--color-text-secondary`.
- Inputs com borda `--color-border-default`, focus ring `--color-brand-primary`.
- Mensagens de erro abaixo do campo com cor `--color-error`.
- Espaçamento entre campos `--space-4`.

### 5.3 Filtros

- Barra de filtros com `Input`, `Select`, `Button` compactos.
- Filtros ativos como chips removíveis.
- Botão "Apply filters" e "Clear" sempre visíveis.
- Ordenação como `Select` com direção toggle.

### 5.4 Painéis laterais

- Largura padrão: 520px (ajustável em telas pequenas).
- Header com título, subtítulo e botão de fechar.
- Body com scroll interno.
- Footer com ações quando aplicável.
- Overlay escuro com click-to-close.

### 5.5 Modais

- Largura máxima: 480px.
- Foco no primeiro elemento focável.
- Ações alinhadas à direita: secundária (cancelar) + primária (confirmar).
- Destrutivas usam `Button` danger.

### 5.6 Cabeçalhos

- Título da página: `--text-2xl`, semibold.
- Breadcrumbs quando houver navegação aninhada.
- Ações principais no canto superior direito.

### 5.7 Feedback ao usuário

- Loading: `Skeleton` para cards/tabelas; `Spinner` para ações.
- Erro: `EmptyState` com ícone, mensagem e ação de retry.
- Vazio: `EmptyState` sem ação quando apropriado.
- Sucesso: `Toast` discreto para ações concluídas.

## 6. Responsividade

| Breakpoint | Comportamento |
| --- | --- |
| `< 520px` | Sidebar colapsada em barra inferior; tabelas rolam horizontalmente; modais full-screen. |
| `520px–760px` | Sidebar ícones + rótulos; painel lateral ocupa 100% da largura. |
| `760px–1200px` | Sidebar expandida; painel lateral 520px. |
| `> 1200px` | Layout completo; múltiplas colunas no dashboard. |

## 7. Acessibilidade

- Contraste mínimo WCAG AA para texto e controles.
- Foco visível em todos os elementos interativos.
- Ordem de tabulação lógica.
- Modais com foco trap e `aria-modal`.
- Tabelas com cabeçalhos semânticos (`<th scope="col">`).
- Ícones com `aria-hidden` e texto alternativo quando necessário.
- `prefers-reduced-motion` respeitado.

## 8. Identidade visual

### 8.1 Uso dos SVGs

- Criar componentes `BrandLogo` e `BrandWordmark`.
- Usar `kubepeep-logo.svg` como ícone da aplicação (favicon, sidebar, loading).
- Usar `kubepeep-name.svg` como wordmark no cabeçalho/sidebar.
- Garantir proporção e alinhamento preciso entre símbolo e nome.

### 8.2 Grafia

- Produto: **Kube Peep**.
- Executável/comando: `kubepeep`.
- Não usar texto comum quando houver SVG correspondente.

## 9. Roteiro de adoção

### Fase 1 — Tokens e componentes atômicos
- Definir tokens em `styles.css` usando `@theme` do Tailwind 4.
- Criar componentes `Button`, `Input`, `Select`, `Badge`, `Card`, `EmptyState`, `Skeleton`.
- Substituir ocorrências mais simples nos componentes existentes.

### Fase 2 — Tabelas e formulários
- Criar `DataTable` e `FormField`.
- Migrar `ResourcePages`, `NamespaceScopeEditor`, `SettingsPage`.

### Fase 3 — Marca e layout
- Integrar SVGs oficiais.
- Revisar sidebar, topbar, responsividade.

### Fase 4 — Telas avançadas
- Painel lateral de detalhes.
- YAML com syntax highlight.
- Terminal exec com `xterm.js`.

## 10. Critérios de aceite

- [ ] Todos os tokens definidos e documentados.
- [ ] Componentes atômicos criados e testados.
- [ ] Nenhuma cor, espaçamento ou raio hardcoded fora dos tokens (salvo exceções justificadas).
- [ ] Tabelas consistentes em todas as telas.
- [ ] SVGs da marca integrados sem distorção.
- [ ] Responsividade testada nos breakpoints definidos.
- [ ] Acessibilidade validada com axe-core ou equivalente.
