# Fase 0 — Operador restrito e cadastro de namespaces em lote

**Prioridade:** P0, premissa básica e gate de entrada. **Entrada:** base atual. **Desbloqueia:** F1 e expansão do catálogo. **Matriz:** U12, U02/U04/U09; complementa R04 sem depender da inspeção do objeto Namespace.

Aplicar a [diretriz do usuário](../reference/direcao-visual-e-premissa-de-acesso.md): o KubePeep deve ser útil sem descoberta global de namespaces nem acesso administrativo. O cadastro por colagem em lote já existe; esta fase preserva e destaca esse caminho, fecha lacunas comprovadas e registra evidência. Não recriar o parser ou o CRUD de scopes.

## Tarefas

- [ ] **V0-01 — Conferir a base.** Revisar parser backend/frontend, `NamespaceScopeEditor`, serviços/handlers de scopes e testes. Registrar formatos, limites, transação, isolamento por profile/contexto e comportamento sem descoberta. Distinguir o que já está implementado dos ajustes necessários.
- [ ] **V0-02 — Entrada em lote visível.** Disponibilizar o cadastro em lote no onboarding/seleção de escopo e na gestão de namespaces mesmo quando a listagem do cluster está negada. Uma colagem de vários nomes e um salvamento devem bastar; entrada individual permanece opcional. Reutilizar texto delimitado, JSON e YAML já aceitos, sem tornar upload de arquivo requisito para usar o fluxo.
- [ ] **V0-03 — Revisão e persistência.** Exibir nomes reconhecidos, duplicados e erros antes de salvar; preservar rascunho e apontar entradas inválidas para correção. Manter trim/deduplicação/limites, sem alterar silenciosamente nomes inválidos ou salvar subconjunto por engano. Persistência atômica do conjunto sintaticamente válido não exige autorização sobre objetos Namespace; salvar escopo não cria recursos Kubernetes.
- [ ] **V0-04 — Acesso por recurso.** Separar descoberta, cadastro local e acesso efetivo. Usar autorização/consultas limitadas nos namespaces informados; não exigir `list/get namespaces` nem contornar negação com listagem global. Capabilities denied/unknown e falhas parciais ficam claras por recurso/origem; conservar nomes salvos durante indisponibilidade ou revogação. Troca de seleção mantém geração/cancelamento existentes.
- [ ] **V0-05 — Evidência e integração.** Cobrir os cenários abaixo com parser/handler/componentes existentes e E2E, acrescentando só a cobertura ausente. Conectar ao harness de RBAC restritivo e documentar a jornada principal. Marcar a fase completa apenas após comprovar o fluxo em lote sem acesso administrativo.
- [ ] **V0-06 — Volume e limites honestos.** Harmonizar feedback do frontend com os limites existentes de importação (1 MiB e 10.000 entradas antes da deduplicação); preview grande deve permanecer responsivo, com renderização limitada/virtualizada quando necessária. Cadastro em massa e consulta têm budgets distintos: hoje as operações de recursos limitam-se a 100 namespaces e fan-out 4. Permitir selecionar/refinar explicitamente um subconjunto consultável de um cadastro maior; nunca cortar os primeiros nomes silenciosamente, disparar fan-out ilimitado ou converter o conjunto em All namespaces.
- [ ] **V0-07 — Timeout com múltiplos namespaces (P0).** Corrigir o timeout relatado pelo usuário ao carregar vários namespaces sob um limite de **60 segundos**. Reproduzir o cenário e identificar a camada que encerra a operação antes de escolher o novo valor: frontend, transporte HTTP/bridge Wails, handler, autorização, fila de concorrência e cliente Kubernetes. Medir cardinalidade, páginas, latência e tempo de espera com fixtures sintéticas; separar deadline por chamada do orçamento total de carregamento. Ajustar a política de tempo ao trabalho permitido, com configuração validada/teto quando necessário, carregamento em lotes ou progressivo e preservação de resultados parciais. Não remover todos os limites nem aumentar apenas um timeout deixando outra camada cancelar antes. Um namespace lento não deve invalidar todo o conjunto; manter cancelamento ao sair/trocar contexto, concorrência limitada e retry seletivo sem tempestade de requests.

