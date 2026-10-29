# Baseline de carregamento e plano de otimização

Levantamento de 29/10/2026 sobre o tempo até a primeira linha útil nas telas
default (Pods, Workloads, Network, Configuration, Storage, Dashboard) e nas
telas custom (Custom Resources / CRDs / visões salvas). Branch de trabalho:
`perf/screen-load-time`, a partir de `afdad9a` (0.12.0).

## Método e limitações

Medido nesta data, nesta máquina:

- `npx vite build` em `web/`, saída real em `internal/web/dist` (diretório
  ignorado pelo Git). Tamanhos brutos por `stat`; tamanhos gzip conforme
  relatório do Vite.
- Grafo eager extraído do `index.html` gerado: a tag `script type="module"` e
  as entradas `modulepreload`.
- Custo de re-render do `DataTable` real, 100 linhas e 13 colunas de Pods, 60
  re-renders em jsdom via Vitest, comparando array de colunas literal contra
  memoizado. Harness temporário, não versionado conforme a política de
  `research/`; números em C1. jsdom não tem layout real, então custos de
  medição do virtualizador ficam fora.
- `npm test`: 54 arquivos, 325 testes, verde antes de qualquer alteração.

Lido no código, não medido em execução: cadeia de bootstrap, gates de
renderização, políticas de cache, exclusões de topic e caminho dinâmico. Cada
item abaixo cita arquivo e linha.

**Não executado:** nenhuma medição contra cluster real, nenhum trace de
navegador, nenhuma execução Wails nativa, nenhum profile de render. Os números
de bundle são de build local e valem para transferência e parse, não para
tempo total até a linha. As afirmações sobre latência de rede e de API server
são inferência a partir do código, não observação.

**Lacuna que precede o trabalho:** `web/src/observability/uxMetrics.ts:179-201`
já instrumenta `time_to_first_row` e `time_to_shell_ready`, e
`ClientLoadDiagnostics.tsx` já os expõe, mas não existe nenhuma meta nem
nenhuma linha de base registrada no repositório. Sem isso, nenhum item deste
plano pode ser provado. Coletar a baseline é a tarefa zero.

## Baseline de bundle (medido)

Carga eager, pedida antes de qualquer pintura útil:

| Artefato | Bruto | gzip (relatado pelo Vite) |
| --- | --- | --- |
| `index-DUqVtkYy.js` (entry) | 375,89 kB | 117,12 kB |
| `useBaseQuery-*.js` | 23,64 kB | 7,81 kB |
| `client-*.js` | 25,04 kB | 7,21 kB |
| `jsx-runtime-*.js` | 8,55 kB | 3,26 kB |
| `QueryClientProvider-*.js` + `gateway-*.js` | 0,73 kB | 0,49 kB |
| `index-DQ6TisRw.css` | 57,48 kB | 11,85 kB |
| **Total eager** | **491,35 kB** | **~148 kB** |

Fontes: 16 arquivos emitidos, 244.716 bytes no total (`woff2` e `woff` por
subset). O navegador baixa apenas os `woff2`, mas só os descobre depois de
baixar e parsear o CSS de entrada.

Maiores chunks lazy: `browser-*.js` 412,29 kB (CodeMirror, usado pelo editor
de YAML), `ExecTerminal-*.js` 334,68 kB (xterm), `YamlViewer-*.js` 53,62 kB
(PrismLight), `ResourceWorkspace-*.js` 52,28 kB, `ResourcePages-*.js`
47,14 kB. Nenhum deles está no grafo eager.

## Já otimizado (não mexer sem motivo)

