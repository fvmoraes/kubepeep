# Plano de implementação e validação

> **Status:** Fases 1 e 2 concluídas; Fase 3 em execução
>
> **Escopo desta entrega:** sequência de produção autorizada pelos contratos
> fechados e evidências dos gates F1/F2.
>
> **Gate atual:** a auditoria documental da Fase 2 foi aprovada em 2026-08-03 e
> o scaffold definitivo da Fase 3 está autorizado. O probe F1 valida o desenho;
> F3-B deve reimplementar e repetir a integração no módulo de produção, e F8
> deve testar os archives/instaladores reais.

## 1. Estratégia

O trabalho segue as oito fases, em ordem. Dentro de cada fase funcional, cada fatia inclui:

1. contrato/DTO;
2. port;
3. serviço de aplicação;
4. adapter;
5. handler/rota;
6. interface;
7. testes permitidos, negados e cancelados;
8. documentação/evidência.

Não se cria uma camada completa “horizontal” sem uma jornada consumidora. Dashboard e telas de recursos reutilizam os mesmos ports/classificadores.

## 2. Pré-condições para implementação

### 2.1 Gate da Fase 1

Antes de criar scaffold definitivo:

- [x] baseline DWYT e Ginger reproduzida;
- [x] ADR 0001 fecha Cobra + Ginger;
- [x] ADR 0002 fecha health/degradação;
- [x] ADR 0003 fecha SSE/WS/exec;
- [x] ADR 0004 fecha processo local/start/stop/status;
- [x] listener, prontidão e shutdown comprovados no spike;
- [x] SQLite sem CGO e cross-build comprovados;
- [x] frontend e migrations embutidos e executados no binário de spike;
- [x] controle local do probe passa nativamente em Linux e Windows amd64, com
  lock, identidade, paths/permissões, `status`, `stop` e cleanup;
- [x] cursor composto autenticado e ligado à geração prototipado;
- [x] uso/complemento dos pacotes Ginger documentado.

Transcripts, hashes e limites estão na
[matriz de evidências F1](research/phase1-evidence.md). Casing do artefato,
paths da implementação definitiva, macOS e Windows arm64 continuam gates
próprios de F3/F8, não pendências retroativas de F1.

### 2.2 Gate da Fase 2

- [x] [product-spec.md](product-spec.md), [architecture.md](architecture.md), [security.md](security.md), [data-model.md](data-model.md), [api.md](api.md) e este documento revisados em conjunto;
- [x] toda pendência restante está ligada a um ID F1/ADR, não a um “a decidir” solto;
- [x] nomes de ports, DTOs, rotas e estados coincidem;
- [x] cada MVP possui teste futuro e owner de fase;
- [x] threat model revisado antes de qualquer rota mutável;
- [x] nenhuma dependência nova sem justificativa.

Evidência: [validação e auditorias da Fase 2](research/phase2-validation.md).

## 3. Sequência de entrega

### 3.1 Fase 3 — Fundação

| Fatia | Resultado observável | Teste/gate |
| --- | --- | --- |
| F3-A scaffold | módulo Go 1.25, Ginger v1.4.4 e Cobra compilam | inspect/doctor/build |
| F3-B lifecycle | comando raiz/start/status/stop e lock de produção funcionam | repetir integração nativa Linux/Windows; macOS conforme adapter |
| F3-C diretórios/config | paths, permissões e precedência local | temp home + casos inválidos |
| F3-D SQLite | migrations embutidas e schema inicial | primeira execução, retry, rollback |
| F3-E HTTP | `/health`, status, envelopes e segurança local | httptest adversarial |
| F3-F frontend | shell React, estados e navegação pela History API | unitário/interação + deep link |
| F3-G embedding | binário serve SPA/migrations e fallback não captura API/health | smoke sem Node.js |
| F3-H observabilidade | arquivo, rotação, redaction e doctor | marcadores sintéticos |

Não conectar a cluster real nesta fase. Flags Kubernetes podem ser parseadas, mas somente a Fase 4 as liga ao bootstrap real.

### 3.2 Fase 4 — Kubernetes e RBAC

