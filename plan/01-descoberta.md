# Fase 1 — Descoberta

**Estado:** concluída em 2026-07-27

**Dependências:** nenhuma
**Gate seguinte:** Fase 1 liberada; o scaffold de produção ainda aguarda o gate
documental da Fase 2.

## Objetivo

Substituir hipóteses por fatos sobre o DWYT, o Ginger v1.4.4 e as dependências técnicas do Kube Peep. A fase deve encerrar com uma estratégia comprovada para o projeto híbrido CLI + serviço HTTP e com os riscos de segurança e distribuição registrados.

## Entregáveis

- `docs/research/dwyt.md`
- `docs/research/ginger-v1.4.4.md`
- `docs/research/compatibility-matrix.md`
- `docs/research/phase1-evidence.md`
- `docs/decisions/0001-cli-service-bootstrap.md`
- `docs/decisions/0002-health-and-degraded-state.md`
- `docs/decisions/0003-streaming-protocols.md`
- `docs/decisions/0004-local-runtime-and-process-lifecycle.md`
- registro do commit do DWYT e da tag `v1.4.4` do Ginger analisados;
- inventário “reutilizar / adaptar / não copiar”;
- spike descartável ou isolado que prove a composição Cobra + Ginger;
- transcripts nativos do controle local em Linux e Windows sob
  `docs/research/evidence/f1-control/`.

## Tarefas ordenadas

### Baseline das referências

- [x] **F1-01** Registrar versões do Go, sistema operacional e arquiteturas de desenvolvimento/CI.
- [x] **F1-02** Fixar o commit do DWYT usado na análise, confirmar sua licença e registrar que regras de negócio não serão copiadas.
- [x] **F1-03** Fixar a tag do Ginger em `v1.4.4` para código, documentação e CLI de geração.
- [x] **F1-04** Instalar a CLI com `go install github.com/fvmoraes/ginger/cmd/ginger@v1.4.4` e comprovar a versão com `ginger version`.

### Inventário do DWYT

- [x] **F1-05** Mapear a estrutura real do backend, frontend, comandos e composição do binário.
- [x] **F1-06** Identificar a implementação de `go:embed`, fallback da SPA, escolha de porta, abertura do navegador, PID/estado e shutdown.
- [x] **F1-07** Inventariar React, TypeScript, Vite, Tailwind, Router, package manager, testes e organização por features.
- [x] **F1-08** Inventariar tokens Catppuccin, tipografia, densidade, navegação, cards e estados visuais que podem inspirar o Kube Peep.
- [x] **F1-09** Auditar GoReleaser, GitHub Actions, `install.sh`, `install.ps1`, checksum, update e remoção.
- [x] **F1-10** Classificar cada achado como `reutilizar`, `adaptar`, `substituir` ou `fora de escopo`.

### Inventário do Ginger v1.4.4

- [x] **F1-11** Ler o código público e os exemplos dos pacotes `app`, `router`, `config`, `logger`, `errors`, `response`, `health`, `sse`, `ws` e `testhelper`.
- [x] **F1-12** Confirmar lifecycle, sinais, configuração de porta, middlewares, envelopes JSON e extensão de health checks.
- [x] **F1-13** Gerar projetos descartáveis `--service` e `--cli` em diretório temporário e comparar entrypoints, Cobra, Makefile e GoReleaser.
- [x] **F1-14** Executar `ginger inspect --json` e `ginger doctor` nos dois scaffolds e guardar os resultados no relatório.
- [x] **F1-15** Executar `ginger add sse --plan` e `ginger add websocket --plan`; revisar o diff planejado sem aplicar ao repositório principal.
- [x] **F1-16** Confirmar quais comandos aceitam `--plan`; qualquer comando sem preview deve ser executado apenas em área descartável.

### Spikes de arquitetura

- [x] **F1-17** Provar que um comando Cobra `start` consegue construir, iniciar e encerrar a aplicação Ginger sem duplicar tratamento de sinais.
- [x] **F1-18** Provar que o comando raiz pode equivaler a `start` e receber flags de contexto, kubeconfig, namespace, navegador e porta.
- [x] **F1-19** Avaliar alternativas seguras para `stop` e `status`, incluindo PID obsoleto, corrida de duas inicializações e diferenças do Windows.
- [x] **F1-20** Provar serving do frontend embutido e fallback de rotas React sem interferir em `/api/v1` e `/health`.
- [x] **F1-21** Validar cancelamento de SSE por `request.Context()` e capacidade do WebSocket do Ginger para o fluxo bidirecional de `exec`.
- [x] **F1-22** Definir como `/health` separará falhas locais críticas de cluster/kubeconfig indisponíveis sem derrubar toda a aplicação.

### Compatibilidade e riscos

