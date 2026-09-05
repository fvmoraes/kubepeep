# Evidência do controle local F1-44

Este diretório preserva a prova nativa do desenho de `start`, `status` e
`stop` antes da implementação de produção da Fase 3.

## Registro preservado

A validação histórica de 2026-07-27 passou em Linux amd64 (incluindo SIGTERM,
cleanup e vinte repetições) e Windows amd64 (suíte `control`, exit 0).
Os transcripts completos e binários ficam privados sob `~/.dwyt/projects/`;
não são versionados. Esta prova valida o spike, não uma release atual.

O [runner Windows](../../../../spikes/phase1/scripts/run-windows-native.cmd)
é fonte reproduzível e acompanha o módulo do spike.

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

Copiar os dois executáveis e `scripts/run-windows-native.cmd` para o mesmo diretório
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

O requisito histórico para o adapter de produção era usar a raiz de dados por usuário
aprovada, testar execução elevada e reduzir operações baseadas apenas em path
por meio de handles Windows quando aplicável. Essas são exigências de
reimplementação/hardening, não lacunas da prova de viabilidade F1-44.
