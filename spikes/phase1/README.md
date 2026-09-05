# Spikes da Fase 1

Este módulo histórico é isolado do módulo de produção. Preserva os probes que
fundamentaram as decisões iniciais de arquitetura:

- composição de Cobra com uma aplicação Ginger `service`;
- ownership externo do listener e do lifecycle HTTP;
- bind real e retry de porta somente em loopback;
- cleanup após shutdown normal, timeout ou erro de servidor;
- rotas raw que preservam `http.Flusher` e `http.Hijacker`;
- stream SSE com duração superior ao `WriteTimeout` fixo de `app.Run`;
- fallback de SPA e embedding conjunto de frontend e migrations;
- cursor composto, opaco, vinculado à consulta e com expiração;
- compilação sem CGO das dependências Kubernetes e SQLite aprovadas;
- single instance por `flock`/`LockFileEx`, estado privado e atômico, fingerprint
  do processo e PID nunca usado como autoridade;
- endpoint de controle loopback em que `status` e `stop` são autenticados,
  provam a identidade completa e rejeitam token, Host, Origin ou peer inválido;
- stop por cancelamento de contexto, sinais foreground e cleanup de estado/lock.

## Evidência histórica

- A suíte Linux completa passou 53 casos em 5 packages. Ela cataloga 37 testes
  top-level, dos quais um é helper de subprocesso ignorado no processo pai.
- O blackbox Linux de controle passou nativamente e em 20 repetições; SIGTERM
  também removeu `instance.json` e liberou o lock.
- A suíte de controle passou integralmente no Windows 10 Pro 10.0.19045 amd64,
  cobrindo `LockFileEx`, fingerprint, DACL/tamper, identidade,
  `status`/`stop`, Host/Origin/token e cleanup, com `TEST_EXIT_CODE=0`.
- Linux, macOS e Windows em amd64/arm64 foram compilados com Go 1.25 e
  `CGO_ENABLED=0`; cross-build não é apresentado como execução nativa.

O resumo das provas e as instruções de reprodução estão em
[`../../docs/research/evidence/f1-control/`](../../docs/research/evidence/f1-control/).
Transcrições, executáveis e dados das execuções ficam no armazenamento privado
do projeto, fora do Git.

## Reprodução

Na raiz deste módulo:

```sh
go test -count=1 ./...
```

A fixture HTML escrita à mão em
[`spike/assets/frontend/index.html`](spike/assets/frontend/index.html) é fonte
versionada e embarcada junto das migrations. O spike não depende de um build
do frontend de produção nem de arquivos locais em `dist/`.

Para a prova nativa Windows, use
[`scripts/run-windows-native.cmd`](scripts/run-windows-native.cmd). Copie o runner
e os dois executáveis da prova para o mesmo diretório; a saída vai para
`%TEMP%\f1-control-results.txt` e não deve ser versionada.

Nada deste diretório é código de produção. As implementações atuais estão em
`internal/` na raiz do repositório; os ADRs preservam as decisões e requisitos
originados nestes probes. As fases da versão 1 estão em
[`../../plan/`](../../plan/).
