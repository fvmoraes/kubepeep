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
| `ResourceSplitView` / `ResourceWorkspacePanel` | lista em toda a área útil e detalhes sobrepostos com 70% da altura da janela em todas as famílias; rolagens internas, histórico local, fechamento com Esc e links diretos |
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

`ResourceSplitView` é compartilhado por todas as rotas. A lista ocupa **100% da área útil**. Um recurso ou agregado de logs abre sobre a lista, ancorado embaixo, com **70dvh** (70% da altura da janela), sem redimensionar o conteúdo principal e sem alça de ajuste. Em janelas de até 600px de altura, o padding interno é menor para preservar o terminal. A lista pode rolar e paginar independentemente. O painel inferior, seu cabeçalho e suas abas permanecem fixos: apenas o conteúdo da aba ativa tem rolagem. Em Logs, o terminal rola e os controles permanecem visíveis; seletores extensos de alvos ficam recolhidos. Em YAML, o documento/editor rola dentro do espaço disponível, e a busca desloca somente o documento. Data / Env e demais abas rolam dentro do próprio conteúdo. Fechar o detalhe remove a sobreposição e mantém toda a altura útil da lista, inclusive após abrir novamente ou mudar de família. Esse contrato também vale em telas estreitas e baixas, sem acrescentar conteúdo abaixo da janela.

Transições de cor e borda duram 120ms; abertura de painéis e navegação dura até 160ms. Apenas opacidade e deslocamento curto são animados nas entradas, sem animar dimensões de tabelas. A preferência de movimento reduzido desativa animações e rolagem suave. Em dispositivos de toque, itens da navegação têm pelo menos 44px e opções de coluna 40px, preservando os controles compactos da tabela.

`LoadingState` reserva espaço enquanto aguarda o primeiro resultado; a indicação visual aparece após 160ms e não atrasa a entrega dos dados. Não há shimmer ou pulso contínuo. O status é acessível (`role="status"`, `aria-live="polite"`); atualizações automáticas mantêm as linhas existentes. Erros e ausência de permissão continuam explícitos. O carregamento de valores sensíveis e ações de escrita mantém sua interação explícita.

## Navegação e leitura operacional (2026-10)

O menu horizontal `ResourceFamilyNav` e a sidebar usam a mesma árvore e resolvem a seleção pela rota, inclusive em detalhes. Acima da tabela há somente busca textual, aplicada após 250 ms ou Enter. Ordenação ascendente/descendente, seleção de valores e ocultação ficam nos cabeçalhos; o seletor de colunas permanece acessível à direita, inclusive durante a rolagem. Menus escapam do contêiner da tabela por portal e suportam teclado. A paleta e a escala geral permanecem; somente células de dados usam 14px (1px a menos), sem quebra de linha. Textos longos têm limite de largura e tooltip.

O editor `ResourceYamlEditor` é compartilhado pelas abas YAML das coleções nativas, incluindo Secrets e objetos sem namespace. O fluxo é **Load authorized YAML → Edit YAML → Save {Kind}**, respeitando `get` e `update` independentes. Documento e rascunho vivem apenas no estado local da aba, fora dos caches de React Query. Validação e conflitos preservam o rascunho; a UI orienta copiar as alterações antes de cancelar e carregar a versão mais recente. Os botões permanecem visíveis enquanto somente o textarea rola. As visões dinâmicas oferecem leitura explícita de YAML, sem edição nesta versão.

A busca global (`⌘K` no macOS, `Ctrl+K` nos demais sistemas) reúne todas as páginas habilitadas e os identificadores dos recursos carregados no contexto ativo, sem resultados de histórico recente. Cada destino aparece uma única vez; favoritos priorizam apenas destinos presentes nesse catálogo. O índice continua limitado aos dados autorizados já carregados: abrir a busca não descobre recursos no cluster nem acessa conteúdo de Secrets. O botão e a ajuda exibem o modificador correspondente ao sistema operacional.

A ordenação e os filtros de coluna atuam nas páginas carregadas, preservando os limites de paginação. Pods de Job/CronJob ficam depois dos demais em qualquer direção. A coluna Tipo permite mostrar/ocultar essas categorias. Selecionar todas marca apenas as linhas que passaram pelos filtros; mudar os filtros limpa a seleção para evitar ações sobre linhas ocultas.