| Fatia | Resultado observável | Teste/gate |
| --- | --- | --- |
| F4-A kubeconfig/client | paths ordenados, contextos, plugin `exec` sanitizado | unitário + servidor controlado |
| F4-B seleção | API/UI/flags alteram generation e cancelam anterior | integração de corrida |
| F4-C scope `single` | gravação transacional | SQLite + UI |
| F4-D parser/list | entrada em massa, contadores e rollback | corpus parser + UI |
| F4-E `all` | com/sem `list namespaces`, nunca `*` | Kind RBAC |
| F4-F autorização | SAR/SSRR tri-state, cache e refresh | servidor auth + Kind |
| F4-G Permissions | matriz textual e capabilities | frontend + integração |
| F4-H hardening | JSON estrito, DTOs, redaction, inspeção DB/log | adversarial |

O harness Kind canônico começa aqui; K3d é apenas alternativa local e RBAC real
não é postergado para a Fase 8.

### 3.3 Fase 5 — Dashboard

Ordem:

1. summary e coverage;
2. restarts;
3. problemas;
4. workloads degradados;
5. eventos `Warning`;
6. scan limitado de logs;
7. métricas opcionais;
8. interface progressiva e navegação filtrada.

Cada bloco tem timeout, budget, partial errors, cancelamento e teste de geração antiga.

### 3.4 Fase 6 — Recursos

Ordem vertical:

1. Pods lista → detalhe → YAML.
2. Workloads lista → detalhe → YAML.
3. logs atuais → anteriores → follow.
4. Events.
5. Services → Ingresses → EndpointSlices.
6. ConfigMaps → Secret metadata-only.
7. watches compartilhados somente depois do HTTP funcional.

Busca/ordenação só são ativadas conforme a semântica honesta de [api.md](api.md).

### 3.5 Fase 7 — Ações

Cada ação fecha seu gate antes da próxima:

1. restart de Deployment;
2. scale de Deployment/StatefulSet;
3. delete de Pod;
4. port-forward;
5. `exec`.

O gate de cada ação exige capability UI, SAR imediato, precondition, confirmação, idempotência quando definida, timeout, cancelamento, cleanup e teste negado.

### 3.6 Fase 8 — Distribuição

1. pipeline reprodutível;
2. GoReleaser/matriz;
3. CI;
4. E2E restritivo;
5. instaladores publicados como assets da tag exata, nunca `raw/main` ou
   `latest`;
6. update/rollback;
7. remoção;
8. smoke de archives nativos;
9. preenchimento das evidências MVP.

Scripts, archives e `checksums.txt` de uma instalação pertencem à mesma versão.
O instalador recebe ou resolve uma tag explícita `v${version}`, baixa somente
assets dessa release e falha fechado se o checksum não corresponder.

## 4. Comandos oficiais futuros

O repositório deve oferecer uma interface estável por Makefile. A implementação desses targets é tarefa da Fase 3; esta seção não afirma que já existem.

| Comando | Contrato |
| --- | --- |
| `make format` | formatar Go, frontend e arquivos suportados |
| `make format-check` | falhar se formatação divergir |
| `make lint` | lint Go/frontend/config sem alterar |
| `make typecheck` | TypeScript sem emissão |
| `make test-unit` | unitários Go e frontend |
| `make test-integration` | HTTP, SQLite e Kubernetes controlado |
| `make test-race` | race detector onde suportado |
| `make test-e2e` | Kind canônico + browser/CLI |
| `make test` | unitário + integração, sem E2E caro |
| `make web-build` | instalar via lockfile e compilar assets |
| `make build` | gerar binário local com frontend/migrations |
| `make smoke` | executar binário compilado em data dir temporário |
| `make cross-build` | matriz sem CGO |
| `make verify-ginger` | `ginger inspect` + `ginger doctor` na v1.4.4 |
| `make verify` | format-check, lint, typecheck, test, build, smoke e Ginger |
| `make release-snapshot` | GoReleaser snapshot sem publicar |

Ferramentas subjacentes iniciais:

- Go: `gofmt`, `go vet`, `go test`;
- frontend: Vite, TypeScript, Vitest e Testing Library;
- E2E browser: Playwright;
- Kubernetes real: Kind canônico no CI/E2E; K3d somente como alternativa local equivalente;
- release: GoReleaser v2.

O package manager é npm 11.16.0, registrado como
`"packageManager": "npm@11.16.0"`, com `package-lock.json` lockfileVersion 3.
Build/CI usam Node.js 24.18.0; Node.js não é dependência do binário distribuído.
Baseline exata aprovada em 2026-08-03:

| Grupo | Pacotes fixados |
| --- | --- |
| runtime web | `react`/`react-dom` 19.2.8; `react-router-dom` 7.18.2; `@tanstack/react-query` 5.101.4; `lucide-react` 1.28.0 |
| build/style | `typescript` 6.0.3; `vite` 8.2.0; `@vitejs/plugin-react` 6.0.5; `tailwindcss`/`@tailwindcss/vite` 4.3.3 |
| testes | `vitest` 4.1.10; `@testing-library/react` 16.3.2; `@testing-library/dom` 10.4.1; `@testing-library/jest-dom` 7.0.0; `jsdom` 30.0.1; `@playwright/test` 1.62.1 |
| tipos/lint | `@types/react` 19.2.18; `@types/react-dom` 19.2.4; `@types/node` 24.13.3; `eslint` 10.8.0; `typescript-eslint` 8.65.0; `eslint-plugin-react-hooks` 7.1.1; `eslint-plugin-react-refresh` 0.5.3; `globals` 17.9.0 |

Versões são exatas, sem `^`/`~`, e somente um lockfile é aceito. Playwright,
Vitest e Testing Library são dependências de desenvolvimento justificadas por
testes de interação/cancelamento, nunca dependências de runtime. A escolha de
TypeScript 6.0.3 preserva o peer range de `typescript-eslint` 8.65.0.

## 5. Estratégia de testes

### 5.1 Unitários

Sem rede ou relógio real quando evitável:

- parser/deduplicação/validação de namespaces;
- invariantes de scope;
- classificadores de pods/workloads/events/logs;
- redaction;
- chave/cache RBAC;
- cursor encode/decode, estrutura namespace/kind, bad-MAC, outra instância,
  mudança de query/geração e expiração;
- DTO converters;
- preferências allowlisted;
- políticas de idempotência;
- cálculo de restarts e ordenação.

### 5.2 Integração

- `httptest`/`pkg/testhelper` para rotas, envelopes, headers e middleware;
- SQLite temporário real, incluindo WAL/journal/backup;
- client-go fake somente para comportamento simples;
- servidor HTTP Kubernetes controlado para erros, paginação, auth e plugins;
- servidor controlado que exige o media type `PartialObjectMetadata` e falha se Secret tipado for solicitado;
- relógio e filesystem injetáveis;
- leak checks para goroutines/listeners.

Fake clientset não comprova RBAC.

### 5.3 Frontend

- componentes com estados loading/vazio/offline/proibido/parcial/cancelado;
- formulários de scope e contadores;
- capabilities ocultas/desabilitadas;
- filtros e navegação preservados;
- cancelamento de query/stream;
- sem armazenamento browser de dados;
- acessibilidade com queries semânticas e auditoria automatizada.

### 5.4 Kind canônico

Executado incrementalmente desde F4:

- API Kubernetes real;
- ServiceAccounts e kubeconfigs efêmeros;
- Role/RoleBinding por namespace;
- ClusterRole apenas quando a operação é cluster-scoped;
- recursos sintéticos;
- Metrics API ausente como cenário padrão;
- nenhum segredo/credencial real no repositório.

### 5.5 E2E de artefato

Executa o binário compilado, não dev server:

- data dir temporário;
- porta obtida por bind;
- browser contra frontend embutido;
- deep link da History API recarregado diretamente, com fallback SPA que não
  captura `/api/v1`, `/health` ou endpoints internos;
- cluster restrito;
- inspeção posterior de DB/logs;
- shutdown e cleanup;
- execução sem Node.js no ambiente runtime.

## 6. Fixtures sintéticas

Namespace base:

```text
kp-allowed
kp-denied
kp-multi-a
kp-multi-b
```

Recursos:

- Deployment `healthy-app`;
- Deployment `degraded-app`;
- StatefulSet escalável;
- DaemonSet incompleto;
- Job falho e CronJob com falha recente;
- Pod `restarting-pod` com status/fixture de restart;
- Pod `log-pattern-pod` com marcadores sintéticos;
- Event `Warning`;
- Service, Ingress e EndpointSlice;
- ConfigMap sintético;
- Secret apenas para comprovar metadata-only, com marcador proibido gerado em runtime.

Marcadores de segurança são aleatórios por execução e inspecionados depois. Manifestos versionados não contêm token ou kubeconfig real.

## 7. Matriz de identidades RBAC