Code splitting por rota completo, 22 `lazy()` em `web/src/App.tsx:39-60`, com
Suspense independente para o shell e para a seção. `sourcemap: false`. Minify
esbuild. CSS code splitting. `retry: false` global, sem tempestade de retry em
bootstrap falho. `refetchOnWindowFocus: false` nos caminhos quentes.
`['local-status']` deduplicado entre cinco assinantes em uma requisição.
Prefetch pós-pintura do chunk `ResourcePages` (`App.tsx:238`). Virtualização
com `@tanstack/react-virtual`. `Cache-Control: public, max-age=31536000,
immutable` nos assets com hash e `no-store` no `index.html`
(`internal/web/handler.go:122-133`). `color-scheme: dark` e `theme-color`
evitando flash branco. PartialObjectMetadata e Protobuf ativos no backend,
com ganho já medido em [protocolo e tuning](../reference/protocol-tuning.md)
(p95 50,5% menor, 51,1% menos bytes para metadata-only).

## Achados, por impacto esperado

### A. Boot e primeira pintura (afeta toda tela)

**A1. Nenhuma compressão HTTP no servidor de assets.**
`internal/web/handler.go:32` é um `http.FileServer` puro: não inspeciona
`Accept-Encoding`, não define `Content-Encoding`, e o build não emite `.br`
nem `.gz`. Os 491 kB eager vão crus pela rede. Em loopback desktop o custo
dominante é parse, não transferência; no modo navegador remoto isso é o maior
custo de fio do produto. Caminho: pré-comprimir no build e fazer o handler
preferir o irmão `.br`/`.gz` preservando a política immutable. Restrição: a
CSP `style-src 'self'` (`handler.go:36`, aplicada em `:115`) proíbe CSS
crítico inline sem hash ou nonce.

**A2. Oito folhas de fonte no CSS de entrada, sem preload.**
`web/src/styles.css:2-9` faz `@import` de quatro subsets Roboto e quatro
Roboto Mono. Elas entram no CSS bloqueante e os `@font-face` só são
descobertos depois do parse: dois saltos até o texto final. `web/index.html`
não tem nenhum `preload`/`preconnect`. Caminho: preload dos dois `woff2`
primários, subsets `latin-ext` e pesos 700 fora da entrada.

**A3. Entry chunk de 375,89 kB carrega o shell inteiro.**
`web/src/App.tsx:1-36` importa estaticamente Sidebar, MobileNavigation,
ContextSelector, CommandCenter, GlobalNamespaceSelect, NamespaceScopeSelector,
ResourceFamilyNav, ResourceSplitView, a árvore de navegação e os bindings
Wails. O CommandCenter (paleta de comandos) não participa da primeira
pintura. Não existe `manualChunks` nem nenhum `rollupOptions` em
`web/vite.config.ts:6-11`, e não há ferramenta de análise de bundle. Caminho:
medir primeiro (adicionar análise), depois extrair o que não pinta.

**A4. `#root` vazio.** `web/index.html` não tem esqueleto; nada aparece até o
entry executar.

### B. Cascata de bootstrap (afeta toda tela de recurso)

**B1. A sequência é estritamente serial.** `local-status` → resolução de scope
→ universo de namespaces → `import()` do chunk da seção → primeiro LIST. O
chunk só é pedido depois que `globalNamespace.ready` libera o `Outlet`
(`App.tsx:391-397`), apesar de não ter dependência de dado nenhuma. Caminho
imediato: disparar o `import()` da seção em paralelo com os gates.

**B2. Ativar o scope default custa quatro saltos extras.**
`web/src/components/DefaultScopeGate.tsx:20-44` encadeia por `enabled`:
`namespace-scopes` → `session` → POST `select` → invalidação que **refaz**
`local-status`. Tudo atrás de um painel de loading, com as queries de recurso
desmontadas (comentário explícito em `:85-87`). Caminho: devolver o scope
default no próprio `/api/v1/status` e colapsar a ativação.

**B3. O lookup do scope default é um laço de paginação.**
`web/src/api/client.ts:386-398` pagina em `limit: 100` até achar `isDefault`,
sequencialmente. Caminho: filtro `?default=true` na rota, ou incluir no
envelope de status.

### C. Telas default

