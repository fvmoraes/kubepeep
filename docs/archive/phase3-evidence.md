# Evidências da Fase 3 — Fundação

> Registro histórico: resultados e gates referem-se à execução datada abaixo.
> A sequência atual de entrega está no [plano v1](../../plan/README.md).

**Data da validação local:** 2026-08-10  
**Plataforma principal:** Linux amd64  
**Estado:** concluída; gates locais e workflow nativo Linux/macOS/Windows aprovados

## Escopo implementado

- binário Go 1.25 em foreground, com CLI Cobra e lifecycle próprio;
- composição Ginger v1.4.4 sem uso de `app.Run`;
- configuração YAML v1 estrita e diretórios privados multiplataforma;
- SQLite `modernc.org/sqlite` v1.54.0 sem CGO, migrations e backup/restore;
- health, status, sessão/CSRF e canal de controle local autenticado;
- frontend React/TypeScript/Vite embutido, com fallback SPA protegido;
- logging JSONL sanitizado e rotativo;
- Makefile, smoke test, cross-build e CI inicial.

## Dependências e toolchain

| Item | Evidência |
| --- | --- |
| Go | linguagem `go 1.25.0`; toolchain mínimo corrigido `go1.25.12` |
| Ginger | `github.com/fvmoraes/ginger v1.4.4` |
| Cobra | `github.com/spf13/cobra v1.10.2` |
| SQLite | `modernc.org/sqlite v1.54.0`, build e testes com `CGO_ENABLED=0` |
| Windows nativo | `golang.org/x/sys v0.46.0` para Known Folder, DACL, handles e locks |
| Node/npm | Node 24.18.0 e npm 11.16.0; lockfileVersion 3 |
| Frontend | versões exatas em `web/package.json`, sem ranges SemVer |