| Identidade | Permissões | Negado/ausente | Uso |
| --- | --- | --- | --- |
| `manual-viewer` | list/get pods/workloads/events em `kp-allowed` | `list namespaces`, logs, ações | scope manual e `all` indisponível |
| `multi-viewer` | leitura em `kp-multi-a/b` | `kp-denied`, logs | fan-out/cobertura parcial |
| `namespace-lister` | `list namespaces` cluster-scoped + leitura limitada | recursos em `kp-denied` | `all` usa resposta real, recursos continuam RBAC |
| `logs-reader` | view + `get pods/log` em allowed | exec/port-forward | logs positivos |
| `no-logs` | view pods | `pods/log` | MVP-16 |
| `restarter` | view + patch deployments | scale/delete pods | restart |
| `scaler` | view + update `deployments/scale`, `statefulsets/scale` | patch `scale`/delete pods | scale exclusivamente por `UpdateScale` |
| `pod-deleter` | view + delete pod | scale/restart | delete |
| `port-forwarder` | view + create `pods/portforward` | exec | sessão port-forward |
| `executor` | view + create `pods/exec` | demais mutações conforme fixture | exec |

O setup do cluster usa credencial administrativa efêmera apenas no harness. O produto sob teste nunca recebe `cluster-admin`.

## 8. `kubePeep doctor`

### 8.1 Checks

| Grupo | Checks |
| --- | --- |
| build | version, commit, Go/runtime e plataforma |
| diretórios | path, criação, permissões e espaço básico |
| processo | lock, PID/identidade, porta e runtime obsoleto |
| configuração | YAML estrito e valores seguros |
| SQLite | abertura, integridade, versão de migration e escrita temporária reversível |
| kubeconfig | resolução de paths, parse e contexto |
| cluster | conexão, versão sanitizada e timeout |
| segurança | bind loopback e configuração OTel |

Doctor não imprime conteúdo do kubeconfig, plugin stderr, token, certificado ou Secret.

### 8.2 Formato

- padrão humano, uma linha por check;
- `--json` emite exatamente um objeto JSON estrito, sem envelope HTTP nem
  trailing content, com o schema v1 abaixo;
- estados `pass`, `warn`, `fail`, `skip`;
- todo achado possui código estável e mensagem sanitizada.

```json
{
  "schema": 1,
  "overall": "warn",
  "checks": [
    {
      "group": "cluster",
      "name": "connection",
      "status": "warn",
      "code": "CLUSTER_UNAVAILABLE",
      "message": "The cluster is temporarily unavailable."
    }
  ]
}
```

`overall` é `pass`, `warn` ou `fail`. `checks` sempre existe, ordenado pela
ordem dos grupos da §8.1 e depois por `name`; cada objeto possui exatamente os
cinco campos mostrados. `group` pertence à lista da §8.1, `name`/`code` são
identificadores ASCII estáveis e `message` é pública, sanitizada e limitada a
1 KiB. `skip` não eleva `overall`; qualquer `fail` produz `fail`, senão qualquer
`warn` produz `warn`. Falha interna antes de construir o relatório escreve um
erro mínimo sanitizado em stderr e usa exit 4.

### 8.3 Códigos de saída

| Código | Significado |
| --- | --- |
| 0 | nenhum `warn`/`fail`; checks opcionais podem estar `skip` |
| 1 | argumento/configuração de invocação inválido |
| 2 | falha local que impede operação correta |
| 3 | aplicação local funcional, mas kubeconfig/contexto/cluster degradado |
| 4 | falha interna inesperada do doctor |

