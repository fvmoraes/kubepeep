# Backlog após a v1

Este recorte preserva o objetivo da referência sem tornar recursos condicionais ou agregação de clusters requisitos implícitos da release. A v1 usa uma seleção ativa e mantém suas ações atuais. Os itens abaixo não podem aparecer como disponíveis até sua própria entrega.

| ID | Item | Motivo do recorte | Pré-requisito para executar |
| --- | --- | --- | --- |
| B01 | Helm Releases | a referência permite preparar navegação para depois | definir acesso aos metadados de release sem ler/expandir Secrets; adapter, RBAC e teste de vazamento |
| B02 | Gateway API: Gateways, GatewayClasses, HTTPRoutes, GRPCRoutes | extensão opcional do cluster | discovery real por GVR/versão; escolher matriz suportada; escopos, routes e RBAC separados |
| B03 | VolumeAttributesClasses | recurso condicional na referência | matriz de versões/discovery; DTO seguro e relações com PVC |
| B04 | ValidatingAdmissionPolicies e ValidatingAdmissionPolicyBindings | condicionais à API disponível | discovery/compatibilidade e DTOs para expressões/param refs com limites e revisão de conteúdo |
| B05 | Multi-contexto simultâneo somente leitura | altera geração, clients, cache e proveniência; não é pré-requisito da expansão UI/UX | ADR, orçamento por origem, clients isolados, RBAC por contexto e testes com dois clusters/um offline |
| B06 | Diff entre contextos ou revisões arbitrárias | depende de identidade/proveniência e permissões separadas | concluir B05 para comparação multi-contexto; manter diff last-applied e comparação no contexto ativo da F5 |
| B07 | Logs agregados de múltiplos pods/contextos | exige budgets e backpressure por fonte; logs atuais permanecem na v1 | contratos explícitos de origem, concorrência, descarte e cancelamento; testes de cliente lento e RBAC divergente |
| B08 | Instâncias de Custom Resources | listar CRDs não autoriza inspecionar qualquer CR | descoberta de schema, política de campos sensíveis, limites e capability por GVR |
| B09 | Novas mutações (delete de Deployment/Service, edição/apply) | exemplos visuais não ampliam o catálogo operacional vigente | requisito de produto próprio, dry-run quando aplicável, preconditions, autorização e confirmação por alvo |

Para B05, preservar os requisitos históricos F9-66–75: seleção de leitura separada da mutável; envelope com origem/geração; fan-out cancelável; capabilities nunca unidas; falhas/retry/stale por origem; mutações somente após selecionar um alvo; testes de resposta atrasada, identidades homônimas e vazamento cruzado. A v1 já deve mostrar esses estados honestamente dentro do contexto ativo.

Não há promessa de versão/data para estes itens. Repriorização deve atualizar esta tabela e a matriz v1, sem simplesmente desmarcar uma tarefa obrigatória da release.
