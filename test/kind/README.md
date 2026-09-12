# Harness Kind canônico — Fase 0 e Fases 4 a 8

Este harness cria ou reutiliza o cluster Kind dedicado `kubepeep-f4`, instala
fixtures do MVP e prova RBAC e operações contra a API Kubernetes real. Ele não
apaga cluster, namespace nem credencial do usuário. O contexto administrativo
do Kind serve apenas para instalar/restaurar objetos rotulados; toda prova de
produto usa ServiceAccounts restritas.

## Identidades

| Identidade | Permissão em `kp-allowed` | Negado |
| --- | --- | --- |
| `manual-viewer` | leitura básica | `kp-denied`, namespaces, logs, Secrets e mutações |
| `namespace-lister` | leitura básica e `get/list/watch namespaces` | recursos em `kp-denied`, Secrets e mutações |
| `dashboard-viewer` | pods, workloads, eventos e `get pods/log` | `kp-denied`, namespaces, Secrets e mutações |
| `resource-viewer` | F6: `get/list/watch` dos recursos permitidos, logs e `get/list` de metadados de Secret | `kp-denied`, watch de Secret e mutações |
| `restart-actor` | `patch apps/deployments` no alvo exato | scale, delete, port-forward e exec |
| `scale-actor` | `update apps/deployments/scale` e `apps/statefulsets/scale` nos alvos exatos | restart, delete, port-forward e exec |
| `delete-actor` | `delete core/pods` no alvo descartável exato | demais mutações |
| `portforward-actor` | `get pods` e `create pods/portforward` no Pod exato | exec e demais mutações |
| `exec-actor` | `get pods` e `create pods/exec` no Pod exato | port-forward e demais mutações |
| `app-e2e` | F6 e as ações HTTP restart/scale/delete/port-forward/exec somente em `kp-allowed` | todas em `kp-denied` |

As cinco identidades de ação permanecem separadas. `app-e2e` existe apenas
para um teste black-box único do binário e recebe a soma exata dessas ações. Nenhuma regra usa wildcard,
`cluster-admin`, `impersonate`, `bind` ou `escalate`, e não há RoleBinding de
produto no namespace `kp-denied`.

## Fixtures cobertas

- os cinco workloads do MVP: Deployment, StatefulSet, DaemonSet, Job e
  CronJob;
- pods estável, reiniciando e descartável, com logs atuais e anteriores;
- Service, Ingress `networking.k8s.io/v1` e EndpointSlice
  `discovery.k8s.io/v1`;
- ConfigMap com conteúdo sintético para detalhe sob demanda;
- Secret metadata-only no YAML versionado;
- alvos equivalentes em `kp-denied` para que as operações negativas atinjam
  recursos existentes.

Todas as imagens de containers usam a forma legível `tag@sha256:digest`; a
imagem do node Kind também é passada explicitamente com digest. A fixture
`kp-degraded` fica indisponível por um `nodeSelector` deliberadamente
insatisfazível, sem depender de tag inexistente ou mutável. `static` rejeita
qualquer imagem que não esteja fixada dessa forma.

O Secret recebe 32 bytes aleatórios somente durante `create`, por patch vindo
de arquivo temporário `0600`. O valor não aparece no repositório, stdout,
evidência ou comando; `validate` não faz GET/LIST do objeto. A prova de que o
produto usa `PartialObjectMetadata` e não inclui campos proibidos ocorre nos
testes Go e, opcionalmente, no `app-e2e` através dos DTOs públicos.

## Uso

Pré-requisitos dinâmicos: Docker em execução, `kind`, `kubectl`, Python 3 e
acesso às imagens das fixtures. A validação offline também requer PyYAML, não
consulta o kubeconfig e não toca no cluster:

```sh
./test/kind/harness.sh static
```

Para criar/reutilizar o cluster e executar a matriz completa:

```sh
./test/kind/harness.sh create
./test/kind/harness.sh validate
```

`create` aplica as fixtures e verifica a matriz RBAC. `validate` também:

