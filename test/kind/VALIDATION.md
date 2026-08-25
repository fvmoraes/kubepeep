# Evidência local do harness Kind — 2026-08-24

Validações offline concluídas nesta estação:

- `sh -n test/kind/harness.sh`: passou;
- `dash -n test/kind/harness.sh`: passou;
- parse do driver `test/kind/app_e2e.py`: passou;
- `./test/kind/harness.sh static`: passou sem consultar o kubeconfig;
- binário real com kubeconfig sintético apontando para loopback fechado:
  `app_e2e.py --mode offline` passou, incluindo sessão/CSRF, dashboard parcial
  ou indisponível e leitura de recurso indisponível;
- parse seguro dos 56 documentos de `rbac.yaml` e de `cluster.yaml`: passou;
- regras exatas das identidades F6/F7, subjects separados e ausência de
  wildcard/`cluster-admin`: passou;
- todas as imagens das fixtures e o node Kind preservam tag legível e estão
  fixados por digest SHA-256; imagem mutável é rejeitada: passou;
- o Deployment degradado usa `nodeSelector` insatisfazível, e não uma imagem
  inexistente: passou;
- Secret versionado sem `data`/`stringData`: passou;
- `kind` v0.32.0 e cliente `kubectl` v1.33.3 estão disponíveis.

Os cenários dinâmicos que dependem de Kind não foram executados nesta estação
porque o daemon Docker não responde no contexto `desktop-linux`. O cenário
offline acima não consulta nem altera Kubernetes. Nenhum cluster nem recurso
Kubernetes foi criado, alterado ou removido durante a validação local.

Com Docker disponível, a reprodução completa é:

```sh
./test/kind/harness.sh create
./test/kind/harness.sh validate
./test/kind/harness.sh kubeconfigs
./test/kind/harness.sh app-e2e ./kubePeep
```

`create` e `validate` nunca removem o cluster. As mutações atingem somente
objetos descartáveis rotulados do harness, restauram restart/scale/delete e
recriam RoleBindings próprias após revogação. Processos de watch,
port-forward, exec e kubePeep possuem cleanup explícito e bounded.

A etapa `app-e2e` é opcional porque exige o binário compilado. Ela cobre API
HTTP/CSRF real; seleção de contexto e scopes `single/list/all`; dashboard
completo/parcial/offline; leituras e ações permitidas/negadas; SSE
snapshot/live/replay e reautorização após revogação; log follow; e exec
WebSocket com heartbeat, canais, ticket one-shot e nova autorização antes do
upgrade. A varredura final exige ausência de credenciais, payload do Secret,
CSRF, tickets e linhas cruas de log no estado/output da aplicação.

Os fluxos dinâmicos dependentes de Kind permanecem pendentes nesta estação
somente pela indisponibilidade do daemon Docker; o workflow CI executa
`app-e2e` com o binário real e falha fechado se qualquer marcador/revogação
exceder os limites.
