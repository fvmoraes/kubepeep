# Evidências da Fase 8 — Distribuição

> Registro histórico: resultados e gates referem-se à execução datada abaixo.
> A sequência atual de entrega está no [plano v1](../../plan/README.md).

**Data da atualização:** 2026-08-30
**Plataformas:** Linux amd64 local; Linux, macOS e Windows nativos na CI
**Estado:** 47 de 50 tarefas comprovadas; somente candidate imutável e
fechamento integral dos gates permanecem pendentes

## Resultado local e nativo

A configuração de distribuição está pronta para seis alvos
Linux/macOS/Windows × amd64/arm64. GoReleaser v2.17.1, Go 1.26.7, Node
24.18.0 e npm 11.16.0 estão fixados. Os workflows de verificação e release são
separados, actions externas usam SHA completo e apenas o job de publicação
possui `contents: write`. O workflow #24 executou o pin atual Go 1.26.7 no head
exato `4ce996b40914f729aefa8d949bde85f94873c8d9` e concluiu os 11 jobs com
sucesso. O workflow #23, em Go 1.25.13, continua descrito apenas como histórico.

O updater, o instalador Unix e os caminhos compartilhados dos instaladores
possuem testes locais. Um snapshot atual do GoReleaser v2.17.1 com Go 1.26.7
produziu os seis archives e o smoke do binário passou. O workflow de verificação
#24 repetiu a prova no toolchain atual: runtime nativo do instalador/updater em
Windows, runtime nativo em macOS, snapshot e smoke dos seis archives em runners
Linux/macOS/Windows × amd64/arm64.

No Kind local anterior, `create` reutilizou o cluster canônico existente;
`validate` e `app-e2e ./dist/kubePeep` passaram e fecharam F8-21–F8-26, mas não
F8-20. A lacuna foi fechada na CI #24: o job `restricted-kind` rodou em runner
efêmero inicialmente vazio e passou `create`, `validate`, `kubeconfigs` e
`app-e2e`; o workflow executou `kind delete cluster` ao final, comprovando a
criação do zero e a remoção posterior.

## Rastreabilidade da implementação

| Área | Evidência no repositório | Validação local |
| --- | --- | --- |
| Build | `go.mod`, `web/package-lock.json`, `Makefile`, embed e ldflags | testes/build Go, frontend build e seis cross-builds passaram |
| GoReleaser | `.goreleaser.yaml` | snapshot v2.17.1 atual com Go 1.26.7 produziu seis archives; smoke do binário passou |
| CI/release | `.github/workflows/verify.yml`, `release.yml` | workflow #24 executado no head `4ce996b`: 11 de 11 jobs passaram |
| Instalador Unix | `install.sh`, `scripts/install_test.sh` | instalação limpa/upgrade, checksum, archives maliciosos/oversized, PATH, rollback, lock, uninstall e purge passaram |
| Instalador Windows | `install.ps1`, `scripts/install_test.ps1` | suíte PowerShell nativa passou no job `native-runtime (windows-latest)` do workflow #24 |
| Update | `internal/updater`, `internal/cli/update.go` | versão exata, SHA-256, limites, archive allowlist, lock, troca/rollback Unix e helper pós-exit Windows passaram, inclusive em execução nativa no workflow #24 |
| Documentação/legal | `README.md`, `LICENSE`, `THIRD_PARTY_NOTICES.md` | instalação, RBAC, dados locais, update, remoção e troubleshooting documentados; licenças diretas auditadas |
| Kind | `test/kind` | histórico local: reutilização + `validate`/`app-e2e`; prova atual: CI #24 criou do zero em runner efêmero, passou `create`/`validate`/`kubeconfigs`/`app-e2e` e executou `kind delete cluster` ao final, fechando F8-20–26 |
| Segurança remota | `scripts/security_check.sh` e refs Git | após o push de `6b96a41`, fetch mostrou somente `origin/main`, nenhuma tag, e o gate passou nos 28 commits alcançáveis por esse ref |

## Workflow de verificação #24 — evidência atual

