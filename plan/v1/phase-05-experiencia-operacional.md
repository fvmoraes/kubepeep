# Fase 5 — Experiência operacional

> **Objetivo:** fechar o backlog da Fase 9 (experiência operacional) que sobrou após o redesign, priorizando o que aparece todos os dias no uso: port-forward, ações contextuais, colunas e refinamentos de YAML/logs.
> **Prioridade:** P1. **Dependências:** nenhuma (paralelizável com Fases 2–4). **Complexidade agregada:** L.

## Tarefas

### Port-forward dedicado

- [ ] **V5-01** Página `/network/port-forwards` promovida a painel completo (hoje é tab simples): sessões ativas com contexto, namespace/alvo, portas, estado, criada/expira (F9-60); iniciar a partir do detalhe de Pod/Service com sugestão de porta (F9-61); bind loopback obrigatório já garantido pelo backend.
- [ ] **V5-02** Porta ocupada → mensagem acionável sem fallback externo (F9-62); parar uma sessão / todas com confirmação (F9-63); encerrar no shutdown e marcar sessões mortas por troca de geração (F9-64); teste de colisão/cleanup (F9-65).

### Ações contextuais

- [ ] **V5-03** Menu de ações rápidas por linha/detalhe com o mesmo catálogo tipado das ações (F9-34): Pod (logs, port-forward, exec, delete), Deployment (restart, scale, YAML), Service (port-forward, YAML) — itens só quando pertinentes ao kind (F9-35).
- [ ] **V5-04** Capability `denied`/`unknown` com tooltip explicativo e ação desabilitada — sem silêncio e sem tentativa às cegas (F9-36, §17); confirmação proporcional em destrutivas (F9-38); reconciliação pós-403/conflito (F9-39).

### Colunas e listas

- [ ] **V5-05** Visibilidade/ordem de colunas por kind a partir de catálogo seguro (F9-24); proibido persistir colunas derivadas de `Secret.data`/annotations arbitrárias (F9-25); preferências via allowlist (Fase 6 fornece o transporte).
- [ ] **V5-06** Ordenação natural estável (Números em `name-2` < `name-10`) com tie-breaker determinístico (F9-21) — decidir client-side para página já carregada, sem reordenar o resultado do servidor.

### YAML e logs

- [ ] **V5-07** YAML viewer: busca local, seções recolhíveis, truncamento honesto em objetos grandes (F9-41/44); download/cópia só por gesto explícito, `no-store` mantido (F9-45); diff entre dois objetos acessíveis com origens lado a lado (F9-46/47) — Secret em qualquer lado → recusa (F9-48).
- [ ] **V5-08** Logs: proveniência por linha ao combinar fontes (F9-52), budgets já existentes; métricas CPU/memória em detalhes de Pod quando a Metrics API autorizar, indisponibilidade como estado local do bloco (F9-57/58).

### Favoritos e recentes

- [ ] **V5-09** Favoritos refinados: remoção silenciosa quando o recurso sai do profile/contexto (F9-28); recentes com limite/expiração/limpar (F9-27) — transporte via preferências allowlisted (Fase 6).

## Critérios de aceite

- Painel de port-forward cobre F9-60..65 com testes de colisão e cleanup (sem goroutine órfã — goleak no service).
- Menu contextual não oferece ação inaplicável ao kind; toda ação continua revalidada no backend.
- Colunas persistidas passam no gate de catálogo seguro (teste negativo com Secret/annotation).
- `make test test-e2e` verdes; e2e novo para port-forward (mock) e menu contextual.

## Testes e rollback

- Frontend: Vitest por componente + Playwright; backend: testes de sessão/port-forward existentes estendidos.
- Rollback: painel e menu são aditivos; colunas/viibilidade dependem da allowlist de preferências (revert = voltar ao padrão fixo).
