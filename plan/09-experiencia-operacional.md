# Fase 9 — Experiência operacional

**Estado atual:** planejada (0/84)

**Benchmark:** [facilitadores oficiais do Aptakube](../docs/research/aptakube-ux-benchmark.md)

**Matriz:** [critérios de aceite UX](matriz-aceite-ux.md)

**Dependências:** contratos e guardrails das Fases 2, 4, 5, 6 e 7; pipeline da Fase 8

**Gate:** todos os critérios `UX-M01` a `UX-M15`, testes, inspeções negativas e CI do commit final precisam possuir evidência executada.

## Objetivo

Reduzir atrito nas tarefas diárias de diagnóstico e operação com uma interface
mais rápida, navegável e contextual, inspirada em facilitadores documentados
pelo Aptakube, mas implementada com identidade própria e sem ampliar
privilégios, retenção ou exposição de dados.

Princípios invariáveis:

> A interface acelera uma capacidade; nunca cria uma capacidade.

> Origem, autorização, retenção e consequência permanecem explícitas.

## Entregáveis

- paleta de navegação e ajuda de atalhos acessíveis;
- filtros, reset, ordenação natural, colunas allowlisted e favoritos;
- menus de ações rápidas baseados em capability e reautorização backend;
- status de conexão/retry/stale por origem;
- YAML somente leitura com busca/recolhimento e política metadata-only;
- leitor de logs aprimorado, limitado e não persistente;
- gerenciador de sessões de port-forward;
- agregação multi-contexto somente leitura com proveniência e falha isolada;
- diff somente leitura com normalização explícita e recusa de Secret;
- documentação, threat model, testes, evidências e matriz de aceite atualizados.

## Fora do gate desta fase

- copiar marca, código, textos, screenshots ou identidade visual do Aptakube;
- mutação em massa entre clusters;
- exibir ou expandir valores/referências de Secret;
- editar/criar/aplicar YAML genericamente;
- persistir logs, YAML, objetos Kubernetes ou resultados de diff;
- alterar kubeconfig, credenciais ou configuração de plugins `exec`;
- instalar componente obrigatório no cluster;
- oferecer telemetria externa habilitada por padrão.

Edição YAML poderá ser reavaliada em fase própria somente com dry-run
server-side, preview de diff, `resourceVersion`, RBAC, confirmação e recusa
absoluta de Secret. Ela não é condição para concluir a Fase 9.

## Tarefas ordenadas

### Baseline, requisitos e não-infringimento

- [ ] **F9-01** Registrar URLs oficiais, data da consulta e resumo factual de cada facilitador usado.
- [ ] **F9-02** Documentar que o benchmark não autoriza cópia de marca, código, textos, screenshots, ícones, layout ou identidade proprietária.
- [ ] **F9-03** Inventariar no frontend e backend quais facilitadores já existem, quais são parciais e quais precisam ser criados.
- [ ] **F9-04** Ligar cada item do inventário a componente, rota, service, adapter, teste e critério `UX-M` correspondente.
- [ ] **F9-05** Atualizar produto, arquitetura, API, dados e segurança antes de implementar qualquer contrato novo.
- [ ] **F9-06** Classificar cada campo persistido, renderizado, copiado, baixado ou diagnosticado.
- [ ] **F9-07** Revisar licenças de qualquer dependência adicional e preferir componentes próprios já usados pelo projeto.
- [ ] **F9-08** Criar flags de entrega apenas quando houver rollback seguro; nenhuma flag pode enfraquecer autorização ou redaction.

### Paleta, atalhos e navegação

- [ ] **F9-09** Criar um catálogo tipado de destinos a partir das rotas reais, sem strings duplicadas espalhadas pela UI.
- [ ] **F9-10** Implementar paleta somente de navegação aberta por `Ctrl+K`/`Cmd+K` e botão acessível.
- [ ] **F9-11** Buscar por tela, tipo de recurso, contexto, escopo e namespace sem indexar objetos remotos em storage persistente.
- [ ] **F9-12** Preservar contexto/escopo/filtros pertinentes ao navegar e rejeitar destino incompatível com a geração atual.
- [ ] **F9-13** Conter foco na paleta, restaurá-lo ao fechar, suportar setas/Enter/Escape e anunciar resultados ao leitor de tela.
- [ ] **F9-14** Não oferecer mutações na primeira versão da paleta; ações continuam no alvo contextual e no backend reautorizado.
- [ ] **F9-15** Criar ajuda de atalhos descoberta por teclado e pela interface, com conflitos de browser documentados.
- [ ] **F9-16** Adicionar atalhos seguros para atualizar, focar busca, abrir seletor e voltar, sem capturar campos de edição.
- [ ] **F9-17** Testar Windows/Linux (`Ctrl`) e macOS (`Meta`), composição de teclado, foco e navegação somente por teclado.
- [ ] **F9-18** Garantir deep links/reload com History API para todos os destinos do catálogo.

### Listas, filtros, colunas, favoritos e recentes

