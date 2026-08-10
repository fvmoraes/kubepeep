# Evidência local do harness Kind — 2026-08-10

Validações concluídas nesta estação:

- `sh -n test/kind/harness.sh`: passou;
- `dash -n test/kind/harness.sh`: passou;
- `./test/kind/harness.sh static`: passou sem consultar o kubeconfig atual;
- parsing seguro de todos os documentos em `rbac.yaml` e de `cluster.yaml`:
  passou;
- inspeção estrutural e textual para `cluster-admin` e wildcards em
  `resources`/`verbs`: passou;
- `kubectl` cliente v1.33.3 está disponível.

A execução contra API real não foi possível nesta estação: o executável
`kind` não está instalado e o daemon Docker não responde. Nenhum cluster ou
recurso Kubernetes foi criado, alterado ou removido durante esta validação.

Quando esses dois pré-requisitos estiverem disponíveis, a evidência dinâmica é
reproduzida por:

```sh
./test/kind/harness.sh create
./test/kind/harness.sh validate
./test/kind/harness.sh kubeconfigs
```

`create` não remove cluster ou namespace; `validate` altera somente a
RoleBinding temporária `kp-denied/kp-refresh-probe`, verifica concessão e
revogação e restaura o estado negado mesmo após interrupção.
