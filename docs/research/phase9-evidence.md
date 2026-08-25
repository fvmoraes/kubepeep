# Evidências da Fase 9 — Experiência operacional

**Estado:** primeira fatia vertical validada localmente; demais facilitadores em
execução.

**Baseline de requisitos:** `5f24a1a`.

## 1. Escopo desta evidência

Esta primeira entrega fecha o núcleo seguro da paleta de navegação. Ela não
afirma conclusão de filtros avançados, favoritos, quick actions, diff, logs
agregados, gerenciador de port-forward ou leitura multi-contexto.

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

## 7. Pendências

- ampliar catálogo com contextos/scopes somente depois de definir
  classificação e ciclo de vida em memória;
- atalhos adicionais de refresh/foco/seletor sem capturar campos editáveis;
- composição/IME, contraste, zoom e auditoria automatizada de acessibilidade;
- deep-link/reload de todos os destinos, não apenas a jornada representativa;
- inspeção E2E final de todos os storages, SQLite, logs e archives;
- CI do commit funcional e smoke nativo do frontend embutido;
- demais critérios `UX-M` da matriz.

## 8. Regra de atualização

Este relatório só recebe evidência CI depois que o workflow do commit que
contém a implementação terminar. Uma execução verde anterior não é
reutilizada. Falha posterior em frontend, segurança, Kind ou archive reabre o
gate correspondente.
