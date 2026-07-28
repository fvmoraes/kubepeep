# ADR 0001 — Bootstrap híbrido CLI e serviço Ginger

- Status: aceito
- Data: 2026-07-27
- Tarefas: F1-13, F1-17, F1-18, F1-37, F1-42

## Contexto

O Kube Peep precisa ser simultaneamente uma CLI Cobra e um serviço HTTP local.
O Ginger v1.4.4 gera esses tipos de projeto separadamente:

- `ginger new <nome> --service` cria router, handlers, health, configuração e
  teste de integração, mas não Cobra;
- `ginger new <nome> --cli` cria Cobra, porém não disponibiliza a estrutura e
  as integrações HTTP de um serviço;
- `ginger generate command` é recusado em projeto `service`.

Além disso, `app.Run()`:

- cria seu próprio listener a partir de host e porta já escolhidos;
- instala seu próprio tratamento de `SIGINT`/`SIGTERM`;
- fixa `WriteTimeout` em 15 segundos;
- não expõe listener nem sinal de prontidão;
- retorna diretamente em erro de `Serve`, sem executar os hooks;
- não executa hooks quando `Server.Shutdown` excede o timeout.

Essas características conflitam com bind por tentativa real, Cobra como
proprietário do processo, streams longos e cleanup determinístico.

## Decisão

1. O scaffold definitivo partirá de um projeto Ginger `service`, gerado e
   revisado fora da árvore principal.
2. Cobra será integrado manualmente no módulo de produção.
3. O comando raiz e `start` usarão exatamente o mesmo `RunE` e as mesmas flags.
4. `start` executará em foreground no MVP. Não haverá daemonização implícita.
5. Cobra criará o contexto raiz e será o único proprietário de sinais e do
   código de saída.
6. O processo construirá o container com `app.New` e usará router, config,
   logger, errors, response, health e testhelper do Ginger, mas não chamará
   `app.Run()`.
7. Um coordenador local adquirirá o listener real em `127.0.0.1`, criará o
   `http.Server`, instalará um mux externo mínimo para o health composto do ADR
   0002, publicará prontidão somente após `/health` responder e executará
   cleanup independentemente do motivo de saída.
8. Rotas HTTP comuns usarão o router/middlewares Ginger. Rotas de stream usarão
   `HandleRaw` com a cadeia definida no ADR 0003.

## Alternativas rejeitadas

### Usar somente o scaffold `--cli`

Rejeitado porque descartaria justamente a estrutura HTTP e os diagnósticos de
projeto `service` do Ginger.

### Chamar `app.Run()` dentro do comando Cobra

Rejeitado porque criaria dois proprietários potenciais de sinais e não resolve
bind real, readiness, erro de servidor, timeout de shutdown e streams longos.

### Executar o serviço como daemon oculto

Rejeitado no MVP. Aumentaria a superfície multiplataforma, dificultaria logs e
encerramento e não é necessário para abrir a aplicação no navegador. O usuário
pode manter o processo no terminal e encerrá-lo com `Ctrl+C` ou `kubePeep stop`.

### Trocar Ginger por outro framework HTTP

Rejeitado por requisito de produto.

## Consequências

- O lifecycle HTTP será pequeno, próprio e testado, mas continuará hospedando o
  router e os componentes Ginger.
- `OnStop` de `pkg/app` não será usado como autoridade, pois seu executor não é
  exportado. O coordenador terá um registro LIFO próprio de cleanups.
- Diagnósticos `ginger inspect`/`doctor` continuarão úteis, mas não comprovam o
  lifecycle customizado.
- A documentação deve deixar explícito que foreground é comportamento, não uma
  limitação acidental.

## Evidências

- Scaffolds `service` e `cli` foram gerados em diretórios temporários, passaram
  por `go mod tidy`, testes e build.
- `ginger generate command probe` em `service` retornou que o generator é
  exclusivo de projetos `--cli`.
- O spike `spikes/phase1` executa raiz e `start` com o mesmo contrato, inicia
  uma aplicação Ginger em listener adquirido previamente; o cancelamento do
  contexto Cobra encerra o servidor e comprova cleanup. Erro de servidor e
  timeout também foram testados.
- O código de `pkg/app.Run`/`shutdown` da tag fixada foi inspecionado no commit
  `6073543b6281be01e4bc97d001dd6e11512f70db`.
