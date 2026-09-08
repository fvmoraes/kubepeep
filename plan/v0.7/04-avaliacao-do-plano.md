# Avaliação do plano v0.7

## Resultado

O plano organiza as entregas e preserva as principais premissas do produto, mas a revisão inicial identificou conflitos de contrato que impediam executá-lo literalmente com segurança. As resoluções documentais estão em [C01–C06](05-contratos-e-aceite.md) e foram incorporadas à matriz e às fases; a validação funcional continua pendente. Esta avaliação não declara F0–F7 implementadas. A base examinada é `d71d488`; `origin/main` local está um commit à frente (`598e167`, release 0.6.2). A integração dessa referência deve preceder o baseline funcional, com revisão do diff.

## Achados da revisão inicial e impacto

| Prioridade | Evidência | Impacto e resolução necessária |
| --- | --- | --- |
| Alta | F1-03 promete sorts globais via chunks; a referência de performance §10 exige cada origem ordenada pelo mesmo comparador. `docs/api.md` §5.3 declara sorts sobre a janela coletada; `MergeOriginPages` em `internal/services/resources/cursor.go` ordena apenas os buffers recebidos. | Um heap não garante ordem global se houver um item anterior ainda não lido na origem. Definir contrato por estratégia antes de F1: manter `filterScope=page` no caminho limitado; só prometer ordenação de coleção com snapshot completo ou origem comprovadamente ordenada. Testar um item extremo que chega no segundo chunk e desempates asc/desc. Inverter namespaces também não resolve, sozinho, a ordem descendente dos nomes dentro deles. |
| Alta | F0-09, D05 e F7 exigem 200 namespaces sem `LIMIT_EXCEEDED`; `MaximumNamespaces` em `internal/services/resources/types.go` vale 100. | Separar cenário global autorizado de fan-out restrito e de benchmark interno do cursor. O aceite não pode exigir remover uma rejeição legítima de limite. Qualquer suporte a 200 namespaces restritos precisa de decisão explícita e orçamento, sem aumentar limites apenas para passar o teste. |
| Alta | F1-09 reduz a chave de autorização a generation/namespace/group/resource/verb. `authorization.Key` também contém `Subresource` e `ResourceName`; existe teste de separação de todas as dimensões. | Preservar a chave completa existente, invalidação por geração e por 403. Uma chave reduzida pode reutilizar autorização entre recurso, subrecurso e alvo diferentes. Revisar D12 junto com F1-09 antes da implementação. |
| Alta | F2-01 inclui generation/selector na chave do cache; F2-03 e D17 descrevem watch apenas por contexto/scope/GVR. `WatchKey` atual inclui generation, namespace, selector e topic. | Especificar a identidade completa de compartilhamento e o tratamento de revogação antes de F2. Cobrir duas seleções diferentes, troca de geração e autorização revogada; cache antigo não pode reaparecer como dado autorizado. |
| Média | D01/D04 exigem instrumentação de cache em F0, mas o resource cache só é entregue em F2. | Em F0 definir o contrato de instrumentação e medir os caches existentes; a evidência de spans e métricas do novo resource cache pertence a F2. Não introduzir implementação fictícia para fechar F0. |
| Média | A matriz diz que todas as linhas são gate; F6 condiciona ativação a ganho medido. | Explicitar que avaliação com resultado sem ganho encerra o item condicional com decisão registrada e flag desligada. Ausência de benchmark continua pendente. F7 precisa distinguir esses resultados. |
| Média | README/escopo/matriz continham cinco destinos Markdown inexistentes e misturavam v1/v0.7. A base declarada era `e48627a`. | Links corrigidos e versão corrente uniformizada nos documentos executáveis. O nome histórico `02-backlog-pos-v1.md` foi preservado para evitar renomeação desnecessária. Referências e plano arquivado não foram reescritos. |

## Mapa de impacto para execução

- F0/F1: `Collect` é consumido por `collectResource` e `clusterCollect` na integração Kubernetes. Paginação afeta handlers, cursor store, DTOs, `meta.page`, queries e testes de todas as coleções; não apenas Pods.
- F2/F3: chave e ciclo de vida de watch/cache afetam SSE, bridge desktop, geração, cancelamento e memória do frontend. Dados anteriores só podem permanecer visíveis dentro da seleção autorizada correspondente.
- F4: default por contexto envolve preferências persistidas, seleção e namespace global. Detalhar migração de preferências existentes, exclusão do default e contextos sem scopes antes de alterar esses contratos.
- F5: investigação deve reutilizar diagnósticos existentes e sinalizar cobertura incompleta do cache sob demanda. Busca local não equivale a inventário completo do cluster.
- F6/F7: benchmarks precisam identificar tamanho, permissões, hardware, aquecimento, repetições e percentil; testes web não demonstram execução nativa Wails.

## Validação e limites desta etapa

Foram lidos os documentos executáveis F0–F7, matriz, escopo e backlog, consultadas as referências relevantes, memória canônica, índice/cobertura, símbolos e consumidores do coletor e o Makefile. A avaliação não é uma auditoria integral da aplicação nem reprodução do bug de Pods.

O escopo alterado é Markdown: validar destinos relativos e diff, além do gate de segurança antes do commit. Build, lint de código, testes de aplicação, E2E, race e benchmark não foram executados nesta revisão documental e permanecem obrigatórios para o aceite funcional das fases. Não há evidência de correção de Pods/cliques nesta etapa.

## Confronto com o pedido

- **IMPLEMENTADO:** avaliação documental e dos contratos destacados, branch dedicada, correção dos links e identificação de versão, preservação das alterações preexistentes do usuário, sem push.
- **ALTERADO:** a etapa inicial é avaliação documental; não se confunde com a Fase 0 funcional do plano. Contratos conflitantes foram resolvidos documentalmente em C01–C06, sem alteração de código ou aceite funcional.
- **PENDENTE:** execução F0–F7 conforme os contratos resolvidos, seus testes, builds, benchmarks e evidências. Impacto: a release não está aprovada. Próxima etapa: implementar e validar F0; avançar somente após autorização do usuário.