- executa LIST, detalhe e YAML permitidos e LIST/watch negados;
- lê logs atuais e anteriores sem imprimi-los;
- observa uma atualização real por watch e encerra o processo;
- aplica e desfaz restart de Deployment;
- atualiza e restaura os subresources scale de Deployment e StatefulSet;
- exclui e recria um Pod descartável;
- abre port-forward em `127.0.0.1`, transporta uma requisição e fecha o
  processo;
- executa `/bin/true` no container utilitário sem registrar saída;
- remove RoleBindings próprias, mantém a sessão port-forward já criada sob
  cleanup explícito, nega novos upgrades port-forward/exec e restaura o estado
  em traps.

O refresh da Fase 4 continua disponível:

```sh
./test/kind/harness.sh refresh-grant
./test/kind/harness.sh refresh-revoke
```

## Laboratório de performance da Fase 0

O laboratório mantém três evidências separadas; nenhuma delas é apresentada
como se fosse outra:

1. **Sintética:** `resources.Collect` e `api.CursorStore` reais, com lister,
   autorização, latência, falhas e watch sintéticos e identificados como tal no
   JSON.
2. **Kind real:** `create`, `validate` e `app-e2e` acima, opcionalmente com um
   dataset gerado explicitamente.
3. **Desktop nativo:** smoke manual Wails, registrado separadamente; o runner
   sintético não mede WebView, pintura ou startup de janela.

A suíte curta reproduz a baseline representativa e a matriz cobre
1/10/25/50/100/200 namespaces, 10/50/100/250/500 Pods por namespace, páginas
25/50/100, latências 0/20/50/100/200 ms, falhas 429/410/timeout/reset, perfis
`global`, `namespace-only`, `mixed`, `authorization-unavailable` e watch
on/off:

```sh
make benchmark
make benchmark-matrix
# ou diretamente, escolhendo o arquivo de saída:
./test/kind/harness.sh benchmark /tmp/kubepeep-representative.json
./test/kind/harness.sh benchmark-matrix /tmp/kubepeep-matrix.json
```

Os defaults ficam em `test/kind/.state/` (ignorado pelo Git), são gravados de
forma atômica com modo `0600` e só substituem artefatos que carregam o schema
do laboratório. O JSON registra SHA, estado clean/dirty, ambiente, aquecimento,
repetições, p50/p95, TTFB, primeira linha, página completa, requests, bytes,
over-fetch, alocações, goroutines, cursor, dimensão de watch e o histograma
fechado de outcomes de cada cenário. Todas as repetições precisam concordar
com o outcome esperado: divergência vira `unstable`, outcome unânime inesperado
vira `unexpected_error` e ambos encerram runner/harness com status não zero. O
relatório é preservado para diagnóstico mesmo quando esse gate falha. Ele não
contém nomes de recursos, namespace, UID, token ou payload.

O contrato C02 é explícito nos cenários: 200 namespaces com LIST global usam
uma única origem autorizada; fan-out público restrito continua limitado a 100
e o pedido acima desse teto é uma rejeição esperada; 200 origens diretas só
existem no perfil `internal-synthetic-origins` para stress de merge/store.

A geração de manifesto é deliberadamente separada e **nunca aplica nada**:

```sh
make benchmark-dataset \
  BENCHMARK_NAMESPACES=10 \
  BENCHMARK_PODS_PER_NAMESPACE=100
# equivalente:
./test/kind/harness.sh benchmark-dataset /tmp/kubepeep-10x100.yaml 10 100
```

Os tamanhos aceitos são 1/10/25/50/100/200 namespaces e 10/50/100/250/500
Pods por namespace. O manifesto determinístico usa imagem `pause` pinada e
inclui identidades RBAC distintas para LIST global, leitura por namespace,
misto e leitura sem watch. Indisponibilidade de autorização e falhas de rede
são somente sintéticas: RBAC sem binding significa “sem opinião”, não simula
um apiserver/SAR indisponível. Aplicar, dimensionar recursos do cluster e
remover o dataset são ações manuais; em especial, 200 × 500 gera 100.000 Pods
e não participa de `create`, `validate`, `verify` nem de qualquer gate padrão.

