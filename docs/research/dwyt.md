# DWYT — pesquisa reproduzível para a Fase 1

Status: concluída em 2026-07-27

Escopo: somente o DWYT, como referência de estrutura, experiência local, frontend,
identidade visual, empacotamento e distribuição.

Repositório: [`fvmoraes/dwyt`](https://github.com/fvmoraes/dwyt)

Commit fixado:
[`a9386823272b928f2289c9020a9ae5951389e0f1`](https://github.com/fvmoraes/dwyt/commit/a9386823272b928f2289c9020a9ae5951389e0f1)

## Resumo executivo

O DWYT confirma que é viável entregar uma aplicação local como um único binário
Go contendo uma SPA React, iniciado por uma CLI que aguarda a API ficar pronta e
só então abre o navegador. A referência é especialmente útil para:

- organização do repositório e separação entre CLI, domínio, infraestrutura e web;
- build Vite diretamente para um diretório consumido por `go:embed`;
- linguagem visual compacta baseada em Catppuccin Mocha;
- instaladores para Unix e Windows, arquivos de release por plataforma e checksum;
- comandos locais de ciclo de vida e persistência de estado.

Ela não deve ser copiada literalmente. O backend usa Gin, a API e os componentes
centrais carregam regras de negócio do DWYT, o dashboard usa porta e URLs
absolutas fixas, o gerenciamento de PID é pouco defensivo e há lacunas relevantes
na cadeia de release. Para o Kube Peep, o valor está nos padrões e nas decisões
visuais, não no domínio.

## Proveniência, licença e limite de cópia

O checkout foi feito diretamente do repositório público e ficou em detached HEAD
no SHA exato solicitado. O commit declara a mensagem `feat(dashboard): refresh UI
with redesigned components and improved UX (#16)` e data de autoria
2026-06-23.

O projeto está sob licença
[MIT](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/LICENSE).
Portanto, trechos substanciais poderiam ser reutilizados com preservação do aviso
de copyright e da licença. Esta pesquisa adota um limite mais restritivo:

- reutilizar conceitos, organização e padrões genéricos;
- adaptar linguagem visual e experiência local;
- não copiar regras de negócio, textos, nomes, mascote, logotipo, endpoints,
  integrações ou componentes específicos do DWYT;
- implementar o backend do Kube Peep sobre Ginger, conforme o plano, sem portar
  handlers ou middleware Gin.

## Reprodução da inspeção

Checkout executado:

```bash
rtk mktemp -d -t kubepeep-dwyt.XXXXXX
rtk git clone --filter=blob:none --no-checkout \
  https://github.com/fvmoraes/dwyt.git \
  /tmp/kubepeep-dwyt.3uHg41/repo
rtk git -C /tmp/kubepeep-dwyt.3uHg41/repo \
  fetch --depth=1 origin a9386823272b928f2289c9020a9ae5951389e0f1
rtk git -C /tmp/kubepeep-dwyt.3uHg41/repo \
  checkout --detach a9386823272b928f2289c9020a9ae5951389e0f1
rtk git -C /tmp/kubepeep-dwyt.3uHg41/repo rev-parse HEAD
rtk git -C /tmp/kubepeep-dwyt.3uHg41/repo status --short
```

Resultado:

- `HEAD` igual a `a9386823272b928f2289c9020a9ae5951389e0f1`;
- árvore limpa antes e depois dos builds;
- indexação estrutural em modo rápido: 1.491 nós e 4.788 relações, incluindo
  93 arquivos Go, 21 TypeScript, 7 Bash, 2 HTML, 2 YAML, 1 JavaScript e 1 CSS.

Ambiente de validação:

- Linux x86-64;
- Go `go1.26.1`;
- Node.js `v24.18.0`;
- npm `11.16.0`;
- GoReleaser não estava instalado, portanto a configuração foi inspecionada,
  mas o release completo não foi executado.

## Inventário da estrutura

```text
dwyt/
├── .github/workflows/release.yml
├── core/
│   ├── main.go
│   ├── go.mod
│   ├── .goreleaser.yaml
│   ├── cmd/dwyt/cli/
│   ├── internal/
│   │   ├── server/
│   │   │   └── dashboard/dist/
│   │   ├── state/
│   │   ├── procutil/
│   │   ├── platform/
│   │   └── demais pacotes de domínio e integração
│   └── web/
│       ├── src/components/
│       ├── src/pages/
│       ├── package.json
│       └── vite.config.ts
├── install-lib/
├── install.sh
├── install.ps1
├── theme-preview.html
├── docs/
├── README.md
└── LICENSE
```

| Área | Responsabilidade observada | Evidência primária |
| --- | --- | --- |
| `core/main.go` | injeta versão e entrega a execução à CLI | [`main.go`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/main.go#L12-L24) |
| `core/cmd/dwyt/cli` | comandos, bootstrap do daemon e ciclo de vida | [`root.go`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/cmd/dwyt/cli/root/root.go#L37-L171) |
| `core/internal` | domínio, estado, processos, segurança, integrações e servidor | [`core/internal`](https://github.com/fvmoraes/dwyt/tree/a9386823272b928f2289c9020a9ae5951389e0f1/core/internal) |
| `core/web` | SPA React/TypeScript e configuração de build | [`core/web`](https://github.com/fvmoraes/dwyt/tree/a9386823272b928f2289c9020a9ae5951389e0f1/core/web) |
| `internal/server/dashboard/dist` | artefatos Vite versionados e incorporados ao binário | [`dashboard/dist`](https://github.com/fvmoraes/dwyt/tree/a9386823272b928f2289c9020a9ae5951389e0f1/core/internal/server/dashboard/dist) |
| `install-lib` | funções modulares do instalador Unix | [`install-lib`](https://github.com/fvmoraes/dwyt/tree/a9386823272b928f2289c9020a9ae5951389e0f1/install-lib) |
| `.github/workflows` e `.goreleaser.yaml` | geração de versão, tag, assets e release | [`release.yml`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/.github/workflows/release.yml), [`.goreleaser.yaml`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/.goreleaser.yaml) |

### Leitura para o Kube Peep

A separação `cmd` → `internal` → `web` é uma boa referência. O Kube Peep pode
manter um entrypoint mínimo, comandos em pacote próprio, casos de uso separados
dos adaptadores Kubernetes/Ginger e a SPA como unidade independente de build.
Os muitos pacotes de integração do DWYT refletem seu domínio e não devem orientar
o escopo funcional do Kube Peep.

## CLI, execução local e ciclo de vida

O fluxo principal observado é:

```text
dwyt [diretório]
  → detecta ambiente e diretório do projeto
  → consulta GET /api/health em 127.0.0.1:2737
  → se já existe daemon, troca o projeto ativo
  → senão, inicia o próprio executável com o comando oculto "daemon"
  → aguarda a API por até aproximadamente 3 segundos
  → abre o dashboard no navegador
```

Achados:

1. A raiz Cobra aceita no máximo um caminho e registra `stop`, `status`,
   `version`, `reinstall`, `uninstall`, `sync` e o comando oculto `daemon`
   ([`root.go:37-77`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/cmd/dwyt/cli/root/root.go#L37-L77)).
2. O navegador só é aberto depois da sondagem de saúde, o que evita uma aba
   prematura
   ([`root.go:79-171`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/cmd/dwyt/cli/root/root.go#L79-L171)).
3. O endereço do dashboard é fixo em `127.0.0.1:2737`; não há negociação de
   porta para esse processo
   ([`root.go:184-206`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/cmd/dwyt/cli/root/root.go#L184-L206)).
4. A abertura de URL é abstraída por plataforma: `open` no macOS,
   `cmd /c start` no Windows e `xdg-open` no Linux
   ([`platform.go`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/internal/platform/platform.go#L128-L153)).
5. O daemon e serviços gravam arquivos `run/*.pid`. Em Unix, a finalização envia
   `SIGTERM`, espera cerca de 3 segundos e escala para `SIGKILL`; em Windows usa
   `taskkill /F /T`
   ([`procutil.go`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/internal/procutil/procutil.go#L15-L79),
   [`procutil_unix.go`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/internal/procutil/procutil_unix.go#L23-L41),
   [`procutil_windows.go`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/internal/procutil/procutil_windows.go#L30-L36)).
6. O encerramento tem fallback Unix por `pkill -f`, inclusive para processos de
   ferramentas relacionadas
   ([`commands.go:165-181`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/cmd/dwyt/cli/root/commands.go#L165-L181)).
7. O diretório de dados é `~/.dwyt` em Unix e `%APPDATA%\dwyt` em Windows
   ([`detect.go:54-68`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/internal/detect/detect.go#L54-L68)).
8. `state.json` guarda projeto, processos, portas e erros. Há tentativa de
   backup em falha de gravação, mas a gravação principal não usa
   `write-temp + fsync + rename`
   ([`state.go`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/internal/state/state.go#L25-L56),
   [`state.go:232-250`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/internal/state/state.go#L232-L250)).

Implicações:

- reutilizar a sequência “iniciar → aguardar readiness → abrir navegador”;
- adaptar os comandos `start`, `stop`, `status` e `version` para semântica
  explícita e idempotente;
- escolher porta livre ou permitir configuração, mantendo bind exclusivo em
  loopback;
- não confiar apenas no número do PID: persistir identidade verificável do
  processo, porta e versão do protocolo;
- usar escrita atômica para estado e evitar `pkill` por padrão;
- implementar encerramento gracioso no Windows antes de qualquer término
  forçado.

## Servidor, embed e roteamento da SPA

O frontend compilado é embutido com `//go:embed dashboard/dist`
([`server.go:30`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/internal/server/server.go#L30)).
O servidor:

- cria um subfilesystem para `dashboard/dist`;
- ignora `/api` no middleware de arquivos;
- entrega assets encontrados com MIME apropriado;
- usa cache imutável de um ano para arquivos com hash em `assets/`;
- usa `no-cache` para `index.html`;
- devolve `index.html` para rotas desconhecidas da SPA;
- escuta somente em `127.0.0.1`
  ([`server.go:151-200`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/internal/server/server.go#L151-L200)).

A API aplica uma guarda local contra DNS rebinding e CSRF, validando `Host` e,
quando presente, `Origin` para `localhost` ou `127.0.0.1` na porta do daemon
([`routes.go:9-40`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/internal/server/routes.go#L9-L40)).

Esse desenho deve ser adaptado ao Ginger e ao prefixo `/api/v1`. O comportamento
de cache e fallback é reutilizável. Os handlers, tipos Gin e endpoints do DWYT
não são reutilizáveis.

## Frontend e build

### Stack encontrada

O [`package.json`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/web/package.json)
fixa a seguinte família tecnológica:

- React `^19.2.5` e React DOM `^19.2.5`;
- React Router DOM `^7.14.2`;
- TypeScript `~6.0.2`;
- Vite `^8.0.10`;
- Tailwind CSS `^4.2.4`, integrado pelo plugin Vite;
- ESLint;
- npm com `package-lock.json`.

Não existem TanStack Query, biblioteca de ícones, framework de testes de
frontend nem script `test`. Chamadas HTTP e EventSource são geridas manualmente.
O Kube Peep pode preservar a família React/TypeScript/Vite/Tailwind/Router, mas
deve adotar TanStack Query e Lucide conforme seu plano, além de testes de unidade
e integração do frontend.

### Organização e navegação

`App.tsx` usa `HashRouter` com rotas `/`, `/dashboard` e `/setup`
([`App.tsx:46-52`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/web/src/App.tsx#L46-L52)).
Há componentes pequenos e próprios (`Button`, `Toggle`, partes de card, logo,
sidebar) e páginas orientadas a fluxo (`Dashboard` e `SetupWizard`).

O Kube Peep não copiará essa escolha. A decisão fechada é React Router sobre a
History API, com URLs navegáveis sem fragmento. O servidor entrega primeiro
assets reais e usa `index.html` somente como fallback de GET/HEAD que aceita
HTML; `/api/v1`, `/health` e endpoints internos ficam excluídos, conforme o
spike de SPA.

Padrões úteis:

- botão com variantes semânticas, estado assíncrono e bloqueio contra clique
  duplicado
  ([`Button.tsx`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/web/src/components/Button.tsx#L1-L75));
- cartões com cabeçalho, linhas e divisores reaproveitáveis;
- sidebar contextual em painel sobreposto;
- estados de loading, vazio, alerta e progresso;
- grade de duas colunas que vira uma coluna em telas menores.

Limites:

- `Logo.tsx`, textos, cartões de ferramentas, métricas de tokens e o wizard são
  identidade e negócio do DWYT; não copiar;
- a API está hard-coded como `http://localhost:2737/api`, inclusive em
  `fetch` e `EventSource`
  ([`api.ts:1`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/web/src/api.ts#L1),
  [`Dashboard.tsx:136-165`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/web/src/pages/Dashboard.tsx#L136-L165));
- o Kube Peep deve usar URLs relativas sob `/api/v1`, permitindo porta
  dinâmica e mantendo frontend e API na mesma origem.

### Pipeline do embed

O [`vite.config.ts`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/web/vite.config.ts#L1-L12)
manda o build diretamente para
`core/internal/server/dashboard/dist` e limpa o diretório antes de gerar os
assets. No DWYT, esse diretório compilado está versionado. O workflow recompila,
cria um commit local com o resultado e faz a tag apontar para esse estado
([`release.yml:34-46`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/.github/workflows/release.yml#L34-L46)).

Para o Kube Peep:

1. instalar dependências de forma reprodutível;
2. executar lint, testes e build do frontend;
3. gerar o diretório de embed;
4. compilar e testar o binário Go;
5. não versionar os assets compilados se o pipeline consegue reproduzi-los.

## Identidade visual

A base é Catppuccin Mocha, com fundo ainda mais escuro:

| Papel | Valor no DWYT | Direção para o Kube Peep |
| --- | --- | --- |
| fundo | `#0c0c14` e gradiente `#0a0a11 → #11111b` | reutilizar |
| card | `rgba(30,30,46,.62)` para `rgba(17,17,27,.82)` | reutilizar/adaptar |
| borda | mauve com alpha `rgba(203,166,247,.14)` | reutilizar |
| texto | Catppuccin Text `#cdd6f4` | reutilizar |
| texto secundário | Overlay 1 `#7f849c` | reutilizar |
| primária DWYT | Yellow `#f9e2af` | substituir por Mauve |
| primária Kube Peep | Mauve `#cba6f7` | usar como ação e foco |
| sucesso | Green `#a6e3a1` | reutilizar |
| atenção | Yellow `#f9e2af` | manter somente como estado |
| erro | Red `#f38ba8` | reutilizar |
| informação | Sky `#89dceb` | reutilizar |

Os tokens completos e seus papéis estão em
[`index.css:3-51`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/web/src/index.css#L3-L51).

Outras características:

- tipografia monoespaçada: `SF Mono`, `Fira Code`, `Cascadia Code`, fallback
  `monospace`;
- base de 12 px, modo compacto de 11 px e botões de 9 px;
- cartões translúcidos, blur de 12 px, sombra discreta, raio de 8 px;
- pontos de status com glow;
- barra de progresso de 4 px;
- cartões compactos com 10 px de padding;
- quebra responsiva da grade abaixo de 768 px
  ([`index.css:53-110`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/web/src/index.css#L53-L110),
  [`index.css:224-226`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/web/src/index.css#L224-L226)).

O Kube Peep deve preservar densidade, contraste, cartões e cores semânticas,
mas criar nome, marca, ícone e ilustração próprios. O yellow deixa de ser cor
primária e volta a representar atenção. O mauve passa a concentrar ação, foco e
seleção.

## Instalação, atualização, release e remoção

### GoReleaser

A configuração v2 gera:

| Sistema | Arquitetura | Formato |
| --- | --- | --- |
| Linux | amd64, arm64 | `tar.gz` |
| macOS | amd64, arm64 | `tar.gz` |
| Windows | amd64 | `zip` |

Todos os builds usam `CGO_ENABLED=0`, `-trimpath` e injetam apenas
`main.version`; o release inclui `checksums.txt`
([`.goreleaser.yaml`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/.goreleaser.yaml#L1-L84)).

Lacunas a corrigir:

- `install.ps1` detecta ARM64 e solicita `dwyt_windows_arm64.zip`, mas o
  GoReleaser só produz Windows amd64
  ([`install.ps1:30-47`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/install.ps1#L30-L47),
  [`.goreleaser.yaml:59-79`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/.goreleaser.yaml#L59-L79));
- faltam commit, data de build e outros metadados úteis no binário;
- a validação local do GoReleaser não foi executada por ausência da ferramenta.

### Instalador Unix

`install.sh` usa `set -euo pipefail` e divide o trabalho em módulos de output,
plataforma, download, localização, configuração e conclusão
([`install.sh:9-24`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/install.sh#L9-L24)).
Ele procura binário local, tenta GitHub Releases e então faz fallback para um
binário na branch `main`. A instalação final usa arquivo temporário irmão,
`chmod` e `mv`, reduzindo o risco de deixar destino parcial
([`download.sh:14-22`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/install-lib/download.sh#L14-L22)).

Problemas:

- quando executado via pipe, baixa os módulos de `raw.githubusercontent.com`
  na branch `main`, sem pin por commit;
- se `checksums.txt` não puder ser baixado, não contiver o asset ou não houver
  ferramenta SHA-256, a verificação Unix retorna sucesso;
- o fallback para o binário da branch `main` não tem checksum
  ([`locate.sh:63-94`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/install-lib/locate.sh#L63-L94)).

O Kube Peep deve tornar SHA-256 obrigatório, falhar fechado e preservar
instalação atômica com rollback. Scripts, archives e `checksums.txt` serão
assets da mesma release `v${version}`; nenhum instalador usará
`raw/main`, branch mutável ou fallback sem versão/checksum.

### Instalador Windows

O PowerShell 5.1:

- detecta amd64/arm64;
- baixa o ZIP e `checksums.txt` do latest release;
- exige entrada correspondente e valida com `Get-FileHash`;
- instala em `%APPDATA%\dwyt\bin`;
- atualiza o `PATH` do usuário;
- executa `version` para verificar o binário
  ([`install.ps1:24-103`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/install.ps1#L24-L103)).

A verificação é mais segura que a variante Unix, mas a cópia usa
`Copy-Item -Force` diretamente no destino. O Kube Peep deve acrescentar staging,
troca/rollback e tratamento explícito de binário em uso. A versão será
parâmetro explícito e todos os downloads virão de
`releases/download/v${version}`, nunca de `latest`.

### Workflow de release

O único workflow:

- dispara em push para `main` quando mudam `core/**`, `install.sh` ou o próprio
  workflow; alterações em `install.ps1` e `install-lib/**` não o disparam;
- usa Go 1.25 e Node 22;
- constrói a SPA, cria um commit local dos assets, calcula versão por mensagens
  de commit, cria e publica uma tag;
- executa GoReleaser com `--skip=validate`;
- publica uma release não draft;
- usa actions referenciadas por tags (`@v4`, `@v5`, `@v6`), não por SHA
  ([`release.yml`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/.github/workflows/release.yml)).

Não há etapa de testes Go, lint/testes do frontend ou workflow de CI para pull
requests. A permissão `contents: write` vale para todo o job. Para o Kube Peep,
separar CI e publicação, exigir todos os gates, validar GoReleaser, fixar actions
por SHA e publicar apenas por tag ou aprovação explícita.

### Atualização e remoção

Não existe comando dedicado `update`. Há verificação de versão no dashboard,
um comando `reinstall` e instruções para executar novamente o instalador. Isso
não constitui um contrato de atualização atômica.

O `uninstall` do DWYT encerra processos e, além do próprio diretório, remove
dados e integrações de ferramentas relacionadas, links, configuração de shell
e `PATH`
([`commands.go:122-162`](https://github.com/fvmoraes/dwyt/blob/a9386823272b928f2289c9020a9ae5951389e0f1/core/cmd/dwyt/cli/root/commands.go#L122-L162)).
Esse alcance não deve ser copiado. A remoção do Kube Peep deve:

- distinguir binário, configuração, cache e dados do usuário;
- operar somente sobre caminhos pertencentes ao Kube Peep;
- preservar dados por padrão;
- pedir confirmação explícita para exclusão irreversível;
- ser idempotente e mostrar exatamente o que removeu.

## Validações executadas

| Comando | Resultado |
| --- | --- |
| `rtk go test ./...` em `core/` | sucesso: 127 testes em 27 pacotes |
| `rtk npm ci` em `core/web/` | sucesso: 167 pacotes; npm informou 6 vulnerabilidades, sendo 1 baixa e 5 altas |
| `rtk npm run build` | sucesso: 41 módulos; JS 292,57 kB (89,81 kB gzip), CSS 12,27 kB (3,70 kB gzip) |
| `rtk npm run lint` | sucesso |
| `rtk go build -trimpath -o /tmp/kubepeep-dwyt.3uHg41/dwyt-smoke .` | sucesso |
| `rtk proxy /tmp/kubepeep-dwyt.3uHg41/dwyt-smoke version` | `dwyt dev — Don't Waste Your Tokens` |
| `rtk git ... status --short` após os builds | sem diferenças rastreadas |

O smoke test HTTP do binário recém-compilado não pôde ser isolado porque
`127.0.0.1:2737` já estava ocupado. Nenhum processo existente foi encerrado ou
alterado. Essa limitação também evidencia o problema prático da porta fixa; não
é correto registrar o runtime HTTP do checkout como validado.

## Matriz de aproveitamento

| Elemento do DWYT | Classificação | Direção para o Kube Peep |
| --- | --- | --- |
| organização `cmd` / `internal` / `web` | Adaptar | manter fronteiras, reduzindo pacotes ao domínio Kubernetes |
| binário Go único com SPA embarcada | Reutilizar conceitualmente | build web antes do Go e `go:embed` |
| fallback SPA e cache de assets com hash | Adaptar | implementar no Ginger e excluir `/api/v1` |
| bind exclusivo em loopback | Reutilizar | manter `127.0.0.1`; validar Host e Origin |
| readiness antes de abrir navegador | Reutilizar | usar health/readiness real e timeout configurável |
| porta fixa `2737` | Substituir | porta configurável ou descoberta segura de porta livre |
| PID files simples e fallback `pkill -f` | Substituir | identidade verificável, shutdown idempotente e gracioso |
| `state.json` persistente | Adaptar | schema próprio, escrita atômica e recuperação explícita |
| React + TypeScript + Vite + Tailwind + Router | Reutilizar a família | acrescentar TanStack Query, Lucide e testes |
| `HashRouter` | Não copiar | usar History API e fallback SPA protegido, com teste de deep link |
| URLs absolutas `localhost:2737/api` | Não copiar | usar mesma origem e `/api/v1` relativo |
| componentes pequenos e próprios | Reutilizar o padrão | criar primitives acessíveis do Kube Peep |
| cartões e wizard específicos | Não copiar | modelar workloads, pods, eventos e logs do Kube Peep |
| Catppuccin Mocha, densidade e status | Adaptar | primary mauve; yellow apenas para atenção |
| logo, mascote, nome e textos DWYT | Não copiar | identidade original do Kube Peep |
| backend e middleware Gin | Não copiar | implementar diretamente com Ginger |
| endpoints e regras de negócio DWYT | Não copiar | casos de uso próprios e API contratada do Kube Peep |
| scripts Unix modulares | Adaptar | assets da tag exata, checksum obrigatório e rollback; nunca `raw/main` |
| instalador PowerShell | Adaptar | alinhar matriz, staging e binário em uso |
| GoReleaser multiplataforma | Adaptar | matriz coerente, metadata e validação |
| workflow que publica em todo push | Substituir | CI em PR e release controlada por tag/aprovação |
| assets compilados versionados | Substituir | gerar no pipeline e manter fora do Git |
| remoção de dados de ferramentas relacionadas | Não copiar | limitar ao namespace e aos dados do Kube Peep |

## Riscos e pendências para as próximas fases

Prioridade alta:

1. implementar o contrato já aceito de bind real, prontidão e controle de
   instância;
2. revalidar no módulo de produção o lock/identidade/stop comprovado pelo probe
   nativo Linux/Windows da Fase 1;
3. implementar segurança de loopback no Ginger, incluindo Host, Origin e
   proteção das operações mutáveis;
4. publicar scripts e artefatos somente pela tag exata, com checksums
   obrigatórios e matriz alinhada;
5. criar gates de testes antes de qualquer publicação.

Prioridade média:

1. testar deep links da History API contra o fallback SPA protegido;
2. implementar e testar o schema e a escrita atômica já definidos para o
   estado local;
3. especificar atualização e rollback do binário;
4. executar auditoria e atualização das dependências npm;
5. estabelecer tokens visuais próprios e componentes acessíveis.

## Cobertura das tarefas da Fase 1

| Tarefa | Evidência produzida |
| --- | --- |
| F1-02 | licença MIT confirmada e fronteira explícita contra cópia de negócio |
| F1-05 | árvore e responsabilidades do repositório inventariadas |
| F1-06 | embed, SPA fallback, bind, porta, browser, PID, estado e shutdown documentados |
| F1-07 | stack, organização, navegação, lacunas de dados e testes documentadas |
| F1-08 | paleta, tipografia, densidade, componentes e adaptação do accent registradas |
| F1-09 | GoReleaser, Actions, instaladores, atualização e remoção auditados |
| F1-10 | matriz reutilizar/adaptar/substituir/não copiar concluída |

## Conclusão

O DWYT é uma referência forte para a forma do produto: CLI curta, processo
local, SPA embarcada, UI compacta e distribuição multiplataforma. Não é uma
base de código a ser transplantada. A implementação do Kube Peep preserva esses
atributos de experiência, mas usa foreground no MVP e substitui Gin por Ginger,
URLs/porta fixas por um contrato local robusto, ciclo de vida por um mecanismo
verificável e a cadeia de release por uma pipeline com checksums obrigatórios e
gates completos.

Nenhum arquivo de `plan/`, `docs/decisions/` ou código do produto foi alterado
por esta pesquisa.