O React Router foi atualizado da baseline 7.18.2 para `react-router` 8.3.0.
A linha 7.x não possui correção para
[GHSA-qwww-vcr4-c8h2](https://github.com/advisories/GHSA-qwww-vcr4-c8h2),
enquanto 8.3.0 é a versão corrigida. As APIs declarativas/History API usadas pelo
SPA permaneceram compatíveis e a auditoria npm passou sem vulnerabilidades.

## Gates locais executados

| Gate | Resultado em 2026-08-03 |
| --- | --- |
| `go mod tidy` + `go mod verify` | passou |
| `go test -count=1` (pacotes do kubePeep) | todos os 19 pacotes do projeto passaram |
| `go test -race -count=1` | todos os pacotes aplicáveis passaram |
| `CGO_ENABLED=0 go test ./...` | passou |
| `go vet` | passou |
| `npm ci` | 254 pacotes instalados pelo lockfile |
| `npm audit` | zero vulnerabilidades |
| frontend format/lint/typecheck | passou |
| Vitest | 7 testes em 2 arquivos, passou |
| Playwright | 1 smoke E2E em Chromium, passou |
| Vite build | 1.890 módulos; JS 269,03 kB e CSS 7,83 kB |
| smoke do binário | lifecycle completo passou com `PATH` sem Node.js |
| cross-build CGO-free | Linux, macOS e Windows; amd64 e arm64 |
| `make verify` | passou pelos gates Go/frontend/build/smoke/Ginger; o E2E foi reexecutado e passou após alinhar o estado de erro esperado |
| `ginger doctor` | cinco checks passaram |
| `govulncheck` nos 19 pacotes do projeto | nenhuma vulnerabilidade alcançável com Go 1.25.12; o host 1.26.1 foi rejeitado por vulnerabilidades da standard library |
| black-box nativo Linux | build do binário + `start` → `status` → `stop` → `status`, passou |
| cross-compile dos testes Windows | `securefs` e `cli` amd64, passou |
| workflow nativo publicado | [run #9](https://github.com/fvmoraes/kubepeep/actions/runs/31392541114), commit `50988d8`: `build-and-test` e os dois jobs da matriz nativa macOS/Windows passaram; duração total 2m49s |

O smoke usa `HOME` temporário e ambiente vazio, inicia com `--no-browser`,
aguarda o canal autenticado, consulta `/health`, `/api/v1/status` e
`/api/v1/session`, executa `stop`, espera o processo e comprova a remoção de
`instance.json`. O binário não encontra Node.js no `PATH` durante todo o teste.

## Ginger inspect/doctor

`ginger doctor` passou em `ginger.yaml`, `go.mod`, tipo `service`, presença de
testes e `go vet`.

O `ginger inspect` mostra três diretórios convencionais ausentes, todos
intencionais:

- `internal/models`: DTOs da fundação pertencem a `internal/api`;
- `internal/repositories`: adapters/repositórios concretos começam com a
  integração Kubernetes da Fase 4;
- `tests`: testes Go permanecem colocados ao lado dos pacotes; E2E ganha harness
  próprio nas fases seguintes.

O detector do Ginger também não reconhece o datastore porque SQLite foi
integrado manualmente, como exigido pela decisão de não executar
`ginger add sqlite`. A dependência, o driver e os testes do adapter estão
presentes e auditados.

## Provas de segurança da fundação

- listener e clientes internos usam apenas `127.0.0.1` e dial target fixo;
- Host e Origin são comparados com o par exato publicado;
- sessão gera nonce em memória com TTL e geração de processo;
- rotas mutáveis exigem Origin, JSON e CSRF; CORS não é habilitado;
- API inexistente retorna envelope, request ID e `Cache-Control: no-store`;
- fallback SPA nunca captura API, health ou controle;
- `instance.json` contém token privado e identidade completa, sem arquivo de
  PID/porta/token paralelo;
- status/stop nunca sinalizam PID e exigem prova HTTP autenticada;
- logs usam allowlist, redaction por conteúdo e rotação 10 MiB/5 backups/14 dias;
- testes validam os defaults 10 MiB/5 backups/14 dias, forçam mais de cinco
  rotações, cobrem falha/recovery e inspecionam current/backups para JSONL e
  ausência de marcadores sensíveis;
- banco, sidecars, journal, backup e temporários existentes passam pelo scanner
  de corpus sintético;
- frontend de produção não usa Service Worker, Cache Storage, IndexedDB,
  storage Web ou persister TanStack;
- OpenTelemetry não é importado nem inicializado; por default não há exporter
  ou tráfego;
- workflow fixa actions externas por SHA completo.

## Portabilidade e residuais conhecidos

O adapter `securefs` usa no-follow, identidade de objeto/handle, publicação
no-replace, owner e modos privados no Unix; no Windows usa handles com
`OPEN_REPARSE_POINT`, DACL protegida/herdável limitada ao `TokenUser` e
`LockFileEx`. O `doctor` reutiliza os validadores reais em todas as plataformas,
sem retornar sucesso incondicional no Windows. O workflow inclui testes nativos
em macOS e Windows, teste adversarial de DACL/reparse e black-box do binário,
além do cross-build local.

Dois limites permanecem explícitos para revalidação contínua:

1. `database/sql` e o driver modernc podem reabrir o arquivo e sidecars por
   pathname ao criar conexões futuras. Os guards detectam substituições normais
   e a raiz `0700`/DACL reduz o atacante, mas eliminar um ataque ABA integral
   exigiria Connector/VFS próprio;
2. a prova de SID/DACL/reparse depende do job Windows nativo e deve permanecer
   na CI; em 2026-08-10 ela passou no run #9, enquanto a cross-compilação local
   continua sendo apenas a verificação adicional de portabilidade do build.

Esses limites não autorizam caminhos alternativos, permissões abertas nem
fallback inseguro. Qualquer falha observada continua fail-closed.

## Auditorias independentes

Três revisões independentes foram executadas durante a implementação:

- HTTP/contrato: encontrou e corrigiu fallback API, deadline não cooperativo,
  geração obsoleta, guards raw e logger opcional;
- dados/configuração: encontrou e corrigiu remoção prematura do backup,
  histórico não-prefixo, teste de persistência vácuo, classe do erro oversized e
  janelas de filesystem tratáveis;
- runtime/CLI: verificou lifecycle, locks, controle autenticado, cleanup e
  paths/DACL multiplataforma.

A auditoria final classificou inicialmente 46 tarefas como `PASS` e oito como
`PARTIAL`. Os oito gaps foram tratados no código local:

- cursor opaco e decoder estrito possuem testes próprios;
- frontend cobre loading, vazio, offline, erro HTTP, todas as rotas e 404;
- logging prova defaults, cinco backups e sanitização após rotação;
- `securefs` aplica/valida DACL por handle e possui testes Windows adversariais;
- lifecycle ganhou black-box do binário para runners nativos;
- `doctor` inspeciona DACL/tipos reais em vez de confiar no adapter;
- schema de observabilidade cobre tipos, sucesso, erro e ausência de contexto;
- shutdown integrado cobre raw ativo, timeout, hook falho, cleanup e erro
  observável convertido em exit code operacional.

A credencial local do GitHub CLI continuou retornando HTTP 401, então a
verificação foi feita pela página pública imutável do run. O run #9 concluiu
com sucesso e fechou F3-46 e F3-49 com execução nativa real.
