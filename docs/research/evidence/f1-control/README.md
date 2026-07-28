# Evidência do controle local F1-44

Este diretório preserva a prova nativa do desenho de `start`, `status` e
`stop` antes da implementação de produção da Fase 3.

## Arquivos

- `linux-native-2026-07-27.txt`: black-box Linux, incluindo encerramento por
  `SIGTERM`, cleanup e repetição do lifecycle vinte vezes;
- `windows-native-2026-07-27.txt`: suíte completa em Windows 10 Pro amd64,
  com `TEST_EXIT_CODE=0`;
- `run-windows-native.cmd`: runner reproduzível usado no guest Windows.

Executáveis e ISO são artefatos temporários e não são versionados. Os hashes
dos artefatos usados na execução Windows final são:

| Artefato | SHA-256 |
| --- | --- |
| `f1-control-probe.exe` | `3d8e3dcac0ce72dcd039070f5be2361630f7fef090a97486409c35a07e2e12b8` |
| `f1-control.test.exe` | `30d21e2367db4f0babc04cac2f21392ed9ebef505d92e899e38192d80409008f` |

## Reprodução

Na raiz de `spikes/phase1`:

```sh
GOTOOLCHAIN=go1.25.0 \
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go test -c ./control -o f1-control.test.exe

GOTOOLCHAIN=go1.25.0 \
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -o f1-control-probe.exe ./cmd/f1-control-probe
```

Copiar os dois executáveis e `run-windows-native.cmd` para o mesmo diretório
no Windows e executar o `.cmd` sem elevação. O runner registra versão/arquitetura
do sistema, hashes e a saída verbosa dos testes em
`%TEMP%\f1-control-results.txt`.

## Cobertura da prova

A suíte nativa cobre:

- lock exclusivo com `flock` no Linux e `LockFileEx` no Windows;
- fingerprint de processo sem usar PID como identidade suficiente;
- estado privado e atômico, schema estrito e prova completa de identidade;
- DACL Windows protegida, limitada ao usuário atual e propagada aos filhos;
- rejeição de DACL adulterada para `Everyone`;
- proteção do arquivo temporário antes de `MoveFileEx` e do lock estável;
- autenticação de `status` e `stop`, Host/Origin/peer loopback;
- segunda inicialização, estado obsoleto, PID alheio, stop idempotente;
- cleanup, liberação do lock e sinais Unix.

O adapter de produção da Fase 3 ainda deve usar a raiz de dados por usuário
aprovada, testar execução elevada e reduzir operações baseadas apenas em path
por meio de handles Windows quando aplicável. Essas são exigências de
reimplementação/hardening, não lacunas da prova de viabilidade F1-44.