O [run público #24](https://github.com/fvmoraes/kubepeep/actions/runs/33295350787)
foi disparado por push e executou o head completo
`4ce996b40914f729aefa8d949bde85f94873c8d9`. O GitHub registrou conclusão
`success`; os 11 jobs ficaram verdes:

| Job | Resultado aceito |
| --- | --- |
| `build-and-test` | passou |
| `native-runtime (windows-latest)` | passou |
| `native-runtime (macos-latest)` | passou |
| `release-snapshot` | passou |
| `restricted-kind` | criou o cluster do zero em runner efêmero, passou `create`, `validate`, `kubeconfigs` e `app-e2e`; o workflow executou `kind delete cluster` ao final |
| `native-archive-smoke (linux, amd64)` | passou |
| `native-archive-smoke (linux, arm64)` | passou |
| `native-archive-smoke (darwin, amd64)` | passou |
| `native-archive-smoke (darwin, arm64)` | passou |
| `native-archive-smoke (windows, amd64)` | passou |
| `native-archive-smoke (windows, arm64)` | passou |

Essa evidência fecha F8-20, F8-34, F8-36 e F8-41. Ela também fecha o critério
MVP-23, porque o pipeline principal do head analisado terminou integralmente
verde. Não fecha F8-42, F8-46, F8-48 nem MVP-26: snapshot e archives de CI não
substituem instaladores executados pelos caminhos públicos de uma candidate
imutável.

## Workflow de verificação #23 — histórico preservado

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

Esse run permitiu fechar F8-34, F8-36 e F8-41 no HEAD então executado. As três
foram reabertas depois da atualização do toolchain para Go 1.26.7. A prova
permanece histórica e não valida binários novos; as tarefas só voltaram a ser
fechadas pela CI #24. A falha Kind daquele run não foi reinterpretada como
evidência parcial positiva: F8-20 foi fechada exclusivamente pelo
`restricted-kind` verde e efêmero da CI #24.

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
- frontend do head `4ce996b`: lint, typecheck, build, 73 Vitest e três
  Playwright passaram na CI #24;
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
- harness Kind local anterior: além dos gates estáticos, `create` reutilizou o
  cluster e `validate`/`app-e2e ./dist/kubePeep` passaram;
- harness Kind atual: a CI #24 criou o cluster em runner efêmero vazio, executou
  validação, kubeconfigs e E2E do aplicativo, e chamou `kind delete cluster` ao
  final;
- segurança remota: após o push de `6b96a41`, fetch mostrou somente
  `origin/main`, nenhuma tag, e `scripts/security_check.sh origin/main` passou
  sobre os 28 commits então alcançáveis.

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
adicional que exija reescrita. Essa conclusão é limitada ao snapshot
`origin/main@6b96a41` e aos 28 commits então inspecionados; não afirma purga de
cache ou objeto inacessível ao Git.

A ausência de dados sensíveis é premissa não negociável, não apenas resultado
de uma varredura: kubeconfigs, credenciais, tokens, chaves privadas, conteúdo
de Secret, logs de aplicações, bancos locais, PII privada e paths específicos
da máquina nunca podem ser enviados ao repositório, logs ou artefatos da CI.
As evidências registram somente resultados sanitizados e identificadores
públicos do próprio projeto.

## Tarefas ainda abertas

| Tarefas | Evidência aguardada |
| --- | --- |
| F8-42 | instaladores exercitados contra uma release candidate imutável, com checksum |
| F8-46 | fechamento dos 27 critérios e de todos os gates complementares |
| F8-48 | comandos publicados exercitados contra uma tag/candidate imutável |

## Reprodução observada e pendente

```sh
./test/kind/harness.sh create
./test/kind/harness.sh validate
./test/kind/harness.sh app-e2e ./dist/kubePeep
```

Esses comandos passaram localmente, mas aquela execução de `create` encontrou e
reutilizou o cluster. O `restricted-kind` da CI #24 eliminou essa pendência ao
executar a sequência completa em runner efêmero. Os gates nativos de Windows,
macOS e dos seis archives também foram repetidos com Go 1.26.7 na CI #24. O run
#23 permanece apenas como histórico do pin anterior. Somente uma tag exata
aprovada pode acionar `release.yml`; os instaladores publicados, o checksum e
os nomes/casing dos assets continuam dependentes de uma candidate imutável.
