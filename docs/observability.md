# Observabilidade do KubePeep

## 1. Princípios

- Nunca registrar payloads, credenciais, valores de Secret, logs de containers ou comandos/saída de `exec`.
- Observabilidade nunca afeta o caminho crítico: falha de exportação é silenciosa e opt-in.
- Sem configuração explícita, nenhum tráfego de observabilidade é gerado para fora do processo.
- Campos de log e séries de métricas são allowlisted por design.

## 2. Logs internos

### 2.1 Schema fechado

O handler JSONL (`internal/logging`) aceita somente os campos:

```text
timestamp, level, component, operation, request_id,
context, namespace, resource, duration, duration_ms, error_code
```

- `duration` é a duração formatada para humanos (ex.: `25ms`).
- `duration_ms` é o valor numérico em milissegundos para agregação (O-01).
- Todo campo textual passa por sanitização: regexes de credenciais (Bearer/Basic, JWT, AWS, GitHub, `authorization:`/`token:`/`secret:`), remoção de caracteres de controle, truncamento em 1024 bytes.

### 2.2 Retenção

- Rotação em 10 MiB, 5 backups, remoção após 14 dias, permissões `0600`.

## 3. Métricas internas

### 3.1 Registro

`internal/observability` implementa um registro próprio, sem dependências externas:

- Contadores e gauges em memória, seguro para concorrência.
- Nomes de métricas e chaves de label são allowlisted; desconhecidos são ignorados (sem explosão de cardinalidade).
- Renderização no formato de texto Prometheus, com escape correto de labels e saída determinística.

### 3.2 Séries expostas

| Série | Tipo | Labels |
| --- | --- | --- |
| `kubepeep_requests_total` | counter | `method`, `route`, `status` |

O label `route` usa o padrão de rota do `http.ServeMux` (Go 1.22+), limitado à tabela estática de rotas — caminhos fornecidos pelo usuário nunca viram labels. Requisições não roteadas recebem `route="unmatched"`.

### 3.3 Endpoint `/metrics`

- **Desabilitado por padrão.** Sem a flag, a rota não é registrada (404).
- Habilitação via `config.yaml`:

```yaml
version: 1
observability:
  metrics:
    enabled: true
```

- Quando habilitado, responde em `http://127.0.0.1:<port>/metrics` — o servidor inteiro já faz bind exclusivo em loopback; nenhuma interface externa é aberta.
- O middleware de contagem envolve toda a cadeia externa (RequestID → Recovery → Requests → Host).

## 4. Traces e OpenTelemetry

- OTel permanece **opt-in** (`observability.otel.enabled`, default `false`).
- Com `enabled=false` nenhum exporter, socket ou goroutine é criado.
- Quando habilitado em versões futuras: spans para requests HTTP, operações Kubernetes e ações mutáveis, apenas com atributos allowlisted (operation, resource, namespace, verb). Falha do exporter nunca afeta o core.

## 5. Doctor

`kubePeep doctor` consulta a saúde local e apresenta diagnóstico sanitizado.
A composição da aplicação inclui a saúde do sink de log e do SQLite;
validação de rotação e resposta de `/metrics` também integra os testes dos
pacotes abaixo. Um resultado de doctor não substitui esses testes.

## 6. Testes

| Garantia | Teste |
| --- | --- |
| Campos fora do schema são descartados; sanitização ativa | `internal/logging` |
| `duration_ms` numérico acompanha `duration` | `internal/logging` |
| Métrica/label desconhecida ignorada; escape; determinismo | `internal/observability` |
| Middleware captura status reais (incluindo 4xx/5xx) | `internal/observability` |
| `/metrics` ausente sem registry; presente e correta com registry | `internal/app/metrics_test.go` |
