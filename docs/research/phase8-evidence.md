# Evidências da Fase 8 — Distribuição

**Data da atualização:** 2026-08-30
**Plataformas:** Linux amd64 local; Linux, macOS e Windows nativos na CI
**Estado:** 43 de 50 tarefas comprovadas; recriação Kind do zero, CI nativa atual e candidate publicada pendentes

## Resultado local e nativo

A configuração de distribuição está pronta para seis alvos
Linux/macOS/Windows × amd64/arm64. GoReleaser v2.17.1, Go 1.26.7, Node
24.18.0 e npm 11.16.0 estão fixados. Os workflows de verificação e release são
separados, actions externas usam SHA completo e apenas o job de publicação
possui `contents: write`. O workflow #23 ainda usou Go 1.25.13; sua evidência
continua histórica e não é atribuída ao novo pin antes da CI correspondente.

O updater, o instalador Unix e os caminhos compartilhados dos instaladores
possuem testes locais. Um snapshot atual do GoReleaser v2.17.1 com Go 1.26.7
produziu os seis archives e o smoke do binário passou. O workflow de verificação
#23 acrescentou historicamente execução nativa do instalador e updater no
Windows, runtime no macOS e smoke dos seis archives, mas precisa ser repetido
com o novo toolchain.

No Kind local, `create` reutilizou o cluster canônico existente; `validate` e
`app-e2e ./dist/kubePeep` passaram. Isso fecha os cenários F8-21–F8-26, mas não
F8-20: a execução não removeu e recriou o cluster do zero.

## Rastreabilidade da implementação

| Área | Evidência no repositório | Validação local |
| --- | --- | --- |
| Build | `go.mod`, `web/package-lock.json`, `Makefile`, embed e ldflags | testes/build Go, frontend build e seis cross-builds passaram |
| GoReleaser | `.goreleaser.yaml` | snapshot v2.17.1 atual com Go 1.26.7 produziu seis archives; smoke do binário passou |
| CI/release | `.github/workflows/verify.yml`, `release.yml` | workflow #23 executado no HEAD: 10 de 11 jobs passaram; somente `restricted-kind` falhou |
| Instalador Unix | `install.sh`, `scripts/install_test.sh` | instalação limpa/upgrade, checksum, archives maliciosos/oversized, PATH, rollback, lock, uninstall e purge passaram |
| Instalador Windows | `install.ps1`, `scripts/install_test.ps1` | suíte PowerShell nativa passou no job `native-runtime (windows-latest)` do workflow #23 |
| Update | `internal/updater`, `internal/cli/update.go` | versão exata, SHA-256, limites, archive allowlist, lock, troca/rollback Unix e helper pós-exit Windows passaram, inclusive em execução nativa no workflow #23 |
| Documentação/legal | `README.md`, `LICENSE`, `THIRD_PARTY_NOTICES.md` | instalação, RBAC, dados locais, update, remoção e troubleshooting documentados; licenças diretas auditadas |
| Kind | `test/kind` | `create` reutilizou o cluster; `validate` e `app-e2e ./dist/kubePeep` passaram localmente, fechando F8-21–26; a recriação do zero e a CI permanecem pendentes |
| Segurança remota | `scripts/security_check.sh` e refs Git | após fetch/prune havia somente `origin/main`, nenhuma tag, e o gate sobre `origin/main` passou nos 25 commits alcançáveis |

## Workflow de verificação #23

