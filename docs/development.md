# Desenvolvimento e organização

## Preparar o ambiente

O baseline fixado na CI usa Go 1.26.7, Node.js 24.18.0, npm 11.16.0,
Ginger v1.4.4 e Wails v2.15.0. Consulte `go.mod`, `web/package.json` e os
workflows antes de atualizar ferramentas.

```sh
rtk git config --local core.hooksPath .githooks
rtk make web-install
rtk make verify
```

`make verify` cobre formato, lint, TypeScript, testes Go/Vitest, Playwright,
build web/Go, smoke e diagnóstico Ginger. A CLI Ginger precisa estar instalada
para `verify-ginger`; o Makefile usa o binário em `$(go env GOPATH)/bin`.
Instale os browsers de Playwright exigidos pela suíte conforme
[o workflow verify](../.github/workflows/verify.yml).

Para instalar Ginger na versão esperada: `rtk go install github.com/fvmoraes/ginger/cmd/ginger@v1.4.4`.

| Necessidade | Comando |
| --- | --- |
| Build CLI/web com assets embutidos | `rtk make build` |
| Frontend | `rtk make web-build` |
| Testes Go e Vitest | `rtk make test` |
| Race detector Go | `rtk make test-race` |
| E2E da interface | `rtk make test-e2e` |
| Execução desktop com reload | `rtk make dev-desktop` |
| Build desktop | `rtk make build-desktop` |
| Cross-build CLI | `rtk make cross-build` |
| Segurança do repositório | `rtk ./scripts/security_check.sh HEAD` |

O [guia desktop](desktop-build.md) detalha bibliotecas nativas e CGO.
O [harness Kind](../test/kind/README.md) valida RBAC e operações em cluster
efêmero. Builds de release e instaladores exigem ainda os testes nativos por
plataforma definidos em CI; um build local não os substitui.

## Layout

| Caminho | Conteúdo |
| --- | --- |
| `cmd/kubePeep/`, `main.go` | Entrypoints CLI e Wails; arquivos da raiz exigidos pelo build desktop |
| `internal/` | Core Go, adapters, serviços, API, lifecycle, migrations e embed |
| `web/` | Fonte React, configuração e testes do frontend |
| `docs/` | Documentação atual; ADRs em `decisions/`, pesquisa em `research/`, histórico em `archive/` |
| `plan/` | Referência UI/UX e fases executáveis da v1 |
| `scripts/` | Ferramentas de desenvolvimento, segurança, smoke e testes de instaladores |
| `test/kind/` | Manifests sintéticos e harness de integração |
| `spikes/phase1/` | Módulo isolado que reproduz decisões históricas de lifecycle |
| `build/`, `packaging/`, `configs/` | Fontes de empacotamento, ícones e configuração de ferramentas |
| `.github/`, `.githooks/` | Workflows e gates locais |
| `install.sh`, `install.ps1` | Entrypoints públicos de instalação, usados pelo workflow e pelos harnesses |

Licenças, README, manifestos de módulos e configuração das ferramentas
permanecem na raiz quando o consumidor exige esse caminho. Ao mover um
arquivo, atualizar referências em scripts, workflows, embed, testes e Markdown
no mesmo commit. Conferir os relacionamentos no Codebase MCP antes de mover
código; o grafo não substitui a validação dos caminhos literais.

`main.go`/`wails.json` são entradas do Wails; `go.mod`/`go.sum`, Makefile,
`ginger.yaml` e `skills-lock.json` seguem suas ferramentas. Os instaladores
permanecem na raiz como fontes públicas referenciadas por versão, testes e CI.
README, CHANGELOG e avisos legais também são consumidos pelo empacotamento.

`configs/app.yaml` é metadado do scaffold verificado pelo Ginger, não o arquivo
de configuração do aplicativo. O runtime lê o `config.yaml` privado do usuário
por `internal/config`. Os destinos `models` e `repositories` de `ginger.yaml`
são defaults do gerador; a aplicação usa suas camadas existentes. Não criar
diretórios vazios nem reorganizar serviços para satisfazer esses destinos.

Os cinco scripts de `scripts/` têm funções distintas: `release.sh` calcula
versões/notas/changelog; `security_check.sh` valida Git; `smoke.sh` verifica
o ciclo local; `install_test.sh`/`.ps1` testam os instaladores com fixtures.
O harness Kubernetes fica com seus manifests em `test/kind/`, e o runner
Windows do spike fica em `spikes/phase1/scripts/`.

## O que versionar

Versionar código, testes reproduzíveis, fixtures sintéticas mínimas, lockfiles,
schemas/migrations, ícones necessários ao build, documentação e receitas de
empacotamento. Testes são parte do produto; seus resultados são artefatos.

Manter fora do Git: `dist/`, `build/bin/`, `internal/web/dist/`, `node_modules`,
caches, cobertura, relatórios Playwright, screenshots de execução, releases,
binários, transcripts e logs. Resultados temporários ficam nos diretórios
ignorados da ferramenta, sem arquivos soltos na raiz. Evidências que precisam
ser preservadas ficam no projeto privado sob `~/.dwyt/projects/`; a documentação
registra método, resultado sanitizado e limitações, sem copiar saídas completas.

Perfis/traces de diagnóstico (`*.prof`, `*.pprof`, `*.trace`) também são locais.
Os intermediários `wails.json.tmp` e `CHANGELOG.md.new` são saídas do tooling
de release e ficam ignorados; os arquivos finais de metadados continuam
versionados. Não ignorar pastas genéricas de fontes apenas para esconder saídas.

Kubeconfigs, credenciais, tokens, chaves privadas, PII, caminhos específicos
da estação e bancos de runtime nunca são versionados. Memória e configuração
local de agentes pertencem a `~/.dwyt`, conforme as instruções do projeto.
Pacotes de skills em `.agents/skills/` ficam locais; `skills-lock.json` é apenas
o manifesto opcional, sem participação no build do produto.

`.gitignore` protege arquivos novos; um arquivo já rastreado precisa ser
retirado do índice após preservar qualquer dado privado necessário. Fontes de
empacotamento em `build/` continuam versionadas: ignorar somente suas saídas,
não o diretório inteiro. Bindings próprios da bridge Wails permanecem fonte;
runtimes gerados e não usados não precisam ser publicados no Git.

## Commit e publicação

**Regra de ouro: apenas commit; nunca push automático.** Publicar, criar
release remota ou executar um workflow que publica exige decisão explícita
da pessoa usuária. Um commit local não autoriza essas ações.

Antes de cada commit, executar `scripts/security_check.sh HEAD`, revisar o
diff e usar identidade GitHub noreply aprovada. Antes de um push explicitamente
autorizado, executar novamente o mesmo gate. Não usar `--no-verify` para
contornar falhas. A [premissa de segurança](security.md#11-repositório-e-cadeia-de-desenvolvimento)
define os bloqueios e a resposta a descobertas sensíveis.

Depois da reorganização, exigir `git status --short` vazio após o commit e
validar que artefatos locais continuam ignorados. Salvar decisões e o contexto
final no Obsidian MCP; os resultados detalhados permanecem privados.
