# Inventário de suporte a `--plan` — Ginger v1.4.4

O inventário combina inspeção do parser na tag fixada e execução em scaffolds
descartáveis. A implementação de `generate` remove `--plan` dos argumentos
antes de selecionar o generator e retorna depois de renderizar o plano, sem
chamar `Apply`:

- [`internal/cli/generate.go`](https://github.com/fvmoraes/ginger/blob/6073543b6281be01e4bc97d001dd6e11512f70db/internal/cli/generate.go#L11-L99)
- [`internal/cli/add.go`](https://github.com/fvmoraes/ginger/blob/6073543b6281be01e4bc97d001dd6e11512f70db/internal/cli/add.go#L109-L127)
- [`internal/cli/docs.go`](https://github.com/fvmoraes/ginger/blob/6073543b6281be01e4bc97d001dd6e11512f70db/internal/cli/docs.go#L11-L50)

## Comandos com preview real

| Comando | `--plan` | Observação |
| --- | --- | --- |
| `docs` | sim | renderiza criação dos documentos e update do manifest |
| `generate crud` / `g crud` | sim | somente service |
| `generate command` / `g command` | sim | somente CLI |
| `generate handler` / `g handler` | sim | somente worker |
| `generate service` / `g service` | sim | CLI ou worker |
| `generate test` / `g test` | sim | recurso específico |
| `generate tests --scan` | sim | varredura de código existente |
| `generate smoke-test` | sim | smoke test de aplicação |
| `generate swagger` | sim | especificação OpenAPI |
| `add <integration>` | sim | todas as integrações registradas |

O help compacto da raiz só anota `--plan` ao lado de `crud` e `tests --scan`,
mas o parser o aplica a todos os subcomandos de `generate`. Isso foi confirmado
em runtime com `generate command probeplan --plan`: o plano foi impresso e
nenhum arquivo `probeplan.go` foi criado.

As integrações cobertas por `add <integration> --plan` são:

- bancos: postgres, mysql, sqlite e sqlserver;
- ORM: gorm, sqlx e bun;
- NoSQL/analítico: couchbase, mongodb e clickhouse;
- cache: redis;
- mensageria: kafka, rabbitmq, nats e pubsub;
- protocolos: grpc e mcp;
- realtime: sse e websocket;
- observabilidade: otel e prometheus;
- documentação: swagger.

## Comandos sem preview

| Comando | Comportamento |
| --- | --- |
| `new` | cria o scaffold imediatamente |
| `init` | renderiza internamente um plano, mas sempre o aplica |
| `run` | executa o entrypoint e repassa argumentos |
| `build` | compila o entrypoint |
| `inspect` | somente leitura; aceita `--json` |
| `doctor` | diagnóstico de leitura; `--fix` não possui correções implícitas nesta versão |
| `version` / `help` | somente leitura |

Todo comando mutável sem preview usado nesta investigação foi executado
exclusivamente na área descartável. Em particular, `new` gerou os scaffolds e
`generate command probe` foi aplicado somente ao scaffold CLI temporário.

## Provas de não mutação

Foram executados com `--plan`:

- `docs`;
- `generate crud widget`;
- `generate tests --scan`;
- `generate command probeplan`;
- `add sse`;
- `add websocket`.

No scaffold service, um índice Git temporário registrou a baseline. Depois dos
previews, `git diff --name-only` permaneceu vazio. No scaffold CLI, os arquivos
planejados por `generate command probeplan --plan` não existiam após a
execução.
