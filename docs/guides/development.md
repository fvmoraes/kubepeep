# Desenvolvimento e organização

## Preparar o ambiente

A CI fixa Go 1.26.9, Node.js 24.18.0, npm 11.16.0, Ginger v1.4.4 e Wails
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
| `docs/research/`, `docs/archive/` | Pesquisa de arquitetura e índice de relatos preservados no Git |
| `scripts/` | Smoke, verificação de documentação e segurança |
| `scripts/release/` | Versionamento, classificação de checks e inspeção de artefatos |
| `test/install/`, `test/release/` | Harnesses de instaladores e distribuição com fixtures isoladas |
| `test/kind/` | Manifests sintéticos, integração e benchmarks Kubernetes |
| `packaging/linux/` | Manifesto nfpm, desktop entry e ícones dos pacotes Linux |
| `build/` | Ícones consumidos pelo Wails e saídas locais em `build/bin/` |
| `configs/` | Metadados do scaffold Ginger |
| `.github/`, `.githooks/` | Workflows e gates locais |
| `install.sh`, `install.ps1` | Entrypoints públicos de instalação |

Testes unitários ficam junto do código testado. `test/` reúne verificações que
atravessam processos, pacotes ou instalação; `web/e2e/` pertence ao frontend.
Os protótipos da Fase 1 foram removidos da árvore atual. O código histórico e
as instruções de reprodução permanecem acessíveis no
[registro de evidências](../research/evidence/f1-control/README.md).

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
A publicação exige quatro checks de
[verify.yml](../../.github/workflows/verify.yml), considerando a execução mais
recente de cada nome:

- `01 · Native runtime (Linux)`: build, testes Go/frontend/E2E, lint e segurança
  no Ubuntu.
- `01 · Native runtime (macOS)` e `01 · Native runtime (Windows)`: caminhos,
  permissões, locks, ciclo de vida e instaladores de cada sistema operacional.
- `02 · Kubernetes integration (Kind, restricted RBAC)`: cluster Kubernetes
  temporário com acesso limitado, operações permitidas/negadas, revogação,
  logs, streams e exec. Começa depois do check Linux e remove o cluster ao fim.

A numeração agrupa etapas; as três validações `01` executam em paralelo.
Os IDs dos jobs permanecem `build-and-test`, `native-runtime` e
`restricted-kind`. Ao renomear checks visíveis, atualizar também
`scripts/release/check-status.sh`, seus testes e eventuais regras de proteção
da branch no GitHub. Preservar os nomes dos artefatos.

`make test-release-gates` cobre classificação, scanner, versionamento,
preservação das notas e metadados nativos. Binários e pacotes passam pelo
scanner antes dos uploads e novamente antes da tag. Em `dry_run`, os jobs
recebem o SHA exato de origem e aplicam a versão calculada ao `wails.json`,
sem criar commit remoto. Notas já revisadas no `CHANGELOG.md` são preservadas.

O [empacotador DMG](../../scripts/release/create-dmg.sh) cria uma imagem HFS+
gravável com capacidade explícita: duas vezes o tamanho contabilizado do bundle
mais 64 MiB, com mínimo de 128 MiB. A contagem considera bytes lógicos e
alocados, inclusive arquivos esparsos, e margem por entrada do filesystem.
Depois de copiar o `.app`, cria ao lado o atalho `Applications` apontando para
`/Applications`, desmonta, converte para UDZO e verifica a imagem.
O log informa capacidade, espaço livre e etapa em caso de falha; uma falta de
espaço no host interrompe o processo antes da criação. Temporários são removidos
após desmontar o volume e um artefato existente só é substituído após validação.
Os testes portáveis cobrem capacidade e falhas; o check nativo macOS executa
[um teste real](../../test/release/dmg.sh) de criação/montagem com um bundle
esparso de 80 MiB, comparando conteúdo, permissão de execução, links internos
e o destino do atalho Applications na imagem final.

Para reproduzir uma versão escolhida: `make build build-desktop VERSION=0.12.0-dev`.
CLI e desktop recebem os mesmos ldflags de versão, commit e data. Builds de
desenvolvimento devem informar `VERSION` explicitamente; o exemplo não publica
uma release. Evidências de versões anteriores estão no
[arquivo](../archive/README.md) e não substituem os gates da árvore atual.

### Validação da experiência 0.12

