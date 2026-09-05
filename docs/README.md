# Documentação do KubePeep

`docs/` descreve o produto e seus contratos. O trabalho pendente vive no
[plano v1](../plan/README.md), baseado na
[referência UI/UX](../plan/reference/KubePeep_UI_UX_Design_System_e_Recursos_Kubernetes.md).

## Usar e desenvolver

| Documento | Conteúdo |
| --- | --- |
| [Produto](product-spec.md) | Base disponível, jornadas, estados e limites |
| [Download e instalação](download.md) | Pacotes, versão explícita, verificação, atualização e remoção |
| [Desenvolvimento](development.md) | Comandos, layout, validação, arquivos privados e regra de commit |
| [Build desktop](desktop-build.md) | Wails e dependências por plataforma |

## Contratos e arquitetura

| Documento | Conteúdo |
| --- | --- |
| [Arquitetura](architecture.md) | Camadas, composição, geração, lifecycle e transportes |
| [Desktop](desktop-architecture.md) | Bridge Wails e loopback para streams |
| [Design system](design-system.md) | Tokens, componentes e resource framework |
| [API](api.md) | Rotas, envelopes, filtros, paginação e streaming |
| [Dados](data-model.md) | SQLite, schemas e persistência permitida |
| [Segurança](security.md) | Loopback, CSRF, RBAC, redaction e conteúdo proibido |
| [RBAC](rbac-requirements.md) | Capabilities e operações Kubernetes |
| [Observabilidade](observability.md) | Logs operacionais, métricas e diagnóstico |

## Decisões e histórico

- [ADRs](decisions/README.md): decisões arquiteturais numeradas, incluindo
  o contexto em que foram tomadas e os documentos que as complementam.
- [Pesquisa](research/README.md): fundamentos e métodos de reprodução;
  benchmarks datados não são garantia de compatibilidade atual.
- [Arquivo](archive/README.md): planejamento MVP e relatos sanitizados de
  fases concluídas; não são checklists de execução da v1.

Uma alteração de contrato atualiza o documento correspondente no mesmo commit.
Novas funcionalidades só são descritas como disponíveis após implementação.
Logs crus, screenshots, JSONs de diagnóstico e resultados de testes ficam
fora do Git, conforme a [política de desenvolvimento](development.md#o-que-versionar).
