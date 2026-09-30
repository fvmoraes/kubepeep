# Fechamento técnico da v0.7.0

**Versão escolhida:** `0.7.0`, conforme decisão do mantenedor. As notas estão
preparadas no `CHANGELOG.md` e o Wails usa `productVersion: 0.7.0`.
A auditoria comportamental e os benchmarks representativo, matriz e protocolo
foram concluídos em `caaf40a`, em árvore limpa; a comparação histórica permanece
em `b148a60`. A revisão de fechamento da branch `review/plan-v0.7` cobre também
os contratos de versão e publicação. A tag oficial será `0.7.0`, sem prefixo `v`.

## Destaques

- paginação incremental com cursores limitados, cancelamento, coalescing,
  selectors e tratamento recuperável de 410/429;
- cache de recursos e watches sob demanda, limitados por memória, geração e
  autorização, sem persistência de conteúdo Kubernetes;
- listas progressivas e virtualizadas, prefetch de uma página e shell Wails
  com carregamento assíncrono;
- scope default obrigatório por contexto, ativado antes das leituras;
- workspace completo, ações por kind, confirmações, RBAC visível e ações em
  massa compatíveis;
- Problems Engine, Investigation, logs agregados, busca local e Diagnostics;
- PartialObjectMetadata e Protobuf nos caminhos medidos, AIMD para 429 e
  prefetch relacionado com prioridade; gzip permanece opt-in;
- auditoria final dos 44 destinos nas quatro resoluções de aceite.

A revisão aprofundada fechou falhas de retry/default, relações/diagnóstico,
cancelamento dos logs, starvation do Dashboard, crescimento e cleanup de
watches e gates de release. Os 224 MiB agora incluem snapshots dos workers;
o scan inclui o desktop real e pacotes aninhados. O gate de CI exige ambas
as plataformas nativas e considera a execução mais recente de cada check.
O candidato agora propaga o SHA exato de origem e a versão dos metadados aos
três jobs nativos. A configuração de npm no Windows usa Bash explicitamente;
reexecuções preservam as notas revisadas sem duplicar entradas no changelog.
Os targets desktop locais recebem versão, commit e data pelos mesmos ldflags
usados no build CLI.

## Compatibilidade e segurança

- Secrets permanecem metadata-only;
- cursor e resource cache permanecem somente em memória;
- exec e port-forward usam autorização exata e bind loopback;
- ações revalidam geração, preconditions, CSRF e RBAC no backend;
- logs de ações não registram contexto, namespace ou nome do objeto;
- formatos antigos/aggregated/CRD preservam fallback JSON.

## Evidência do candidato

| Gate | Resultado |
| --- | --- |
| `make verify`, `make test`, `make test-e2e`, `make test-race` | aprovado |
| `make format-check lint typecheck` | aprovado; 8 avisos Fast Refresh conhecidos, zero erro |
| `make build smoke`, `make build-desktop` | aprovado |
| Kind `create/validate` e `app-e2e` | aprovado; cluster preservado |
| Wails nativo + AT-SPI | scope default restaurado; Pods exibiu 9 linhas Kind reais |
| benchmark representativo/matriz/protocolo | repetido em `caaf40a`, árvore limpa; 10 + 47 cenários e protocolo Kind |
| segurança, módulos, vulnerabilidades e artefatos | aprovado; zero vulnerabilidade Go alcançável e zero advisory npm |

O comparativo completo está em [performance-baseline.md](performance-baseline.md)
e o registro item a item em
[03-evidencias-execucao.md](../plan/v0.7/03-evidencias-execucao.md).

## Limitações conhecidas

- medições sintéticas não modelam jitter, etcd, admission webhooks nem cluster
  remoto real;
- gzip remoto usa rede modelada de 30 ms/10 Mbit/s;
- a amostra Wails final comprova render e dados via acessibilidade, mas a série
  de startup controlada continua sendo a registrada na F3;
- esta auditoria repetiu o build Wails Linux e o E2E Kind; a interação nativa
  por AT-SPI é a evidência histórica da F7. Os workflows corrigidos passaram
  por actionlint e harness local; builds Windows/macOS e publicação não foram
  executados nesta revisão;
- Helm, Gateway API, edição arbitrária de YAML, Prometheus e multi-contexto
  simultâneo continuam fora do escopo da v0.7.

## Publicação oficial

A versão está definida e preparada. A integração da branch em `main` aciona
`verify.yml` e `release.yml`; o pipeline calcula o incremento minor desde a
última tag oficial e preserva as notas da `0.7.0`. A tag e a GitHub Release só
são criadas depois dos quatro checks obrigatórios e dos builds/artefatos
multiplataforma aprovados. A revisão local não declara esses jobs remotos como
executados nem substitui o aceite das plataformas Windows/macOS.
