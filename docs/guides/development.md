# Desenvolvimento e organização

## Preparar o ambiente

A CI fixa Go 1.26.7, Node.js 24.18.0, npm 11.16.0, Ginger v1.4.4 e Wails
v2.15.0. Consulte `go.mod`, `web/package.json` e os workflows antes de atualizar
ferramentas. Python 3.9+ é usado pelo verificador de documentação e pelos harnesses
Kind; não é dependência do aplicativo distribuído.

Na raiz do repositório:

```sh
git config --local core.hooksPath .githooks
make web-install
make verify
```

`make verify` cobre links da documentação, formato, lint, TypeScript, testes
Go/Vitest, Playwright, build web/Go, smoke, diagnóstico Ginger, instalador Unix
e gates de release. Instale os browsers de Playwright conforme o
[workflow verify](../../.github/workflows/verify.yml).

A CLI Ginger precisa estar instalada para `verify-ginger`:
`go install github.com/fvmoraes/ginger/cmd/ginger@v1.4.4`.
O Makefile usa o binário em `$(go env GOPATH)/bin`.

| Necessidade | Comando |
| --- | --- |
| Links e âncoras da documentação | `make docs-check` |
| Build CLI/web com assets embutidos | `make build` |
| Frontend | `make web-build` |
| Testes Go e Vitest | `make test` |
| Race detector Go | `make test-race` |
| E2E da interface | `make test-e2e` |
| Testes do instalador Unix | `make test-install` |
| Testes dos gates de release | `make test-release-gates` |
| Execução desktop com reload | `make dev-desktop` |
| Build desktop | `make build-desktop` |
| Cross-build CLI | `make cross-build` |
| Segurança do repositório | `./scripts/security_check.sh HEAD` |

O [guia desktop](desktop-build.md) detalha bibliotecas nativas e CGO.
O [harness Kind](../../test/kind/README.md) valida RBAC e operações em cluster
efêmero. Sua execução depende de um ambiente explicitamente preparado e não
faz parte de `make verify`. Os testes PowerShell do instalador são executados
no runner Windows, com `./test/install/windows.ps1` a partir da raiz.
Um build local não substitui os testes nativos das demais plataformas.

## Layout

| Caminho | Responsabilidade |
| --- | --- |
| `cmd/kubePeep/` | Entrypoint CLI; lógica em `internal/` |
| `main.go`, `wails.json` | Entrypoint e configuração exigidos pelo Wails na raiz |
| `internal/` | Core Go, composição, adapters, serviços, API, lifecycle, migrations e embed |
| `web/` | Fonte React, configuração, testes unitários e E2E do frontend |
| `docs/guides/` | Instruções de desenvolvimento e operação |
| `docs/architecture/`, `docs/decisions/` | Arquitetura atual, design system e ADRs |
| `docs/reference/` | Contratos de produto, API, segurança, RBAC e protocolo |
| `docs/research/`, `docs/archive/` | Pesquisa reproduzível e relatos históricos por versão |
| `scripts/` | Smoke, verificação de documentação e segurança |
| `scripts/release/` | Versionamento, classificação de checks e inspeção de artefatos |
| `test/install/`, `test/release/` | Harnesses de instaladores e distribuição com fixtures isoladas |
| `test/kind/` | Manifests sintéticos, integração e benchmarks Kubernetes |
| `spikes/phase1/` | Módulo histórico isolado que reproduz decisões de lifecycle |
| `packaging/linux/` | Manifesto nfpm, desktop entry e ícones dos pacotes Linux |
| `build/` | Ícones consumidos pelo Wails e saídas locais em `build/bin/` |
| `configs/` | Metadados do scaffold Ginger |
| `.github/`, `.githooks/` | Workflows e gates locais |
| `install.sh`, `install.ps1` | Entrypoints públicos de instalação |

Testes unitários ficam junto do código testado. `test/` reúne verificações que
atravessam processos, pacotes ou instalação; `web/e2e/` pertence ao frontend.
O spike continua isolado porque seus probes são citados pelos ADRs e pesquisas;
não é dependência do módulo de produção nem parte da CI atual.

