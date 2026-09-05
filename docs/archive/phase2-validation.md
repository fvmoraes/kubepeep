# Evidência de fechamento da Fase 2

**Data:** 2026-08-03  
**Resultado:** aprovado; Fase 3 autorizada  
**Snapshot de conteúdo auditado:**
`b461e8fc3f5b19a27a43f8a3fd5413c68b694321e48952c35cecc6a93b8df1c5`

## Escopo

Foram revisados em conjunto `product-spec.md`, `architecture.md`,
`security.md`, `data-model.md`, `api.md`, `implementation-plan.md`, ADRs
0001–0004, planos F3–F8 e a matriz dos 27 critérios MVP.

## Auditorias independentes

1. **Contrato:** PASS depois de fechar DTOs, filtros, capabilities,
   classificadores, lifecycle local, concorrência de seleção, cursor, SSE,
   port-forward e exec sem variantes implícitas.
2. **Plano/rastreabilidade:** PASS; F2-01–61 e MVP-01–27 possuem owner,
   fase, tarefa e evidência futura. IDs F1–F8 são contínuos e únicos.
3. **Validação estática:** PASS no snapshot acima:
   - 129 links/âncoras locais válidos;
   - 149 fences fechados;
   - 47 exemplos JSON, 2 JSONL, 2 YAML e 2 arquivos JSON válidos;
   - 120 tabelas Markdown consistentes;
   - zero referência inválida, whitespace final ou falha em `git diff --check`;
   - zero segredo real, e-mail ou path pessoal/temporário concreto.

## Decisões bloqueantes fechadas

- Ginger v1.4.4 + Cobra v1.10.2 no módulo Go 1.25;
- `modernc.org/sqlite v1.54.0` sem CGO;
- lifecycle foreground, lock e canal de controle autenticado;
- `config.yaml`, tree local, `InstanceStateV1`, CLI e doctor versionados;
- API local protegida por loopback/Host/Origin/CSRF e `no-store`;
- DTOs próprios, permissões allowlisted e RBAC como autoridade;
- geração monotônica para seleção/PUT/DELETE de scopes;
- cursor HMAC com TTL fixo, fan-out e merge determinísticos;
- contratos completos de logs, watch/SSE, port-forward e exec WebSocket;
- Secrets metadata-only, persistência e observabilidade com allowlists;
- versões frontend exatas e Kind como harness canônico.

As versões frontend foram conferidas no registry npm antes de serem fixadas em
`implementation-plan.md`; TypeScript 6.0.3 foi mantido dentro da faixa de peer
dependencies aprovada. Nenhuma dependência de produção fora da tabela
justificada foi autorizada.

## Conclusão

Não resta decisão de segurança, lifecycle, persistência ou contrato necessária
para iniciar a fundação. Validações executáveis continuam pertencendo às fases
de implementação correspondentes e não foram marcadas antecipadamente como
evidência de produto concluído.
