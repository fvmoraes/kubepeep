# ADR 0004 — Runtime local e lifecycle do processo

- Status: aceito
- Data: 2026-07-27
- Tarefas: F1-19, F1-30, F1-36, F1-37, F1-43, F1-44

## Contexto

O processo precisa escolher porta a partir de 2748, evitar duas instâncias,
abrir o navegador somente quando pronto e permitir `stop`/`status` em Unix e
Windows. PID isolado não prova identidade: pode estar obsoleto ou ter sido
reutilizado por outro processo.

O DWYT fornece uma referência de readiness e browser, mas usa porta fixa, PID
simples e término específico por plataforma. Essas escolhas não serão copiadas.

## Decisão

### Modo de processo

- `kubePeep` e `kubePeep start` executam em foreground.
- Sinais são tratados uma única vez pelo contexto Cobra.
- Não existe daemon implícito no MVP.

### Single instance e identidade

O diretório `runtime/` conterá:

- `kubePeep.lock`: lock real mantido aberto durante toda a execução;
- `instance.json`: schema versionado com instance ID aleatório, PID, fingerprint
  de início, porta, versão de protocolo e token de controle aleatório.

PID e porta são publicados juntos em `instance.json`; arquivos independentes
não são autoridade e não podem expor estado parcial. O estado será gravado por
`temp + flush + substituição atômica`, com modo `0600`/`0700` em Unix e DACL
limitada ao usuário atual em Windows. O temporário precisa estar protegido
antes do replace. O token não será persistido em SQLite nem registrado.

O lock usará adapters com primitives do sistema operacional:

- Unix: lock advisory exclusivo mantido pelo file descriptor;
- Windows: `LockFileEx` mantido pelo handle.

### Bind e prontidão

1. tentar `net.Listen` diretamente em `127.0.0.1:2748`;
2. em `address in use`, tentar a próxima porta dentro do limite configurado;
3. nunca fazer probe e fechar antes do bind definitivo;
4. manter PID, porta e identidade somente em memória;
5. iniciar HTTP usando o listener adquirido;
6. sondar `/health` pela instância recém-criada;
7. gravar o estado privado temporário, fazer flush e substituir
   `instance.json` atomicamente;
8. publicar prontidão e só então abrir o navegador.

### `status` e `stop`

Ambos leem `instance.json`, validam schema/permissões, apresentam o token de
controle e consultam um endpoint interno em loopback. Tanto `status` quanto
`stop` são autenticados. A resposta precisa provar o mesmo instance ID, PID,
fingerprint, porta e versão de protocolo.

O endpoint:

- exige peer loopback e Host exatamente igual ao listener;
- não aceita Origin de browser;
- usa comparação em tempo constante;
- não devolve o token;
- faz `stop` idempotente;
- no `stop`, apenas cancela o contexto raiz; não chama `os.Exit`.

Essa estratégia é igual em Unix e Windows e evita tratar sinais ou `taskkill`
como caminho normal. SIGINT/SIGTERM recebidos pelo próprio processo foreground
também cancelam seu contexto, mas o controlador nunca sinaliza o PID lido do
estado.

### Cleanup

Um registro LIFO pertence ao coordenador e fecha, mesmo após erro ou timeout:

- listener e servidor;
- sessões SSE/WS;
- port-forwards e execs;
- watches;
- SQLite;
- lock e arquivos transitórios de runtime.

Em timeout de `Server.Shutdown`, o coordenador chama `Server.Close`, fecha
sessões hijacked pelo registro e continua os hooks. Erros são agregados e
refletidos no código de saída.

## Alternativas rejeitadas

### Encontrar uma porta com bind temporário e reabrir

Rejeitado pela janela de corrida entre descoberta e bind.

### Confiar em `kill(pid)` ou `taskkill`

Rejeitado como fluxo normal porque PID pode apontar para outro processo.

### Usar apenas PID e nome do executável

Rejeitado: nome não é identidade suficiente e pode coincidir.

### Daemonizar e retornar imediatamente

Rejeitado no MVP conforme ADR 0001.

## Evidências

- O spike reserva uma porta e comprova retry pelo segundo bind real, sempre em
  loopback.
- O spike de readiness comprova health antes de publicar prontidão/abrir o
  browser; o adapter F3 integrará esse passo à publicação atômica do estado.
- O runtime executa cleanup em cancelamento normal, erro de `Serve` e timeout de
  shutdown.
- Conexões hijacked são fechadas pelo cleanup explícito.
- O probe isolado F1 passou no Linux nativo, inclusive em 20 repetições do
  blackbox, e no Windows 10 Pro 10.0.19045 amd64. A execução Windows completa
  cobriu `LockFileEx`, fingerprint de criação, DACL privada e rejeição de
  adulteração, identidade, `status`/`stop`, Host/Origin/token e cleanup, com
  `TEST_EXIT_CODE=0`. A transcrição e os hashes estão em
  [`../research/evidence/f1-control/windows-native-2026-07-27.txt`](../research/evidence/f1-control/windows-native-2026-07-27.txt).

Essa evidência fecha F1-44 e a decisão arquitetural. O código do probe é
descartável: F3 reimplementa o adapter de produção e repete os testes; F8
executa `start`/`status`/`stop` nos archives reais de release.

## Consequências

- `stop` funciona de forma segura sem exigir daemon.
- Um atacante em outra página web não recebe o token de controle e é bloqueado
  por Host/Origin; um processo local do mesmo usuário já possui autoridade
  equivalente sobre seus arquivos e processos.
- Remoção de runtime obsoleto exige prova de lock livre e falha da identidade,
  nunca apenas idade do arquivo.
- A prova Windows usa owner e APIs de DACL por nome de forma fail-closed. Sem
  bloquear F1-44, F3 deve cobrir token elevado (`TOKEN_OWNER` versus SID do
  usuário) e eliminar ou conter TOCTOU/reparse por meio de raiz per-user
  confiável e handles vinculados ao objeto.