## Kubeconfigs restritos

O comando abaixo grava tokens TokenRequest de uma hora, com modo `0600`, no
diretório ignorado `.state/`:

```sh
./test/kind/harness.sh kubeconfigs
```

São produzidos kubeconfigs para as três identidades F4/F5, `resource-viewer`,
as cinco ações isoladas e `app-e2e`. O harness recusa sobrescrever symlink,
arquivo não regular ou kubeconfig de outro contexto.

## Black-box opcional do binário

Com as fixtures já criadas, informe um binário executável:

```sh
./test/kind/harness.sh app-e2e ./dist/kubePeep
```

O comando cria kubeconfigs e diretórios HOME/XDG temporários, inicia quatro
instâncias isoladas (`kp-allowed`, `kp-denied`, `all` e `offline`), descobre a porta via
`kubePeep status` (contrato `kubepeep-control/v1`) e usa Origin/Host e CSRF
obtidos em `/api/v1/session`. Ele
seleciona o contexto real; cria e seleciona scopes `single`, `list` e `all`;
prova `all` permitido somente com `list namespaces` e negado sem essa
capacidade; verifica dashboard completo, parcial e offline; e exige falha
fechada das leituras e ações do produto no namespace negado. Um no-match de
SSAR permanece `503/AUTHORIZATION_UNAVAILABLE`; somente uma negação
autoritativa observada diretamente é publicada como `403/FORBIDDEN`.

No fluxo permitido, o próprio produto abre SSE de recursos e logs, observa
snapshot/live, reconecta com `Last-Event-ID` para replay e permanece conectado
enquanto o harness revoga a RoleBinding F6. Ambos os streams precisam terminar
por reautorização periódica. Exec usa ticket efêmero e WebSocket RFC 6455 real:
valida `ready`, heartbeat com eco, canais stdout/stderr, terminal, fechamento e
ticket one-shot. Outro ticket é criado antes da revogação de RBAC. Como um
RBAC no-match do Kind produz SSAR sem opinião, tanto o upgrade quanto uma nova
ação exec do produto precisam falhar fechados com
`503/AUTHORIZATION_UNAVAILABLE`; o harness comprova separadamente o `Forbidden`
autoritativo em uma leitura direta do apiserver. Todas as RoleBindings, Pods e
escalas são restauradas em traps.

O driver nunca imprime bodies, CSRF, kubeconfig, ticket, logs ou dados de
cluster. Depois de parar cada processo e antes de apagar seu diretório, o
harness compara o estado/output contra o token do kubeconfig e contra o payload
aleatório real do Secret; o driver também procura CSRF, protocolos efêmeros e
linhas cruas de log. A instância offline usa somente um token sintético e um
endpoint loopback fechado, sem derrubar nem alterar o cluster Kind.

Em cluster reutilizado, verificações de ownership distinguem `NotFound` de
falha do apiserver. O Pod de previous-log e o Event inicial são substituídos um
por vez, somente após reaplicar/validar o conjunto; os deletes de fixtures e
RoleBindings levam a UID observada como precondição, e cada fixture removida
possui recuperação canônica imediata. A recuperação é armada antes do DELETE:
HUP/INT/TERM encerram o fluxo pelo trap de saída, uma resposta de DELETE perdida
é reconciliada repetindo a operação com a mesma UID e um objeto novo em
terminação nunca é aceito como restaurado. O teste negativo de delete também
valida ownership, exige `Forbidden` autoritativo e confirma que a UID negada
permaneceu inalterada. Restaurações normais usam `create`, não `apply`, para não
adotar um homônimo surgido entre a leitura e a recuperação.

Para outro cluster Kind dedicado, defina `KUBEPEEP_KIND_CLUSTER`. Não existe
comando de destruição automática; remover o cluster é sempre decisão manual.
