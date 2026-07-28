# Evidências sanitizadas — Ginger v1.4.4

Data da coleta: 2026-07-27

Todas as chamadas usaram a CLI instalada pelo módulo `v1.4.4`, priorizada no
ambiente como `PATH=<GO_BIN>:$PATH`. Os scaffolds foram criados fora do
repositório e removidos depois da validação.

Sanitização aplicada:

- o diretório descartável foi substituído por `<TEMP>`;
- o diretório de binários do usuário foi substituído por `<GO_BIN>`;
- nenhum token, kubeconfig, variável secreta ou caminho do workspace aparece
  nestes arquivos;
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

Arquivos:

- `cli-version.txt`: resolução e build info da CLI;
- `service-inspect.json` e `service-doctor.txt`: saída do scaffold service;
- `cli-inspect.json` e `cli-doctor.txt`: saída do scaffold CLI;
- `scaffold-comparison.md`: entrypoints, Makefiles e GoReleaser;
- `plan-support.md`: inventário completo de suporte a `--plan`.
