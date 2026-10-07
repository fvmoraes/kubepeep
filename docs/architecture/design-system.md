# Design System do KubePeep (v2)

> **Status:** vigente. A fonte da verdade é o código: `web/src/tokens.css`, `web/src/components/ui/` e `web/src/components/resource/`.
> **Direção visual:** superfícies escuras e roxo KubePeep preservados; menus sincronizados, filtros comuns e escala tipográfica ampliada. Acesso restrito e cadastro de namespaces em lote continuam premissas do produto.

## 1. Fundamentos

- Tailwind CSS 4 com tokens em `web/src/tokens.css` (`@theme`); novos valores visuais devem usar os tokens.
- **Família da interface: Roboto**, também usada pelo [Kubernetes Dashboard](https://github.com/kubernetes/dashboard/blob/master/modules/web/src/_variables.scss). Roboto Mono (`--font-mono`, classe `.mono`) fica restrita a logs, YAML/JSON, código e terminal; nomes de recursos, menus e dados de inventário usam Roboto.
- Fontes empacotadas localmente com `@fontsource/roboto` e `@fontsource/roboto-mono`, em latim/latim estendido, apenas nos pesos **normal (400)** e **negrito (700)**. Nenhuma requisição de fonte a serviços externos.
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
| Sucesso | `kp-green #4ADE80` (texto) · `kp-green-bg/border` (ações e estados) |
| Destrutivo | `kp-red #F87171` (texto) · `kp-red-bg/border` (ações e estados) |
| Aviso | `kp-yellow #FBBF24` (texto) · `kp-yellow-bg/border` (ações e estados) · `kp-peach #FB923C` (attention) |
| Tipografia | Cinco papéis: `--text-title` 22px · `--text-heading` 18px · `--text-menu` 15px · `--text-column` 13px · `--text-content` 14px |
| Controles | `--control-height` 28px · `--control-radius` 2px · borda 1px · texto 14px; ícones 28×28px; indicadores `--badge-height` 24px |
| Layout | `--sidebar-width 240px` · `--sidebar-width-compact 56px` · `--header-height 56px` · `--content-max-width 100%` |
| Movimento | `--motion-fast` 120ms · `--motion-enter` 160ms · `--loading-delay` 160ms · `--ease-responsive` cubic-bezier(0.16, 1, 0.3, 1) |

## 3. Componentes (`web/src/components/ui/`)

| Componente | Variantes / notas |
| --- | --- |
| `Button` | `primary` (azul) · `secondary` · `success` · `danger` · `warning` · `ghost` · `icon`; altura única de 28px, fundos sutis e bordas semânticas; estados pressionado, foco e desabilitado explícitos |
| `Badge` / `StatusBadge` | `default/healthy/warning/danger/info/unknown/accent`; 24px, cantos de 2px; StatusBadge inclui dot semântico |
| `Input` / `Select` / `Checkbox` | Campos de 28px alinhados às ações, cantos de 2px, bg crust, foco borda mauve + ring |
| `Card` (+Header/Title/Content) | surface-0, borda overlay-0, rounded-xl, sem sombra pesada |
| `DataTable<T>` | células 14px sem quebra, nomes de colunas 13px em negrito, ordenação asc/desc e filtros por valores carregados no cabeçalho; visibilidade por coluna |
| `ResourceSplitView` / `ResourceWorkspacePanel` | divisão fixa 40% lista / 60% detalhes em todas as famílias; rolagens internas, histórico local, fechamento com Esc e links diretos |
| `ResourceTabStrip` | 28px, underline com accent no ativo; mesma geometria da navegação horizontal e vertical |
| `Banner` (+Error/Warning/Info/Success) | título humano + mensagem; detalhes técnicos em `<details>` |
| `PageHeader` | título 22px + descrição + ações — toda página começa por ele (ou `ResourcePage`) |
| `EmptyState` / `LoadingState` | vazio compacto; carregamento discreto compartilhado (`inline`, `block` ou `table`), com placeholders estáticos |

Botões novos usam `Button`, sem variações locais de tamanho. Controles nativos que precisam manter sua semântica (abas, cabeçalhos de tabela e navegação) usam os mesmos tokens de geometria. Resultados compostos da busca e da investigação usam `.control-row`: os mesmos cantos, com altura flexível para preservar descrições e avisos. Indicadores de estado não são ações; usam 24px para manter as linhas de dados compactas.

### Escala tipográfica única

| Papel | Classe | Tamanho | Peso / aplicação |
| --- | --- | --- | --- |
| Títulos | `text-title` | 22px | Negrito; páginas e diálogos, além dos números de destaque |
| Menus | `text-menu` | 15px | Normal; item ativo em negrito. Menu lateral, horizontal, abas e seletores de navegação |
| Cabeçalhos | `text-heading` | 18px | Negrito; seções, painéis e identificação do recurso aberto |
| Colunas | `text-column` | 13px | Negrito; nomes das colunas e rótulos de campos |
| Conteúdo | `text-content` | 14px | Normal; dados, parágrafos, ajuda, status, botões e campos. Negrito apenas para ênfase |

Todos os tamanhos usam `rem`, respeitando zoom e tamanho de fonte do navegador. A escala genérica do Tailwind e os pesos intermediários foram removidos: componentes devem usar somente esses cinco papéis e `font-normal`/`font-bold`. Código e terminal usam os mesmos 14px do conteúdo, com Roboto Mono para alinhar caracteres. O terminal lê o tamanho calculado do conteúdo e recalcula sua grade após o carregamento da fonte. Tabelas mantêm números tabulares e células em uma linha; prosa longa usa medida de até 65–75 caracteres.

## 4. Resource framework (`web/src/components/resource/`)

Páginas de recursos **não** duplicam estrutura: usam `ResourcePage` (scaffold + PageHeader), `SelectionGate`/`QueryState` (estados), `useInfiniteCollection`/`ResourceCollectionTable`/`InfiniteCollectionFooter` (paginação limitada por geração), `ResourceListControls` (somente busca textual), `TableLink`, `Facts`, `ResourceTabStrip` e os helpers `format.ts` (age/dateTime), `status.ts` (status→cor semântica), `errors.ts`.

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
4. Densidade primeiro: conteúdo 14px em uma linha, colunas 13px em negrito e menus 15px; use os cinco papéis tipográficos.

## 7. Validação

- `make lint typecheck`, `npm --prefix web test` (Vitest) e `make test-e2e` (Playwright) são os gates de validação.
- Validar a UI em 1366×768, 1440×900, 1920×1080 e 2560×1440;
  screenshots de evidência ficam **fora do Git**.
- Navegável em todas essas resoluções sem scroll horizontal global.
- Validar também 320×568 e 390×844 com toque, tablet 768×1024 e janelas baixas 844×390 e 1024×600, incluindo menu, colunas, detalhes e logs; respeitar `prefers-reduced-motion`.

## Responsividade, movimento e carregamento

Até 760px de largura, ou em janelas de até 1024×500px, a navegação abre pelo botão no topo, usando o mesmo catálogo da sidebar em um `dialog` nativo. O menu contém o foco, fecha com Escape ou seleção de página e devolve o foco ao botão ao fechar. Mudar para desktop remove o menu e libera a rolagem. Não há faixa fixa de navegação sobre o conteúdo no rodapé. Em telas maiores a sidebar ocupa de 190 a 240px, ou 56px quando recolhida.

O shell respeita as áreas seguras do dispositivo e ocupa `100dvh`, sem rolagem vertical da página. A topbar e a navegação de família ficam fora das áreas roláveis; flex/grid distribuem o espaço restante sem medir alturas em JavaScript. Menus horizontais e abas revelam o item ativo ao navegar ou redimensionar, rolando somente no eixo horizontal. Cabeçalhos do detalhe se adaptam à largura do próprio painel por container query. As tabelas preservam células em uma linha e rolagem horizontal local, sem comprimir ou reduzir a fonte.

`ResourceSplitView` é compartilhado por todas as rotas. Com um recurso ou agregado de logs aberto, a lista ocupa **40%** e os detalhes **60%** da altura útil, descontado o intervalo de 12px. Em janelas de até 600px de altura, os detalhes recebem **70%**, com padding interno menor para preservar o terminal. A lista pode rolar e paginar independentemente. O painel inferior, seu cabeçalho e suas abas permanecem fixos: apenas o conteúdo da aba ativa tem rolagem. Em Logs, o terminal rola e os controles permanecem visíveis; seletores extensos de alvos ficam recolhidos. Em YAML, o documento/editor rola dentro do espaço disponível, e a busca desloca somente o documento. Data / Env e demais abas rolam dentro do próprio conteúdo. Fechar o detalhe devolve toda a altura útil à lista. Esse contrato também vale em telas estreitas e baixas, sem acrescentar conteúdo abaixo da janela.

Transições de cor e borda duram 120ms; abertura de painéis e navegação dura até 160ms. Apenas opacidade e deslocamento curto são animados nas entradas, sem animar dimensões de tabelas. A preferência de movimento reduzido desativa animações e rolagem suave. Em dispositivos de toque, itens da navegação têm pelo menos 44px e opções de coluna 40px, preservando os controles compactos da tabela.

`LoadingState` reserva espaço enquanto aguarda o primeiro resultado; a indicação visual aparece após 160ms e não atrasa a entrega dos dados. Não há shimmer ou pulso contínuo. O status é acessível (`role="status"`, `aria-live="polite"`); atualizações automáticas mantêm as linhas existentes. Erros e ausência de permissão continuam explícitos. O carregamento de valores sensíveis e ações de escrita mantém sua interação explícita.

## Navegação e leitura operacional (2026-10)

O menu horizontal `ResourceFamilyNav` e a sidebar usam a mesma árvore e resolvem a seleção pela rota, inclusive em detalhes. Acima da tabela há somente busca textual, aplicada após 250 ms ou Enter. Ordenação ascendente/descendente, seleção de valores e ocultação ficam nos cabeçalhos; o seletor de colunas permanece acessível à direita, inclusive durante a rolagem. Menus escapam do contêiner da tabela por portal e suportam teclado. A paleta e a escala geral permanecem; somente células de dados usam 14px (1px a menos), sem quebra de linha. Textos longos têm limite de largura e tooltip.

O editor `ResourceYamlEditor` é compartilhado por todas as abas YAML, incluindo Secrets e objetos sem namespace. O fluxo é **Load authorized YAML → Edit YAML → Save {Kind}**, respeitando `get` e `update` independentes. Documento e rascunho vivem apenas no estado local da aba, fora dos caches de React Query. Validação e conflitos preservam o rascunho; a UI orienta copiar as alterações antes de cancelar e carregar a versão mais recente. Os botões permanecem visíveis enquanto somente o textarea rola.

A busca global (`⌘K` no macOS, `Ctrl+K` nos demais sistemas) reúne todas as páginas habilitadas e os identificadores dos recursos carregados no contexto ativo, sem resultados de histórico recente. Cada destino aparece uma única vez; favoritos priorizam apenas destinos presentes nesse catálogo. O índice continua limitado aos dados autorizados já carregados: abrir a busca não descobre recursos no cluster nem acessa conteúdo de Secrets. O botão e a ajuda exibem o modificador correspondente ao sistema operacional.

A ordenação e os filtros de coluna atuam nas páginas carregadas, preservando os limites de paginação. Pods de Job/CronJob ficam depois dos demais em qualquer direção. A coluna Tipo permite mostrar/ocultar essas categorias. Selecionar todas marca apenas as linhas que passaram pelos filtros; mudar os filtros limpa a seleção para evitar ações sobre linhas ocultas.

Inventários visíveis revalidam automaticamente a cada 15 s; SSE inicia ao abrir a tela e agrupa deltas em 2 s. Erros transitórios de LIST, incluindo `503/AUTHORIZATION_UNAVAILABLE`, continuam tentando nesse intervalo. Enquanto a autorização estiver indeterminada, as linhas permanecem ocultas até uma leitura autorizada ter sucesso. Respostas 401/403 ou geração antiga suspendem a revalidação. Sem WATCH, permanecem as consultas HTTP autorizadas, sem botão Refresh. Polling de listas para em background; detalhe, valores de Secret e catálogos de logs não participam dessa política.

Todas as famílias de recursos usam o mesmo painel de detalhes na divisão fixa abaixo da lista. A URL identifica o objeto inspecionado, enquanto a rota de origem mantém a tabela montada, com busca, colunas e rolagem. Navegar entre objetos relacionados preserva essa origem; fechar volta à rota inicial (incluindo query e âncora). Links diretos abrem o detalhe sobre a lista da família; navegar pelo menu fecha o painel anterior. Componentes de rota que abrem logs agregados usam `ResourceDetailPortal` para ocupar o mesmo espaço inferior.

Inventários de configuração, ServiceAccounts, Leases, armazenamento, RBAC e administração usam a mesma paginação incremental de Pods: até 100 itens por requisição, cinco páginas retidas e prefetch perto do fim da rolagem. Namespace e nome são colunas independentes; idade ordena pelo valor numérico. Visibilidade de colunas usa os identificadores de coleção aceitos em `/api/v1/preferences`, também para configuração, RBAC, administração e Nodes. Port Forwarding usa a mesma tabela compacta e busca textual; a matriz de permissões usa a mesma busca e controles de coluna. A tabela tem cabeçalho fixo e viewport proporcional à tela (`clamp(12rem, 52dvh, 44rem)`), com linhas virtualizadas.

A aba Logs de Pod segue somente o alvo aberto; selecionar vários Pods e usar Aggregate logs abre o agregado abaixo da mesma tabela. Workloads também mostram logs no painel, reutilizando até 100 referências de Pods autorizadas no detalhe e permitindo selecionar até cinco alvos, sem nova descoberta. O leitor carrega sob demanda e encerra os streams ao mudar alvo, aba, contexto ou fechar o painel. O dashboard abre inspeção/logs no mesmo painel e revalida seus blocos automaticamente a cada 15 s (métricas a cada 8 s).

Nesses painéis, logs abrem ao vivo e reconectam automaticamente. **Follow** acompanha a última linha, inclusive ao redimensionar; rolar para cima suspende esse acompanhamento até ativá-lo novamente ou voltar ao fim. **Pause** congela a visualização, mantendo captura e conexão; **Resume** mostra as linhas mais recentes. O seletor **Download** oferece Session logs (captura desta sessão), Visible logs (linhas filtradas), All logs (log atual retido no cluster) e Previous logs (instância anterior do container). A exportação completa é independente do buffer de visualização e mantém os alvos selecionados. Downloads longos podem ser cancelados; erro e truncamento têm feedback explícito. Limites e semântica estão no [contrato de logs](../reference/api.md#downloads-no-painel-de-logs).

Falhas parciais do dashboard mostram um resumo por código em **Collection issues**. Os detalhes ficam recolhidos inicialmente, sem duplicar mensagens iguais, e abrem pelo teclado ou mouse em uma lista com altura limitada e rolagem. Os indicadores e dados permanecem acessíveis mesmo quando muitos namespaces falham.

CPU/memória mostram uso atual / orçamento configurado e percentual. HPA de utilização usa requests; sem alvo compatível usa limit ou request e alerta a 80%. Amarelo inicia no alvo (limitado a 90%); acima de 90% é vermelho. Ausência de amostra/orçamento não significa zero.
