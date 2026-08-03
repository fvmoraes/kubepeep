# ADR 0002 — Saúde local e dependências degradadas

- Status: aceito
- Data: 2026-07-27
- Tarefas: F1-12, F1-22, F2-50

## Contexto

A interface deve continuar disponível quando kubeconfig, contexto, cluster ou
Metrics API estiverem indisponíveis. O handler `pkg/health` do Ginger marca a
resposta inteira como 503 quando qualquer checker falha, executa checkers com o
contexto da requisição sem deadline individual e serializa `err.Error()`.

Registrar o cluster como checker crítico faria um incidente externo parecer uma
falha do processo local e poderia expor detalhes de autenticação.

## Decisão

`GET /health` terá um payload composto pelo Kube Peep, mantendo os tipos e
checkers compatíveis com `pkg/health`, mas aplicando deadline e sanitização:

```json
{
  "data": {
    "status": "healthy",
    "components": {
      "application": {"status": "healthy", "code": "OK", "message": "Application is ready.", "checkedAt": "2026-07-27T12:00:00Z"},
      "sqlite": {"status": "healthy", "code": "OK", "message": "SQLite is available.", "checkedAt": "2026-07-27T12:00:00Z"},
      "kubeconfig": {"status": "unknown", "code": "NOT_CHECKED", "message": "Kubeconfig has not been checked.", "checkedAt": null},
      "context": {"status": "unknown", "code": "NOT_SELECTED", "message": "No context is selected.", "checkedAt": null},
      "cluster": {"status": "unknown", "code": "NOT_CHECKED", "message": "The cluster has not been checked.", "checkedAt": null}
    }
  }
}
```

Estados possíveis por componente:

- `healthy`: verificação concluída com sucesso;
- `degraded`: dependência externa configurada, porém indisponível ou negada;
- `unhealthy`: falha local crítica;
- `unknown`: ainda não configurada, não executada ou cancelada.

O estado agregado pode ser `degraded` por uma dependência externa e ainda
responder HTTP 200; somente `unhealthy` local controla HTTP 503. O contrato
canônico, incluindo códigos públicos, permanece em `docs/api.md`.

Semântica HTTP:

- `200`: aplicação e SQLite saudáveis, ainda que uma dependência Kubernetes
  esteja `degraded` ou `unknown`;
- `503`: bootstrap local incompleto, SQLite inutilizável ou shutdown iniciado;
- `500`: somente falha inesperada ao construir a própria resposta.

Cada checker terá deadline interno. A mensagem pública será allowlisted e
estável; o erro original passará pela sanitização central antes de qualquer log.

`GET /api/v1/status` conterá a visão operacional detalhada: versão, commit,
build date, porta, contexto selecionado, cluster, kubeconfig e Metrics API. Tanto
`/health` quanto `/api/v1/status` usarão `Cache-Control: no-store`.

Metrics API nunca será checker crítico.

## Uso do Ginger

- `pkg/health.Checker` será o contrato dos checks locais.
- `pkg/health.Status` orientará o formato interno.
- O handler padrão não será exposto diretamente, porque sua semântica
  all-or-nothing não representa dependências degradadas.
- Como `app.New` registra `/health` internamente, um mux externo mínimo
  registrará o health composto com precedência e delegará todas as demais rotas
  ao router Ginger. O endpoint padrão ficará inacessível externamente.

## Alternativas rejeitadas

### Registrar todos os componentes no handler Ginger padrão

Rejeitado porque cluster offline produziria 503 global e erros brutos seriam
serializados.

### Sempre responder 200

Rejeitado porque impediria automação local de detectar banco ou bootstrap
realmente indisponível.

### Remover kubeconfig/cluster do health

Rejeitado porque o prompt exige visibilidade separada. Eles permanecem no
payload, mas não controlam a saúde local.

## Consequências

- Health e status compartilham um serviço de snapshots para não divergir.
- A UI pode distinguir “Kube Peep indisponível” de “cluster offline”.
- Testes precisam cobrir cada combinação crítica/degradada e confirmar que
  erros de plugin não aparecem no JSON.
