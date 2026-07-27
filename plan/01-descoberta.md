# Fase 1 — Descoberta

**Estado inicial:** pendente

**Dependências:** nenhuma
**Gate seguinte:** nenhuma estrutura de produção deve ser gerada antes da conclusão desta fase.

## Objetivo

Substituir hipóteses por fatos sobre o DWYT, o Ginger v1.4.4 e as dependências técnicas do Kube Peep. A fase deve encerrar com uma estratégia comprovada para o projeto híbrido CLI + serviço HTTP e com os riscos de segurança e distribuição registrados.

## Entregáveis

- `docs/research/dwyt.md`
- `docs/research/ginger-v1.4.4.md`
- `docs/research/compatibility-matrix.md`
- `docs/decisions/0001-cli-service-bootstrap.md`
- `docs/decisions/0002-health-and-degraded-state.md`
- `docs/decisions/0003-streaming-protocols.md`
- `docs/decisions/0004-local-runtime-and-process-lifecycle.md`
- registro do commit do DWYT e da tag `v1.4.4` do Ginger analisados;
- inventário “reutilizar / adaptar / não copiar”;
- spike descartável ou isolado que prove a composição Cobra + Ginger.

## Tarefas ordenadas

### Baseline das referências

- [ ] **F1-01** Registrar versões do Go, sistema operacional e arquiteturas de desenvolvimento/CI.
- [ ] **F1-02** Fixar o commit do DWYT usado na análise, confirmar sua licença e registrar que regras de negócio não serão copiadas.
- [ ] **F1-03** Fixar a tag do Ginger em `v1.4.4` para código, documentação e CLI de geração.
- [ ] **F1-04** Instalar a CLI com `go install github.com/fvmoraes/ginger/cmd/ginger@v1.4.4` e comprovar a versão com `ginger version`.

### Inventário do DWYT

- [ ] **F1-05** Mapear a estrutura real do backend, frontend, comandos e composição do binário.
- [ ] **F1-06** Identificar a implementação de `go:embed`, fallback da SPA, escolha de porta, abertura do navegador, PID/estado e shutdown.
- [ ] **F1-07** Inventariar React, TypeScript, Vite, Tailwind, Router, package manager, testes e organização por features.
- [ ] **F1-08** Inventariar tokens Catppuccin, tipografia, densidade, navegação, cards e estados visuais que podem inspirar o Kube Peep.
- [ ] **F1-09** Auditar GoReleaser, GitHub Actions, `install.sh`, `install.ps1`, checksum, update e remoção.
- [ ] **F1-10** Classificar cada achado como `reutilizar`, `adaptar`, `substituir` ou `fora de escopo`.

### Inventário do Ginger v1.4.4

- [ ] **F1-11** Ler o código público e os exemplos dos pacotes `app`, `router`, `config`, `logger`, `errors`, `response`, `health`, `sse`, `ws` e `testhelper`.
- [ ] **F1-12** Confirmar lifecycle, sinais, configuração de porta, middlewares, envelopes JSON e extensão de health checks.
- [ ] **F1-13** Gerar projetos descartáveis `--service` e `--cli` em diretório temporário e comparar entrypoints, Cobra, Makefile e GoReleaser.
- [ ] **F1-14** Executar `ginger inspect --json` e `ginger doctor` nos dois scaffolds e guardar os resultados no relatório.
- [ ] **F1-15** Executar `ginger add sse --plan` e `ginger add websocket --plan`; revisar o diff planejado sem aplicar ao repositório principal.
- [ ] **F1-16** Confirmar quais comandos aceitam `--plan`; qualquer comando sem preview deve ser executado apenas em área descartável.

### Spikes de arquitetura

- [ ] **F1-17** Provar que um comando Cobra `start` consegue construir, iniciar e encerrar a aplicação Ginger sem duplicar tratamento de sinais.
- [ ] **F1-18** Provar que o comando raiz pode equivaler a `start` e receber flags de contexto, kubeconfig, namespace, navegador e porta.
- [ ] **F1-19** Avaliar alternativas seguras para `stop` e `status`, incluindo PID obsoleto, corrida de duas inicializações e diferenças do Windows.
- [ ] **F1-20** Provar serving do frontend embutido e fallback de rotas React sem interferir em `/api/v1` e `/health`.
- [ ] **F1-21** Validar cancelamento de SSE por `request.Context()` e capacidade do WebSocket do Ginger para o fluxo bidirecional de `exec`.
- [ ] **F1-22** Definir como `/health` separará falhas locais críticas de cluster/kubeconfig indisponíveis sem derrubar toda a aplicação.

### Compatibilidade e riscos