A base de build atual usa Go 1.26.9, módulos Kubernetes alinhados em 0.36.5
e Helm 3.21.4. A matriz da Fase 1 em `docs/research` é histórica e não valida
essas versões em um cluster real; os gates abaixo continuam obrigatórios.

Os testes Go cobrem descoberta/Table/fallback dinâmicos, autorização e retries,
Gateway API, Helm, importação de kubeconfig, codificação e persistência após
reabrir SQLite. `go test -race ./...` acrescenta verificação de concorrência.
Vitest cobre estado, preferências, namespace, cache recente e editor.

Playwright executa o bundle de produção com fixtures sintéticas. Os arquivos
`resource-workspace.spec.ts`, `dynamic-resources.spec.ts`, `helm.spec.ts` e
`phase07.spec.ts` cobrem menus, detalhes, relações, YAML, colunas, contexto,
refresh e tamanhos de janela. Os cenários `phase02-real`/`phase03-real` são
opt-in e dependem de origem/cluster preparados; skips devem ser informados,
nunca apresentados como validação em cluster real. Testes Windows/macOS e
restricted Kind permanecem gates separados na CI.

### Matriz de cobertura da interface

Os testes abaixo exercitam comportamento visível e limites de autorização; a
existência de um teste não garante todas as combinações de dados, permissões ou
plataformas. As fixtures do navegador não substituem Kubernetes nem WebKit nativo.

| Superfície/opções | Cobertura automatizada |
| --- | --- |
| Catálogo completo do menu, reload e atalhos | `web/e2e/app.spec.ts`, `phase07.spec.ts` em oito tamanhos de janela e `Sidebar.test.tsx` |
| Inventários, detalhes e conteúdo das famílias | `resource-workspace.spec.ts`, `features.spec.ts`, `ResourcePages.test.tsx`, `AccessPages.test.tsx` |
| Tabelas: rolagem, paginação, retenção, filtros e identidade após refresh | `phase03.spec.ts`, `ui/DataTable.test.tsx` |
| Abas comuns, específicas e ordem alfabética | `ResourcePages.test.tsx`, `workspace/ResourceWorkspace.test.tsx`; conteúdo de Metrics, Investigation, Events, Containers, Rollout, relações, Rules e Backends em `detail-tabs.spec.ts` |
| YAML: autorização, identidade, conflitos, cores, undo e documento extenso | `workspace/ResourceYamlEditor.test.tsx`, `resource-workspace.spec.ts`, `helm.spec.ts` |
| Logs: follow, filtros, agregação, download, pausa e cancelamento | `LogsPage.test.tsx`, `workspace/PodLogsPanel.test.tsx`, `workspace/WorkloadLogsPanel.test.tsx`, `resource-workspace.spec.ts` |
| Ações: permissões, seleção, reinício, exclusão e port-forward | `ResourceActions.test.tsx`, `ResourcePages.test.tsx`, `resource-workspace.spec.ts`, `helm.spec.ts` e testes Go de actions |
| Pods: última execução por CronJob, histórico, RBAC e cache | `resources_job_runs_test.go`, `pods_test.go`, `resource-workspace.spec.ts` |
| Port-forward: substituir, abrir, parar, excluir e reativar | `PortForwardList.test.tsx`, `resource-workspace.spec.ts`, `portforward_replacement_test.go`; reabertura SQLite e migration 0005 em `preferences_test.go` |
| Escopos: criar/editar, reset, defaults, seleção, retry e cancelamento | `NamespaceScopeEditor.test.tsx`, `NamespaceScopeSelector.test.tsx`, `features.spec.ts`, `phase07.spec.ts`, `resource-workspace.spec.ts` |
| Contexto/cor: persistência, isolamento, respostas atrasadas e latência | `ContextSelector.test.tsx`, `api/selection.test.ts`, `resource-workspace.spec.ts` |
| Custom Resources: descoberta, salvar, reordenar, remover e links antigos | `dynamic-resources.spec.ts` e testes Go de recursos dinâmicos/preferências |
| Settings, idioma e preferências | `SettingsPage.test.tsx`, `SavedFilterControls.test.tsx`, `resource/columns.test.tsx` e testes Go de preferences |
| Events: referências, reconhecimento, seleção e novas ocorrências | `ResourcePages.test.tsx`, `navigation/events.test.ts`, `resource-workspace.spec.ts`, `internal/services/resources/events_test.go` |
| Aviso de release: versões, cache, falha, tempo e download | `internal/releases/checker_test.go`, `internal/api/handlers/releases_test.go`, `ReleaseNotice.test.tsx`, `resource-workspace.spec.ts` |
| DMG: capacidade, falhas e atalho Applications | `test/release/test_dmg.py`; montagem do arquivo final em macOS por `test/release/dmg.sh` |

