# Backlog após a v1

Este recorte preserva os objetivos das referências sem transformar recursos condicionais, multi-contexto ou ecossistema em requisitos implícitos da release. Itens abaixo não podem aparecer como disponíveis até sua própria entrega.

## Itens em continuidade do plano anterior

Os itens **B01–B09** do plano anterior permanecem em vigor com os mesmos pré-requisitos (ver `plan/v0/02-backlog-pos-v1.md`): Helm Releases (B01); Gateway API — Gateways, GatewayClasses, HTTPRoutes, GRPCRoutes (B02); VolumeAttributesClasses (B03); ValidatingAdmissionPolicies/Bindings (B04); multi-contexto somente leitura (B05); diff entre contextos/revisões (B06); logs agregados multi-contexto (B07); instâncias de Custom Resources (B08); novas mutações gerais como edit/apply (B09). Nenhum deles é desbloqueado implicitamente pelo trabalho da v1.

## Novos itens das referências v1

| ID | Item | Motivo do recorte | Pré-requisito para executar |
| --- | --- | --- | --- |
| B10 | Multi-contexto simultâneo (P2 da avaliação) | multiplica chamadas; exige cache, watches, fan-out, scheduler e índices estáveis primeiro | concluir F1–F2 com benchmarks; ADR; orçamento por origem; clients isolados; RBAC por contexto; testes com dois clusters/um offline |
| B11 | Prometheus como datasource opcional | histórico além do Metrics Server; deve ser opcional, desacoplado e não obrigatório | F2 concluída; decisão de UI; configuração por contexto sem ampliar permissões |
| B12 | Resource Diff entre origens (namespace×namespace, contexto×contexto, live×YAML) | depende de identidade/proveniência e permissões separadas | B10; preservar o diff live × last-applied existente como caso distinto |
| B13 | Sharded List and Watch (alpha 1.36) | experimental, desabilitado por padrão upstream; não pode ser dependência | discovery de feature; avaliação por benchmark da F0; flag experimental |
| B14 | Supply chain extra (CodeQL, SBOM CycloneDX/SPDX, Artifact Attestations, Dependency Review, OpenSSF Scorecard) | não compete com performance; pipeline atual já cobre o essencial | manter gates existentes; adicionar item a item com custo de CI medido |
| B15 | Extensibilidade/plugins | após estabilizar núcleo, cache e contratos de leitura | contratos de API estáveis; modelo de isolamento e permissões próprio |

## Regra de repriorização

Não há promessa de versão/data para estes itens. Promover um item exige atualizar esta tabela, a [matriz](01-matriz-de-entregas.md) e a fase responsável — nunca desmarcar uma tarefa obrigatória da release para abrir espaço.