**C1. `DataTable` recomputa tudo a cada render do pai — medido, e menor do que
parecia.** `web/src/components/ui/DataTable.tsx:104-120` tem três `useMemo`
dependentes de `allColumns`: `values` (linhas × todas as colunas, inclusive
ocultas), `filterValues` (únicos + `Intl.Collator` por coluna) e `rows`
(filtro + ordenação). Os arrays de coluna são literais criados no corpo dos
componentes de página (`ResourcePages.tsx:519` e equivalentes para workloads,
events, network, config), então a identidade de `allColumns` muda em todo
render e os três memos invalidam. `renderRow` ainda chama `columnValue` de
novo só para montar o `title` (`:183`), fora de qualquer memo.

Medido em 29/10/2026 com o componente real, 100 linhas e 13 colunas de Pods,
60 re-renders em jsdom: **5,54 ms por re-render com array literal contra
3,41 ms com array memoizado, diferença de 2,13 ms, ou 38% do custo do
re-render.** Só a aritmética dos três memos, isolada em Node sem React, custa
0,180 ms; o resto é reconciliação e invocação de `cell()`.

Duas correções a uma versão anterior deste documento, que superestimava o
achado: o fallback `textContent(column.cell(...))` em `columnValue`
(`DataTable.tsx:292-299`) atinge **4 de 13 colunas** de Pods (`ready`, `node`,
`owner`, `ip`), não todas — as demais têm `value()` ou resolvem por campo
escalar; e dessas quatro só `owner` constrói JSX de verdade.

Consequência para o escopo deste plano: 2,13 ms por re-render é ganho de
**interação e estado estável**, não de carregamento. O primeiro render não é
beneficiado, porque não há nada memoizado para reaproveitar. jsdom também não
tem layout real, então o custo de re-medição do virtualizador causado por
`getRowKey` instável (`DataTable.tsx:128`, com comentário explícito no código:
"A new key extractor invalidates TanStack's measurements of every loaded row")
**não** entra nessa medição e permanece não quantificado. Este item sai da
frente da fila: não serve ao objetivo de tempo de carregamento.

**C2. Nenhuma tela aquece a própria primeira página.**
`internal/api/handlers/resources.go:958-970` rejeita `likely-next` sem cursor,
por design, então prefetch especulativo nunca cobre a primeira página. Só o
Dashboard aquece Pods (`App.tsx:258-267`, 20 linhas, um namespace).

**C3. O cache de página exige cobertura de watch.**
`internal/integration/kubernetesruntime/resources_backend.go:378-387` só serve
o cache quando `manager.Covers(...)`, e o WATCH nasce do endpoint de stream,
não do LIST. Primeira navegação sempre paga LIST real.

**C4. `overscan: 2`** (`DataTable.tsx:133`) é apertado para scroll rápido.
Afeta fluidez, não primeira linha.

### D. Telas custom (maior diferença)

**D1. Todo LIST custom frio paga uma varredura completa de discovery antes.**
`internal/integration/kubernetesruntime/dynamic_views.go:25` chama
`resolveDynamicResource`, que em `resource_discovery.go:212` chama
`DiscoverResources`: `GET /api` + `GET /apis` + um `GET /apis/{gv}` por
group-version, até 512, com 4 workers. O cache é **um único slot**
(`resource_discovery.go:19-24`), TTL de 5 min caindo para 10 s se algum grupo
falhar (`:161-168`), não persistido: perdido a cada restart e a cada troca de
contexto. Mesmo quando o alvo já é conhecido, a varredura inteira precede o
LIST. Caminho: resolver o alvo pelo próprio `/apis/{group}/{version}` (uma
requisição) e só cair na varredura completa se falhar; cache por GVR em vez de
slot único; aquecer discovery em background depois de shell-ready.

**D2. Um erro de printer pode custar dois LIST upstream na mesma página.**
`dynamic_views.go:36-61`: tenta Table, e em `IsNotAcceptable` ou
`ErrTableUnsupported` repete via metadata client, perdendo todas as colunas.

