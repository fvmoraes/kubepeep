# Revisão Funcional do KubePeep

## 1. Objetivo

Mapear e validar todo o fluxo de uso do KubePeep, comparar o comportamento real com a documentação e identificar problemas, riscos e melhorias.

## 2. Metodologia

- Leitura dos documentos normativos (`product-spec.md`, `architecture.md`, `api.md`, `data-model.md`, `security.md`).
- Inspeção do código de handlers, serviços e adapters.
- Execução local com kubeconfig real e com cluster Kind.
- Navegação pelas 10 rotas principais com screenshots.
- Análise dos testes existentes.

## 3. Fluxos mapeados

### 3.1 Inicialização da aplicação

**Como funciona:**
- `cmd/kubePeep/main.go` → `cli.Execute()` → `runtime.RunForeground()` → `application.Compose()` → `app.New()`.
- Resolve data root, kubeconfig, config.yaml, SQLite, migrations, listener loopback, health, estado da instância e browser.

**Problemas:**
- Sem P0/P1. A inicialização é robusta e testada.
- **P2:** mensagens de erro em primeira execução poderiam ser mais guiadas (ex.: "Nenhum contexto selecionado" vs. "Kubeconfig não encontrado").

**Riscos:**
- Falha de plugin `exec` é tratada como dependência externa, mas a mensagem pode não ser compreensível para usuários não técnicos.

**Melhoria recomendada:**
- Adicionar tela de onboarding para primeiro uso quando não houver contexto selecionado.

**Prioridade:** P2. **Complexidade:** M.

**Critério de aceite:**
- Usuário sem contexto vê uma tela clara com ação de seleção, não uma lista vazia.
- Nenhuma informação sensível do kubeconfig é exibida.

---

### 3.2 Seleção de contexto e escopo

**Como funciona:**
- Seletor em cascata: profile → contexto.
- Escopos `single`, `list` e `all` persistidos em SQLite.
- `--namespace` cria scope `single` efêmero.
- Troca de contexto/escopo cria nova geração e cancela trabalho anterior.

**Problemas:**
- **P2:** o parser de importação em massa é funcional, mas a UI não oferece preview visual rico de chips coloridos por status (válido/duplicado/inválido).
- **P2:** escopo `all` requer `list namespaces`; quando negado, o fallback para lista manual é básico.

**Riscos:**
- Usuário pode não entender por que `all` está indisponível sem abrir Permissions.

**Melhoria recomendada:**
- Melhorar o feedback de validação de escopos com preview interativo.
- Adicionar explicação inline quando `all` for negado.

**Prioridade:** P2. **Complexidade:** M.

**Critério de aceite:**
- Importação em massa mostra chips com cores/ícones distintos por categoria.
- Mensagem de `all` negado explica a permissão necessária (`list namespaces`).

---

### 3.3 Dashboard

**Como funciona:**
- Blocos independentes: summary, problems, restarts, workloads degradados, events warning, log scan, metrics.
- Cada bloco carrega em paralelo e isola falhas parciais.
- Contadores distinguem `available`, `denied`, `unavailable`, `notCollected`, etc.

**Problemas:**
- **P1:** os cards não navegam automaticamente para a lista filtrada correspondente (ex.: clique em "Pods problemáticos" não abre `/pods?problematic=true`).
- **P2:** não há visualização de saúde por namespace no overview.
- **P2:** log scan requer ação explícita; não há indicação de que o bloco está desatualizado.

**Riscos:**
- Dashboard perde parte do valor como ponto de partida para investigação quando não oferece navegação contextual.

**Melhoria recomendada:**
- Tornar cada card clicável, preservando filtros na navegação.
- Adicionar resumo de saúde por namespace quando o escopo contiver múltiplos namespaces.
- Indicar idade/stale de cada bloco individualmente.

**Prioridade:** P1. **Complexidade:** M.

**Critério de aceite:**
- Clique em card de problemas abre `/pods?problematic=true` com os mesmos filtros.
- Blocos exibem `collectedAt` e indicador de stale.

---

### 3.4 Listas de recursos (Workloads, Pods, Events, Network, Config)

**Como funciona:**
- Listas paginadas com cursor opaco ligado à geração.
- Filtros draft/applied, ordenação enviada ao backend, busca textual limitada.
- Detalhes e YAML sob demanda.

**Problemas:**
- **P1:** busca é substring simples; não suporta termos compostos, exclusão ou alta cardinalidade (`UX-M03`).
- **P1:** não há painel lateral persistente para detalhes; detalhe ocupa tela inteira e perde contexto da lista.
- **P2:** ordenação é de página limitada, mas a UI poderia comunicar isso mais explicitamente.
- **P2:** Secret não oferece YAML (correto por segurança), mas a mensagem poderia explicar melhor.

**Riscos:**
- Busca limitada dificulta encontrar recursos em clusters grandes.
- Perda de contexto na navegação detalhe → lista aumenta a carga cognitiva.

