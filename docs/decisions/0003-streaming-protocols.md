# ADR 0003 — Protocolos de streaming

- Status: aceito
- Data: 2026-07-27
- Tarefas: F1-15, F1-21, F1-28, F1-29, F1-31, F1-41

## Contexto

O produto precisa de atualizações unidirecionais, logs follow e terminal
bidirecional. O router Ginger aplica middlewares comuns por meio de um wrapper de
`http.ResponseWriter`; o logger padrão não propaga `http.Flusher` ou
`http.Hijacker`. `app.Run()` também fixa `WriteTimeout` em 15 segundos.

`pkg/ws` v1.4.4 faz handshake e frames básicos, mas não oferece, por si só, as
garantias necessárias para um terminal:

- não valida `Origin`;
- não impõe limite de payload;
- não modela heartbeat, resize ou backpressure;
- seu decoder não cobre integralmente fragmentação, opcodes de controle e
  política de masking exigida para um endpoint exposto ao browser.

## Decisão

### SSE

SSE será usado para:

- logs follow;
- atualizações simples de watches;
- progresso de operações longas quando não houver fluxo de entrada.

O encoder `pkg/sse` será reutilizado. A rota será registrada com `HandleRaw` e
uma cadeia própria que:

- preserva o `ResponseWriter` original;
- reaplica recuperação e request ID do Ginger;
- valida Host e Origin;
- usa logger sem wrapper de status;
- limita duração, bytes, frequência e fila;
- encerra pelo `request.Context()`.

O servidor terá `WriteTimeout=0`; deadlines e budgets serão aplicados por rota,
não por uma duração global incompatível com streams.

O wire contract de `/api/v1/stream`, incluindo ID opaco, snapshot em chunks,
replay ring, payloads de update, heartbeat e eventos terminais `reset`/`error`,
está fixado em [`../api.md`](../api.md#182-atualizações-de-recursos). Nenhum
evento ou cursor SSE é persistido.

### Exec

Exec usará `github.com/coder/websocket v1.8.15` entre browser e Kube Peep.
`pkg/ws` não será usado no caminho de exec na forma atual. A integração manterá:

- `Origin` allowlisted;
- payload e frame máximos;
- texto/binário/opcodes explicitamente aceitos;
- ping/pong e deadline;
- fila limitada e backpressure;
- cancelamento e fechamento com código;
- mensagens tipadas para stdin, stdout, stderr, resize e exit.

O fluxo local possui duas etapas, sem variante negociável:

1. `POST /api/v1/pods/{namespace}/{name}/exec` recebe o `ExecInit` completo
   (`container`, `command` como argv, `tty`, `stdin`, confirmação e geração
   esperada), aplica JSON estrito, limites, CSRF, validação do alvo e SAR
   `create pods/exec`;
2. somente depois dessas validações o backend canonicaliza método, path, alvo,
   geração e body, liga seu hash a um ticket one-shot de TTL curto e o devolve
   como oferta de `Sec-WebSocket-Protocol`;
3. `GET /api/v1/exec/{sessionId}/stream` valida Origin, consome o ticket,
   compara o binding em tempo constante, repete geração e SAR e só então faz o
   upgrade e abre o stream Kubernetes.

Command, container, TTY e stdin não podem ser definidos ou substituídos no
primeiro frame. Depois do upgrade, os únicos frames browser→backend são os tipos
de stream allowlisted (`stdin`, `resize`, `heartbeat` e `close`) que o
`ExecInit` autorizou. O protocolo selecionado na resposta nunca ecoa o ticket;
URL, query e logs também não o contêm.

O backend continuará usando as bibliotecas oficiais Kubernetes para o stream
remoto; o WebSocket local não altera nem amplia RBAC.

O wire contract não fica a cargo da implementação: encoding binário dos três
streams, JSON de controle, sequência `ready`/`exit`, limites, heartbeat,
backpressure e close codes estão fixados em
[`../api.md`](../api.md#191-encoding-e-schemas-de-frames). Compressão WebSocket
permanece desabilitada.

### Port-forward

Port-forward abrirá uma porta TCP em loopback no backend e será controlado por
HTTP. Não usará WebSocket entre browser e backend.

## Alternativas rejeitadas

### Usar WebSocket para todos os streams

Rejeitado porque adiciona complexidade bidirecional onde SSE é suficiente.

### Usar a cadeia padrão do router em SSE/WS

Rejeitado porque o wrapper atual remove interfaces necessárias.

### Usar `pkg/ws` sem complementos

Rejeitado pelo conjunto de gaps demonstrado na inspeção. O pacote permanece
avaliado e justificado na matriz Ginger, sem ser tratado como seguro por mera
presença no framework.

### Polling frequente

Rejeitado por carga, latência e proliferação de requisições.

## Evidências

- `ginger add sse --plan` e `ginger add websocket --plan` foram executados sem
  alterar o projeto principal.
- O spike comprova `Flusher` e `Hijacker` em uma cadeia raw com request ID,
  recuperação, Host/Origin guard e logging.
- Um stream criado com `pkg/sse` observa o cancelamento de
  `request.Context()` quando o cliente fecha a conexão.
- Um stream SSE real permaneceu ativo por 16 segundos e entregou o evento
  final, superando o timeout fixo do lifecycle padrão.
- Um teste TCP de `pkg/ws` recebeu `101 Switching Protocols` com Origin externa
  e entregou ao handler um frame de cliente sem masking, comprovando que o
  pacote isolado não é suficiente para o terminal.
- O teste de shutdown força timeout com stream ativo e ainda executa cleanup.
- O teste de conexão hijacked comprova que o registro de sessões, e não
  `http.Server`, deve possuir e fechar essas conexões.

## Consequências

- Rotas raw terão uma suíte de segurança própria.
- Toda sessão terá owner, geração de contexto, limites e cancelamento.
- O ticket autoriza exatamente um alvo e um `ExecInit`; qualquer alteração
  exige novo POST, nova confirmação e nova autorização.
- Ações `exec` permanecem bloqueadas até o transporte endurecido e os testes de
  RBAC da Fase 7.