Inventários visíveis revalidam automaticamente a cada 10 s; SSE inicia ao abrir a tela e agrupa deltas em 10 s. O controle global liga/desliga essas atualizações, com início sempre ligado. Erros transitórios de LIST, incluindo `503/AUTHORIZATION_UNAVAILABLE` e autenticação expirada, continuam tentando nesse intervalo. Enquanto a autorização estiver indeterminada, as linhas permanecem ocultas até uma leitura autorizada ter sucesso. Negação explícita 403 e geração antiga suspendem a revalidação. As cinco últimas consultas de inventário permanecem em memória e as inativas revalidam sequencialmente em background, sem disputar várias requisições simultâneas com a tela atual. Cada consulta retém até cinco páginas; consultas antigas são descartadas. Ocultar a aplicação ou desligar Auto suspende esse trabalho. Valores de Secret e documentos YAML/Helm não entram nesse cache.

Todas as famílias de recursos usam o mesmo painel de detalhes sobreposto à parte inferior da lista. A URL identifica o objeto inspecionado, enquanto a tabela permanece montada atrás do detalhe. Navegar entre objetos relacionados muda para a lista do objeto de destino, seleciona seu namespace dentro do scope autorizado, aplica filtro de nome exato e abre o detalhe. Voltar/avançar no painel restaura a lista correspondente a cada objeto. Fechar preserva a lista e o filtro do objeto atual; “Clear object filter” remove esse filtro. Links diretos abrem o detalhe sobre a lista da família; navegar pelo menu fecha o painel anterior. Componentes de rota que abrem logs agregados usam `ResourceDetailPortal` para ocupar o mesmo espaço inferior.

Inventários de configuração, ServiceAccounts, Leases, armazenamento, RBAC e administração usam a mesma paginação incremental de Pods: até 100 itens por requisição, cinco páginas retidas e prefetch perto do fim da rolagem. Namespace e nome são colunas independentes; idade ordena pelo valor numérico. Visibilidade de colunas usa os identificadores de coleção aceitos em `/api/v1/preferences`, também para configuração, RBAC, administração e Nodes. Port Forwarding usa a mesma tabela compacta e busca textual; a matriz de permissões usa a mesma busca e controles de coluna. O inventário principal tem cabeçalho fixo e preenche a altura restante abaixo dos filtros, com rolagem interna e linhas virtualizadas. Tabelas auxiliares podem manter um limite de altura. A ordem das colunas é ajustada no seletor de colunas com os botões de mover para a esquerda/direita e persistida por coleção.

O filtro global de namespace inicia no `defaultNamespace` do scope e restaura esse padrão ao trocar contexto/scope ou reabrir a aplicação. `All` é uma escolha explícita e mantém a prioridade de carregamento do namespace padrão antes dos demais autorizados. Scopes antigos sem default usam `default`, quando presente no scope, ou o primeiro namespace disponível; as listas aguardam essa resolução sem fazer uma consulta inicial em `All`.

A aba Logs de Pod segue somente o alvo aberto; selecionar vários Pods e usar Aggregate logs abre o agregado abaixo da mesma tabela. Workloads também mostram logs no painel, reutilizando até 100 referências de Pods autorizadas no detalhe e permitindo selecionar até cinco alvos, sem nova descoberta. O leitor carrega sob demanda e encerra os streams ao mudar alvo, aba, contexto ou fechar o painel. O dashboard abre inspeção/logs no mesmo painel e revalida seus blocos automaticamente a cada 10 s, incluindo métricas.

