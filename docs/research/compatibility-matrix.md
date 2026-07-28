# Matriz de compatibilidade da Fase 1

Status: validada em 2026-07-27, incluindo probe nativo Linux e Windows

Esta matriz fixa a primeira combinação de dependências que satisfaz o requisito
de Go 1.25 e compila sem CGO. Ela não autoriza atualizações independentes dos
módulos Kubernetes: `api`, `apimachinery`, `client-go` e `metrics` devem avançar
sempre na mesma minor.

## Ambiente observado

| Item | Valor | Observação |
| --- | --- | --- |
| host | Ubuntu 24.04, kernel 7.0, linux/amd64 | ambiente local de descoberta |
| Go instalado | `go1.26.1` | usado apenas para comparar com o alvo |
| Go alvo validado | `go1.25.0` | testes e cross-builds executados com `GOTOOLCHAIN` |
| Node.js | `v24.18.0` | disponível para o frontend |
| npm | `11.16.0` | package manager disponível |
| Docker | `29.6.2` | disponível para o futuro harness Kind |
| kubectl | `v1.33.3` | cliente local; não fixa a versão das bibliotecas |
| kind | ausente | instalar antes do gate da Fase 4 |
| host CGO | habilitado | todos os cross-builds abaixo forçaram `CGO_ENABLED=0` |
| Windows nativo F1 | Windows 10 Pro 10.0.19045, amd64 | probe isolado de controle, não archive de produção |

## Matriz de CI fixada

Os labels evitam `*-latest` para que uma migração de imagem não altere o
ambiente silenciosamente. A imagem e versão efetivamente resolvidas serão
registradas pelo step `Set up job` de cada execução.

| Runner | Arquitetura | Uso |
| --- | --- | --- |
| `ubuntu-24.04` | x86_64 | verify, race, Kind e smoke Linux |
| `ubuntu-24.04-arm` | arm64 | smoke Linux arm64 quando disponível ao repositório |
| `macos-15-intel` | x86_64 | archive e smoke macOS Intel |
| `macos-15` | arm64 | archive e smoke macOS Apple Silicon |
| `windows-2025` | x86_64 | archive, lock/control, installer e smoke Windows |