### Investigação e saída de V0-07

O limite de 60 s e o sintoma são um relato de uso; a causa exata ainda precisa ser reproduzida. Registrar o encadeamento dos deadlines e a política escolhida no contrato da API/configuração, incluindo defaults, teto, tempo de fila e limites de cada chamada. Comparar antes/depois com 1, 10, 50 e 100 namespaces, permissões mistas e latências controladas. A solução deve permitir concluir ou continuar explicitamente um carregamento válido que ultrapasse 60 s no total, mostrando progresso/cobertura e preservando o que já chegou. Operações sem progresso ou dependências travadas continuam terminando dentro de limites definidos.

Pontos iniciais confirmados na base: `internal/config/config.go` limita o timeout de bloco do dashboard a 60 s (padrão 8 s); `internal/services/dashboard/budgets.go` também normaliza esse teto por bloco/fan-out. `internal/adapters/kubernetes/client_factory.go` define 15 s por chamada unary. Esses valores não provam qual deadline disparou no caso relatado: capturar a configuração efetiva. Conferir também propagação de cancelamento entre `web/src/api/client.ts`, `web/src/api/desktop.ts` e `internal/desktop/bridge.go`, pois os modos web e desktop usam transportes distintos.

## Cenários obrigatórios de aceite

| Cenário | Resultado exigido |
| --- | --- |
| Operador sem `list/get namespaces`, sem criação de Namespace e sem `cluster-admin`; recursos permitidos em namespaces conhecidos | cadastra e seleciona escopo em lote; Pods/workloads/logs permitidos funcionam conforme seus verbos, sem depender da descoberta |
| Colar 100 nomes válidos, incluindo separadores mistos e repetições | preview e contagens corretos; conjunto único salvo em uma operação; nenhuma sequência de 100 cadastros |
| Nome inválido, JSON/YAML malformado ou limite excedido | erro acionável, rascunho preservado e nenhuma persistência parcial silenciosa |
| Cadastro válido com mais de 100 nomes, dentro do limite de importação | conjunto é preservado; UI explica o limite de consulta e permite escolher subconjunto, sem travar o preview nem descartar nomes |
| Um namespace permite Pods e outro nega; Events/Logs têm permissões diferentes | mantém os resultados autorizados e mostra cobertura/estado por recurso; não deduz acesso de outra capability |
| Autorização unknown, timeout ou namespace inacessível | não afirma inexistência; não apaga cadastro nem amplia escopo |
| Reabrir aplicativo ou alternar contexto | recupera os scopes locais da origem correta; não mistura nomes/consultas da seleção anterior |
| Nodes/PVs ou descoberta global negados | shell e fluxo namespaced continuam úteis; All namespaces não é selecionado como fallback |
| Carregamento de vários namespaces leva mais de 60 s no total, mas as chamadas individuais progridem dentro dos limites definidos | não perde todo o carregamento por um corte fixo de 60 s; conclui ou oferece continuação explícita, mantendo resultados e cobertura visíveis |
| Um namespace lento/travado entre vários saudáveis | resultados saudáveis preservados; timeout identificado na origem afetada e retry limitado; UI permanece responsiva |
| Cancelar/sair/trocar contexto durante carregamento longo | chamadas e tarefas em fila são canceladas; respostas antigas não aparecem e não há trabalho órfão |

Usar fixtures sintéticas. A prova deve distinguir chamadas à API local das operações Kubernetes e verificar que nenhuma listagem/get de Namespace nem criação de objeto é necessária ao cadastro manual. Verificação opcional de descoberta não pode bloquear esse caminho.

**Saída:** U12 comprovado na base e protegido pelas fases seguintes. **Rollback:** preservar scopes/dados anteriores e o cadastro em lote funcional; nunca voltar a exigir um cadastro por nome ou acesso administrativo para entrar no produto.
