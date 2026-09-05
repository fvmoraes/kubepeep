# Matriz rastreável de evidências da Fase 1

> Registro histórico: resultados e gates referem-se à execução datada abaixo.
> A sequência atual de entrega está no [plano v1](../../plan/README.md).

Status da auditoria: concluída em 2026-07-27; gate da Fase 1 fechado.

Fonte dos requisitos:
[`plan/01-descoberta.md`](../../plan/README.md).

Esta matriz separa conclusão de pesquisa, prova executável e prontidão de
produção. “Fechado” significa que a pergunta ou experimento exigido pela Fase 1
tem evidência suficiente; não significa que a implementação definitiva já
exista. O código em `spikes/phase1` é deliberadamente isolado e descartável.

## Regra de leitura

| Estado | Significado |
| --- | --- |
| Fechado — documento | inspeção ou decisão registrada com fonte rastreável |
| Fechado — teste | comportamento exercitado pelo spike no host observado |
| Fechado — build | compilação comprovada; não implica execução nativa |
| Fechado — teste nativo | comportamento do probe executado no sistema operacional alvo |

## Ambiente observado

| Item | Valor observado | Escopo da prova |
| --- | --- | --- |
| data | 2026-07-27 | auditoria e reexecução desta matriz |
| host | Ubuntu 24.04.4 LTS, kernel 7.0, `linux/amd64` | testes nativos e blackbox |
| Go instalado | `go1.26.1 linux/amd64` | ambiente do host |
| Go alvo | `go1.25.0` | selecionado com `GOTOOLCHAIN` nos testes e builds |
| CGO do host | habilitado | cross-builds forçaram `CGO_ENABLED=0` |
| Node.js / npm | `v24.18.0` / `11.16.0` | inspeção e validação do DWYT |
| módulo do spike | `github.com/fvmoraes/kubepeep/spikes/phase1` | não é o futuro módulo de produção |
| Ginger | módulo e CLI `v1.4.4` | CLI resolvida por `PATH` explícito |