**D3. Custom está fora de quatro otimizações das telas default.**
`topicForCollection` (`resources_backend.go:321-342`) e `collectionCacheTopic`
(`internal/services/resources/collection_cache.go:314-333`) têm o mesmo switch
de sete topics com `default: return "", false`, e `Topic` é enum fechado com
mapa estático de GVR (`internal/services/resources/watch.go:41-55`). Logo,
coleções `dynamic:*` não têm cache de página, não têm fast path de snapshot de
watch, não têm WATCH/live updates e não têm stream preview — este último
porque `DynamicResourcePage.tsx` não chama `useResourceStreamPreview`, que as
páginas built-in chamam em `ResourcePages.tsx:323,457,697,813,955`. Resultado
visível: a tabela custom fica vazia até o HTTP responder, enquanto as default
já mostram linhas do stream. Caminho: a cobertura de topic dinâmico é mudança
estrutural; o stream preview e o cache de página são separáveis e mais baratos.

**D4. Custo por linha mais alto.** `dynamic_views.go:102-130` faz um
`json.Unmarshal` por linha e duplica até 24 structs de coluna em cada linha. No
cliente, `DynamicResourcePage.tsx:115-121` re-hasha a chave da coluna (FNV-1a,
`web/src/navigation/dynamic.ts:19-24`) dentro de um `findIndex`, para cada
célula, e de novo no `value()` das colunas numéricas.

**D5. `/custom-resources` sem parâmetro tem um salto serial a mais**, porque
`selected = views[0]` depende de `['preferences']`
(`DynamicResourcePage.tsx:42-44`). Deep link não paga isso.

**D6. O que custom não perde:** coalescing, scheduler de admissão, cursores,
`GlobalGrantFastPath` (`list.go:106` não exclui CRD), metadata-only, página de
100 linhas, 30 s/120 s de freshness e prefetch de próxima página — tudo pelo
hook compartilhado `useInfiniteCollection`.

## Ordem de trabalho proposta

Cada item é verificável isoladamente. Nada aqui está implementado.

Ordem revisada em 29/10/2026 depois de medir C1 e constatar que ele não serve
ao objetivo de carregamento.

| # | Item | Escopo | Por quê nesta posição |
| --- | --- | --- | --- |
| 0 | Baseline medida de `time_to_first_row` e `time_to_shell_ready`, default e custom, em cluster real | Medição | Nenhum ganho de carregamento é demonstrável sem isso |
| 1 | Resolver alvo dinâmico por GVR antes da varredura completa; cache de discovery por GVR | D1 | Maior custo isolado de carregamento das telas custom |
| 2 | Pré-compressão de assets no build + negociação no handler | A1 | 491 kB crus medidos; alto ganho no modo navegador, preserva política immutable |
| 3 | Preload dos `woff2` primários; subsets secundários fora da entrada | A2 | Barato, encurta o caminho crítico do texto em duas etapas para uma |
| 4 | Disparar `import()` da seção em paralelo com os gates | B1 | Remove serialização que não tem dependência de dado |
| 5 | Scope default no envelope de `/api/v1/status` | B2, B3 | Colapsa quatro saltos em um; muda contrato de API |
| 6 | Análise de bundle e extração do que não pinta do entry chunk | A3 | Precisa de medição de composição antes de decidir o corte |
| 7 | Stream preview para coleções dinâmicas | D3 | Percepção imediata; menor que cobertura de topic |
| 8 | Memoizar arrays de coluna e estabilizar `getRowKey` nas páginas | C1, C4 | 2,13 ms por re-render medidos: ganho de interação, não de carregamento |

Itens 5 e 7 alteram contrato ou estrutura e exigem decisão registrada antes da
implementação. Os itens 1 a 4 são locais e reversíveis. O item 8 só se
justifica junto a uma medição de responsividade de interação em navegador
real, que ainda não existe; a parcela de `getRowKey` permanece não
quantificada.

## Restrições herdadas

Compressão do cliente Kubernetes permanece desativada por default e QPS/Burst
em 10/20, conforme [protocolo e tuning](../reference/protocol-tuning.md);
nenhum item acima propõe mudar isso. Secrets seguem exclusivamente em metadata.
A matriz responsiva de aceite (1366x768, 1440x900, 1920x1080, 2560x1440)
continua valendo para qualquer mudança de layout de tabela.