Nesses painéis, logs abrem ao vivo e reconectam automaticamente. **Follow** acompanha a última linha, inclusive ao redimensionar; rolar para cima suspende esse acompanhamento até ativá-lo novamente ou voltar ao fim. **Pause** congela a visualização, mantendo captura e conexão; **Resume** mostra as linhas mais recentes. O seletor **Download** oferece Session logs (captura desta sessão), Visible logs (linhas filtradas), All logs (log atual retido no cluster) e Previous logs (instância anterior do container). A exportação completa é independente do buffer de visualização e mantém os alvos selecionados. Downloads longos podem ser cancelados; erro e truncamento têm feedback explícito. Limites e semântica estão no [contrato de logs](../reference/api.md#downloads-no-painel-de-logs).

Falhas parciais do dashboard mostram um resumo por código em **Collection issues**. Os detalhes ficam recolhidos inicialmente, sem duplicar mensagens iguais, e abrem pelo teclado ou mouse em uma lista com altura limitada e rolagem. Os indicadores e dados permanecem acessíveis mesmo quando muitos namespaces falham.

CPU/memória mostram uso atual / orçamento configurado e percentual. HPA de utilização usa requests; sem alvo compatível usa limit ou request e alerta a 80%. Amarelo inicia no alvo (limitado a 90%); acima de 90% é vermelho. Ausência de amostra/orçamento não significa zero.

### Atualização e identificação do contexto

O controle `Auto · 10s` no cabeçalho liga/desliga a atualização automática de todas as listas e métricas. O intervalo é fixo em 10 segundos e inicia ligado em cada sessão. Desligar também encerra streams de atualização do inventário; logs mantêm seu controle próprio. Falhas explícitas de autorização continuam interrompendo novas tentativas automáticas.

Na tabela de Pods, `Containers` mostra a quantidade total e estados individuais (incluindo init e ephemeral): verde para running, amarelo para starting, vermelho para falha e cinza para concluído/inativo. `Starting` substitui `problem` durante inicialização e a tolerância inicial de readiness; CrashLoopBackOff, falhas de imagem, exit não zero e falta de agendamento continuam problemas.

A cor de contexto é opcional, persistida pelo par profile/contexto e aplicada sutilmente às divisórias vertical da navegação e horizontal do cabeçalho, além do ícone no seletor. Presets azul (desenvolvimento), amarelo (staging) e vermelho (produção), cor personalizada e remoção não alteram o tema nem as cores semânticas de status.

`Add kubeconfig` oferece seleção de arquivo/texto (mesclado em `~/.kube/config`) ou registro de caminho local mantendo os arquivos relativos no local original. Conflitos preservam o arquivo anterior e o texto do formulário para correção. Após importar, o usuário seleciona explicitamente o contexto a ativar.

### Visões personalizadas e primeiro uso das colunas

Workloads inclui **Custom resources** e um botão **+**. O diálogo descobre APIs nativas e CRDs do contexto ativo, pesquisa por nome, kind, short name ou grupo e permite fixar até 32 visões por perfil/contexto/cluster. Remover ou reordenar uma visão altera somente preferências locais. Recursos indisponíveis e falhas parciais de descoberta aparecem com diagnóstico e atualização explícita do catálogo.

As tabelas reutilizam paginação, namespace padrão, Auto de 10 s e cache dos cinco inventários recentes. As colunas adicionais vêm da representação Table do Kubernetes (incluindo `additionalPrinterColumns` dos CRDs), sem exigir leitura separada da definição do CRD. O detalhe usa o painel inferior de 70% e fornece propriedades do printer e YAML somente leitura, carregado explicitamente; ações específicas continuam nas telas nativas.

Sem preferência salva, a sequência comum é namespace, nome, status, ready/progresso, restarts, CPU, memória, tipo e idade. Pods usam essas nove colunas em telas largas; outras famílias usam os campos equivalentes disponíveis e um conjunto reduzido de atributos relevantes. Colunas adicionais ficam no seletor. O primeiro uso se adapta à largura útil: até cinco colunas abaixo de 920 px e três abaixo de 600 px. Preferências explícitas não são reduzidas automaticamente e podem usar rolagem horizontal.

Arrastar o cabeçalho ou usar os botões esquerda/direita do seletor muda a ordem. Visibilidade e ordem são persistidas no SQLite, sem expiração. Cada tipo de workload pode ter sua organização própria; preferências antigas compartilhadas são herdadas até a primeira personalização. Uma lista salva vazia de colunas ocultas significa mostrar todas, e é distinta da ausência de preferência. A migração 0004 mantém as preferências existentes e aceita cores, ordem de colunas e visões customizadas.
