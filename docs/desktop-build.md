# Guia de Build Desktop (Wails v2)

## 1. Visão geral

O binário desktop compartilha o mesmo núcleo do binário web
(`internal/application.Compose`): a diferença é a tag `desktop`, que compila a
shell Wails (`internal/desktop/wails`) servindo o frontend embutido por
`wails://wails` com bridge in-process para streams. O frontend é o mesmo
`internal/web/dist` embutido por `go:embed` — o build desktop pode pular o
passo de frontend quando o dist já está atualizado.

## 2. Build nativo (requer GTK/WebKit)

Requisitos Linux: `libgtk-3-dev`, `libwebkit2gtk-4.0-dev`, `pkg-config`,
`gcc` (CGO), Go 1.26+ e Wails CLI (`go install
github.com/wailsapp/wails/v2/cmd/wails@v2.15.0`).

```bash
npm --prefix web run build      # atualiza internal/web/dist
wails build -tags desktop -clean -s -skipbindings -nopackage \
  -platform linux/amd64 -o dist/desktop/linux-amd64/kubePeep
```

Flags:

- `-s` — pula o build do frontend (o dist embutido já está atualizado).
- `-skipbindings` — os bindings TS já existem em `web/src/wailsjs`.
- `-nopackage` — produz o binário sem empacotamento extra.

Makefile: `make build-desktop-linux` (equivalente, sem `-s`).

## 3. Build via contêiner (sem dependências nativas no host)

Em máquinas sem `libgtk-3-dev`/`libwebkit2gtk-4.0-dev` (ou sem sudo), o build
roda num contêiner Ubuntu 22.04 (Debian bookworm não distribui
`webkit2gtk-4.0`). O Go, o cache de módulos e a CLI Wails do host são
montados; `GOPROXY=off` evita o sum verifier em redes com TLS interceptado:

```bash
docker run --rm \
  -v "$PWD":/src -w /src \
  -v "$(go env GOMODCACHE)":/go/pkg/mod \
  -v "$(go env GOCACHE)":/root/.cache/go-build \
  -v "$(go env GOROOT)":/opt/go:ro \
  -v "$(go env GOPATH)/bin/wails":/go/bin/wails:ro \
  -e GOROOT=/opt/go -e GOPATH=/go -e GOMODCACHE=/go/pkg/mod \
  -e PATH=/opt/go/bin:/go/bin:/usr/bin:/bin \
  -e HOME=/root -e GOPROXY=off -e GOSUMDB=off \
  ubuntu:22.04 \
  bash -c "apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
    build-essential libgtk-3-dev libwebkit2gtk-4.0-dev pkg-config && \
  wails build -tags desktop -clean -s -skipbindings -nopackage \
    -platform linux/amd64 -o /src/dist/desktop/linux-amd64/kubePeep"
```

Notas:

- O Wails CLI prefixa `build/bin/` ao `-o` quando o destino é relativo ao
  repositório; mova o binário para `dist/desktop/linux-amd64/`.
- O binário resultante é `dynamically linked` (GTK/WebKit do sistema do
  usuário final) — distribute com a nota de dependências
  `libgtk-3-0 libwebkit2gtk-4.0-37`.
- `./kubePeep doctor` funciona sem GUI e valida o frontend embutido
  (`build/embedded_frontend`).

## 4. Smoke pós-build

```bash
HOME=$(mktemp -d) dist/desktop/linux-amd64/kubePeep doctor
HOME=$(mktemp -d) dist/desktop/linux-amd64/kubePeep --help
```

Ambos não exigem display; a janela só abre no modo interativo
(`kubePeep` sem subcomando).
