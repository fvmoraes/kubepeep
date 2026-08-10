# Harness Kind — Fases 4 e 5

Este harness cria um cluster Kind dedicado e fixtures RBAC mínimas para os
fluxos Kubernetes da Fase 4. Ele não apaga cluster, namespace ou credencial do
usuário. O contexto administrativo efêmero do Kind serve somente para instalar
as fixtures; os kubeconfigs entregues ao kubePeep pertencem a ServiceAccounts
restritas.

## Identidades e cenários

| Identidade | Permitido | Negado | Cenário |
| --- | --- | --- | --- |
| `manual-viewer` | leitura de recursos em `kp-allowed` | `kp-denied`, `list namespaces`, logs, exec, Secrets e mutações | `single`, `list` parcial e `all` indisponível |
| `namespace-lister` | o mesmo acesso em `kp-allowed` mais `get/list/watch namespaces` | recursos em `kp-denied`, Secrets e mutações | `all` enumera somente namespaces reais e preserva negações por namespace |
| `dashboard-viewer` | pods, workloads, eventos e `get pods/log` em `kp-allowed` | `kp-denied`, `list namespaces`, Secrets e mutações | dashboard permitido/parcial com logs explícitos e Metrics API ausente |

Nenhuma regra usa `cluster-admin`, `*`, `impersonate`, `bind` ou `escalate`.
`kp-denied` contém a mesma Role de leitura, mas não possui RoleBinding no estado
base. Isso permite mudar RBAC entre duas consultas sem ampliar permissões além
daquele namespace.

Mapeamento esperado no produto:

- `single`: `manual-viewer` + `kp-allowed` retorna leitura permitida;
- `list`: `manual-viewer` + `kp-allowed,kp-denied` produz cobertura parcial,
  sem inventar dados no namespace negado;
- `all`: `manual-viewer` recebe negação autoritativa de `namespaces.list`,
  enquanto `namespace-lister` enumera a resposta real da API;
- denial: `pods/log`, `pods/exec`, Secrets e mutações continuam negados;
- refresh: a RoleBinding temporária alterna `pods.list` em `kp-denied` entre
  `denied` e `allowed`; `GET /api/v1/permissions?...&refresh=true` deve observar
  a decisão atual, e a operação Kubernetes real permanece a autoridade final.
- dashboard: `kp-restarting` gera restart e log sintético, `kp-degraded`
  permanece indisponível, `kp-warning` preserva um Event `Warning` com count 3
  e o cluster Kind canônico não instala Metrics API.

## Uso

Pré-requisitos para o cluster: Docker em execução, `kind` e `kubectl`. A
validação offline usa `python3` com PyYAML. Primeiro valide os arquivos sem
tocar em cluster nem consultar o contexto Kubernetes atual:

```sh
./test/kind/harness.sh static
```

Depois crie (ou reutilize) somente o cluster dedicado `kubepeep-f4`, aplique as
fixtures e execute as asserções base:

```sh
./test/kind/harness.sh create
./test/kind/harness.sh validate
```

O comando `validate` prova os cenários `single`, `list`, `all` e denial com
`SelfSubjectAccessReview` real por meio de `kubectl auth can-i`. Ele também cria
e remove uma única RoleBinding rotulada, verificando a transição de permissão
nos dois sentidos. Um trap remove essa concessão se o teste for interrompido.

Para executar o kubePeep com credenciais restritas, gere kubeconfigs com token
de uma hora. O diretório padrão `.state/` é ignorado pelo Git e os arquivos são
gravados com modo `0600`:

```sh
./test/kind/harness.sh kubeconfigs
KUBECONFIG=test/kind/.state/manual-viewer.kubeconfig \
  go run ./cmd/kubePeep start --no-browser
```

O contexto `kubepeep-f4-manual` testa `single`/`list`; o contexto
`kubepeep-f4-all` testa `all`; `kubepeep-f5-dashboard` testa o overview e o
scan explícito de logs. Para intercalar uma chamada de refresh do produto
entre mudanças explícitas de RBAC:

```sh
./test/kind/harness.sh refresh-grant
# GET /api/v1/permissions?namespace=kp-denied&capability=pods.list&refresh=true
./test/kind/harness.sh refresh-revoke
# repetir a mesma chamada com refresh=true
```

`refresh-revoke` recusa remover uma RoleBinding de mesmo nome que não tenha o
rótulo de propriedade do harness. O script também recusa reutilizar qualquer
um dos três namespaces se ele já existir sem esse rótulo.

Para outro cluster Kind dedicado, defina `KUBEPEEP_KIND_CLUSTER`. Não há comando
automático de destruição; a remoção do cluster é uma decisão manual e explícita.
