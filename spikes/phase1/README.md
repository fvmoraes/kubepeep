# Spikes da Fase 1

Este módulo é isolado do futuro módulo de produção. Ele existe para validar, antes
do scaffold definitivo:

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

## Evidência atual

- A suíte Linux completa passou 53 casos em 5 packages. Ela cataloga 37 testes
  top-level, dos quais um é helper de subprocesso ignorado no processo pai.
- O blackbox Linux de controle passou nativamente e em 20 repetições; SIGTERM
  também removeu `instance.json` e liberou o lock.
- A suíte de controle passou integralmente no Windows 10 Pro 10.0.19045 amd64,
  cobrindo `LockFileEx`, fingerprint, DACL/tamper, identidade,
  `status`/`stop`, Host/Origin/token e cleanup, com `TEST_EXIT_CODE=0`.
- Linux, macOS e Windows em amd64/arm64 continuam compilando com Go 1.25 e
  `CGO_ENABLED=0`; cross-build não é apresentado como execução nativa.

As transcrições estão em
[`../../docs/research/evidence/f1-control/`](../../docs/research/evidence/f1-control/).

Nada deste diretório é código de produção. F1 valida o desenho em um probe
isolado; F3 reimplementa lifecycle e adapters no scaffold definitivo; F8
executa os comandos nos archives e instaladores reais. As decisões e requisitos
de reimplementação estão registrados nos ADRs.
