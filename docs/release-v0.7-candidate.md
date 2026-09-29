# Candidato local da v0.7

**Estado:** pronto para decisão de publicação. O candidato funcional é o commit
`52aa3db` da branch `review/plan-v0.7`; o comparativo de performance foi
coletado no ancestral funcional `b148a6028dd800e30b9cc54a1cd14a8c862f1fac`.
Esta preparação não cria tag, release remota nem push; o nome e a versão final
dos pacotes continuam sujeitos à decisão do mantenedor.

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
| benchmark representativo/matriz/protocolo | aprovado em árvore limpa; 10 + 47 cenários e protocolo Kind |
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
- Helm, Gateway API, edição arbitrária de YAML, Prometheus e multi-contexto
  simultâneo continuam fora do escopo da v0.7.

## Decisão de publicação

O repositório está preparado para o mantenedor escolher versão/tag, revisar as
notas, gerar os pacotes finais e publicar. Até essa decisão, o candidato fica
somente em commits locais e pode ser descartado sem rollback de ambiente.
