# Plano de desenvolvimento do Kube Peep

Este diretório transforma o [prompt inicial](initial_prompt.md) em uma sequência executável de trabalho. O plano mantém as oito fases definidas no prompt e usa gates objetivos: uma fase só termina quando seus entregáveis, testes e documentação estiverem concluídos.

## Resultado esperado

Ao final da Fase 8, o Kube Peep deve ser um binário local autocontido, construído com Go 1.25 e Ginger v1.4.4, contendo a interface React embutida, operando com o kubeconfig do usuário e respeitando integralmente o RBAC do cluster.

Princípio de produto que governa todas as decisões:

> Mostrar somente o que o usuário pode acessar e habilitar somente o que ele pode executar.

## Referências de trabalho

- [DWYT](https://github.com/fvmoraes/dwyt): referência de organização, identidade visual, experiência local, embedding, instalação e distribuição. Não é fonte de regras de negócio.
- [Ginger v1.4.4](https://github.com/fvmoraes/ginger/tree/v1.4.4): framework obrigatório do backend.
- [Documentação do Ginger v1.4.4](https://pkg.go.dev/github.com/fvmoraes/ginger@v1.4.4): API pública fixada para o projeto.

A Fase 1 confirmou que os templates do Ginger para serviço HTTP e CLI Cobra
são mutuamente exclusivos e que `ginger generate command` não se aplica a um
projeto `service`. A decisão aceita é manter `project.type: service`, integrar
Cobra manualmente e usar lifecycle próprio com os componentes Ginger.

O DWYT foi analisado no commit imutável
`a9386823272b928f2289c9020a9ae5951389e0f1`, com licença e limites de cópia
registrados.

O Ginger foi reproduzido na tag `v1.4.4`, commit
`6073543b6281be01e4bc97d001dd6e11512f70db`. Scaffolds, diagnósticos,
cross-builds, lifecycle e controle local nativo Linux/Windows estão rastreados
em [Evidências da Fase 1](../docs/research/phase1-evidence.md). O probe isolado
confirma a arquitetura; não substitui os testes da implementação de produção.

## Sequência e gates

```text
Fase 1 ──> Fase 2 ──> Fase 3 ──> Fase 4 ──> Fase 5 ──> Fase 6 ──> Fase 7 ──> Fase 8
```

As fases seguem a ordem determinada pelo prompt. Para evitar duplicação, a Fase 5 cria somente as consultas compartilhadas mínimas necessárias ao overview; a Fase 6 estende essas mesmas interfaces para listas e detalhes completos. A CI mínima começa na Fase 3 e cresce com cada fatia, mas o gate de distribuição só fecha na Fase 8.

| Fase | Foco | Resultado do gate |
| --- | --- | --- |
| [1 — Descoberta](01-descoberta.md) | DWYT, Ginger e riscos | Arquitetura híbrida e decisões técnicas validadas |
| [2 — Especificação](02-especificacao.md) | Produto, arquitetura, API, segurança e dados | Contratos objetivos e rastreáveis aprovados |
| [3 — Fundação](03-fundacao.md) | Executável local, Ginger, Cobra, SQLite e React | Primeiro binário autocontido funcionando |
| [4 — Kubernetes e RBAC](04-kubernetes-rbac.md) | Kubeconfig, contextos, escopos e autorização | Seleção segura de cluster e namespaces |
| [5 — Dashboard](05-dashboard.md) | Saúde, problemas, restarts, eventos, logs e métricas | Overview útil, progressivo e tolerante a falhas parciais |
| [6 — Recursos](06-recursos.md) | Consultas somente leitura e streaming | Navegação completa pelos recursos permitidos |
| [7 — Ações](07-acoes.md) | Restart, scale, delete, port-forward e exec | Ações autorizadas, confirmadas e canceláveis |
| [8 — Distribuição](08-distribuicao.md) | Releases, instaladores, CI e aceite | MVP multiplataforma publicável |

Estado atual: **Fases 1 e 2 concluídas**; a **Fase 3 concluiu os gates locais**
e aguarda a execução nativa macOS/Windows do workflow. O fechamento documental da Fase 2 está registrado em
[Evidências da Fase 2](../docs/research/phase2-validation.md).

O acompanhamento dos critérios finais está em [Matriz de aceite do MVP](matriz-aceite-mvp.md).

## Regras de execução

1. Trabalhar em tarefas pequenas, seguindo a ordem de cada arquivo de fase.
2. Não iniciar código de produção antes dos gates de descoberta e especificação.
3. Instalar e usar a CLI do Ginger na versão `v1.4.4`; não usar `@latest` durante o desenvolvimento deste projeto.
4. Executar `ginger add`, `ginger generate` e `ginger docs` primeiro com `--plan`; o campo `require_plan_before_apply` não é imposto automaticamente pela v1.4.4.
5. Como `ginger new` e `ginger init` não oferecem `--plan`, gerar scaffolds comparativos apenas em diretório temporário e revisar qualquer inicialização antes de escrever no repositório.
6. Nunca usar `--force` sem uma justificativa registrada.
7. Implementar funcionalidades em fatias verticais: porta/serviço, adapter, handler/DTO, frontend e testes.
8. Tratar o backend e a API Kubernetes como autoridades finais de autorização; a interface é apenas uma representação da capacidade atual.
9. Não declarar uma tarefa ou fase concluída sem executar a evidência indicada.
10. Atualizar documentação e ADRs na mesma mudança que altera um contrato ou uma decisão.

## Definition of Done transversal

Uma tarefa funcional somente pode ser marcada como concluída quando, conforme aplicável:

- [ ] possui testes unitários;
- [ ] possui teste de integração do caminho permitido;
- [ ] possui teste explícito do caminho negado por RBAC;
- [ ] respeita timeout e cancelamento de contexto;
- [ ] não persiste kubeconfig, credenciais, Secrets ou logs de aplicações;
- [ ] não registra tokens, certificados, cabeçalhos de autorização ou comandos de `exec`;
- [ ] converte objetos Kubernetes em DTOs próprios;
- [ ] mantém erros parciais isolados da funcionalidade restante;
- [ ] atualiza contrato de API, documentação e ADR relacionado;
- [ ] passa nas verificações de backend e frontend;
- [ ] passa em `ginger inspect` e `ginger doctor`, ou documenta claramente um diagnóstico conhecido, sem tratar esses comandos heurísticos como substitutos dos testes;
- [ ] registra os comandos executados e seus resultados na entrega da tarefa.

## Guardrails permanentes

- Ginger v1.4.4 é a camada principal de aplicação HTTP; Gin e frameworks equivalentes estão fora de escopo.
- O frontend usa componentes próprios e pequenos; Electron, Material UI, Ant Design e kits visuais pesados estão fora de escopo.
- A API escuta/publica somente `127.0.0.1` no MVP. Proteções de `Host`, `Origin` e requisições mutáveis seguem o threat model antes das ações da Fase 7.
- Rotas SSE/WS não podem usar cegamente a cadeia padrão do Ginger v1.4.4, que não preserva `http.Flusher`/`http.Hijacker`; elas exigem `HandleRaw` e uma cadeia segura que preserve as interfaces.
- `pkg/ws` não sustenta `exec`; o ADR 0003 fixou `coder/websocket v1.8.15` e o wire contract endurecido.
- `SelfSubjectRulesReview` pode otimizar a exibição, mas não substitui `SelfSubjectAccessReview` nem a autorização da operação real.
- O Kube Peep não faz impersonation, não pede credenciais adicionais e não cria autorização paralela ao Kubernetes.
- O modo `all` significa apenas os namespaces retornados pela API para a identidade atual; nunca significa `*`.
- Metrics API é opcional e sua ausência não torna a aplicação local indisponível.
- OpenTelemetry é opcional, não entra no caminho crítico e permanece desativado por padrão.
- Watches, streams, port-forwards e sessões de `exec` precisam ter dono, limite, cancelamento e encerramento observáveis.
- Qualquer YAML de Secret deve omitir valores; o MVP exibe somente metadados de Secrets.
- O scan de logs é limitado, cancelável, não persistente e apresenta “possíveis erros”, nunca diagnósticos inventados.
- O banco não armazena snapshots do cluster. Preferências persistidas usam uma allowlist e nunca recebem credenciais, Secrets ou conteúdo de logs.
- Respostas com dados Kubernetes, logs ou permissões usam política `no-store`; somente assets estáticos versionados podem receber cache longo.
- “Sem dependências de runtime” significa não instalar dependências próprias do Kube Peep. Um plugin `exec` já referenciado pelo kubeconfig continua sendo responsabilidade do ambiente do usuário.
- A SPA usa History API. O servidor entrega `index.html` apenas como fallback de
  GET/HEAD que aceita HTML; `/api/v1`, `/health` e endpoints internos nunca
  caem no fallback.
- Instaladores, scripts auxiliares e downloads usam tag/commit ou asset de
  release com versão exata e checksum; `raw/main`, `latest` e branch mutável não
  são fontes de instalação.

## Riscos que atravessam as fases

| Risco | Tratamento planejado |
| --- | --- |
| Integração do lifecycle bloqueante do Ginger com Cobra | Spike/ADR F1 aprovados; reimplementar o coordenador no módulo F3 |
| `pkg/app` fixar timeout de escrita enquanto SSE/WS precisam ser duradouros | F1 provou stream acima de 15 s; F3 aplica `WriteTimeout=0` e budgets por rota |
| `app.Run()` não aceitar listener/contexto e poder pular hooks após falha de shutdown | ADR escolheu lifecycle próprio com componentes Ginger; repetir matriz de cleanup em F3 |
| Defaults do Ginger aceitarem bind não-loopback | Sobrescrever e testar somente `127.0.0.1`; falhar fechado em configuração insegura |
| Health do Ginger transformar todo checker falho em 503 e serializar seu erro | Registrar apenas checks locais críticos; status externo separado e sanitizado |
| Middleware padrão remover `Flusher`/`Hijacker` | Rotas raw com middlewares próprios e testes de interface/segurança |
| Logger do Ginger escrever apenas em stdout e redigir só por nome da chave | Estratégia explícita de arquivo/rotação e sanitização por conteúdo antes de logar |
| Paginação Ginger ser page/per-page, diferente de `continue` | DTO cursor próprio mantendo o envelope `data` |
| Cursor Kubernetes não cobrir fan-out multi-namespace/kind | Cursor composto ligado à query/generation, merge limitado e estado truncado explícito |
| Resultado incompleto de `SelfSubjectRulesReview` | Cache apenas como dica; SAR e chamada Kubernetes continuam sendo autoridade |
| Revisão RBAC indisponível ser confundida com negação | Capability tri-state; `FORBIDDEN` somente por negação explícita/operação real |
| Plugins `exec` vazarem dados em erros | Sanitização central, allowlist de campos de log e testes com mensagens sensíveis |
| Scan de logs causar carga ou expor segredo | Limites rígidos, concorrência controlada, redaction e nenhuma persistência |
| Goroutines ou conexões sobreviverem à troca de contexto | Contextos hierárquicos, registro de sessões e testes de cancelamento |
| Fake clientset não representar RBAC real | Casos simples com fake e cenários restritos no Kind canônico |
| Diferenças de processos e paths entre sistemas | Adapters por plataforma e smoke tests dos artefatos reais |
| PID/sinal não implementarem stop seguro no Windows | Probe F1 nativo validou lock/identidade/controle; reimplementar e repetir em F3 e nos archives F8 |
| SQLite ou dependências quebrarem builds sem CGO | F1 validou `modernc.org/sqlite`; repetir a matriz com o módulo definitivo na Fase 3 |
| Reuso do DWYT virar cópia indevida de negócio | Inventário explícito do que pode e não pode ser reaproveitado |

## Estado do plano

A Fase 1 está concluída com 44 tarefas comprovadas e
[matriz rastreável](../docs/research/phase1-evidence.md). As tarefas das fases
seguintes permanecem pendentes até sua implementação e evidência próprias.
Marcar um checkbox somente quando o resultado correspondente existir no
repositório; evidência de spike não antecipa checkbox de produção ou release.

## Convenção de nomes

- Produto e textos de marca: **Kube Peep**.
- Executável e comando literal: `kubePeep`.
- Diretório Unix literal: `~/.kubePeep/`.
- Diretório Windows literal: `%LOCALAPPDATA%\kubePeep\`.
- Repositório e módulo Go: `github.com/fvmoraes/kubepeep`.
- Nomes de telas e ações podem permanecer em inglês conforme o prompt; a documentação do plano permanece em português.

## Handoff obrigatório

Ao concluir cada tarefa relevante e cada gate de fase, registrar:

- arquivos criados;
- arquivos alterados;
- comandos executados;
- testes executados;
- resultados observados;
- pendências e exceções;
- próxima tarefa recomendada;
- caminhos das evidências produzidas.