O [run #23](https://github.com/fvmoraes/kubepeep/actions/runs/33289508322)
executou o commit `9d7c2ea`. O resultado observável foi 10 de 11 jobs verdes:

| Job | Resultado aceito |
| --- | --- |
| `build-and-test` | passou |
| `native-runtime (windows-latest)` | passou |
| `native-runtime (macos-latest)` | passou |
| `release-snapshot` | passou |
| seis jobs `native-archive-smoke` — Linux, macOS e Windows em amd64/arm64 | passaram |
| `restricted-kind` | falhou naquele run; a execução local posterior fecha F8-21–26, mas não F8-20 nem a CI atual |

Esse run permitiu fechar F8-34, F8-36 e F8-41 no HEAD executado. As três foram
reabertas depois da atualização do toolchain para Go 1.26.7: a prova permanece
registrada como histórica, mas não valida binários novos. A falha Kind daquele
run não foi reinterpretada como evidência parcial positiva; a evidência Kind
aceita agora vem da execução local posterior descrita abaixo.

## Atualização de segurança do toolchain

Em 2026-08-30, a [política oficial de segurança do Go](https://go.dev/doc/security/)
mantinha correções somente para as duas versões major mais recentes. Como o
[histórico oficial de releases](https://go.dev/doc/devel/release) já registrava
Go 1.27.0 e Go 1.26.7, a linha 1.25 deixou de ser uma base de build suportada.
Uma varredura com o host Go 1.26.1 também encontrou vulnerabilidades alcançáveis
da biblioteca padrão que o próprio banco marca como corrigidas em patches da
linha 1.26.

O contrato atual passa a ser linguagem Go 1.26.0 e toolchain exato Go 1.26.7.
O mesmo pin foi aplicado a `go.mod`, verificação, release e teste nativo de
processo. O achado do host desatualizado não é tratado como vulnerabilidade de
dependência resolvida por `go.sum`; binários e scanners devem executar com o
toolchain corrigido. A evidência desse novo pin exige as suítes locais e o CI do
commit que contém a atualização.

## Gates locais executados

- Go 1.26.7: o pacote Kubernetes runtime passou 48/48 testes normais e os
  mesmos 48/48 com race;
- `govulncheck` v1.7.0 com Go 1.26.7: zero vulnerabilidades alcançáveis;
- frontend: lint, typecheck, build, 73 Vitest e três Playwright passaram;
- GoReleaser v2.17.1: snapshot atual gerou os seis archives com Go 1.26.7;
- smoke do binário atual: passou;
- Ginger v1.4.4: `version`, `inspect` e `doctor` passaram;
- instalador Unix: `./scripts/install_test.sh` passou;
- workflow/config: parse YAML, actionlint v1.7.12 e `goreleaser check`
  passaram;
- evidência histórica Go 1.25.13: o binário Linux extraído passou `version` e
  `--help` com ambiente mínimo e `PATH` sem Node.js;
- evidência histórica Go 1.25.13: checksums repetíveis, allowlist exata de
  quatro entradas, documentos iguais à fonte, `trimpath` nos seis alvos e
  Gitleaks sem achados;
- harness Kind: além dos gates estáticos, `create` reutilizou o cluster e
  `validate`/`app-e2e ./dist/kubePeep` passaram;
- segurança remota: fetch/prune mostrou somente `origin/main`, nenhuma tag, e
  `scripts/security_check.sh origin/main` passou sobre 25 commits.

Para tornar a repetição idempotente, o harness valida ownership sem confundir
erro de API com ausência, reaplica o conjunto e recria sequencialmente o Pod de
`previous-log` e o Event `000-kp-warning`. Deletes de fixtures e RoleBindings
usam UID como precondição; cada substituição possui recuperação canônica armada
antes do DELETE. Sinais saem pelo trap de recuperação, respostas perdidas são
reconciliadas pela mesma UID, e restaurações não adotam objetos homônimos. A validação
comprovou os contratos fail-closed: autorização indisponível/no-opinion retorna
HTTP 503, negação autoritativa retorna HTTP 403 e dashboard parcialmente
autorizado permanece HTTP 200 com falhas isoladas.

O gate remoto não encontrou evidência de segredo nos refs alcançáveis nem ref
adicional que exija reescrita. Essa conclusão é limitada a `origin/main` e aos
25 commits inspecionados; não afirma purga de cache ou objeto inacessível ao
Git.

## Tarefas ainda abertas

| Tarefas | Evidência aguardada |
| --- | --- |
| F8-20 | remover e recriar do zero o cluster Kind canônico; a execução aceita reutilizou o cluster existente |
| F8-34, F8-36 e F8-41 | repetir instalador, updater e smokes nativos dos seis archives na CI com Go 1.26.7 |
| F8-42 | instaladores exercitados contra uma release candidate imutável, com checksum |
| F8-46 | fechamento dos 27 critérios e de todos os gates complementares |
| F8-48 | comandos publicados exercitados contra uma tag/candidate imutável |

## Reprodução observada e pendente

```sh
./test/kind/harness.sh create
./test/kind/harness.sh validate
./test/kind/harness.sh app-e2e ./dist/kubePeep
```

Esses comandos passaram localmente, mas `create` encontrou e reutilizou o
cluster. Para fechar F8-20, repetir `create` em ambiente comprovadamente sem o
cluster canônico. Os gates nativos de Windows, macOS e dos seis archives do
workflow #23 são históricos em Go 1.25.13; ainda é necessário repetir a CI com
Go 1.26.7. Somente uma tag exata aprovada pode acionar `release.yml`; os
instaladores publicados e seus nomes/casing continuam dependentes de uma
candidate imutável.