Os dois pacotes `internal/app` e `internal/application` têm responsabilidades
distintas: o primeiro controla o bootstrap CLI, o segundo compõe o core
compartilhado com o desktop. Mudar a estrutura não exige fundir essas camadas.

Licenças, README, manifestos de módulos e configurações exigidas por ferramentas
permanecem na raiz. `docs/download.md` e os instaladores preservam os caminhos
públicos usados por releases existentes. README, CHANGELOG e avisos legais
são consumidos pelo empacotamento. Ao mover uma fonte, atualizar seus imports,
scripts, workflows, fixtures, embeds e links no mesmo conjunto de alterações.

`configs/app.yaml` é metadado verificado pelo Ginger. A configuração real do
aplicativo é o `config.yaml` privado lido por `internal/config`. Os destinos
`models` e `repositories` de `ginger.yaml` são defaults do gerador; não criar
camadas vazias para reproduzi-los.

## Release e validação

[release.yml](../../.github/workflows/release.yml) organiza os jobs em
`01 Prepare` → `02 Build` (Linux/Windows/macOS) → `03 Publish` → `04 Latest`.
A publicação exige `build-and-test`, `restricted-kind` e as duas pernas
`native-runtime (macos-latest)` / `native-runtime (windows-latest)` de
[verify.yml](../../.github/workflows/verify.yml), considerando a execução mais
recente de cada nome. Preservar nomes de checks, IDs e nomes dos artefatos.

`make test-release-gates` cobre classificação, scanner, versionamento,
preservação das notas e metadados nativos. Binários e pacotes passam pelo
scanner antes dos uploads e novamente antes da tag. Em `dry_run`, os jobs
recebem o SHA exato de origem e aplicam a versão calculada ao `wails.json`,
sem criar commit remoto. Notas já revisadas no `CHANGELOG.md` são preservadas.

Para reproduzir uma versão escolhida: `make build build-desktop VERSION=0.7.0`.
CLI e desktop recebem os mesmos ldflags de versão, commit e data. Builds de
desenvolvimento usam `0.7.0-dev`. Evidências de versões anteriores estão no
[arquivo](../archive/README.md) e não substituem os gates da árvore atual.

## O que versionar

Versionar código, testes, fixtures sintéticas, lockfiles do produto, migrations,
ícones necessários ao build, documentação e receitas de empacotamento.
Manter fora do Git binários, `dist/`, `build/bin/`, `internal/web/dist/`,
`node_modules`, caches, cobertura, relatórios Playwright, capturas, releases,
transcripts e logs. Não ignorar pastas de fonte para esconder saídas de build.

Perfis (`*.prof`, `*.pprof`, `*.trace`), `wails.json.tmp`, `CHANGELOG.md.new` e
`web/package.json.md5` são artefatos locais. Bindings próprios da bridge Wails
continuam fonte; os builds usam `-skipbindings` para preservá-los.

Kubeconfigs, credenciais, tokens, chaves privadas, PII, caminhos da estação e
bancos de runtime nunca são versionados. Configurações, skills, hooks e
instruções de IA também ficam locais: `.codex/`, `.impeccable/`, `.agents/`,
`.kiro/`, `AGENTS.md` e `skills-lock.json`, entre outros, estão no `.gitignore`
e não participam do build. Resultados detalhados de validação ficam privados;
a documentação registra método, resultado sanitizado e limitações.

Um arquivo já rastreado precisa ser retirado do índice para que o `.gitignore`
passe a protegê-lo, preservando qualquer dado local necessário.

## Commit e publicação

**Regra de ouro: apenas commit; nunca push automático.** Publicar, criar
release remota ou executar workflow de publicação exige decisão explícita
da pessoa usuária. Um commit local não autoriza essas ações.

Antes de um commit e de um push autorizado, executar
`./scripts/security_check.sh HEAD`, revisar o diff e usar identidade GitHub
aprovada em [segurança](../reference/security.md#11-repositório-e-cadeia-de-desenvolvimento).
Não usar `--no-verify` para contornar falhas. Conferir também que os arquivos
locais continuam ignorados e que não há referências aos caminhos removidos.
