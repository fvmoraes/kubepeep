# Documentação do KubePeep

> Documentação concisa e executável. O **plano de execução** vive em [`../plan/`](../plan/README.md); este diretório descreve o produto, a arquitetura e os contratos **como estão no código**.

## Índice

| Documento | Conteúdo |
| --- | --- |
| [architecture.md](architecture.md) | Arquitetura Go/React, módulos internos, fluxos, decisões incorporadas (ADRs) |
| [design-system.md](design-system.md) | Design System v2: tokens, componentes, resource framework, regras de adoção |
| [product-spec.md](product-spec.md) | Especificação de produto: escopo, personas, fluxos, critérios |
| [api.md](api.md) | Contrato da API local (`/api/v1/…`): rotas, envelope, filtros, streaming |
| [data-model.md](data-model.md) | Modelo de dados local (SQLite), allowlists e retenção |
| [security.md](security.md) | Modelo de segurança: loopback, CSRF, RBAC, redaction, Secrets metadata-only |
| [rbac-requirements.md](rbac-requirements.md) | Catálogo de capabilities e mapeamento para SAR |
| [observability.md](observability.md) | Logs JSONL, métricas, health checks |
| [desktop-architecture.md](desktop-architecture.md) | Bridge Wails, loopback para streams, ciclo de vida desktop |
| [desktop-build.md](desktop-build.md) | Build desktop por plataforma (Wails, dependências nativas) |
| [download.md](download.md) | Instalação e distribuição por plataforma |
| [implementation-plan.md](implementation-plan.md) | Plano de implementação do MVP (referência histórica; execução atual em `../plan/`) |

## Estrutura

```
docs/
  decisions/   ADRs numerados (0001–0005) — decisões arquiteturais vigentes
  research/    Pesquisas vivas (benchmark de UX, matriz de compatibilidade, tooling)
    evidence/  Evidências reproduzíveis citadas pela arquitetura
  archive/     Evidências de execução de fases concluídas (phase1–9)
```

## Layout do repositório

| Caminho | Papel |
| --- | --- |
| `cmd/kubePeep/` + `main.go` | Entrypoints: CLI/serviço e desktop (Wails exige `main.go` na raiz) |
| `internal/` | Todo o código Go de produção (adapters, services, api, desktop, runtime) |
| `web/` | Frontend React (tokens em `src/tokens.css`, UI em `src/components/ui/`, framework em `src/components/resource/`, navegação em `src/navigation/`) |
| `plan/` | Plano de execução v1 (`v1/phase-*.md`) + especificação de referência (`reference/`) |
| `docs/` | Esta documentação |
| `scripts/` | Ferramentas de desenvolvimento (smoke, security_check, testes dos instaladores) |
| `install.sh` / `install.ps1` | Instaladores públicos — ficam na raiz por contrato: a CI os empacota na raiz dos archives e os harnesses os exercem por path relativo fixo |
| `build/` `packaging/` `configs/` | Ícones/empacotamento por plataforma e configuração do serviço |
| `test/kind/` | Harness E2E em cluster Kind efêmero (CI) |
| `spikes/phase1/` | Protótipo histórico de lifecycle (independente de `internal/`) |
| `.github/` `.githooks/` | CI (verify/release) e hooks locais de segurança |
| `*.exe`, `dist/`, `test-results/`, logs | Artefatos locais — **nunca versionados** (`.gitignore`) |

## Regras

1. Documento descreve **o código da tag atual** — nada "planejado" documentado como pronto.
2. Toda mudança de contrato (API, dados, segurança) atualiza o doc afetado no mesmo commit da feature.
3. Evidências novas (screenshots, saídas de CLI) ficam **fora do Git** (artefatos locais); o texto registra método e resultado sanitizado.
4. ADR novo = `decisions/NNNN-titulo.md` numerado sequencialmente, imutável após merge (correção em ADR novo).
