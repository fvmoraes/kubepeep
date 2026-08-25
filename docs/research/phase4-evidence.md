# Evidências da Fase 4 — Kubernetes e RBAC

**Data da validação local:** 2026-08-24
**Plataforma principal:** Linux amd64
**Estado:** implementação local concluída; 57 de 59 tarefas comprovadas; validação dinâmica no Kind pendente

## Resultado

A conexão Kubernetes, os contextos, os escopos de namespace e a matriz RBAC
estão implementados em fatias completas de backend e frontend. A suíte local
com Go 1.25.13 passou, incluindo race detector, `go vet`, build e
`govulncheck` sem vulnerabilidade alcançável. O frontend também passou por
lint, typecheck, build, 63 testes Vitest e três cenários Playwright.

Não há run atual de CI nem execução dinâmica do harness Kind a registrar. O
harness canônico e a validação offline com o binário real passaram, mas Docker
está indisponível nesta estação; por isso F4-49 e F4-50 permanecem abertas.

## Rastreabilidade da implementação

| Área | Implementação | Evidência automatizada |
| --- | --- | --- |
| Kubeconfig, precedência e contexto | `internal/adapters/kubernetes/loader.go` | `loader_test.go`, incluindo fonte inválida sem fallback, múltiplos arquivos e precedência de contexto |
| Fingerprints e cache concorrente | `fingerprint.go`, `client_cache.go`, `generation.go` | `fingerprint_test.go` e `client_cache_test.go`, cobrindo ordem, symlink, replace atômico, certificados/tokens referenciados, invalidação e corridas |
| Clients unary/stream e autenticação | `client_factory.go`, `connectivity.go` | `client_factory_test.go`, incluindo timeout separado, cancelamento de geração, plugin `exec`, cluster offline e ausência de impersonation |
| Contextos e profiles | `internal/services/contexts`, `internal/api/handlers/contexts.go`, `cluster_profiles.go` | testes de serviço e handlers para validação pré-commit, seleção degradada pós-commit, geração monotônica e DTO sanitizado |
| Escopos `single`, `list` e `all` | `internal/services/namespaces`, `internal/adapters/sqlite/namespace_scopes.go`, `namespace_scopes.go` | parser estrito texto/JSON/YAML, quatro contadores, transações, optimistic concurrency, seleção e semântica `all` sem `*` |
| RBAC tri-state | `internal/services/authorization` | SAR exato, SSRR apenas como resumo, TTL/deduplicação, refresh, `allowed`/`denied`/`unknown`, fail-closed e autoridade da operação real |
| API e UI | handlers de profiles/contextos/scopes/permissions; componentes `ContextSelector`, `NamespaceScopeEditor` e `PermissionsMatrix` | testes Go dos envelopes/gramática e testes React de loading, cancelamento, imports, escopos e decisões textuais |
| Bootstrap e diagnóstico | `internal/cli`, `internal/app`, runtime Kubernetes | testes de flags, namespace efêmero, health/status, plugin ausente e capabilities somente leitura no doctor |

## Provas de segurança

- a seleção persiste somente referências de profile/contexto e escopo; os
  testes de adapter rejeitam serialização de `rest.Config` e impersonation;
- falhas de parsing, autenticação e plugins são convertidas para mensagens
  públicas estáveis antes de resposta ou log;
- mutações de seleção usam JSON estrito, limite de body, CSRF e geração;
- `all` é persistido como modo com lista vazia e usa exatamente o catálogo de
  namespaces autorizado;
- o cache RBAC inclui geração, namespace, API group, recurso, subresource,
  verbo e `resourceName`, com refresh que não converte indisponibilidade em
  negação;
- `k8s.io/api`, `client-go`, `apimachinery` e `metrics` estão fixados em
  v0.35.7 e a versão é verificada por teste.

## Gates locais executados

| Gate | Resultado |
| --- | --- |
| `GOTOOLCHAIN=go1.25.13 go test ./...` | passou em todos os pacotes |
| `GOTOOLCHAIN=go1.25.13 go test -race ./internal/...` | passou em todos os pacotes aplicáveis |
| `GOTOOLCHAIN=go1.25.13 go vet ./...` e build | passaram |
| `govulncheck` v1.7.0 | zero vulnerabilidades alcançáveis |
| frontend lint/typecheck/build | passou |
| Vitest / Playwright | 63 / 3 testes passaram |
| Ginger v1.4.4 `inspect` / `doctor` | passou; diagnósticos heurísticos conhecidos continuam documentados |
| `./test/kind/harness.sh static` e driver `offline` | passaram; o segundo usou o binário real |

## Pendências exatas

- **F4-49:** executar a matriz real de contexto, escopos, permissões e mudança
  de RBAC: `./test/kind/harness.sh validate`;
- **F4-50:** completar a prova contra a API real, inclusive revogação entre
  capability e operação. Ela faz parte do mesmo `validate` e do black-box
  `./test/kind/harness.sh app-e2e ./kubePeep`.

A indisponibilidade do Docker e os checks offline executados estão registrados
em `test/kind/VALIDATION.md`. Nenhum URL de run foi inventado.
