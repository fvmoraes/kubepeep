# Contratos e critérios de aceite v0.7

Estas decisões resolvem os conflitos da [avaliação](04-avaliacao-do-plano.md). Complementam as fases e a matriz; as referências originais permanecem preservadas. São especificação aprovada para execução, não evidência de implementação.

## C01 — Paginação e ordenação

`GlobalNative` descreve o alcance autorizado da chamada LIST, não uma garantia de ordenação global. Preservar `meta.page.filterScope=page` nas consultas que ordenam apenas a janela coletada. Usar `collection` somente quando filtro e ordenação abrangem comprovadamente toda a coleção correspondente à consulta.

O heap seleciona candidatos já conhecidos; não descobre um item anterior ainda não lido. Lazy merge global exige que **toda a sequência** de cada origem, incluindo refills, seja monotônica pelo mesmo comparador e desempate. Ordenar cada chunk isoladamente não satisfaz essa pré-condição. Exemplo de regressão: primeira página da origem contém restarts 1 e a segunda contém 100; ordenar chunks não permite anunciar o primeiro resultado como máximo global.

Na F1, otimizar o merge da janela limitada sem alterar seu contrato público. NamespaceSequential só usa o caminho antecipado quando a ordem completa requerida está demonstrada; asc/desc inclui nomes e desempates dentro do namespace. Nos demais casos preservar o caminho limitado existente, com indicação honesta de ordem por página. A meta de primeira página sublinear aplica-se aos caminhos elegíveis, não a sorts globais arbitrários sobre dados ainda não lidos.

Na F2, um snapshot completo, autorizado e dentro do orçamento permite ordenação de coleção; parcial, expirado ou em reconstrução não deve ser anunciado como completo. Não carregar a coleção inteira sem limite apenas para satisfazer o comparador. Cobertura, geração, RV por origem e desempates continuam explícitos; não prometer um snapshot atômico entre GVRs/origens independentes.

Aceite F1/F2/F7: itens extremos em chunks posteriores, empates, asc/desc dentro do mesmo namespace, múltiplos kinds, cursor entre páginas e origem parcial. Verificar ausência de duplicatas/gaps em dataset estável e a semântica declarada sob mutações; um reset de snapshot é informado e não mistura páginas de gerações distintas.

## C02 — 200 namespaces e limites de segurança

O teto público atual de namespaces explícitos/fan-out restrito é 100. A regressão do cursor pesado não autoriza remover limites legítimos. Separar os cenários na F0, F2, F5 e F7:

| Cenário | Aceite |
| --- | --- |
| Cluster com 200 namespaces, scope all e LIST global autorizado | página 100 sem erro causado pelo tamanho do cursor; medir over-fetch e memória |
| Cluster com 200 namespaces, sem LIST global autorizado, seleção dentro do teto de 100 | leitura limitada, autorização por origem e coverage honesto |
| Pedido explícito/fan-out acima do teto | rejeição prevista pelo contrato atual, sem contorno de RBAC ou expansão de orçamento; não confundir essa rejeição com cursor pesado |
| Benchmark interno com 200 origens sintéticas | estressar store/merge e medir limites; não apresentar como suporte HTTP a 200 namespaces restritos |

Suporte público a 200 namespaces restritos permanece fora desta adaptação: exige proposta de contrato e budget própria. Testar exaustão legítima de memória e limite de entradas; não exigir que `LIMIT_EXCEEDED` seja impossível em qualquer carga.

## C03 — Identidade de autorização e compartilhamento

F1/D12 preserva a chave existente `(generation, namespace, apiGroup, resource, subresource, verb, resourceName)`, inclusive dimensões vazias quando cabíveis. Reutilizar o serviço atual; TTL curto não elimina revalidação/invalidação em 403, mudança de geração ou revogação detectada. Coalescing não pode unir operações com alvos ou capacidades diferentes.

F2/D17 preserva a identidade de `WatchKey`: `(generation, context, scope, topic, GVR, namespace, selector)`. Resolução efetiva de scope/origens e selectors normalizados devem coincidir antes de compartilhar. O cache de snapshot acompanha essa identidade; caches de páginas/consultas acrescentam filtros, sort, ordem e paginação relevantes. Não compartilhar snapshots apenas por contexto/scope/GVR.

Aceite: selectors e namespaces diferentes não recebem dados uns dos outros; trocar geração encerra trabalho antigo; 403/revogação detectada invalida dados e capacidades afetados; resultado antigo não repovoa o cache após invalidação. `unknown` não vira autorização positiva nem lista vazia. F3 só retém placeholder da mesma seleção autorizada; transição de contexto/scope/namespace/geração remove dados incompatíveis imediatamente.

## C04 — Instrumentação por ciclo de vida

D01/D04 têm duas evidências: F0 instrumenta list, merge, cursor, watch e caches existentes e define o contrato de telemetria do novo resource cache; F2 integra e testa métricas/spans reais desse novo cache. F0 não depende de construir o cache antecipadamente, e D01/D04 só ficam integralmente concluídos após F2. Não emitir atividade simulada nem criar placeholders para fechar o gate.

Baseline F0 e comparativo F7 registram SHA, versões, hardware/recursos do laboratório, dataset, seleção/autorização, latência/erros injetados, página, aquecimento, número de repetições, percentis e unidade. Escolher e documentar essas condições antes da coleta; usar as mesmas no comparativo ou explicitar a diferença. Distinguir benchmark sintético, cluster real e desktop nativo. Medições ficam sanitizadas.

## C05 — Fechamento de otimizações condicionais

Cada item F6 termina em um destes resultados, com evidência individual:

- **Aplicado:** ganho medido, funcionamento integrado e fallback/segurança aprovados; ativação limitada ao cenário validado.
- **Avaliado, não ativado:** comparação executada não demonstra ganho ou demonstra regressão; registrar números e motivo, preservar defaults. A decisão encerra a avaliação, sem alegar implementação/benefício da otimização.
- **Pendente:** comparação não executada, ambiente indisponível ou falha sem resolução. Bloqueia fechamento da F6/F7; não equivale a “não ativado”.

A matriz D30–D34 segue esta regra; F6-06 também exige evidência própria. F7 aceita os dois primeiros resultados, sem exigir ativar uma otimização rejeitada pelas medições. F0–F5 continuam exigindo suas entregas funcionais.

## C06 — Defaults e cobertura da investigação

F4 preserva scopes/preferências existentes durante migração. Contexto sem default conduz à marcação explícita; contexto sem scopes conduz ao cadastro e marcação, sem assumir All nem listar namespaces obrigatoriamente. Exclusão do default exige escolha explícita de substituto ou retorno ao fluxo de configuração, sem seleção implícita de outro universo. Testar atualização de versão, reabertura, troca de contexto e exclusão, preservando a regra de um default por contexto configurado.

F5 reutiliza os diagnósticos existentes; resultados do cache sob demanda informam quais origens estão carregadas e autorizadas. Ausência no cache não prova ausência no cluster, nem zero problemas. Aceite da busca local usa recursos previamente carregados; uma seção ainda não sincronizada permanece incompleta, sem varredura global oculta.
