# Evidências da Fase 9 — Experiência operacional

**Estado:** duas fatias verticais validadas localmente; demais facilitadores
em execução.

**Baseline de requisitos:** `5f24a1a`.

## 1. Escopo desta evidência

As entregas registradas fecham o núcleo seguro da paleta de navegação e o
estado canônico/visível das listas paginadas. Elas não afirmam conclusão de
filtros negativos/multitermo, favoritos, quick actions, diff, logs agregados,
gerenciador de port-forward ou leitura multi-contexto.

Fontes funcionais do benchmark:

- [repositório oficial Aptakube](https://github.com/aptakube/aptakube);
- [command palette](https://aptakube.com/blog/aptakube-1.4);
- [keyboard/navigation](https://aptakube.com/blog/aptakube-1.5).

O Kube Peep não reutilizou código, textos, screenshots, ícones, layout ou
identidade da referência. O componente usa o catálogo, os tokens, as rotas e
os componentes próprios do projeto.

## 2. Inventário anterior à mudança

O grafo e a revisão do frontend confirmaram:

| Área | Estado anterior |
| --- | --- |
| shell/rotas | React Router e dez destinos estáticos no menu lateral |
| estado remoto | TanStack Query somente em memória, cancelado/removido por `generation` |
| dashboard | blocos independentes, estados parciais e métricas opcionais |
| recursos | listas/detalhes, filtros salvos, YAML elegível e SSE |
| logs | atuais, anteriores, follow, busca, pausa, cópia e download explícito |
| autorização | matriz tri-state e actions reautorizadas no backend |
| ações | restart, scale, delete, port-forward e exec já possuem componentes próprios |
| lacuna | não existiam paleta global, ajuda de atalhos ou caminho rápido por teclado |

O modo multi-contexto simultâneo não foi confundido com essa fatia: a
arquitetura atual mantém uma seleção/generation ativa e exige contrato próprio
antes de qualquer agregação.

## 3. Implementação

### Catálogo único

`web/src/App.tsx` mantém o catálogo canônico de navegação com:

- path estático;
- label;
- descrição própria;
- palavras-chave estáticas;
- ícone usado apenas no menu lateral.

O `CommandCenter` recebe uma projeção desse catálogo. Ele não consulta API,
cache de recursos, contexto, scope, logs, YAML, Secrets ou histórico de uso.

### Interação

- botão visível no topbar;
- `Ctrl+K` e `Cmd+K` abrem a busca;
- `?` abre a ajuda apenas fora de input, textarea, select, contenteditable ou
  elemento com papel de textbox;
- setas percorrem resultados com wrap determinístico;
- `Enter` abre o destino selecionado;
- `Escape` fecha;
- `Tab` e `Shift+Tab` permanecem contidos no diálogo;
- o foco retorna ao elemento que abriu o diálogo;
- o diálogo bloqueia scroll do body enquanto aberto e restaura o estado ao
  fechar;
- busca por múltiplos termos usa somente strings estáticas normalizadas com
  `toLowerCase()` e exige todos os termos;
- zero resultados possui status textual acessível.

### Segurança e privacidade

O componente deliberadamente não possui:

- `fetch` ou outra chamada remota;
- mutação ou link direto para action Kubernetes;
- armazenamento local, de sessão, IndexedDB, Cache API ou service worker;
- favoritos, recentes ou telemetria;
- indexação de recursos, logs, YAML, terminal, erros ou Secrets;
- interpolação de endpoint, kubeconfig path, token, certificado ou conteúdo do
  cluster em rota/query string.

As rotas continuam sob o shell existente. Navegar não muda autorização, e
nenhuma capability antiga é usada para executar operação.

## 4. Arquivos

| Arquivo | Papel |
| --- | --- |
| `web/src/components/CommandCenter.tsx` | diálogo, busca local, atalhos, foco e navegação |
| `web/src/components/CommandCenter.test.tsx` | interação, privacidade e acessibilidade por teclado |
| `web/src/App.tsx` | catálogo de dez destinos e integração no topbar |
| `web/src/App.test.tsx` | catálogo exato e busca por keyword estática |
| `web/src/styles.css` | apresentação responsiva com identidade própria |
| `web/e2e/app.spec.ts` | jornada no browser e deep link/reload |

Nenhuma dependência foi adicionada. O componente usa React, React Router e
Lucide já fixados no lockfile.

## 5. Testes executados

| Gate | Resultado observado |
| --- | --- |
| ESLint | passou |
| TypeScript | passou sem emissão |
| Vitest/Testing Library | 16 arquivos, 69 testes, todos passaram |
| build Vite | 1.903 módulos transformados; bundle de produção gerado |
| Playwright Chromium | 3 de 3 cenários passaram |
| `git diff --check` | passou |
| cobertura do grafo | seis arquivos consultados sem lacuna registrada; sinal best-effort |

Casos específicos comprovados:

- abertura por botão, Ctrl e Meta;
- ajuda por `?` recusada dentro de campo editável;
- catálogo limitado às dez páginas reais;
- busca por descrição/keyword;
- nenhum `fetch` do componente;
- localStorage e sessionStorage vazios;
- ArrowUp/ArrowDown com wrap;
- navegação por Enter;
- focus trap e retorno de foco;
- Escape;
- caminho `/workloads` recarregado diretamente pela History API.

## 6. Critérios fechados por esta fatia

- `UX-M02`: paleta/atalhos de navegação acessíveis, sem mutação e sem
  persistência remota.
- F9-01/F9-02: fontes e limite de não-infringimento documentados.
- F9-03: inventário inicial do frontend registrado.
- F9-07: nenhuma dependência adicional; baseline de licenças permanece.
- F9-09/F9-10: catálogo tipado e paleta `Ctrl/Cmd+K`.
- F9-13/F9-14/F9-15: foco/teclado, ausência de mutações e ajuda de atalhos.
- F9-76: threat-model delta versionado em `docs/security.md`.

## 7. Segunda fatia: filtros e ordenação de página

### Contrato de estado

Workloads, Pods e Events agora mantêm objetos serializáveis distintos para
`draft` e `applied`. Network e Config mantêm o mesmo contrato separadamente
por aba. Apenas `applied` alimenta:

- a chave do TanStack Query;
- a query enviada à API local;
- o resumo acessível de filtros ativos;
- o payload allowlisted de um filtro salvo.

Digitar ou escolher um filtro/ordenação muda somente `draft` e publica o
estado textual “Filter changes pending”. `Apply filters` promove a cópia
completa, fecha detalhe/YAML em memória e reinicia a paginação. `Clear filters`
restaura filtros e ordenação padrão nos dois estados. Aplicar filtro salvo faz
o mesmo por uma query clonada e sanitizada contra enums locais fechados.

Os cursores são vinculados à `generation`; quando a seleção muda, o valor da
geração anterior deixa de compor a query sem precisar persistir ou interpretar
o token no browser. Network e Config preservam rascunho, estado aplicado,
ordenação e cursor independentes por tipo de recurso.

### Ordenação e honestidade

O catálogo visual reproduz exatamente os campos aceitos pelo backend para
Workloads, Pods, Events, Services, Ingresses, EndpointSlices, ConfigMaps e
Secrets. `sort` e `order` seguem para a API; não existe ordenação local de um
resultado paginado. A interface usa “Sort this bounded page” e
“Bounded-page order”, pois `filterScope: page` não promete ordenação global de
uma coleção incompleta.

Valores de filtro salvo ausentes ou fora do catálogo retornam ao default.
`problematic: false` é preservado como boolean e não é confundido com ausência.
`limit`, `continue`, cursor, corpo de recurso, YAML e log não entram no estado
persistível.

O runtime aplica comparação natural aos campos textuais das oito coleções
atuais: sequências ASCII numéricas são comparadas pelo valor, sem converter para
inteiros e sem risco de overflow (`pod-2` precede `pod-10`). A direção altera
somente a chave primária; empates usam identidade canônica lexical ascendente.
Os comparadores que definem a identidade/cursor Kubernetes não foram alterados,
portanto a melhoria é restrita à ordenação honesta da página já coletada.

### Superfície e segurança

`ResourceListControls` centraliza:

- busca em rascunho;
- filtros ativos efetivamente aplicados;
- aviso de alterações pendentes;
- ordenação/direção do backend;
- apply, refresh e reset explícito;
- status textual que não depende somente de cor.

O catálogo de Config/Secrets contém somente identidade, nome e instante de
criação. Nenhum valor, annotation, label arbitrária, `Secret.data`,
`Secret.stringData`, YAML ou campo remoto livre pode virar opção persistida.
Não houve nova dependência nem uso de localStorage, sessionStorage, IndexedDB,
Cache API ou service worker.

### Evidência local executada

| Gate | Resultado observado |
| --- | --- |
| ESLint | passou |
| TypeScript | passou sem emissão |
| Vitest/Testing Library | 17 arquivos, 73 testes, todos passaram |
| build Vite | 1.904 módulos transformados; bundle de produção gerado |
| Playwright Chromium | 3 de 3 cenários passaram |
| storage do browser | localStorage/sessionStorage permaneceram vazios nos testes de recursos |
| Go — runtime Kubernetes | 48 testes passaram; os mesmos 48 passaram com detector de corrida |
| Go vet | passou para todos os pacotes |

Casos específicos comprovados:

- nenhuma consulta nova antes de `Apply filters`;
- `sort`/`order` allowlisted na URL e defaults omitidos de forma equivalente;
- filtros ativos mostram somente o estado aplicado;
- busca, filtro, ordenação e cursor removidos no reset;
- `continue` nunca entra em filtro salvo;
- valor salvo inválido volta ao default fechado;
- `problematic=false` permanece filtro explícito;
- Network mantém estado independente por aba;
- troca real de `generation` descarta o cursor obtido na geração anterior;
- aplicação de filtro salvo preserva ordenação;
- Secret continua metadata-only e sem ação de YAML;
- jornada Playwright cobre aplicar, ordenar, limpar, recuperar e salvar.
- nomes e demais chaves textuais usam ordem natural sem parsing inteiro;
- direção descendente preserva tie-breaker canônico ascendente;
- ConfigMaps, Secrets, Services, Ingresses, EndpointSlices, Pods, Workloads e
  Events possuem cobertura da comparação textual aplicável e do catálogo exato
  de campos de ordenação aceitos pelo backend.

Esta fatia fecha F9-19 e F9-20. A parcela de ordenação natural de F9-21 está
implementada para as coleções de contexto único, mas a tarefa permanece aberta:
a futura agregação multi-contexto ainda precisa tornar a origem explícita e
participar do desempate. `UX-M03` também permanece aberto até a conclusão e
evidência de filtros positivos/negativos/multitermo, colunas allowlisted e alta
cardinalidade.

## 8. Reindexação integral do marco atual

Depois do commit funcional `41d419c`, o Codebase Knowledge Graph foi gerado em
modo `full`, sem persistir um novo artefato no repositório. O índice ficou
`ready`, com 6.083 nodes, 32.489 edges após o commit documental, 397 File nodes, 10 linguagens e nenhum
arquivo skipped por falha.

A cobertura conferiu os 35 paths alterados contra a mesma geração: 34 não
possuíam issue registrada e `test/kind/harness.sh` manteve `parse_partial` na
linha 960. As quatro faixas parciais globais — migration SQL, instalador
PowerShell, harness e cliente TypeScript — foram lidas diretamente. Esse sinal
é best-effort e não foi apresentado como prova de completude.

O diff desde `9d7c2ea` produziu 181 símbolos-semente e blast radius inbound de
32 símbolos, completo e sem truncamento em três hops. O rollup foi:
`internal/integration` 15, `test/kind` 11, `web/src` 5 e `web/e2e` 1. A
arquitetura registrou 98 Route nodes, 54 Package nodes, 20 entry points e 12
clusters.

A primeira tentativa falhou antes da análise porque o daemon executava uma
geração antiga do binário já substituída. O reinício controlado somente do
daemon preservou vault e índice; a repetição pelo binário atual concluiu em
cerca de 16 segundos. F9-84 continua aberta porque futuras fatias ainda mudarão
o HEAD e exigirão nova evidência final.

## 9. Pendências

- ampliar catálogo com contextos/scopes somente depois de definir
  classificação e ciclo de vida em memória;
- atalhos adicionais de refresh/foco/seletor sem capturar campos editáveis;
- composição/IME, contraste, zoom e auditoria automatizada de acessibilidade;
- deep-link/reload de todos os destinos, não apenas a jornada representativa;
- inspeção E2E final de todos os storages, SQLite, logs e archives;
- CI do commit funcional e smoke nativo do frontend embutido;
- demais critérios `UX-M` da matriz.

## 10. Regra de atualização

Este relatório só recebe evidência CI depois que o workflow do commit que
contém a implementação terminar. Uma execução verde anterior não é
reutilizada. Falha posterior em frontend, segurança, Kind ou archive reabre o
gate correspondente.