Metrics API ausente é `warn` ou `skip`, nunca falha local. O comportamento dos helpers Ginger é evidência complementar, não substitui estes checks.
Nos demais comandos, os mesmos códigos-base e cada caso operacional estão
fixados em [architecture.md §7.3](architecture.md#73-resultados-dos-comandos).

## 9. Evidência planejada dos 27 MVPs

Os caminhos abaixo são **planejados**; serão criados nas fases indicadas.

| ID | Teste/evidência planejada | Nível |
| --- | --- | --- |
| MVP-01 | `tests/e2e/cli/start_test.go` | artefato/CLI |
| MVP-02 | `internal/adapters/browser/browser_test.go` + smoke nativo | unitário/smoke |
| MVP-03 | `internal/web/embed_test.go` | integração do binário |
| MVP-04 | workflow smoke em imagem/runner sem Node.js | release |
| MVP-05 | `tests/e2e/contexts.spec.ts` | browser + API |
| MVP-06 | `tests/e2e/namespace-scope-single.spec.ts` | browser + SQLite |
| MVP-07 | `tests/e2e/namespace-scope-list.spec.ts` | browser + API |
| MVP-08 | parser unitário + índice/rollback SQLite | unitário/integração |
| MVP-09 | validação unitária + erro API/UI | três níveis |
| MVP-10 | `tests/e2e/rbac/all-namespaces.spec.ts` | Kind |
| MVP-11 | classificador + `overview-problems.spec.ts` | unitário/E2E |
| MVP-12 | restart calculation + `overview-restarts.spec.ts` | unitário/E2E |
| MVP-13 | classificadores por kind + overview | unitário/E2E |
| MVP-14 | evento/count/agregação + overview | unitário/E2E |
| MVP-15 | logs permitidos e follow | Kind/stream |
| MVP-16 | log-scan e logs negados com 403 real | Kind/API/UI |
| MVP-17 | `internal/services/logscan/limits_test.go` | unitário/integração |
| MVP-18 | overview sem `metrics.k8s.io` | integração/E2E |
| MVP-19 | matriz de capabilities por ação | frontend/Kind |
| MVP-20 | SAR imediato antes de cada ação/upgrade | integração/Kind |
| MVP-21 | suíte Kind inteira com identidade restrita | E2E |
| MVP-22 | scanner DB/WAL/backup/log após E2E | segurança |
| MVP-23 | workflow `verify` verde | CI |
| MVP-24 | relatórios `ginger inspect/doctor` por gate | CI/evidência |
| MVP-25 | `goreleaser release --snapshot --clean` | release |
| MVP-26 | casos checksum correto/adulterado Unix/Windows | installer |
| MVP-27 | configuração e smoke Linux/macOS/Windows | release nativa |

## 10. Critérios complementares não numerados

Também exigem evidência:

- bind sem corrida e browser somente após prontidão;
- timeout de shutdown ainda executa cleanup;
- `start`/`stop`/`status` seguros no Windows;
- cursor composto, adulteração, outra instância, mismatch de query/geração,
  expiração e mapeamentos 400/410;
- LIST/watch com permissão distinta e relist;
- backpressure e cliente lento;
- Secret metadata-only sem valores no processo/saída;
- OTel sem tráfego por padrão;
- `no-store` e storage browser vazio;
- Settings rejeitando chaves arbitrárias;
- packages Ginger usados ou justificados por ADR.

## 11. Quality gates por mudança

### Backend

- formatação e vet;
- unitários;
- integração pertinente;
- race quando suportado;
- sem importação clientset em handler;
- erro/cancelamento/RBAC negativo;
- inspeção de logs para marcador sensível.

### Frontend

- format/lint/typecheck;
- testes de interação;
- estados vazio/proibido/parcial;
- acessibilidade;
- cancelamento;
- nenhuma persistência de dados remotos.

### API/contrato

- docs e schema atualizados;
- exemplos validados;
- unknown fields/body limit;
- `no-store`;
- request ID;
- caminho permitido e negado.

### Sessões

- limite;
- idle timeout;
- cancelamento;
- backpressure;
- troca de geração;
- shutdown;
- leak test.

## 12. Dependências e justificativa

| Dependência | Motivo | Condição |
| --- | --- | --- |
| Ginger v1.4.4 | framework obrigatório | pin exato |
| github.com/spf13/cobra v1.10.2 | CLI híbrida | composição e versão aprovadas no ADR/matriz F1 |
| client-go/api/apimachinery v0.35.7 | integração Kubernetes oficial | minor alinhada e Go 1.25 |
| k8s.io/metrics v0.35.7 | Metrics API opcional | adapter isolado |
| modernc.org/sqlite v1.54.0 | SQLite sem CGO | cross-build F1; comportamento F3 |
| coder/websocket v1.8.15 | transporte local de `exec` | hardening do ADR 0003 |
| React/TypeScript/Vite/Tailwind/React Router | família frontend obrigatória; History API | lockfile + teste de deep link/fallback |
| TanStack Query | estado remoto/cancelamento | sem persister |
| Lucide React | ícones acessíveis/compactos | sem kit visual pesado |
| Vitest/Testing Library | teste de componentes | dev-only |
| Playwright | jornadas no binário embutido | dev/CI-only |

Qualquer dependência adicional precisa de justificativa, licença, impacto de binário e alternativas.

## 13. Política de evidências

Ao concluir uma tarefa:

```text
Tarefa:
Arquivos criados:
Arquivos alterados:
Comandos:
Testes:
Resultado:
Pendências/exceções:
Evidência:
Próxima tarefa:
```

Saída de comando relevante é guardada em relatório versionado ou artefato CI, sem segredo. “Compilou” não substitui execução quando o critério exige comportamento.

## 14. Riscos de execução

| Risco | Resposta |
| --- | --- |
| evidência nativa divergir da arquitetura | corrigir ADR e seis docs antes de avançar o gate afetado |
| tarefas horizontais crescerem | manter fatias verticais e gates |
| fake esconder RBAC real | Kind desde F4 |
| CI tardia | target `verify` na F3 e expansão incremental |
| teste E2E instável | fixtures pequenas, readiness e timeouts explícitos |
| segredo em fixture/evidência | marcadores sintéticos + scanner |
| divergência docs/API | contrato validado em testes |
| plataforma compilar e não rodar | smoke nativo de archive |

## 15. Rastreabilidade completa F2

| ID | Documento/seção responsável |
| --- | --- |
| F2-01 | product-spec §§2–5 |
| F2-02 | product-spec §7 |
| F2-03 | product-spec §8 |
| F2-04 | product-spec §9 |
| F2-05 | product-spec §6 |
| F2-06 | product-spec §10 |
| F2-07 | product-spec §12; implementation-plan §9 |
| F2-08 | architecture §§2–3 |
| F2-09 | architecture §4 |
| F2-10 | architecture §5 |
| F2-11 | architecture §6 |
| F2-12 | architecture §8 |
| F2-13 | architecture §9 |
| F2-14 | architecture §§11–12 |
| F2-15 | architecture §13 |
| F2-16 | security §§1–5, §16 |
| F2-17 | security §7 |
| F2-18 | security §6 |
| F2-19 | security §§11–12 |
| F2-20 | security §9 |
| F2-21 | security §9.3; api §4 |
| F2-22 | security §14 |
| F2-23 | security §10; api §16 |
| F2-24 | data-model §§2–4 |
| F2-25 | data-model §§6–9 |
| F2-26 | data-model §3.5 |
| F2-27 | data-model §3.5 |
| F2-28 | data-model §§5, 12 |
| F2-29 | api §7 |
| F2-30 | api §§2, 6 |
| F2-31 | api §§2, 5 |
| F2-32 | api §§3, 5 |
| F2-33 | api §§3–4 |
| F2-34 | api §§8–17 |
| F2-35 | api §§18–19 |
| F2-36 | api §20 |
| F2-37 | implementation-plan §3 |
| F2-38 | implementation-plan §§6–7 |
| F2-39 | implementation-plan §4 |
| F2-40 | implementation-plan §§2.2, 16 |
| F2-41 | product-spec §§1.1–1.2 |
| F2-42 | architecture §§6–7 |
| F2-43 | data-model §4; product-spec §7.9 |
| F2-44 | security §§8–9 |
| F2-45 | security §7.4; api §2 |
| F2-46 | security §12; architecture §15 |
| F2-47 | implementation-plan §§3.2, 5.4 |
| F2-48 | security §9.1; api §6.3 |
| F2-49 | architecture §10; api §5 |
| F2-50 | architecture §14; api §8 |
| F2-51 | security §12; architecture §15 |
| F2-52 | api §§14–16 |
| F2-53 | security §10; api §16 |
| F2-54 | architecture §13; api §§5, 13 |
| F2-55 | security §11; api §§3, 18–19 |
| F2-56 | data-model §§7–10 |
| F2-57 | architecture §11; api §18 |
| F2-58 | architecture §9.1; data-model §3.2 |
| F2-59 | api §17 |
| F2-60 | data-model §4; api §12 |
| F2-61 | implementation-plan §8 |

## 16. Revisão final da Fase 2

Procedimento:

1. validar links Markdown e nomes de arquivo;
2. extrair rotas e detectar divergências;
3. verificar cobertura F2-01–61 e MVP-01–27;
4. procurar `TODO`, `TBD`, “a decidir” e afirmações sem gate;
5. conferir ports/DTOs/estados entre documentos;
6. revisar security/data/API em conjunto;
7. registrar as pendências que dependem de F1;
8. depois de F1, atualizar e aprovar os seis documentos;
9. somente então abrir a Fase 3.

F2-40 foi fechado em 2026-08-03 com o checklist de §2.2 e as auditorias de
[contrato, plano e conteúdo estático](research/phase2-validation.md) verdes. A
prova Windows do probe isolado fechou F1-44;
portabilidade do módulo de produção e dos artefatos continua sendo revalidada
em F3/F8 e não pode ser inferida do spike.
