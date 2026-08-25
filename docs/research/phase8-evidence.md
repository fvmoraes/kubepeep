# Evidências da Fase 8 — Distribuição

**Data da validação local:** 2026-08-25
**Plataforma principal:** Linux amd64
**Estado:** 37 de 50 tarefas comprovadas localmente; Kind, Windows, matriz nativa/CI e candidate publicada pendentes

## Resultado local

A configuração de distribuição está pronta para seis alvos
Linux/macOS/Windows × amd64/arm64. GoReleaser v2.17.1, Go 1.25.13, Node
24.18.0 e npm 11.16.0 estão fixados. Os workflows de verificação e release são
separados, actions externas usam SHA completo e apenas o job de publicação
possui `contents: write`.

O updater, o instalador Unix e os caminhos compartilhados dos instaladores
possuem testes locais. Dois snapshots limpos e consecutivos produziram os seis
archives com checksums idênticos; cada archive contém exatamente licença,
README, avisos e binário, e o binário Linux rodou sem Node.js. Ainda não há
evidência dos archives atuais em runners macOS/Windows, do instalador
PowerShell em Windows ou do Kind dinâmico. Por isso a fase permanece aberta.

## Rastreabilidade da implementação

| Área | Evidência no repositório | Validação local |
| --- | --- | --- |
| Build | `go.mod`, `web/package-lock.json`, `Makefile`, embed e ldflags | testes/build Go, frontend build e seis cross-builds passaram |
| GoReleaser | `.goreleaser.yaml` | `goreleaser check` e dois snapshots v2.17.1 passaram; seis checksums foram idênticos e todos os binários registram Go 1.25.13, CGO desativado e `trimpath=true` |
| CI/release | `.github/workflows/verify.yml`, `release.yml` | YAML e actionlint passaram; workflows ainda não foram executados no commit atual |
| Instalador Unix | `install.sh`, `scripts/install_test.sh` | instalação limpa/upgrade, checksum, archives maliciosos/oversized, PATH, rollback, lock, uninstall e purge passaram |
| Instalador Windows | `install.ps1`, `scripts/install_test.ps1` | implementação e testes existem; execução em PowerShell/Windows permanece pendente |
| Update | `internal/updater`, `internal/cli/update.go` | versão exata, SHA-256, limites, archive allowlist, lock, troca/rollback Unix e ownership do helper Windows possuem testes; helper ainda precisa de execução nativa |
| Documentação/legal | `README.md`, `LICENSE`, `THIRD_PARTY_NOTICES.md` | instalação, RBAC, dados locais, update, remoção e troubleshooting documentados; licenças diretas auditadas |
| Kind | `test/kind` | shell/Python/YAML e RBAC passaram estaticamente; Docker indisponível impediu execução dinâmica |

## Gates locais executados

- Go 1.25.13: tidy/verify, suíte completa, race, vet, build e seis
  cross-builds passaram;
- `govulncheck` v1.7.0: zero vulnerabilidades alcançáveis;
- frontend: `npm audit` sem vulnerabilidades, lint, typecheck, build, 63
  Vitest e três Playwright passaram;
- Ginger v1.4.4: `version`, `inspect` e `doctor` passaram;
- instalador Unix: `./scripts/install_test.sh` passou;
- workflow/config: parse YAML, actionlint v1.7.12 e `goreleaser check`
  passaram;
- binário Linux extraído: `version` e `--help` passaram com ambiente mínimo e
  `PATH` sem Node.js;
- archives: checksums repetíveis, allowlist exata de quatro entradas, documentos
  byte a byte iguais à fonte, `trimpath` nos seis alvos e Gitleaks sem achados;
- diff staged: sem paths da máquina ou credenciais reais; os quatro alertas
  Gitleaks foram hashes públicos e fixtures sintéticos de WebSocket/redaction;
- harness Kind: `sh -n`, `dash -n`, parse Python/YAML e comando `static`
  passaram, sem acessar cluster.

## Tarefas ainda abertas

| Tarefas | Evidência aguardada |
| --- | --- |
| F8-20–F8-26 | `create`, `validate`, `kubeconfigs` e `app-e2e` no Kind com Docker ativo |
| F8-34 | execução nativa de `scripts/install_test.ps1`, incluindo PATH e upgrade |
| F8-36 | teste nativo do helper Windows com binário em uso, replace e rollback |
| F8-41–F8-42 | smoke dos archives e instaladores reais nos seis runners da matriz |
| F8-46 | fechamento dos 27 critérios e de todos os gates complementares |
| F8-48 | comandos publicados exercitados contra uma tag/candidate imutável |

## Reprodução pendente

```sh
./test/kind/harness.sh create
./test/kind/harness.sh validate
./test/kind/harness.sh kubeconfigs
./test/kind/harness.sh app-e2e ./kubePeep
```

Em Windows, executar `powershell.exe -NoProfile -ExecutionPolicy Bypass -File
scripts/install_test.ps1`. Depois, o workflow `verify.yml` deve validar os seis
runners nativos; somente uma tag exata aprovada pode acionar `release.yml`.
Nenhum run URL ou release inexistente foi registrado como evidência.
