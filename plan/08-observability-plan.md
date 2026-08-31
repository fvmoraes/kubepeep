# Plano de Observabilidade do KubePeep

## 1. Objetivo

Definir a estratégia de observabilidade do KubePeep, cobrindo logs internos, métricas, traces e OpenTelemetry, mantendo a segurança como premissa inegociável.

## 2. Estado atual

- Logger Ginger envolve `slog.Logger`.
- Logs operacionais em `~/.kubePeep/logs/kubePeep.log` e stdout.
- Campos allowlisted: timestamp, level, component, operation, request_id, context, namespace, resource, duration, error_code.
- OpenTelemetry é opt-in, desativado por padrão.
- Sem exporter, socket ou tráfego quando não configurado.
- Métricas internas não expostas.

## 3. Princípios

- Nunca registrar payloads, credenciais, valores de Secret, logs Kubernetes, comandos/saída de `exec`.
- Sanitizar todo conteúdo externo antes de logar.
- Observabilidade não pode afetar o caminho crítico.
- OTel é opcional; sem configuração explícita, nenhum tráfego é gerado.
- Dados sensíveis nunca viram atributos/events de traces.

## 4. Logs internos

### 4.1 Campos padronizados

Manter e expandir os campos allowlisted:

```text
timestamp
level
component
operation
request_id
context
namespace
resource
verb
duration_ms
error_code
error_type
```

### 4.2 Melhorias recomendadas

| ID | Melhoria | Prioridade | Complexidade |
| --- | --- | --- | --- |
| O-01 | Adicionar `duration_ms` numérico para facilitar agregação. | P2 | XS |
| O-02 | Logar início/fim de operações de longa duração (scans, streams). | P2 | S |
| O-03 | Rate limiting de logs de erro repetidos. | P2 | S |
| O-04 | Correlação de request_id entre frontend e backend. | P2 | M |
| O-05 | Logs de lifecycle (startup, shutdown, cleanup) estruturados. | P2 | S |

### 4.3 Rotação e retenção

Manter política atual:
- Rotacionar ao atingir 10 MiB.
- Manter no máximo 5 backups.
- Remover backups com mais de 14 dias.
- Permissões restritas (`0600`/DACL equivalente).

## 5. Métricas internas

### 5.1 Métricas propostas

| Métrica | Tipo | Uso |
| --- | --- | --- |
| `kubepeep_requests_total` | counter | Total de requests por método/rota/status. |
| `kubepeep_request_duration_ms` | histogram | Duração de requests. |
| `kubepeep_active_streams` | gauge | Streams SSE/WS ativas. |
| `kubepeep_active_sessions` | gauge | Sessões de port-forward/exec ativas. |
| `kubepeep_kubernetes_requests_total` | counter | Chamadas client-go por recurso/verbo/status. |
| `kubepeep_kubernetes_request_duration_ms` | histogram | Duração de chamadas Kubernetes. |
| `kubepeep_rbac_cache_size` | gauge | Tamanho do cache de autorização. |
| `kubepeep_sqlite_connections_active` | gauge | Conexões SQLite ativas. |

### 5.2 Exposição

- Expor métricas em `/metrics` apenas quando habilitado via configuração.
- Default desabilitado.
- Bind loopback obrigatório.
- Sem autenticação no MVP (rede local confiável).

## 6. Traces e OpenTelemetry

### 6.1 Configuração

Manter schema fechado em `config.yaml`:

```yaml
observability:
  otel:
    enabled: false
    endpoint: null
    protocol: http/protobuf
    insecure: false
```

### 6.2 Melhorias

| ID | Melhoria | Prioridade | Complexidade |
| --- | --- | --- | --- |
| O-06 | Criar spans para requests HTTP, operações Kubernetes e ações mutáveis. | P2 | M |
| O-07 | Propagar trace context internamente. | P2 | M |
| O-08 | Adicionar atributos seguros (operation, resource, namespace, verb). | P2 | S |
| O-09 | Garantir que falha do exporter não afete o core. | P1 | S |
| O-10 | Validar que `enabled=false` não inicia exporter/socket. | P1 | S |

### 6.3 Segurança de traces

- Nunca incluir: payloads, credenciais, valores de Secret, logs, comandos de exec.
- Atributos de erro apenas códigos allowlisted.

## 7. Alertas e diagnóstico

### 7.1 `kubepeep doctor`

Expandir checks:

| Grupo | Check |
| --- | --- |
| logging | sink acessível, rotação funcional. |
| observability | OTel configurado corretamente; endpoint acessível quando habilitado. |
| metrics | `/metrics` habilitado e respondendo quando configurado. |

### 7.2 Dashboard de saúde interna

Considerar adicionar na UI (Settings ou página oculta):
- Status do processo local.
- Últimos erros operacionais (sanitizados).
- Estatísticas de requests/stream/sessões.

**Prioridade:** P3. **Risco:** não expor informação sensível.

## 8. Implementação

### Fase 1 — Logs (P2)
- O-01 a O-05.
- Manter sanitização.

### Fase 2 — Métricas (P2)
- Implementar métricas internas com biblioteca leve (ex.: Prometheus client ou métricas próprias).
- Expor `/metrics` opcional.

### Fase 3 — Traces OTel (P2)
- O-06 a O-10.
- Testar com collector local.

### Fase 4 — UI de saúde (P3)
- Página oculta ou Settings avançado.

## 9. Critérios de aceite

- [ ] Logs nunca contêm dados sensíveis.
- [ ] OTel desligado não gera tráfego.
- [ ] Métricas são opcionais e seguras.
- [ ] `doctor` reporta estado de observabilidade.
- [ ] Documentação `docs/observability.md` criada.
- [ ] Testes validam ausência de vazamento.