- [x] **F1-23** Montar matriz de compatibilidade entre Go 1.25, Ginger v1.4.4, Cobra, client-go, APIs Kubernetes e `modernc.org/sqlite`.
- [x] **F1-24** Validar cross-build sem CGO para Linux, macOS e Windows; incluir arm64 na análise.
- [x] **F1-25** Confirmar suporte do client-go a kubeconfig com `exec`, certificados, tokens referenciados e múltiplos arquivos em `KUBECONFIG`.
- [x] **F1-26** Registrar riscos de RBAC, sanitização, logs, watches, streams, port-forward, exec, processos e paths multiplataforma.
- [x] **F1-27** Abrir rascunhos dos ADRs, registrando hipóteses, alternativas e resultados ainda pendentes dos spikes seguintes.
- [x] **F1-28** Executar SSE por mais de 15 segundos e validar como neutralizar com segurança o `WriteTimeout` fixo de `pkg/app`.
- [x] **F1-29** Testar `pkg/ws` com Origin inválida, masking, opcode, fragmentação, ping/pong, heartbeat, resize, payload grande, desconexão e duração superior ao timeout do servidor.
- [x] **F1-30** Validar a corrida entre descoberta e bind de porta e definir retry sem abrir o navegador para uma instância que falhou.
- [x] **F1-31** Provar uma cadeia `HandleRaw` que preserve `http.Flusher`/`http.Hijacker` e reaplique request ID, segurança, recuperação e logging compatíveis com streaming.
- [x] **F1-32** Definir como o logger Ginger será integrado ao arquivo local, rotação e sanitização por conteúdo sem perder logs do middleware padrão.
- [x] **F1-33** Prototipar cursor composto por contexto/consulta/namespace/kind, com validação de retomada e expiração, sem depender de `response.Paginated`.
- [x] **F1-34** Registrar limitações observadas de `ginger inspect` e `ginger doctor` para que seus resultados não sejam interpretados como cobertura funcional.
- [x] **F1-35** Confirmar que SQLite sem CGO será integrado manualmente; não aplicar `ginger add sqlite`, que seleciona um driver com CGO.
- [x] **F1-36** Fixar a nomenclatura canônica: repositório/módulo `github.com/fvmoraes/kubepeep`, produto `Kube Peep` e comando/artefato `kubePeep`, testando implicações de case em cada plataforma.
- [x] **F1-37** Decidir e provar se `start` executa em foreground, relança um daemon ou suporta ambos; alinhar essa decisão a `stop`, `status`, logs e sinais.
- [x] **F1-38** Documentar que plugins `exec` já exigidos pelo kubeconfig são uma dependência externa do usuário, não uma dependência instalada pelo produto.
- [x] **F1-39** Finalizar os ADRs depois dos spikes, incluindo alternativas rejeitadas, consequências e evidências reproduzíveis.
- [x] **F1-40** Provar embedding conjunto de frontend e migrations e execução das migrations a partir do binário compilado.
- [x] **F1-41** Criar matriz “usar / complementar / justificar” para cada pacote Ginger obrigatório, incluindo `health`, `sse`, `ws` e `testhelper`.
- [x] **F1-42** Comparar `app.Run()` com lifecycle HTTP próprio usando os componentes Ginger e escolher uma estratégia que adquira o listener por bind real, exponha prontidão e permita cleanup determinístico.
- [x] **F1-43** Testar shutdown normal, timeout de shutdown e erro de servidor com SSE/WS/hijack ativos, garantindo que hooks e cleanup rodem mesmo quando conexões não encerram.
- [x] **F1-44** Provar `stop` seguro separadamente em Unix e Windows, sem tratar PID file como identidade suficiente do processo.

## Evidências e validações

- Os scaffolds temporários compilam e seus health checks respondem.
- O spike Cobra + Ginger inicia, recebe sinal e encerra sem goroutine ou arquivo de runtime órfão.
- O listener é adquirido por bind real, a porta publicada corresponde à porta efetivamente aberta e o navegador só abre após prontidão.
- Falha/timeout de shutdown ainda executa cleanup de banco, sessões e arquivos de runtime.
- `ginger inspect` e `ginger doctor` foram executados na versão correta.
- O relatório do DWYT aponta caminhos e commits, não apenas observações visuais.
- A matriz de compatibilidade registra resultado real de compilação para cada plataforma testável.
- Todo gerador aplicado foi precedido por `--plan`, quando suportado.
- O probe isolado de controle passou nativamente no Linux e no Windows amd64;
  transcripts, exit 0 e hashes estão na
  [matriz de evidências](../docs/research/phase1-evidence.md).

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

O critério foi atendido sem escolher `app.Run()`: bind real, prontidão, streams
longos, timeout, hooks/cleanup e parada segura foram exercitados. A prova
Windows fecha o desenho do probe isolado F1; o módulo definitivo e os archives
continuam sujeitos às revalidações próprias de F3 e F8.