- [ ] **F1-23** Montar matriz de compatibilidade entre Go 1.25, Ginger v1.4.4, Cobra, client-go, APIs Kubernetes e `modernc.org/sqlite`.
- [ ] **F1-24** Validar cross-build sem CGO para Linux, macOS e Windows; incluir arm64 na análise.
- [ ] **F1-25** Confirmar suporte do client-go a kubeconfig com `exec`, certificados, tokens referenciados e múltiplos arquivos em `KUBECONFIG`.
- [ ] **F1-26** Registrar riscos de RBAC, sanitização, logs, watches, streams, port-forward, exec, processos e paths multiplataforma.
- [ ] **F1-27** Abrir rascunhos dos ADRs, registrando hipóteses, alternativas e resultados ainda pendentes dos spikes seguintes.
- [ ] **F1-28** Executar SSE por mais de 15 segundos e validar como neutralizar com segurança o `WriteTimeout` fixo de `pkg/app`.
- [ ] **F1-29** Testar `pkg/ws` com Origin inválida, masking, opcode, fragmentação, ping/pong, heartbeat, resize, payload grande, desconexão e duração superior ao timeout do servidor.
- [ ] **F1-30** Validar a corrida entre descoberta e bind de porta e definir retry sem abrir o navegador para uma instância que falhou.
- [ ] **F1-31** Provar uma cadeia `HandleRaw` que preserve `http.Flusher`/`http.Hijacker` e reaplique request ID, segurança, recuperação e logging compatíveis com streaming.
- [ ] **F1-32** Definir como o logger Ginger será integrado ao arquivo local, rotação e sanitização por conteúdo sem perder logs do middleware padrão.
- [ ] **F1-33** Prototipar cursor composto por contexto/consulta/namespace/kind, com validação de retomada e expiração, sem depender de `response.Paginated`.
- [ ] **F1-34** Registrar limitações observadas de `ginger inspect` e `ginger doctor` para que seus resultados não sejam interpretados como cobertura funcional.
- [ ] **F1-35** Confirmar que SQLite sem CGO será integrado manualmente; não aplicar `ginger add sqlite`, que seleciona um driver com CGO.
- [ ] **F1-36** Fixar a nomenclatura canônica: repositório/módulo `github.com/fvmoraes/kubepeep`, produto `Kube Peep` e comando/artefato `kubePeep`, testando implicações de case em cada plataforma.
- [ ] **F1-37** Decidir e provar se `start` executa em foreground, relança um daemon ou suporta ambos; alinhar essa decisão a `stop`, `status`, logs e sinais.
- [ ] **F1-38** Documentar que plugins `exec` já exigidos pelo kubeconfig são uma dependência externa do usuário, não uma dependência instalada pelo produto.
- [ ] **F1-39** Finalizar os ADRs depois dos spikes, incluindo alternativas rejeitadas, consequências e evidências reproduzíveis.
- [ ] **F1-40** Provar embedding conjunto de frontend e migrations e execução das migrations a partir do binário compilado.
- [ ] **F1-41** Criar matriz “usar / complementar / justificar” para cada pacote Ginger obrigatório, incluindo `health`, `sse`, `ws` e `testhelper`.
- [ ] **F1-42** Comparar `app.Run()` com lifecycle HTTP próprio usando os componentes Ginger e escolher uma estratégia que adquira o listener por bind real, exponha prontidão e permita cleanup determinístico.
- [ ] **F1-43** Testar shutdown normal, timeout de shutdown e erro de servidor com SSE/WS/hijack ativos, garantindo que hooks e cleanup rodem mesmo quando conexões não encerram.
- [ ] **F1-44** Provar `stop` seguro separadamente em Unix e Windows, sem tratar PID file como identidade suficiente do processo.

## Evidências e validações

- Os scaffolds temporários compilam e seus health checks respondem.
- O spike Cobra + Ginger inicia, recebe sinal e encerra sem goroutine ou arquivo de runtime órfão.
- O listener é adquirido por bind real, a porta publicada corresponde à porta efetivamente aberta e o navegador só abre após prontidão.
- Falha/timeout de shutdown ainda executa cleanup de banco, sessões e arquivos de runtime.
- `ginger inspect` e `ginger doctor` foram executados na versão correta.
- O relatório do DWYT aponta caminhos e commits, não apenas observações visuais.
- A matriz de compatibilidade registra resultado real de compilação para cada plataforma testável.
- Todo gerador aplicado foi precedido por `--plan`, quando suportado.

## Riscos específicos

| Risco | Mitigação nesta fase |
| --- | --- |
| Escolher o template errado do Ginger | Comparar `--service` e `--cli` antes da decisão |
| `app.Run()` conflitar com sinais do Cobra | Spike de lifecycle e um único proprietário do shutdown |
| Health do Ginger retornar 503 por cluster offline | ADR para distinguir saúde local de dependência externa |
| `app.Run()` encerrar SSE/WS pelo timeout fixo | Testar stream longo e documentar ajuste de deadline por request |
| Middleware padrão remover `Flusher`/`Hijacker` | `HandleRaw`, cadeia própria e testes de streaming real |
| `pkg/ws` não validar Origin nem cobrir todo o protocolo de terminal | Guard local obrigatório; bloquear exec até o spike aprovar um transporte seguro |
| Logger não aceitar writer e redigir apenas por chave | ADR de sink local e sanitização anterior à chamada de log |
| Código atual do DWYT divergir do prompt | Fixar commit e documentar divergências |
| Cross-build falhar tarde | Compilação mínima com SQLite/client-go ainda na descoberta |

## Fora de escopo

- Scaffold definitivo do Kube Peep.
- Implementação de endpoints Kubernetes.
- Criação das telas finais.
- Cópia de regras de negócio do DWYT.

## Critério de saída

A fase termina somente quando os relatórios e ADRs permitem responder, com evidência:

1. qual scaffold Ginger será a base;
2. como Cobra controlará o serviço Ginger;
3. como saúde local e cluster degradado serão representados;
4. como SSE e WebSocket serão divididos;
5. como o frontend e migrations entrarão no binário;
6. quais padrões do DWYT serão reaproveitados;
7. quais dependências e plataformas são compatíveis.

Não é permitido fechar o gate escolhendo `app.Run()` sem resolver, por teste, bind dinâmico, prontidão, streams longos, timeout de shutdown, hooks e parada no Windows.