Os testes `.test.tsx` estão em `web/src/components/`, salvo prefixos `api/` e
`navigation/` relativos a `web/src/`. Os arquivos `.spec.ts` estão em `web/e2e/`.

Para medir sem disputar CPU com outras suítes, execute com um worker:

```sh
cd web
npm run test:e2e -- e2e/phase03.spec.ts e2e/resource-workspace.spec.ts --workers=1 --grep '12k synthetic|context switching latency'
```

Esse ensaio registra linhas no DOM, tempo até a primeira linha, fluidez de
rolagem e latência de contexto com/sem cor. As respostas são simuladas; não
representam latência de um cluster real. Os probes reais exigem kubeconfig
dedicado e datasets isolados conforme [o harness Kind](../../test/kind/README.md).

Na medição local de 12/10/2026 (Chromium, bundle de produção, um worker),
reutilizar as opções dos filtros e estabilizar as chaves do virtualizador
elevou a rolagem do cenário sintético de 12 mil Pods de 35,6 para 58,0 FPS.
O percentil 95 do intervalo entre quadros caiu de 50 para 16,8 ms, com apenas
18 linhas montadas ao final da amostra. A troca de contexto levou 202–320 ms,
com resultados equivalentes com/sem cor e nenhuma consulta extra de
preferências. Esses números são evidência local, não um limite garantido para
outros equipamentos ou clusters; o teste de 12 mil itens em uma resposta
excede deliberadamente o limite real de 100 itens por página.

## O que versionar

Versionar código, testes, fixtures sintéticas, lockfiles do produto, migrations,
ícones necessários ao build, documentação e receitas de empacotamento.
Protótipos descartados e relatórios de versões antigas ficam no histórico Git,
com links por commit em [archive/](../archive/README.md). A documentação atual
mantém os contratos, ADRs e instruções de reprodução necessários ao produto.
Componentes e funções substituídos são removidos após conferir consumidores;
rotas de API ainda documentadas e testes de compatibilidade permanecem ativos.
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

### Evidência local da revisão 0.12 (12/10/2026)

- Go: 1.798 testes aprovados em 42 pacotes com `-race`; `go vet` sem problemas.
- Frontend: 325 testes Vitest aprovados; build e typecheck aprovados. ESLint sem
  erros, com dez avisos existentes de organização para Fast Refresh.
- Navegador: 150 cenários sintéticos aprovados, incluindo os 48 destinos do menu
  em oito tamanhos de janela. O teste de ordem de colunas foi atualizado para
  comparar posições relativas após a ordenação alfabética e reexecutado com
  sucesso; quatro cenários opt-in de cluster real permaneceram desativados.
- Execução isolada, um worker: tabela com 12 mil Pods manteve 16 linhas no DOM,
  60,0 FPS e p95 de 16,8 ms por frame. Com outros testes/processos simultâneos,
  a medição caiu para 21,8 FPS; os números isolados não são garantia de hardware.
- Contexto com cor: 203/307/301 ms; sem cor: 214/306/318 ms. Ambos fizeram zero
  leituras adicionais de preferências durante a troca nas fixtures do navegador.
- Agrupamento local de 10 mil Jobs: mediana 5,50 ms e 1,84 MiB por operação em
  Linux/amd64, Intel i7-1255U. O benchmark exclui rede, JSON e consulta Kubernetes.
- SQLite: migração 4→5, preservar preferências, salvar/remover portas e reabrir o
  banco passaram. Port-forward cobre substituição concorrente, mesma porta,
  limite global, autorização, cancelamento e fechamento no shutdown.
- Binário CLI 0.12.0: build e smoke isolado de start/status/HTTP/stop aprovados;
  instalador Unix, gates de release, empacotamento DMG portável e docs-check passaram.

Este host Linux não executou Finder/WebKit nativo macOS nem o instalador Windows.
O daemon Docker não estava disponível para os cenários de cluster real; esses
checks continuam separados na CI. Nenhum cluster pessoal foi usado na revisão.