- [ ] **F9-19** Centralizar busca/filtro/ordenação em estado serializável e validado, sem incluir corpo de recurso.
- [ ] **F9-20** Oferecer botão explícito de limpar filtros e mostrar quais filtros estão ativos.
- [ ] **F9-21** Implementar ordenação natural estável, com tie-breaker determinístico e origem explícita em agregações.
- [ ] **F9-22** Preservar filtros por tela/escopo somente em preferências allowlisted e limitadas.
- [ ] **F9-23** Implementar filtros positivos, negativos e por múltiplos termos com parser determinístico e mensagens acessíveis.
- [ ] **F9-24** Permitir visibilidade/ordem de colunas somente de um catálogo seguro por tipo de recurso.
- [ ] **F9-25** Proibir `Secret.data`, `Secret.stringData`, valores de annotations/labels não aprovados e campos arbitrários como coluna persistida.
- [ ] **F9-26** Adicionar favoritos para destinos e referências mínimas de recursos, sem endpoint, kubeconfig path, corpo, YAML ou log.
- [ ] **F9-27** Adicionar recentes com limite, expiração e ação de limpar; não registrar recursos proibidos ou Secrets.
- [ ] **F9-28** Remover favoritos/recentes que deixem de pertencer ao profile/contexto visível sem revelar a causa do sumiço.
- [ ] **F9-29** Testar alta cardinalidade, ordenação, paginação, reset, migração de preferências e storage sem dados remotos.

### Visão humana e ações rápidas

- [ ] **F9-30** Definir colunas e resumo humano por kind suportado, sempre derivados de DTOs allowlisted.
- [ ] **F9-31** Mostrar status com texto, ícone e cor; cor nunca é o único canal semântico.
- [ ] **F9-32** Ligar condições/problem summaries ao objeto ou evento real que as sustenta; não inventar diagnóstico.
- [ ] **F9-33** Preservar relações navegáveis entre workload, ReplicaSet, Pod, Service, evento e logs quando o recurso relacionado for acessível.
- [ ] **F9-34** Criar menu contextual de ações rápidas em linha e detalhe usando o mesmo catálogo tipado.
- [ ] **F9-35** Exibir logs, YAML, restart, scale, delete, port-forward e exec somente quando pertinentes ao kind/alvo.
- [ ] **F9-36** Representar capability `allowed`, `denied` e `unknown`; tooltip/texto explica indisponibilidade sem expor detalhes internos.
- [ ] **F9-37** Repetir SAR e preconditions no backend para toda ação, ignorando qualquer estado antigo da UI.
- [ ] **F9-38** Exibir contexto, namespace, tipo, nome, consequência e confirmação proporcional antes de mutação destrutiva.
- [ ] **F9-39** Reconciliar a lista/detalhe após sucesso, conflito, 403, cancelamento ou alvo alterado sem anunciar sucesso prematuro.
- [ ] **F9-40** Detectar scale controlado por autoscaler quando possível e alertar/bloquear conforme contrato aprovado, sem inferir autorização.

### YAML somente leitura e diff

- [ ] **F9-41** Criar visualizador YAML somente leitura com syntax highlighting local, busca e seções recolhíveis.
- [ ] **F9-42** Manter conteúdo YAML apenas em memória, sem local/session storage, IndexedDB, service worker, SQLite, log ou telemetria.
- [ ] **F9-43** Recusar rota, visualização, cópia e download de YAML de Secret; detalhe de Secret permanece metadata-only.
- [ ] **F9-44** Limitar tamanho/tempo de renderização e oferecer estado truncado honesto para objetos grandes.
- [ ] **F9-45** Fazer cópia/download somente por gesto explícito, preservando `no-store` e sem criar arquivo interno.
- [ ] **F9-46** Definir contrato de diff somente leitura entre dois objetos acessíveis e com origens exibidas lado a lado.
- [ ] **F9-47** Oferecer normalização opt-in de campos gerenciados pelo servidor, mantendo opção de comparação integral.
- [ ] **F9-48** Proibir diff quando qualquer lado for Secret ou quando a leitura de um lado não estiver autorizada.
- [ ] **F9-49** Testar ausência versus vazio, tipos/versões diferentes, objetos grandes, 403, origem offline e marcador sensível.

### Logs e métricas

- [ ] **F9-50** Evoluir o leitor de logs com seleção explícita de pod/container, atuais/anteriores, timestamps, wrap, pause e follow.
- [ ] **F9-51** Permitir busca, filtro e destaque somente no buffer em memória, com próximo/anterior e estatística limitada.
- [ ] **F9-52** Identificar a fonte em cada linha ao combinar pods/containers; nunca apresentar uma sequência sem proveniência.
- [ ] **F9-53** Aplicar budgets de fontes, linhas, bytes, duração e concorrência antes de abrir streams agregados.
- [ ] **F9-54** Implementar backpressure, descarte/truncamento explícito e cancelamento ao sair, trocar escopo ou mudar geração.
- [ ] **F9-55** Exigir `get pods/log` por namespace/pod e isolar 403/falha de cada fonte sem elevar permissão das demais.
- [ ] **F9-56** Manter exportação explícita e efêmera; não persistir log nem incluir conteúdo em erro/diagnóstico.
- [ ] **F9-57** Melhorar métricas em listas/detalhes com CPU/memória, requests/limits e unidades consistentes quando autorizadas.
- [ ] **F9-58** Tratar Metrics API ausente, proibida ou parcial como indisponibilidade local do bloco, nunca falha global.
- [ ] **F9-59** Testar memória limitada, cliente lento, follow cancelado, container anterior, múltiplas origens e sentinela sensível.