Os labels e arquiteturas foram conferidos no
[catálogo oficial de runners GitHub-hosted](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)
e no
[inventário oficial das imagens](https://github.com/actions/runner-images).
Windows arm64 continua cross-build nesta matriz; só será declarado nativo se
um runner suportado for incorporado e executado.

## Dependências aprovadas

| Componente | Versão | `go` mínimo do módulo | Decisão |
| --- | --- | --- | --- |
| Ginger | `v1.4.4` | 1.22 | fixar exatamente; commit `6073543b6281be01e4bc97d001dd6e11512f70db` |
| Cobra | `v1.10.2` | 1.15 | aprovada para o entrypoint híbrido |
| `github.com/coder/websocket` | `v1.8.15` | 1.23 | transporte local endurecido de `exec`; `pkg/ws` foi rejeitado para esse caminho |
| `k8s.io/api` | `v0.35.7` | 1.25 | manter alinhada com os demais módulos Kubernetes |
| `k8s.io/apimachinery` | `v0.35.7` | 1.25 | manter alinhada |
| `k8s.io/client-go` | `v0.35.7` | 1.25 | última linha estável observada compatível com Go 1.25 |
| `k8s.io/metrics` | `v0.35.7` | 1.25 | opcional em runtime, mas alinhada no build |
| `modernc.org/sqlite` | `v1.54.0` | 1.25 | driver puro Go aprovado para o primeiro scaffold |

`client-go v0.36.3` foi deliberadamente rejeitado porque seu `go.mod` exige Go
1.26. A seleção de `v0.35.7` evita declarar Go 1.25 no projeto e depender
silenciosamente de um toolchain mais novo.

## Validações executadas

O módulo isolado está em `spikes/phase1`.

| Validação | Resultado |
| --- | --- |
| `go mod tidy` | sucesso |
| `go test ./...` com Go 1.26.1 | sucesso |
| `GOTOOLCHAIN=go1.25.0 go test ./...` | sucesso |
| `go test -race ./spike` | sucesso |
| binário `kubePeep` de prova lendo frontend e migration embutidos | sucesso |
| stream SSE real por 16 segundos | sucesso; recebeu o evento final |
| Linux amd64/arm64, Go 1.25, sem CGO | sucesso |
| macOS amd64/arm64, Go 1.25, sem CGO | sucesso |
| Windows amd64/arm64, Go 1.25, sem CGO | sucesso |
| controle nativo Linux amd64 | sucesso; blackbox e SIGTERM, blackbox repetido 20 vezes |
| controle nativo Windows amd64 | sucesso; suíte `control`, `TEST_EXIT_CODE=0` |

Comando reproduzível por alvo:

```bash
rtk proxy env \
  GOTOOLCHAIN=go1.25.0 \
  CGO_ENABLED=0 \
  GOOS=<linux|darwin|windows> \
  GOARCH=<amd64|arm64> \
  go build ./...
```

O pacote `compatibility` força o compilador a carregar clientsets Kubernetes,
`clientcmd`, Metrics API e o driver SQLite. Portanto, os resultados não são um
cross-build vazio. O mesmo comando também compila o entrypoint de prova
`cmd/kubePeep-spike`, preservando o casing do artefato.

## Evidência nativa do controle local

F1-44 foi exercitada com o probe isolado, separadamente nos dois sistemas:

| Plataforma | Evidência | Resultado |
| --- | --- | --- |
| Linux amd64 | [`linux-native-2026-07-27.txt`](evidence/f1-control/linux-native-2026-07-27.txt) | blackbox, SIGTERM e cleanup passaram; repetição `-count=20` passou |
| Windows 10 Pro amd64 | [`windows-native-2026-07-27.txt`](evidence/f1-control/windows-native-2026-07-27.txt) | blackbox, `LockFileEx`, fingerprint, DACL, controle autenticado e cleanup passaram; exit 0 |

Integridade:

- transcript Linux:
  `84dc8f74fc128e1add97fe71faf4ceca4c21543420a69cfe36e843493cda4aa4`;
- transcript Windows:
  `53b7e2eb49d2b5528aa0366d7b701da5d8de0b4d6ec05266125bfd9eafe31b00`;
- probe Windows executado:
  `3D8E3DCAC0CE72DCD039070F5BE2361630F7FEF090A97486409C35A07E2E12B8`;
- test binary Windows executado:
  `30D21E2367DB4F0BABC04CAC2F21392ED9EBEF505D92E899E38192D80409008F`.

Essa prova valida a decisão de F1, não uma implementação de produção. F3 deve
repetir o lifecycle com o módulo definitivo; F8 deve executar os archives,
instaladores, update e rollback nos runners nativos.

## Compatibilidade de kubeconfig

O loader oficial de `client-go v0.35.7` foi inspecionado e confirma:

- `ExplicitPath` carrega somente o arquivo informado pela flag;
- na ausência de flag, `KUBECONFIG` é dividido pela lista de paths nativa do
  sistema operacional, tem duplicados removidos e preserva precedência;
- sem `KUBECONFIG`, o fallback é o arquivo recomendado no diretório do usuário;
- múltiplos arquivos são mesclados com precedência documentada pelo próprio
  loader;
- caminhos relativos de certificados e chaves são resolvidos em relação ao
  arquivo de origem;
- `rest.Config` e o plugin oficial `client-go/plugin/pkg/client/auth/exec`
  suportam tokens ou certificados retornados por plugins `exec`.

Consequências para o Kube Peep:

1. persistir apenas a lista ordenada de paths, nunca o conteúdo dos arquivos;
   fingerprints de modificação permanecem transitórios em memória;
2. não copiar tokens, certificados ou chaves para SQLite;
3. executar plugins já declarados pelo kubeconfig no ambiente do usuário;
4. sanitizar erros de plugin antes de logar ou devolver ao frontend;
5. reconstruir o cliente quando qualquer arquivo da lista mudar.

## Limites desta evidência

- Cross-build continua provando somente compilação. O probe de `stop` e
  permissões passou nativamente no Windows amd64, mas código e artefatos de
  produção ainda precisam da revalidação F3/F8; macOS permanece sem runtime
  nativo nesta fase.
- Windows arm64 permanece cross-build; não há alegação de execução nativa.
- Docker está disponível, mas `kind` ainda não; RBAC real pertence à Fase 4.
- A versão Kubernetes deve ser reavaliada somente por mudança deliberada, com
  repetição da matriz completa.
- `modernc.org/sqlite` será validado novamente com migrations, WAL/SHM,
  concorrência e permissões na Fase 3.

## Cobertura do plano

Esta evidência cobre `F1-01`, `F1-03`, `F1-04`, `F1-23`, `F1-24`, `F1-25`,
`F1-35`, `F1-36`, `F1-38`, `F1-44` e a parte compilável de `F1-40`. Runtime
nativo do probe F1 e validação futura dos artefatos de produção permanecem
explicitamente separados.
