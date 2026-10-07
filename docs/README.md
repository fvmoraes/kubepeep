# Documentação do KubePeep

A documentação atual descreve o comportamento implementado e seus contratos.
Relatos datados ficam no histórico Git, com links no índice do arquivo; decisões arquiteturais têm ADRs
próprios. Comece pelo [escopo do produto](reference/product-spec.md).

## Guias

| Documento | Conteúdo |
| --- | --- |
| [Download e instalação](download.md) | Pacotes, versão explícita, checksum, atualização e remoção |
| [Desenvolvimento](guides/development.md) | Ambiente, comandos, layout e validação |
| [Build desktop](guides/desktop-build.md) | Wails e dependências nativas por plataforma |
| [Investigação e diagnóstico](guides/investigation-diagnostics.md) | Problemas, relações, logs e diagnósticos |
| [Observabilidade](guides/observability.md) | Logs operacionais, métricas e instrumentação |

## Arquitetura

| Documento | Conteúdo |
| --- | --- |
| [Visão geral](architecture/overview.md) | Camadas, composição, geração, lifecycle e transportes |
| [Desktop](architecture/desktop.md) | Bridge Wails e loopback para streams |
| [Modelo de dados](architecture/data-model.md) | SQLite, schemas e persistência permitida |
| [Design system](architecture/design-system.md) | Tokens, navegação, componentes e resource framework |
| [ADRs](decisions/README.md) | Decisões arquiteturais e contexto |

## Referência

| Documento | Conteúdo |
| --- | --- |
| [Produto](reference/product-spec.md) | Recursos disponíveis, jornadas, estados e limites |
| [API](reference/api.md) | Rotas, envelopes, filtros, paginação e streaming |
| [Segurança](reference/security.md) | Loopback, CSRF, RBAC, redaction e dados sensíveis |
| [RBAC](reference/rbac-requirements.md) | Capabilities e operações Kubernetes |
| [Protocolo e tuning](reference/protocol-tuning.md) | Configuração, métodos de medição e rollback |

## Pesquisa e histórico

- [Pesquisa](research/README.md): fontes e métodos de reprodução; medições
  datadas não garantem compatibilidade atual.
- [Arquivo](archive/README.md): links fixados para o planejamento do MVP,
  relatos de fases e evidências da v0.7 preservados no Git.

Uma alteração de contrato atualiza o documento correspondente. Novas
funcionalidades só são descritas como disponíveis após implementação.
`make docs-check` verifica caminhos e âncoras dos links locais. Resultados de
testes, logs e capturas seguem a [política de versionamento](guides/development.md#o-que-versionar).