### Port-forward e sessões

- [ ] **F9-60** Criar painel de sessões ativas com contexto, namespace, alvo, portas, estado e instante de início sanitizados.
- [ ] **F9-61** Iniciar port-forward a partir de ações rápidas com sugestão de porta e bind loopback obrigatório.
- [ ] **F9-62** Tratar porta ocupada sem escolher exposição externa ou encerrar processo alheio.
- [ ] **F9-63** Permitir parar uma sessão e parar todas após confirmação, respeitando dono e geração.
- [ ] **F9-64** Encerrar sessões ao shutdown e marcar claramente sessões encerradas por troca de contexto/escopo.
- [ ] **F9-65** Testar limite, colisão, cancelamento, reconexão, cleanup e ausência de processo/goroutine órfã.

### Multi-contexto somente leitura e resiliência

- [ ] **F9-66** Especificar seleção de um conjunto de contextos para leitura sem alterar a seleção mutável principal.
- [ ] **F9-67** Definir envelope de origem com profile, contexto, cluster, namespace, geração e erro parcial sanitizado.
- [ ] **F9-68** Fazer fan-out limitado/cancelável e merge determinístico sem cache persistente de objetos.
- [ ] **F9-69** Exibir origem em toda linha, card, log, detalhe, diff e erro resultante de agregação.
- [ ] **F9-70** Avaliar RBAC separadamente por contexto/namespace/recurso; nunca unir capabilities.
- [ ] **F9-71** Restringir o modo multi-contexto a leitura; toda mutação exige selecionar um único alvo/origem e repetir autorização.
- [ ] **F9-72** Isolar offline, timeout, autenticação, 403 e resultado truncado por contexto sem apagar dados válidos dos demais.
- [ ] **F9-73** Cancelar requests/watches anteriores na mudança do conjunto e rejeitar resposta de geração obsoleta.
- [ ] **F9-74** Implementar retry com backoff/jitter/limite e estados conectado, reconectando, parcial, offline e stale por origem.
- [ ] **F9-75** Testar dois clusters, um offline, permissões divergentes, resposta atrasada e ausência de vazamento entre origens.

### Hardening, acessibilidade e evidência

- [ ] **F9-76** Executar threat-model delta cobrindo paleta, preferências, logs agregados, diff e multi-contexto.
- [ ] **F9-77** Validar teclado, foco, leitor de tela, contraste, zoom e estados que não dependem apenas de cor.
- [ ] **F9-78** Validar loading, vazio, offline, proibido, unknown, parcial, cancelado, stale e truncado em cada superfície nova.
- [ ] **F9-79** Inspecionar localStorage, sessionStorage, IndexedDB, Cache API/service workers, SQLite, WAL/journal, backups e logs após E2E.
- [ ] **F9-80** Injetar sentinelas sintéticas em Secret, log, YAML e erro e comprovar ausência em persistência, diagnóstico e artefatos.
- [ ] **F9-81** Executar unitários, integração, frontend, acessibilidade, Kind restritivo, E2E do binário e race/leak checks aplicáveis.
- [ ] **F9-82** Repetir build/smoke dos seis archives nativos porque a interface embutida e os contratos mudaram.
- [ ] **F9-83** Atualizar matriz UX, especificações, ADRs, evidências, README, plano e Project Brain no mesmo commit funcional.
- [ ] **F9-84** Reindexar o repositório, revisar cobertura/parse misses e registrar arquitetura/blast radius do estado final sanitizado.

## Matriz mínima de perfis

| Perfil | Leitura | Logs | Métricas | Ações | Resultado esperado |
| --- | --- | --- | --- | --- | --- |
| sem acesso | negada | negados | negadas | negadas | superfície oculta/desabilitada sem vazamento |
| leitura restrita | permitida em parte | negados | desconhecidas | negadas | dados válidos + falhas locais explícitas |
| logs | permitida | permitidos | opcionais | negadas | leitor disponível, ações mutáveis indisponíveis |
| operador restrito | permitida | conforme Role | opcionais | subconjunto permitido | menu reflete capability e backend reautoriza |
| multi-contexto divergente | diferente por origem | diferente por origem | diferente por origem | somente alvo único | nenhuma capability ou dado cruza origens |

## Critério de saída

A Fase 9 fecha somente quando os quinze critérios `UX-M` possuem evidência
executada, a suíte restritiva passa, a inspeção negativa não encontra dados
proibidos, a CI do commit final está verde e o índice do projeto representa o
estado versionado. Recursos classificados como futuros na matriz não contam
como concluídos e não podem ser sugeridos pela interface.
