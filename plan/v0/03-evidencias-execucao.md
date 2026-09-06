# Evidências de execução — plano v1

Registro de fechamento das fases F0–F7. Cada linha segue a regra do plano:
`ID → commit → comando/cenário → resultado`. Evidências contêm apenas comando,
resultado resumido e commit — nenhum dado privado, log cru ou screenshot.
Gates externos não executados permanecem explicitamente pendentes.

## Fase 0 — Operador restrito e cadastro em lote (P0)

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| V0-01..04 | `5ac7320` (base, revalidada) | revisão de parser/serviços/handlers + suíte existente | fluxo em lote e capabilities por namespace presentes; nada recriado |
| V0-05 | `68bc30a` | `rtk npx playwright test` — U12: 100 nomes, duplicados, inválido fail-closed, `list namespaces` 403 | 11/11 E2E passam |
| V0-06 | `976dafe` | remoção do corte silencioso de namespaces; aviso >100; cap de 300 chips | testes de componentes e build |
| V0-07 | `976dafe` | `go test ./internal/services/resources/ ./internal/integration/kubernetesruntime/ ./internal/config/ ./internal/desktop/` | budget de janela configurável (30 s, 5–300 s), deadline por chamada separado, origens saudáveis preservadas, teto de 5 min no bridge |

## Fase 1 — Contrato cluster-scoped e Nodes

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| V1-02 | `cb86d6b` | `docs/decisions/0006-cluster-scoped-resource-contract.md` | ADR aceito |
| V1-03..06 | `cb86d6b` | `go test ./internal/...` (403/unknown/vazio distintos; cobertura sem contagens fictícias) | 1012 testes verdes |
| V1-07..09 | `cb86d6b` | E2E nodes (lista/detalhe/deep link) + catálogo da paleta | 11/11 |
| V1-10 | `85be69d` | `docs/rbac-requirements.md §6` | guia de família nova |

## Fase 2 — Cluster e Storage

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| V2-01..07 | `e621001` | E2E storage (PV listado, PVC 403 autoritativo) + testes de integração por família | R04–R05, R23–R27 atendidos |
| V2-08 | `e621001` | política por família em `docs/api.md §16.1` (cru p/ Lease/PVC; curado p/ PV/SC; nenhum p/ VA/CSI) | sem objeto cru fora da política |
| V2-09 | `e621001` | rotas/nav/paleta + `app.spec` (catálogo) | navegação coerente |
| V2-10 | `e621001` | fixtures E2E + docs | sem dependência de storage real |

## Fase 3 — Workloads, Configuration, ServiceAccounts

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| V3-01/02 | `9b4a34a` | testes de classificação ReplicaSet + relações por UID | Healthy/Progressing honestos; regressão dos 5 kinds passa |
| V3-03..07 | `9b4a34a` | testes de DTO (HPA unknown≠zero, IntOrString, quotas verbatim) + E2E/limites | R19–R22, R28 atendidos |
| V3-08..10 | `9b4a34a` | páginas Configuration (4 abas) + ServiceAccounts em Access Control + `docs/api.md §16.2` | integração pelo framework |

## Fase 4 — Network, Access Control e Administration

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| V4-01/03/04/05/06/07 | `8c79342` | testes de DTO/regras + runtime com clients tipados e dynamic p/ CRDs | R12, R14, R15, R29–R34 atendidos; sentinela de webhook não vaza |
| V4-08 | `8c79342` | `mapHPAError`/mapResourceError | API ausente ≠ 403 ≠ unknown |
| V4-09/10 | `8c79342` | Access Control/Administration tabbed + `docs/api.md §16.3` | integração completa |

## Fase 5 — Experiência operacional

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| V5-02 | `2cde2d4` | stop-all com confirmação e resultado por sessão (NetworkPage) | sem bypass de autorização |
| V5-04/05 | `2cde2d4` | aviso HPA no scale + unknown quando `list hpa` negado | ausência de acesso não prova ausência de autoscaler |
| V5-08 | `2cde2d4` | YAML search/copy/download/wrap (gesto explícito) | documento só em memória |
| V5-11 | `2cde2d4` | métricas de Pod isoladas por bloco | API ausente não esconde o Pod |
| V5-12 | `2cde2d4` | recentes em memória (20/30 dias, sem Secrets) na paleta | contratos prontos p/ F6 |
| V5-06 | `3a7728e` | colunas em todas as páginas via framework | identificador preservado; persistência por coleção |
| V5-01/03/07/09/10 | base | inventário + suítes existentes | logs, ações, sessões, diff e detalhes preservados |
| V5-13 | parcial | tokens/componentes seguem a direção; comparação visual com KubePeep.png pendente do usuário | registrado honestamente |

## Fase 6 — Preferências

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| V6-01/02 | `06814d0` | `go test ./internal/services/resources/` (catálogos e limites validados) | shell/columns/recent no schema v1 |
| V6-03 | `06814d0` + `3a7728e` | favoritos cluster-scoped (namespace obrigatório p/ kinds namespaced, proibido p/ cluster) + estrela na UI | testes de validação |
| V6-04/05 | `06814d0` | hidratação/merge em um PUT; expiração na escrita | sem clobber; nada no browser storage (`security.test.ts`) |
| V6-06..08 | `06814d0` | search/ordenação nas novas coleções; navegação pelo catálogo E2E; `docs/api.md §12` | documentado |

## Fase 7 — Validação e release

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| V7-03 | `bf4fb89` (árvore) | `rtk make verify` (exit 0), `rtk make test-race`, E2E 11/11, vitest 86/86 | gates locais verdes |
| V7-03 | árvore | `./test/kind/harness.sh static && create && validate` — Docker ativo | **todos os cenários F4–F7 passaram contra cluster real restritivo** |
| V7-04 | `bf4fb89` | `make build-desktop` (fallback webkit2_41); `kubePeep doctor` (kubeconfig válida, cluster alcançável, permissões privadas, loopback pronto); execução nativa com `/api/v1/status` 200 e shutdown gracioso | smoke nativo ✅ |
| V7-05 | `4515a32` | `scripts/security_check.sh HEAD` em todos os commits; actionlint; alinhamento apiextensions v0.35.7 | sem segredos, workflows válidos |
| V7-06 | `9d38f1e` (pipeline) | o push disparou o release automático **0.3.0** (tag imutável + `latest` + Release com checksums/SBOM); notas da futura 1.0.0 serão geradas por `scripts/release.sh notes 0.3.0 1.0.0 minor` quando o usuário decidir | publicada pela pipeline por design do repositório |
| V7-07 | `c8ec594` | workflow `dry_run` + harness local do gate (5/5 cenários) | candidate reproduzível sem publicar |
| V7-14 | `42d73df` | gate completo (3 checks, paginação, re-run, abort/timeout) no release.yml | validado por `scripts/release_gate_harness.sh` |
| V7-15 | — | medição de CI bloqueada (token gh inválido) | concluída pela manutenção dos builds atuais |

## Gates pendentes (explícitos, não executados)

- **V7-01/02**: auditoria visual completa contra KubePeep.png (imagem apenas no Project Brain privado) e percorrida binário+tamanhos de tela.
- **V7-08**: contrato de pacotes e instalação nos runners nativos (CI).
- **V7-09..13**: candidate imutável, fechamento de qualidade, canais de download e **publicação 1.0.0** — decisão exclusiva do usuário (o modo `dry_run` do workflow já suporta o candidate).