**Melhoria recomendada:**
- Implementar parser composto de `search` (positivo/negativo/multitermo).
- Adicionar painel lateral de detalhes que mantém a lista visível.
- Aprimorar mensagem de ausência de YAML para Secret.

**Prioridade:** P1. **Complexidade:** L.

**Critério de aceite:**
- Busca `payment !failed` retorna recursos com "payment" e sem "failed".
- Detalhe abre em painel lateral nas telas de lista.
- Ordenação de página é rotulada como "page sort".

---

### 3.5 Logs

**Como funciona:**
- Leitura atual e anterior via HTTP, follow via SSE.
- Seleção de pod/container, tailLines, busca local, wrap, timestamps, copy, download.
- Redaction e detecção de padrões de erro.

**Problemas:**
- **P1:** não há syntax highlighting para logs estruturados (JSON).
- **P1:** não há pausa/continuação explícita no follow; apenas desligar o follow.
- **P2:** não há filtro por nível de log (`error`, `warn`, `info`).
- **P2:** reconexão automática não é visível quando o SSE cai.

**Riscos:**
- Logs densos são difíceis de escanear sem highlight e filtros.

**Melhoria recomendada:**
- Adicionar toggle de pausa no follow.
- Destacar JSON e níveis de log (`error`, `warn`).
- Exibir indicador de reconexão/backoff.

**Prioridade:** P1. **Complexidade:** M.

**Critério de aceite:**
- Botão de pausa/continuar no follow.
- Linhas JSON têm keys/values destacados.
- Filtro por nível reduz as linhas exibidas localmente.

---

### 3.6 Ações (restart, scale, delete, port-forward, exec)

**Como funciona:**
- UI consulta capabilities; backend revalida SAR antes da operação.
- Confirmação contextual, idempotência, preconditions.
- Port-forward e exec com sessões limitadas e cleanup.

**Problemas:**
- **P2:** não há gerenciador centralizado de port-forwards ativos.
- **P2:** terminal exec é rudimentar (`<pre>`), sem resize automático ou scrollback avançado.
- **P2:** não há ações em massa.

**Riscos:**
- Usuário pode perder sessões de port-forward sem saber onde encerrá-las.

**Melhoria recomendada:**
- Criar página/gerenciador de port-forwards ativos.
- Avaliar `xterm.js` para terminal exec.
- Manter ações individuais no MVP; ações em massa como evolução futura.

**Prioridade:** P2. **Complexidade:** L.

**Critério de aceite:**
- Gerenciador lista port-forwards ativos com namespace/pod/porta local/remota e botão de parar.
- Terminal exec suporta resize e scrollback configurável.

---

### 3.7 Estados de erro e permissão

**Como funciona:**
- Estados padronizados: loading, empty, offline, forbidden, unknown, partial, canceled, truncated, stale.
- `FORBIDDEN` reservado para negação autoritativa; `unknown` para autorização indisponível.

**Problemas:**
- **P2:** a representação visual dos estados varia entre telas (ex.: Dashboard usa `ResultBody`, ResourcePages usa `QueryState`, LogsPage usa renderização inline).
- **P2:** mensagens de `unknown` podem ser confundidas com erro técnico.

**Melhoria recomendada:**
- Padronizar componente `QueryBoundary` para todos os estados.
- Melhorar copy de `unknown` para "Não foi possível confirmar a permissão; tente novamente."

**Prioridade:** P2. **Complexidade:** S.

---

### 3.8 Persistência de preferências

**Como funciona:**
- Preferências versionadas em SQLite: idioma, logs, dashboard, filtros salvos.
- PUT exige snapshot completo; rejeita campos desconhecidos e sensíveis.

**Problemas:**
- **P2:** filtros salvos não podem ser renomeados/editados na UI.
- **P3:** não há preferência de tema (escuro é único).

**Melhoria recomendada:**
- Permitir renomear e excluir filtros salvos na tela de Settings.

**Prioridade:** P2. **Complexidade:** S.

---

## 4. Funcionalidades por área

| Área | Implementado | Parcial | Ausente |
| --- | --- | --- | --- |
| Inicialização/local lifecycle | x | | |
| Seleção de contexto/escopo | x | | |
| Dashboard | x | navegação contextual | |
| Workloads/Pods/Events/Network/Config | x | busca composta, painel lateral | |
| Logs | x | pause/follow, highlight | filtro por nível |
| Ações | x | gerenciador de port-forward | terminal avançado |
| Settings | x | edição de filtros | |
| Fase 9 (paleta, atalhos) | x | | |
| Favoritos/recentes | | | x |
| Diff de YAML | | | x |
| Multi-contexto | | | x |
| Busca global | | | x |

## 5. Recomendações prioritárias

1. **P1:** tornar cards do dashboard navegáveis.
2. **P1:** implementar parser composto de busca.
3. **P1:** adicionar painel lateral de detalhes.
4. **P1:** melhorar logs com pause/highlight.
5. **P2:** gerenciador de port-forwards.
6. **P2:** preview visual de importação de escopos.
7. **P2:** padronizar `QueryBoundary`.
8. **P3:** favoritos/recentes e diff.
