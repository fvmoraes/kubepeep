# Fase 6 — Preferências e integração final

**Prioridade:** P1, obrigatória. **Entrada:** F2–F5 e catálogo de colunas/referências definido. **Matriz:** R36, U02/U04/U08/U09. Multi-contexto simultâneo está no [backlog](02-backlog-pos-v1.md), não é uma condição variável desta fase.

Persistir estado do shell no backend local e completar integração de navegação/filtros. Busca avançada e ordenação natural já existem; não reimplementar parser nem prometer ordenação global sobre páginas parciais.

## Tarefas

- [ ] **V6-01 — Schema e migração.** Evoluir preferências em `internal/services/resources/preferences.go`, repositório SQLite e DTOs/handlers do endpoint existente. Registrar schema versionado e migração/defaults que preservem language/logs/dashboard/filters/favorites. Novos campos: sidebar compacta, grupos recolhidos, colunas por coleção e recentes; nenhuma chave arbitrária.
- [ ] **V6-02 — Catálogos limitados.** IDs de grupos/coleções/colunas vêm dos catálogos revisados; definir limites de itens/bytes e reset seguro. Persistir somente identificadores de referência validados, nunca objeto/labels/annotations/YAML/log/endpoints/kubeconfig path. Não proibir nomes de namespaces necessários à referência: validar sintaxe/tamanho/origem e a política de sensibilidade existente.
- [ ] **V6-03 — Referências e origem.** Favoritos cluster-scoped não exigem namespace; namespaced exige. Associar referências a profile/contexto local de forma estável, sem credenciais/path. Não vincular favorito antigo sem origem silenciosamente ao contexto aberto: migrar como pendente de associação explícita ou preservar de forma segura até reconfirmação. Itens de outro contexto ficam indisponíveis/ocultos sem serem apagados por navegação.
- [ ] **V6-04 — Recentes.** Fixar limite (20), expiração (30 dias) e limpeza explícita. Registrar só navegação concluída a um alvo elegível, sem histórico automático de Secrets nem gravação por polling. Testar LRU, expiração, exclusão de origem e ausência de dados proibidos.
- [ ] **V6-05 — Hidratação e escrita.** Sidebar, colunas, favoritos e recentes hidratam via `/api/v1/preferences`; estado inicial não sobrescreve preferência antes da leitura. Coordenar atualizações do documento para não perder filtros/favoritos por saves concorrentes; erro de persistência mantém controle utilizável e mostra falha recuperável. Nenhum browser storage é introduzido.
- [ ] **V6-06 — Busca e ordenação existentes.** Integrar `ParseSearch` e `naturalTextCompare` nas novas coleções; documentar tokens, negação, frases e limites reais em `docs/api.md`. Usar filtros permitidos por kind e `meta.page.filterScope`; busca/sort local à página não é anunciada como global. Testar `name-2`/`name-10`, empates, unicode, filtros vazios e cursor após mudança de query.
- [ ] **V6-07 — Navegação completa.** Conferir todos os destinos habilitados: árvore → rota → handler → capability e paleta → detalhe/deep link. Persistir/restaurar filtros permitidos sem transportar scope/cursor de outra geração. Favoritos recém-integrados respeitam contexto/namespace/cluster; resourceEntryPath e catálogo de kinds concordam.
- [ ] **V6-08 — Usabilidade e docs.** Sidebar mantém tooltip/foco ao compactar, grupos recolhidos preservam página ativa e atalhos não interceptam edição/IME. Atualizar data-model/API/security/design-system com apenas comportamento implementado.

## Aceite

| Cenário | Resultado |
| --- | --- |
| Reiniciar aplicativo | sidebar/grupos/colunas e preferências anteriores preservados |
| Banco/schema anterior | migração/defaults previsíveis sem perder filtros/favoritos |
| Documento com coluna/kind/chave desconhecida | rejeição ou descarte conforme contrato, sem gravar conteúdo arbitrário |
| Dois salvamentos de áreas distintas | nenhum filtro/favorito apagado por snapshot antigo |
| Favoritos homônimos em contextos diferentes | origem não se mistura; item antigo sem origem não abre alvo errado |
| Recentes expirados, cheios, limpar e Secret visitado | limites cumpridos; Secret não entra no histórico automático |
| Mudança de scope/query/contexto | cursor inválido descartado; filtro não amplia acesso |
| Browser storage e SQLite inspecionados | só preferências allowlisted; nenhuma credencial/conteúdo remoto |

Unitários de schema/migração/concorrência/validação; testes React de hidratação/erro; E2E de reinício e mudança de contexto. Executar gate integrado do [plano](../README.md).

**Saída:** preferência “local” da referência cumprida e destinos integrados. **Rollback:** leitura compatível e defaults seguros; não apagar banco ou reverter destrutivamente a migração. Recursos novos desativados têm preferência ignorada sem corromper as antigas.
