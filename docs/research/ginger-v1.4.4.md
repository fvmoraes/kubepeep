# Ginger v1.4.4 — pesquisa reproduzível para a Fase 1

Status: concluída em 2026-07-27

Repositório: [`fvmoraes/ginger`](https://github.com/fvmoraes/ginger)

Tag e commit fixados:
[`v1.4.4`](https://github.com/fvmoraes/ginger/tree/v1.4.4) /
[`6073543b6281be01e4bc97d001dd6e11512f70db`](https://github.com/fvmoraes/ginger/commit/6073543b6281be01e4bc97d001dd6e11512f70db)

## Resumo executivo

O Ginger v1.4.4 fornece os componentes adequados para a base do Kube Peep:
container de aplicação, router sobre `net/http`, config, logger compatível com
`slog`, erros, envelopes, health, SSE e helpers de teste. O scaffold correto é
`--service`, complementado manualmente com Cobra.

Dois métodos não podem governar o runtime final:

- `app.Run()` possui listener/sinais privados, timeouts fixos e caminhos que
  pulam cleanup;
- `pkg/ws` não implementa o hardening de WebSocket necessário ao terminal.

Essa conclusão não abandona Ginger. O produto continuará usando seus tipos e
componentes, com lifecycle, health composto, middleware raw, logging e
WebSocket endurecidos pelo Kube Peep.

## Proveniência e anomalia de versão

O checkout detached confirmou o commit
`6073543b6281be01e4bc97d001dd6e11512f70db`, árvore limpa e `go 1.22` no
`go.mod`.

Há uma divergência upstream reproduzível:

- a tag é `v1.4.4`;
- `internal/buildinfo/version.txt` dentro da tag contém `1.4.3`;
- `go run ./cmd/ginger version` no checkout imprime `ginger 1.4.3`;
- o binário instalado por
  `go install github.com/fvmoraes/ginger/cmd/ginger@v1.4.4` imprime
  `ginger 1.4.4` e seu build info aponta para o módulo `v1.4.4`.

O workflow de release atualiza o arquivo no worktree e cria a tag sem incluir
essa alteração em commit. Portanto:

1. o projeto fixa módulo, documentação e gerador pela tag/commit;
2. a CLI canônica é instalada com `go install ...@v1.4.4`;
3. `go run` do checkout não será usado para gerar o scaffold;
4. a anomalia é documentada e não será “corrigida” com fork local silencioso.

A verificação final resolveu explicitamente a CLI com
`PATH=<GO_BIN>:$PATH`: `which` apontou para o binário instalado pelo módulo
`github.com/fvmoraes/ginger v1.4.4`, `ginger version` imprimiu `ginger 1.4.4`
e o build info confirmou o checksum do módulo. Isso impediu que uma versão
anterior disponível no `PATH` do sistema contaminasse os scaffolds. A saída
sanitizada está em
[`cli-version.txt`](evidence/ginger-v1.4.4/cli-version.txt).

## Scaffolds comparados

Os dois projetos foram criados em diretórios temporários com o toolchain alvo
Go 1.25.0.

| Aspecto | `--service` | `--cli` |
| --- | --- | --- |
| entrypoint | aplicação Ginger HTTP | Cobra |
| router/health | sim | não |
| Cobra | não | sim |
| teste gerado | health integration | nenhum |
| Makefile | run/build/test/lint/tidy + docker/up/down | run/build/test/lint/tidy |
| GoReleaser | não | sim |
| Docker/Kubernetes/Helm | sim | não |
| `doctor` após tidy/vet | passa | falha apenas por ausência de testes no próprio template |
| base do Kube Peep | escolhida | rejeitada |

`ginger generate command probe` foi recusado no projeto service com a mensagem
de que o generator só existe para projetos `--cli`. A CLI Cobra será integrada
manualmente e a configuração GoReleaser será criada na Fase 8.

Os dois scaffolds foram validados com Go 1.25.0 por `go mod tidy`,
`go test ./...`, `go vet ./...` e `go build ./...`. Entry points, hashes,
Makefiles completos e a configuração GoReleaser estão registrados na
[`comparação sanitizada`](evidence/ginger-v1.4.4/scaffold-comparison.md).

O parser de `generate` aceita `--plan` uniformemente para `crud`, `command`,
`handler`, `service`, `test`, `tests --scan`, `smoke-test` e `swagger`, embora
o help compacto só anote explicitamente alguns deles. `docs` e todas as
integrações de `add` também oferecem preview real. `new`, `init`, `run` e
`build` não oferecem preview; `inspect`, `doctor`, `version` e `help` são
operações de leitura. Todo comando mutável sem preview usado na pesquisa foi
executado apenas nos diretórios descartáveis.

As provas de runtime cobriram `docs`, `generate crud`, `generate tests --scan`,
`generate command`, `add sse` e `add websocket`; a comparação contra a baseline
não encontrou mutação após os previews. SSE e WebSocket planejam somente um
handler e atualização do manifest, sem montar a rota, e também são aceitos em
projeto CLI, onde deixam handlers HTTP sem aplicação. O Kube Peep não aplicará
integrações apenas porque `doctor` as lista como disponíveis. O inventário de
comandos, integrações e comandos sem preview está em
[`plan-support.md`](evidence/ginger-v1.4.4/plan-support.md).

## Lifecycle de `pkg/app`

Inspeção e harness real de `pkg/app/app.go` demonstraram:

- `Run()` cria `http.Server` e usa `ListenAndServe`, sem aceitar listener;
- o endereço real de porta zero não é exposto;
- `app_started` é logado antes de o bind ser confirmado;
- sinais `SIGINT` e `SIGTERM` pertencem ao próprio método;
- `ReadTimeout=15s`, `WriteTimeout=15s` e `IdleTimeout=60s` são fixos;
- erro de bind/Serve retorna diretamente e não executa hooks;
- timeout de `Server.Shutdown` retorna antes dos hooks;
- hooks compartilham o tempo restante do mesmo contexto;
- conexões hijacked não são fechadas por `http.Server.Shutdown`.

Provas executadas:

| Cenário | Resultado do lifecycle padrão |
| --- | --- |
| bind falha | `Run` retorna; hook não executa |
| request ativo + shutdown de 1 s | retorna erro; hook não executa |
| SSE envia evento após 16 s | cliente recebe EOF antes do evento final |
| conexão hijacked | não pertence ao shutdown HTTP |

Decisão: usar `app.New` como container e o router Ginger como handler, mas
entregar listener, sinais, prontidão e cleanup ao coordenador descrito nos ADRs
0001 e 0004.

O spike `spikes/phase1` prova o caminho escolhido:

- Cobra é o único owner do contexto;
- raiz e `start` compartilham a função;
- bind real escolhe porta e permanece em loopback;
- readiness/health precede a publicação de prontidão e a abertura do browser;
- stream dura 16 segundos com `WriteTimeout=0`;
- cancelamento, erro de Serve e timeout executam cleanup;
- registry explícito fecha conexão hijacked;
- o probe isolado de controle autentica `status` e `stop`, prova a identidade
  completa, nunca usa PID como autoridade e libera estado/lock em Linux e
  Windows nativos.

São camadas deliberadamente distintas: F1 valida decisões em código
descartável; F3 reimplementa lifecycle e adapters no scaffold definitivo; F8
testa os mesmos contratos nos archives e instaladores de release.

## Router e middlewares

Rotas comuns registradas por `router.Handle` recebem a cadeia configurada.
`HandleRaw` registra o handler diretamente e não aplica middleware.

`pkg/middleware.Logger` envolve o writer em um tipo que implementa somente
`Header`, `Write` e `WriteHeader`. Isso remove `http.Flusher`,
`http.Hijacker`, `http.Pusher` e otimizações como `io.ReaderFrom`.

O scaffold service também instala Request ID global do Ginger e gera outro
middleware de Request ID no grupo da API; o harness observou IDs diferentes na
resposta e no log. O segundo middleware será removido.

Decisão:

- rotas comuns usam Request ID, recover e logger Ginger uma única vez;
- streams usam `HandleRaw` e cadeia própria, preservando o writer original;
- a cadeia raw reaplica Host/Origin, Request ID, recovery e logging;
- o fallback SPA rejeita `/api`, `/health`, métodos mutáveis e requests que não
  aceitam HTML;
- um handler JSON específico para `GET /api/v1/` impede que API desconhecida
  retorne `index.html`.

## Health

`pkg/health`:

- lança uma goroutine por checker e espera sem deadline próprio;
- serializa `err.Error()` integralmente;
- transforma qualquer falha em 503;
- não recupera panic dentro da goroutine do checker;
- é registrado como raw e não recebe Host guard/request ID.

Harnesses provaram bloqueio por checker que ignora contexto, exposição de
token/path em erro e término do subprocesso por panic.

O Kube Peep usará `health.Checker` como contrato, mas executará wrappers com:

- timeout individual;
- `recover`;
- mensagem pública estável;
- log original sanitizado;
- classificação crítica ou externa.

Aplicação e SQLite controlam 503. Kubeconfig, contexto e cluster aparecem
separadamente como `degraded`/`unknown` e não derrubam a saúde local. A Metrics
API aparece apenas em `/api/v1/status`.

## SSE

`pkg/sse.New` configura headers e exige `http.Flusher`. `Send` é útil como
encoder simples, mas:

- ignora erros de `Fprintf`;
- aceita newline em strings, ID e type;
- não fornece heartbeat, replay, limite ou backpressure;
- seu retorno não é prova confiável de conexão viva.

Uso aprovado:

- somente em rota raw com writer preservado;
- payload sempre DTO JSON;
- ID e type gerados/allowlisted;
- fila e bytes limitados pelo serviço;
- heartbeat controlado pelo Kube Peep;
- cancelamento governado por `request.Context()`.

O spike comprovou cancelamento pelo contexto e stream real de 16 segundos.

## WebSocket

`pkg/ws` faz uma implementação manual mínima. O harness provou:

- Origin externa foi aceita;
- ausência de `Sec-WebSocket-Version` ainda produziu 101;
- frame de cliente sem masking foi aceito;
- ping foi entregue ao decoder JSON como mensagem comum.

O código também não impõe teto de payload e não cobre integralmente FIN/RSV,
fragmentação, close, ping/pong, subprotocol, deadlines ou backpressure.

Decisão: rejeitar `pkg/ws` para `exec`. O transporte local usará
`github.com/coder/websocket v1.8.15`, com Origin, subprotocol, read limit,
heartbeat, deadlines, backpressure e close testados. O stream backend→cluster
continua com `client-go/tools/remotecommand`.

## Logger

`logger.New` cria sempre um pretty JSON handler em `os.Stdout`; o segundo
parâmetro de formato é ignorado. O tipo `logger.Logger`, porém, incorpora
`*slog.Logger`.

Estratégia aprovada:

1. construir `*logger.Logger` com um `slog.Logger` cujo handler pertence ao
   Kube Peep;
2. usar JSON line em stdout e arquivo local rotativo;
3. sanitizar mensagem e atributos recursivamente antes dos sinks;
4. allowlist dos campos observáveis;
5. não depender apenas do nome da chave para redaction;
6. usar o mesmo logger nos middlewares Ginger e na cadeia raw;
7. testar logs HTTP reais, não somente chamadas manuais.

O writer rotativo será implementado localmente para evitar dependência de
runtime desnecessária. Falha no arquivo degrada para stderr de forma observável,
sem interromper a aplicação.

## Static/SPA e embedding

Ginger v1.4.4 não oferece helper para FileServer, `fs.Sub`, fallback SPA ou
assets. O único `go:embed` upstream é o arquivo de versão da CLI.

Isso será implementação própria montada sobre o router:

- assets reais antes do fallback;
- cache imutável somente para nomes versionados por hash;
- `index.html` com `no-cache`/`no-store`;
- fallback apenas GET/HEAD com Accept HTML;
- exclusão explícita de `/api/v1` e `/health`;
- migrations no mesmo binário.

O spike compilado abriu `index.html` e `001_initial.sql` a partir do mesmo
`embed.FS`.

## Helpers de resposta e teste

- `pkg/response` será usado para envelopes comuns; cursor, erros parciais e
  streams terão DTOs próprios compatíveis com o contrato público.
- `pkg/errors` será mapeado para códigos estáveis do Kube Peep; mensagens
  públicas não receberão erros de plugin brutos.
- `pkg/testhelper` é adequado para handlers JSON comuns.
- Streams, hijack, listener, lifecycle e browser precisam de servidores TCP e
  binários reais; `testhelper` não substitui esses testes.

## Matriz usar / complementar / justificar

| Pacote | Tratamento | Justificativa |
| --- | --- | --- |
| `pkg/app` | usar + complementar | container `app.New`; lifecycle próprio, sem `Run`/`OnStop` |
| `pkg/router` | usar | rotas comuns; raw somente com cadeia explícita |
| `pkg/config` | usar + complementar | bootstrap Ginger e schema estrito próprio |
| `pkg/logger` | usar + complementar | tipo/contexto Ginger com handler, sink e redactor próprios |
| `pkg/errors` | usar + complementar | erro base mapeado para códigos públicos estáveis |
| `pkg/response` | usar + complementar | envelopes comuns; cursor/agregação próprios |
| `pkg/health` | usar + complementar | contrato de checker; executor/handler seguro próprio |
| `pkg/sse` | usar com limites | encoder em raw; sessão/backpressure próprios |
| `pkg/ws` | não usar em exec | gaps de protocolo e segurança comprovados |
| `pkg/testhelper` | usar + complementar | handlers comuns; TCP/binário real para streams/lifecycle |

## `inspect` e `doctor`

Os comandos foram executados nos dois scaffolds recém-gerados, depois de
tidy/test/vet/build. No service, `inspect --json` detectou `/ping`, um teste de
integração e as features de implantação, e `doctor` terminou com código 0. No
CLI, `inspect --json` não detectou rotas nem testes e `doctor` terminou com
código 1 exclusivamente porque o próprio template não gera teste.

As saídas integrais e sanitizadas são:

- service:
  [`inspect --json`](evidence/ginger-v1.4.4/service-inspect.json) e
  [`doctor`](evidence/ginger-v1.4.4/service-doctor.txt);
- CLI:
  [`inspect --json`](evidence/ginger-v1.4.4/cli-inspect.json) e
  [`doctor`](evidence/ginger-v1.4.4/cli-doctor.txt).

Limitações reproduzidas:

- `inspect` lista `/ping`, mas não o `/health` registrado dentro de `app.New`;
- não detecta bind inseguro, middleware duplicado, captura indevida da SPA,
  perda de Flusher/Hijacker ou gaps de lifecycle;
- capabilities não apresentam SSE/WS, embora `add` aceite ambos;
- `doctor` do template CLI falha por o próprio template não conter teste;
- diagnóstico verde não comprova segurança nem comportamento funcional.

Esses comandos serão registrados como evidência auxiliar em cada fase, nunca
como substitutos de testes.

## Validações upstream e descartáveis

| Comando/cenário | Resultado |
| --- | --- |
| `GOTOOLCHAIN=go1.25.0 go test ./...` | sucesso |
| `go vet ./...` | sucesso |
| `go test -race ./pkg/...` | sucesso |
| cross-build sem CGO, Linux/macOS/Windows amd64/arm64 | sucesso |
| scaffold service: tidy/test/vet/build/doctor | sucesso |
| scaffold CLI: tidy/test/vet/build | sucesso; doctor sinaliza ausência de teste |
| resolução explícita da CLI | `ginger 1.4.4`; build info do módulo v1.4.4 |
| service `inspect --json` / `doctor` | saídas persistidas; exit 0 / 0 |
| CLI `inspect --json` / `doctor` | saídas persistidas; exit 0 / 1 esperado |
| comparação de Makefiles e GoReleaser | diferenças e hashes persistidos |
| inventário completo de `--plan` | parser auditado; previews sem alteração |
| harness Ginger original | 15 provas passaram, incluindo stream de 16 s |
| suíte `spikes/phase1` no Linux | 53 casos passaram em 5 packages; são 37 testes top-level catalogados, dos quais 1 é helper de subprocesso ignorado no processo pai |
| blackbox de controle Linux | execução nativa e 20 repetições passaram; SIGTERM também comprovou cleanup |
| controle Windows amd64 | suíte nativa completa passou no Windows 10 Pro 10.0.19045; `TEST_EXIT_CODE=0` |

Cobertura upstream observada para `health`, `middleware`, `router`, `sse` e
`ws` é 0%; `app` ficou em 48,6% e não cobre os caminhos de `Run` citados acima.
O Kube Peep mantém testes próprios para todos os comportamentos em que depende.

## Cobertura das tarefas da Fase 1

As evidências persistidas fecham especificamente F1-03, F1-04, F1-13, F1-14 e
F1-16: CLI v1.4.4 resolvida, dois scaffolds validados, Makefiles comparados,
suporte a `--plan` inventariado e saídas de `inspect`/`doctor` preservadas para
ambos.

Este relatório e o spike cobrem F1-03–04, F1-11–22, F1-27–35 e F1-39–44.
F1-44 foi fechado pelo blackbox nativo Linux e pela execução completa no
Windows amd64, persistida em
[`windows-native-2026-07-27.txt`](evidence/f1-control/windows-native-2026-07-27.txt).
Os cross-builds Windows arm64 continuam sendo somente prova de compilação. A
implementação de produção permanece tarefa F3 e os binários empacotados
retornam à matriz nativa na Fase 8.