A matriz de runners de CI planejada, incluindo os limites de arm64 e Windows,
está em
[`compatibility-matrix.md`](../research/compatibility-matrix.md#matriz-de-ci-fixada).

## Comandos reexecutáveis e resultados

Os comandos abaixo partem da raiz do repositório. `<GO_BIN>` deve ser
substituído pelo diretório que contém a CLI Ginger instalada.

```bash
rtk proxy env PATH="<GO_BIN>:$PATH" ginger version

rtk proxy env \
  GOTOOLCHAIN=go1.25.0 \
  KUBEPEEP_LONG_TEST=1 \
  go test -count=1 ./...

rtk proxy env \
  GOTOOLCHAIN=go1.25.0 \
  go test -race -count=1 ./control ./spike

rtk proxy env GOTOOLCHAIN=go1.25.0 go vet ./...

rtk proxy env \
  GOTOOLCHAIN=go1.25.0 \
  go test -count=20 ./control \
  -run '^TestNativeControlLifecycleBlackBox$'
```

Os quatro comandos Go devem ser executados em `spikes/phase1`. Resultado desta
reexecução:

| Validação | Resultado |
| --- | --- |
| CLI Ginger | `ginger 1.4.4`; build info do módulo `v1.4.4` persistido |
| suíte completa com `KUBEPEEP_LONG_TEST=1` | passou; inclui SSE e WebSocket por mais de 15 segundos |
| race em `control` e `spike` | passou |
| `go vet ./...` | passou |
| blackbox nativo de controle, `-count=20` | passou no Linux |

Transcripts nativos preservados:

| Plataforma | Resultado | Integridade da evidência |
| --- | --- | --- |
| Linux amd64 | [`linux-native-2026-07-27.txt` — registro histórico](../research/evidence/f1-control/README.md): blackbox e SIGTERM passaram; blackbox repetido 20 vezes | SHA-256 `84dc8f74fc128e1add97fe71faf4ceca4c21543420a69cfe36e843493cda4aa4` |
| Windows 10 Pro amd64 | [`windows-native-2026-07-27.txt` — registro histórico](../research/evidence/f1-control/README.md): suíte `control` passou, `TEST_EXIT_CODE=0` | SHA-256 `53b7e2eb49d2b5528aa0366d7b701da5d8de0b4d6ec05266125bfd9eafe31b00` |
| Harness Windows | [`run-windows-native.cmd`](../../spikes/phase1/scripts/run-windows-native.cmd) usado para coletar SO, hashes e saída | SHA-256 `bac93695b2b1274d05cb51b711712a9d2c32b014ea6040e3b4f04d9acd73c751` |

O transcript Windows registra ainda os binários executados:

- probe: SHA-256
  `3D8E3DCAC0CE72DCD039070F5BE2361630F7FEF090A97486409C35A07E2E12B8`;
- test binary: SHA-256
  `30D21E2367DB4F0BABC04CAC2F21392ED9EBEF505D92E899E38192D80409008F`.

O cross-build usa o mesmo comando para cada par
`linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`,
`windows/amd64` e `windows/arm64`:

```bash
rtk proxy env \
  GOTOOLCHAIN=go1.25.0 \
  CGO_ENABLED=0 \
  GOOS=<sistema> \
  GOARCH=<arquitetura> \
  go build ./...
```

Os seis pares passaram. Adicionalmente, o probe e o binário de testes de
`./control` foram compilados com `go test -c` para Windows amd64 e arm64.
O artefato amd64 foi então executado nativamente no Windows depois do ajuste do
validador de DACL; blackbox, `LockFileEx`, fingerprint, controle autenticado,
cleanup, DACL privada e rejeição de adulteração passaram com exit 0. Windows
arm64 permanece somente cross-build, o que não é apresentado como runtime.

## Matriz F1-01 a F1-44

### Baseline das referências

| ID | Estado | Evidência e resultado | Limite ou lacuna |
| --- | --- | --- | --- |
| F1-01 | Fechado — documento | [Matriz de compatibilidade](../research/compatibility-matrix.md#ambiente-observado) registra host, Go alvo e instalado, ferramentas e runners/arquiteturas de CI. | Os runners não foram executados nesta máquina; execução nativa é distinguida de cross-build. |
| F1-02 | Fechado — documento | [Pesquisa DWYT](../research/dwyt.md#proveniência-licença-e-limite-de-cópia) fixa `a9386823272b928f2289c9020a9ae5951389e0f1`, confirma MIT e proíbe copiar regras de negócio. | A licença permite mais reuso do que a política deliberadamente restritiva adotada. |
| F1-03 | Fechado — documento | [Pesquisa Ginger](../research/ginger-v1.4.4.md#proveniência-e-anomalia-de-versão) fixa tag `v1.4.4` e commit `6073543b6281be01e4bc97d001dd6e11512f70db` para código, docs e geração. | A tag contém `version.txt` 1.4.3; a anomalia upstream está registrada. |
| F1-04 | Fechado — teste | [`cli-version.txt` — registro de reprodução](../research/evidence/ginger-v1.4.4/README.md) preserva `ginger 1.4.4`, módulo, checksum e resolução da CLI instalada por `go install ...@v1.4.4`. | Uma CLI mais antiga existia no `PATH` geral; todas as provas finais priorizaram explicitamente `<GO_BIN>`. |

### Inventário do DWYT

| ID | Estado | Evidência e resultado | Limite ou lacuna |
| --- | --- | --- | --- |
| F1-05 | Fechado — documento | [Inventário da estrutura](../research/dwyt.md#inventário-da-estrutura) mapeia backend, frontend, comandos, entrypoint e composição do binário no commit fixado. | É inspeção da referência, não arquitetura importada para produção. |
| F1-06 | Fechado — documento | [CLI/lifecycle](../research/dwyt.md#cli-execução-local-e-ciclo-de-vida) e [embed/SPA](../research/dwyt.md#servidor-embed-e-roteamento-da-spa) cobrem `go:embed`, fallback, porta, browser, PID, estado e shutdown. | O smoke HTTP do DWYT não foi executado porque sua porta fixa já estava ocupada; o achado vem de fonte e builds, não desse runtime. |
| F1-07 | Fechado — documento | [Frontend e build](../research/dwyt.md#frontend-e-build) inventaria React, TypeScript, Vite, Tailwind, Router, npm, testes ausentes e organização. | Dependências observadas não são automaticamente as versões finais do Kube Peep. |
| F1-08 | Fechado — documento | [Identidade visual](../research/dwyt.md#identidade-visual) registra tokens Catppuccin, tipografia, densidade, navegação, cards e estados, com limites de cópia. | Marca, logo, mascote, textos e componentes de negócio não serão copiados. |
| F1-09 | Fechado — documento | [Instalação, release e remoção](../research/dwyt.md#instalação-atualização-release-e-remoção) audita GoReleaser, Actions, instaladores, checksums, atualização e uninstall. | GoReleaser não estava instalado; sua configuração foi inspecionada, mas um release DWYT completo não foi executado. |
| F1-10 | Fechado — documento | [Matriz de aproveitamento](../research/dwyt.md#matriz-de-aproveitamento) classifica cada achado como reutilizar, adaptar, substituir ou não copiar/fora do escopo. | As classificações orientam projeto; não autorizam copiar domínio do DWYT. |

### Inventário do Ginger v1.4.4

| ID | Estado | Evidência e resultado | Limite ou lacuna |
| --- | --- | --- | --- |
| F1-11 | Fechado — documento | [Pesquisa Ginger](../research/ginger-v1.4.4.md) possui seções e matriz para `app`, `router`, `config`, `logger`, `errors`, `response`, `health`, `sse`, `ws` e `testhelper`, vinculadas ao commit fixado. | A cobertura upstream baixa foi registrada; os comportamentos críticos usam provas próprias. |
| F1-12 | Fechado — documento | Seções [lifecycle](../research/ginger-v1.4.4.md#lifecycle-de-pkgapp), [router](../research/ginger-v1.4.4.md#router-e-middlewares), [health](../research/ginger-v1.4.4.md#health) e [helpers](../research/ginger-v1.4.4.md#helpers-de-resposta-e-teste), mais [ADR 0002](../decisions/0002-health-and-degraded-state.md), fixam sinais, porta, middleware, envelopes e extensão de checks. | A semântica composta de health é decisão do Kube Peep, ainda não código de produção. |
| F1-13 | Fechado — teste | [`scaffold-comparison.md`](../research/evidence/ginger-v1.4.4/scaffold-comparison.md) compara entrypoints, Cobra, Makefiles, GoReleaser e hashes dos scaffolds `--service`/`--cli`; ambos passaram por tidy/test/vet/build com Go 1.25. | O scaffold definitivo não foi gerado, conforme o gate da descoberta. |
| F1-14 | Fechado — teste | Service: [`inspect` — registro de reprodução](../research/evidence/ginger-v1.4.4/README.md) e [`doctor` — registro de reprodução](../research/evidence/ginger-v1.4.4/README.md), exits 0/0. CLI: [`inspect` — registro de reprodução](../research/evidence/ginger-v1.4.4/README.md) e [`doctor` — registro de reprodução](../research/evidence/ginger-v1.4.4/README.md), exits 0/1. | O exit 1 do CLI é esperado: o próprio template não gera testes. Saídas foram sanitizadas. |
| F1-15 | Fechado — teste | [`plan-support.md`](../research/evidence/ginger-v1.4.4/plan-support.md#provas-de-não-mutação) registra `add sse --plan` e `add websocket --plan`; a baseline permaneceu sem diff. | Os planos não montam rotas e não foram aplicados ao repositório principal. |
| F1-16 | Fechado — documento | [Inventário completo de `--plan`](../research/evidence/ginger-v1.4.4/plan-support.md) cobre `docs`, todos os generators, todas as integrações e comandos sem preview; probes sem preview ficaram em área descartável. | Parte da cobertura é prova do parser uniforme, complementada por amostras de runtime; não se aplicou cada integração. |

### Spikes de arquitetura

| ID | Estado | Evidência e resultado | Limite ou lacuna |
| --- | --- | --- | --- |
| F1-17 | Fechado — teste | `TestCobraContextOwnsTheGingerRuntime` em [`spike_test.go`](../../spikes/phase1/spike/spike_test.go) e `TestCompiledBinaryOwnsSIGINTAndSIGTERM` em [`signal_unix_test.go`](../../spikes/phase1/cmd/kubePeep-spike/signal_unix_test.go) provam Cobra como único owner, aplicação Ginger e cleanup. | Sinais pertencem ao caminho Unix; no Windows, o probe usa o canal de controle autenticado validado em F1-44. Ambos ainda serão reimplementados em F3. |
| F1-18 | Fechado — teste | `NewRootCommand` em [`command.go`](../../spikes/phase1/spike/command.go) registra contexto, kubeconfig, namespace, browser e porta; `TestRootAndStartUseTheSameContract` executa raiz e `start` pelo mesmo `RunE`. | O teste comportamental afirma contexto/namespace/browser/porta; a ligação de kubeconfig é evidência direta do registro da flag. |
| F1-19 | Fechado — teste | [ADR 0004](../decisions/0004-local-runtime-and-process-lifecycle.md) compara PID, sinais, `taskkill` e controle autenticado. [`control/blackbox_test.go`](../../spikes/phase1/control/blackbox_test.go) prova lock, identidade, PID obsoleto, segunda inicialização, `status` e `stop` no Linux e no Windows amd64. | É um probe isolado de decisão; o lifecycle do módulo de produção será revalidado em F3 e nos archives em F8. |
| F1-20 | Fechado — teste | `TestSPAFallbackDoesNotCaptureHealthOrAPI` e `TestFrontendAndMigrationsAreEmbeddedTogether` em [`spike_test.go`](../../spikes/phase1/spike/spike_test.go) provam SPA embarcada, fallback e exclusão de `/api/v1` e `/health`. | Usa assets mínimos de spike, não o frontend final. |
| F1-21 | Fechado — teste | `TestGingerSSEStopsWhenTheRequestContextIsCanceled` prova cancelamento; [`ws_matrix_test.go`](../../spikes/phase1/spike/ws_matrix_test.go) e o teste de Origin/masking em `spike_test.go` demonstram que `pkg/ws` não é seguro para `exec`. [ADR 0003](../decisions/0003-streaming-protocols.md) escolhe `coder/websocket`. | O resultado WebSocket é deliberadamente negativo; o endpoint `exec` de produção continua bloqueado até fases posteriores. |
| F1-22 | Fechado — documento | [ADR 0002](../decisions/0002-health-and-degraded-state.md) separa aplicação/SQLite críticos de kubeconfig/contexto/cluster externos e define 200/503, `degraded` e `unknown`. | Contrato decidido e testável; implementação definitiva pertence às fases 2/3. |

### Compatibilidade e riscos

| ID | Estado | Evidência e resultado | Limite ou lacuna |
| --- | --- | --- | --- |
| F1-23 | Fechado — build | [Dependências aprovadas](../research/compatibility-matrix.md#dependências-aprovadas) fixa Go 1.25, Ginger, Cobra, WebSocket, Kubernetes v0.35.7 e modernc SQLite; a suíte carrega e compila os módulos. | Não houve cluster real/Kind; integração Kubernetes é gate da Fase 4. |
| F1-24 | Fechado — build | [Validações](../research/compatibility-matrix.md#validações-executadas) e a reexecução desta matriz compilam sem CGO Linux/macOS/Windows em amd64/arm64. | Cross-build, isoladamente, não comprova runtime, filesystem, instalador ou `stop`; a prova nativa específica de controle está em F1-44. |
| F1-25 | Fechado — documento | [Compatibilidade de kubeconfig](../research/compatibility-matrix.md#compatibilidade-de-kubeconfig) confirma no loader oficial arquivos múltiplos, precedência, paths relativos, certificados/tokens referenciados e plugin `exec`. | Fixtures que executem plugins e autenticação real ficam para fases 3/8. |
| F1-26 | Fechado — documento | [`security.md`](../security.md) cobre RBAC, credenciais, sanitização, logs, watches, streams, port-forward, exec, processos, filesystem e supply chain. | É threat model/contrato; os controles de produto serão testados nas fases de implementação. |
| F1-27 | Fechado — documento | [ADRs 0001](../decisions/0001-cli-service-bootstrap.md), [0002](../decisions/0002-health-and-degraded-state.md), [0003](../decisions/0003-streaming-protocols.md) e [0004](../decisions/0004-local-runtime-and-process-lifecycle.md) registram contexto, alternativas e evidências. | Os rascunhos já foram promovidos a “aceito”; o estado final é rastreado em F1-39. |
| F1-28 | Fechado — teste | `TestSSESurvivesLongerThanGingerRunWriteTimeout`, em [`spike_test.go`](../../spikes/phase1/spike/spike_test.go), passou com `KUBEPEEP_LONG_TEST=1` e entregou evento após 16 s usando `WriteTimeout=0`. | O limite/budget por rota ainda será implementado no produto. |
| F1-29 | Fechado — teste | [`ws_matrix_test.go`](../../spikes/phase1/spike/ws_matrix_test.go) cobre opcode/FIN/fragmentação, ping/pong, heartbeat, resize, payload grande, desconexão e duração longa; `spike_test.go` cobre Origin e masking. | Os testes demonstram lacunas de `pkg/ws`; não o aprovam para terminal. |
| F1-30 | Fechado — teste | `TestBindLoopbackRetriesUsingTheRealListener`, `TestBindLoopbackDoesNotHideNonAddressInUseErrors` e `TestOuterHealthMuxAndReadinessCallbackPrecedeBrowser`, em [`spike_test.go`](../../spikes/phase1/spike/spike_test.go), provam bind real, retry e callback de browser somente após readiness. | O callback substitui a abertura gráfica real no teste automatizado. |
| F1-31 | Fechado — teste | `TestRawChainPreservesStreamingInterfacesAndRejectsForeignOrigin`, em [`spike_test.go`](../../spikes/phase1/spike/spike_test.go), afirma `Flusher`/`Hijacker`, request ID, Host/Origin, recovery e logging estruturado. | É cadeia de spike; será reescrita e testada no scaffold definitivo. |
| F1-32 | Fechado — documento | [Logger Ginger](../research/ginger-v1.4.4.md#logger), [`security.md`](../security.md#12-logging-e-observabilidade) e [`architecture.md`](../architecture.md#15-observabilidade) definem handler `slog`, stdout, arquivo rotativo, allowlist e sanitização por conteúdo. | Definição concluída; sink, rotação e corpus de redaction ainda não são implementação de produção. |
| F1-33 | Fechado — teste | `TestCompoundCursorRejectsTamperingQueryChangesAndExpiry`, em [`spike_test.go`](../../spikes/phase1/spike/spike_test.go), prova cursor opaco HMAC ligado a contexto/consulta/namespace/kind, geração e expiração. | O cursor usa modelo mínimo do spike; integração com paginação Kubernetes virá depois. |
| F1-34 | Fechado — documento | [`ginger-v1.4.4.md`](../research/ginger-v1.4.4.md#inspect-e-doctor) registra rotas/checks não detectados e explicita que diagnóstico verde não equivale a cobertura funcional. | Nenhuma inferência de segurança é feita a partir de `doctor`. |
| F1-35 | Fechado — build | [Matriz de compatibilidade](../research/compatibility-matrix.md#dependências-aprovadas) fixa `modernc.org/sqlite v1.54.0`; builds usam CGO 0 e o spike aplica migration. A pesquisa Ginger registra que `ginger add sqlite` não será aplicado. | WAL/SHM, concorrência, backup e permissões pertencem à Fase 3. |
| F1-36 | Fechado — build | A nomenclatura está fixada como módulo `github.com/fvmoraes/kubepeep`, produto “Kube Peep” e artefato `kubePeep`; o entrypoint [`cmd/kubePeep-spike`](../../spikes/phase1/cmd/kubePeep-spike/main.go) preservou casing nos seis cross-builds. | Invocação e empacotamento case-sensitive/case-insensitive ainda serão smoke tests nativos de release; não são inferidos do cross-build. |
| F1-37 | Fechado — teste | [ADRs 0001](../decisions/0001-cli-service-bootstrap.md) e [0004](../decisions/0004-local-runtime-and-process-lifecycle.md) escolhem foreground; os testes Cobra e o [`control/blackbox_test.go`](../../spikes/phase1/control/blackbox_test.go) executam processo foreground real no Linux e no Windows amd64. | Não há daemon implícito; a implementação definitiva continua sendo gate de F3. |
| F1-38 | Fechado — documento | [Matriz](../research/compatibility-matrix.md#compatibilidade-de-kubeconfig) e [`security.md`](../security.md#8-kubeconfig-e-plugins-exec) registram plugins `exec` do kubeconfig como dependência externa do usuário, nunca instalada pelo produto. | Nenhum plugin real foi executado nesta fase. |
| F1-39 | Fechado — documento | Os quatro ADRs estão com status “aceito”, alternativas rejeitadas, consequências e evidências reproduzíveis. | Uma nova evidência contraditória deve reabrir o ADR correspondente. |
| F1-40 | Fechado — teste | `TestFrontendAndMigrationsAreEmbeddedTogether` abre ambos os `embed.FS`, aplica SQL em modernc SQLite e verifica a tabela; o [`kubePeep-spike`](../../spikes/phase1/cmd/kubePeep-spike/main.go) aplica a migration antes da readiness em binário compilado. | Assets e schema são mínimos e descartáveis. |
| F1-41 | Fechado — documento | [Matriz Ginger](../research/ginger-v1.4.4.md#matriz-usar--complementar--justificar) decide usar/complementar/justificar para todos os pacotes obrigatórios, incluindo health, SSE, WS e testhelper. | `pkg/ws` permanece avaliado, porém rejeitado para `exec`. |
| F1-42 | Fechado — teste | [Pesquisa lifecycle](../research/ginger-v1.4.4.md#lifecycle-de-pkgapp), [ADR 0001](../decisions/0001-cli-service-bootstrap.md) e testes de [`runtime.go`](../../spikes/phase1/spike/runtime.go) comparam `app.Run()` ao coordenador que possui listener, readiness e cleanup. | A decisão usa componentes Ginger, mas não `app.Run()`/`OnStop`. |
| F1-43 | Fechado — teste | `TestLifecycleMatrixCleansSSEWebSocketAndHijack`, em [`lifecycle_matrix_test.go`](../../spikes/phase1/spike/lifecycle_matrix_test.go), cobre shutdown normal, erro de servidor e timeout com SSE, WebSocket e hijack ativos, verificando cleanup. | Prova de lifecycle isolada; os serviços reais ainda precisarão registrar suas sessões no owner. |
| F1-44 | **Fechado — teste nativo** | Linux: [`linux-native-2026-07-27.txt` — registro histórico](../research/evidence/f1-control/README.md), incluindo blackbox repetido 20 vezes. Windows 10 Pro amd64: [`windows-native-2026-07-27.txt` — registro histórico](../research/evidence/f1-control/README.md), suíte completa `control`, DACL corrigida, `LockFileEx`, fingerprint, identidade, stale PID, `status`/`stop` e cleanup, `TEST_EXIT_CODE=0`; hashes dos binários constam do transcript. | A prova fecha o desenho F1 do probe isolado. Não substitui integração do código de produção em F3 nem smoke de archive/instalador em F8; Windows arm64 continua cross-build. |

## Gate fechado e revalidações futuras

F1-01 a F1-44 possuem evidência no nível exigido pela descoberta. O probe
isolado comprovou o desenho de controle nativamente em Unix/Linux e Windows
amd64; a Fase 1 está concluída.

Essa conclusão libera a especificação, não promove o spike a código de produção.
As seguintes revalidações continuam obrigatórias sem reabrir F1:

- os seis cross-builds não provam filesystem, instalador ou runtime macOS/Windows;
- não houve cluster Kind nem plugin `exec` real; esses testes pertencem às
  fases Kubernetes;
- GoReleaser do DWYT foi auditado por fonte, sem release local completo;
- logger rotativo, health composto, transporte WebSocket endurecido e controle
  local serão reimplementados e testados no módulo de produção em F3;
- F8 executará os archives e instaladores reais em runners nativos, inclusive
  casing, paths, permissões, atualização e rollback;
- nenhum scaffold definitivo foi criado durante a descoberta.
