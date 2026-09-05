# Fase 5 — Experiência operacional

**Prioridade:** P1, obrigatória. **Entrada:** contrato F1; pode avançar junto de F2–F4, integrando os kinds ao concluí-los. **Matriz:** R03/R07–R08/R16–R18/R35 e U04–U09.

Fechar as jornadas de diagnóstico e operação reaproveitando logs, ações, sessões, favoritos e diff last-applied existentes. O catálogo de colunas/referências funciona em memória nesta fase; sua persistência é responsabilidade da F6.

## Tarefas

- [ ] **V5-01 — Inventário por jornada.** Relacionar componentes/serviços/testes existentes aos itens abaixo. Marcar como integração o que já existe, registrando lacunas concretas. Nenhuma tarefa autoriza substituir uma implementação funcional inteira.
- [ ] **V5-02 — Painel de port-forward.** Organizar o destino `/network/port-forwards`: alvo/origem, portas, status, criada/expira/encerrada e motivo sanitizado. Iniciar a partir de Pod/Service autorizado, sugerir porta válida e permitir parar uma sessão ou todas após confirmação. “Parar todas” opera sobre sessões pertencentes ao usuário/geração, com resultado por sessão e sem novo bypass de autorização.
- [ ] **V5-03 — Ciclo das sessões.** Reusar bind loopback e encerramento existentes. Cobrir porta ocupada, limite, alvo desaparecido, cliente desconectado, troca de contexto/scope e shutdown. Não matar processo alheio nem expor porta externa como fallback; UI deve refletir encerramento e falha, sem sucesso antecipado.
- [ ] **V5-04 — Catálogo contextual.** Menu por linha/detalhe via componentes compartilhados: Pod (detalhes/logs/YAML/exec/port-forward/delete); Deployment (YAML/restart/scale); StatefulSet (YAML/scale); Service (YAML/port-forward). Demais kinds recebem somente ações implementadas e pertinentes. Paleta continua apenas navegação.
- [ ] **V5-05 — Autorização e confirmação.** Allowed/denied/unknown visíveis com explicação acessível. Backend reautoriza alvo e preconditions; confirmação de mutação contém contexto/namespace/kind/nome/consequência. Reconciliar sucesso, 403, conflito, cancelamento e geração alterada. HPA conhecido no alvo de scale gera aviso adequado; não possuir acesso ao HPA não autoriza afirmar ausência de autoscaler.
- [ ] **V5-06 — Colunas.** Definir catálogo de IDs seguros por kind e controle acessível de visibilidade/ordem/reset. Preservar coluna identificadora e ações essenciais; nenhum path arbitrário de objeto/annotation/label/Secret vira coluna configurável. Aplicar no framework, não em cópias por página.
- [ ] **V5-07 — Detalhes e relações.** Consolidar Overview/Details/YAML/Events e tabs específicas úteis (Containers/Logs/Metrics/Conditions/Ports). Relações workload/ReplicaSet/Pod/Service/evento usam origem e identidade validadas. Eventos dependem de autorização própria; ausência de permissão num bloco não apaga o restante do detalhe.
- [ ] **V5-08 — YAML.** Estender viewer com busca local, próximo/anterior, recolhimento e cópia/download por gesto. Respeitar budgets e truncamento, abortar leitura/diff ao trocar alvo/geração e descartar respostas tardias. Manter conteúdo só em memória; Secret não possui rota/visualização/exportação YAML.
- [ ] **V5-09 — Diff.** Preservar vivo × last-applied já implementado. Comparação adicional da v1 limita-se a dois objetos autorizados do contexto ativo: origens visíveis, ausência versus vazio, normalização de campos gerenciados opt-in, limites e recusa se qualquer lado for Secret/inacessível. Contrato novo é documentado/testado antes da UI; comparação multi-contexto permanece no backlog.
- [ ] **V5-10 — Logs.** Revisar atuais/anteriores/follow/stop, namespace/pod/container, tail/since, timestamps/wrap, busca/filtro de nível, limpar e exportar explicitamente. Preservar buffer limitado/backpressure e cancelamento ao sair/trocar origem. Cada origem é identificada; não adicionar agregação de múltiplos streams nesta fase. Ausência de timestamps/nível não deve produzir metadados inventados.
- [ ] **V5-11 — Métricas e estados.** Integrar CPU/memória/requests/limits em detalhe Pod quando autorizados; API ausente/negada/parcial afeta somente o bloco. Mensagens de loading/vazio/offline/proibido/unknown/parcial/stale/truncado usam componentes existentes e texto legível.
- [ ] **V5-12 — Favoritos/recentes em memória.** Definir referências mínimas com origem e kind/escopo, limite e expiração para recentes, ação limpar e comportamento de item indisponível. Não manter histórico automático de Secrets. Navegação não revela favorito de outro contexto nem remove dados duráveis por causa de um timeout; persistência/migração ficam na F6.
- [ ] **V5-13 — Estilo visual aprovado.** Alinhar shell, Overview e páginas ao estilo da imagem KubePeep.png, descrito na [direção visual](../reference/direcao-visual-e-premissa-de-acesso.md), usando o Design System existente. Preservar superfície escura, densidade, hierarquia, sidebar/topbar, cards e tabelas; gráficos e tendências exigem dados e cobertura reais. O cadastro em lote deve ser fácil de encontrar no seletor e na gestão de scopes, inclusive no estado sem descoberta; não reproduzir All namespaces como pressuposto de acesso.

## Aceite e validação

- E2E exercita menu autorizado/negado/unknown, confirmação e reconciliação de erro; teste backend prova reautorização mesmo com UI antiga.
- Port-forward cobre criação/colisão/parada individual/todas/troca de seleção/shutdown, com teste de encerramento de conexões/goroutines. A rota nova não duplica o serviço de sessões.
- YAML/diff cancelado não renderiza o objeto anterior; Secret em qualquer entrada é recusado antes de buscar conteúdo. Sentinelas sintéticas não chegam à persistência/diagnóstico.
- Logs com cliente lento/buffer cheio/previous container/follow cancelado continuam limitados; exportação só acontece por gesto. Métricas indisponíveis não escondem o Pod.
- Colunas funcionam com teclado e reset; páginas úteis em 1280×720 e 1920×1080, sem overflow global. Validar alto volume com fixtures sintéticas, sem tornar logs/screenshots artefatos Git.
- Rodar testes pertinentes e gate integrado do [plano](../README.md); registrar limitações de plataforma honestamente.

**Saída:** jornadas operacionais completas no contexto ativo e contratos de UI prontos para F6. **Rollback:** por jornada; catálogos têm defaults seguros e não exigem apagar preferências já salvas.
