# Evidências sanitizadas — Ginger v1.4.4

Data da coleta: 2026-07-27

Todas as chamadas usaram a CLI instalada pelo módulo `v1.4.4`, priorizada no
ambiente como `PATH=<GO_BIN>:$PATH`. Os scaffolds foram criados fora do
repositório e removidos depois da validação.

Sanitização aplicada:

- o diretório descartável foi substituído por `<TEMP>`;
- o diretório de binários do usuário foi substituído por `<GO_BIN>`;
- nenhum token, kubeconfig, variável secreta ou caminho do workspace aparece
  nas análises versionadas;
- nomes determinísticos `serviceprobe` e `cliprobe` foram preservados porque
  fazem parte da reprodução, não de dados do usuário.

## Comandos e códigos de saída

| Evidência | Comando normalizado | Exit |
| --- | --- | --- |
| versão | `PATH=<GO_BIN>:$PATH ginger version` | 0 |
| service inspect | `ginger inspect --json` | 0 |
| service doctor | `ginger doctor` | 0 |
| CLI inspect | `ginger inspect --json` | 0 |
| CLI doctor | `ginger doctor` | 1, esperado: template não contém testes |

Antes dos diagnósticos, ambos os scaffolds passaram por `go mod tidy`,
`go test ./...`, `go vet ./...` e `go build ./...` com
`GOTOOLCHAIN=go1.25.0`.

## Arquivos preservados

- [scaffold-comparison.md](scaffold-comparison.md): análise de entrypoints,
  Makefiles e GoReleaser dos templates históricos;
- [plan-support.md](plan-support.md): inventário do suporte a `--plan`.

As saídas completas de versão, `inspect` e `doctor` foram retiradas do Git.
A cópia histórica fica no projeto privado sob `~/.dwyt/projects/`; este
registro preserva o método, os resultados sanitizados e os limites.
Novas saídas devem ser geradas fora do Git, sem caminhos ou dados locais na
documentação pública.
