# Build desktop

O desktop usa a mesma composição Go e o mesmo frontend embutido do modo web.
A tag `desktop` seleciona Wails; JSON passa pela bridge e SSE/WebSocket pelo
loopback interno. Veja a [arquitetura desktop](desktop-architecture.md).

## Ambiente

Go, Node, npm e Wails seguem as versões do [guia de desenvolvimento](development.md).
Bibliotecas nativas dependem da plataforma:

| Plataforma | Requisito |
| --- | --- |
| Linux | Compilador C, `pkg-config`, GTK 3 e WebKit2GTK; desktop exige CGO |
| macOS | Xcode Command Line Tools para compilar a shell nativa |
| Windows | WebView2 em runtime; NSIS para gerar o instalador |

O workflow Linux usa GTK 3 e WebKit2GTK 4.1 com `desktop,webkit2_41`.
Em Debian/Ubuntu compatível, instalar `build-essential`, `pkg-config`,
`libgtk-3-dev` e `libwebkit2gtk-4.1-dev`. Com WebKit2GTK 4.0, usar a tag
`desktop` e o pacote `libwebkit2gtk-4.0-dev` correspondente.

## Desenvolvimento e compilação

Na raiz do repositório, com a CLI Wails v2.15.0 instalada:

```sh
rtk make dev-desktop DESKTOP_TAGS=desktop,webkit2_41
rtk make build-desktop DESKTOP_TAGS=desktop,webkit2_41
```

O override `webkit2_41` é específico do Linux com essa biblioteca. Em macOS,
Windows ou Linux com WebKit2GTK 4.0, usar os targets sem esse override.
Os targets `build-desktop-linux`, `build-desktop-windows` e
`build-desktop-darwin` selecionam a plataforma; dependências nativas ainda
precisam estar disponíveis no runner adequado.

O Makefile preserva os bindings próprios usando `-skipbindings`, como o
workflow de release. Geração deliberada de bindings exige revisar o contrato
em `web/src/wailsjs/go/desktop/Bridge.*`; não substituir esses arquivos como
parte de um build comum.

Wails executa os comandos de frontend definidos em `wails.json`. Para um
build direto após `make web-build`, a flag `-s` permite reutilizar o embed
atualizado. O destino relativo de `-o` é resolvido pelo Wails dentro de
`build/bin/`; consulte o caminho impresso no build. Esses resultados são
locais e ignorados pelo Git.

`make build` produz o binário CLI/web com `CGO_ENABLED=0`. Essa opção não
remove a dependência nativa de GTK/WebKit dos builds desktop Linux.

## Distribuição e validação

O [workflow release](../.github/workflows/release.yml) contém a receita
canônica: Linux em contêiner Ubuntu 22.04 para o baseline de glibc,
Windows em runner nativo com NSIS e macOS em runners nativos. Os arquivos de
empacotamento ficam em `build/` e `packaging/`; somente saídas ficam fora do Git.

Após compilar, validar `version` e `--help` usando o caminho do binário gerado.
O smoke isolado de CLI é `make smoke`; uma janela desktop exige também
validação visual no ambiente gráfico, streams e encerramento das sessões.
Para a release, repetir os gates nativos e de instalação descritos no
[plano v1](../plan/README.md). Não tratar cross-compilation como execução nativa.

Transcripts, capturas e pacotes gerados ficam privados. Commit local não
autoriza publicação: nunca iniciar push ou workflow de release automaticamente.
