# Fase 8 — Distribuição

**Estado inicial:** pendente

**Dependências:** funcionalidades e testes das Fases 3 a 7
**Gate final:** os 27 critérios e todos os gates técnicos complementares da matriz do MVP precisam ter evidência executada.

## Objetivo

Transformar o projeto testado em artefatos reprodutíveis para Linux, macOS e Windows, com frontend/migrations embutidos, checksums obrigatórios, instalação sem privilégios administrativos e release automatizada com segurança.

## Entregáveis

- `.goreleaser.yaml` v2 validado;
- workflows separados de CI e release;
- `install.sh` e `install.ps1`;
- comando `update` funcional e atômico;
- experiência documentada de remoção;
- artefatos e checksums da matriz suportada;
- smoke tests dos binários empacotados;
- E2E com cluster restrito;
- README e documentação de instalação, atualização e troubleshooting;
- [matriz de aceite](matriz-aceite-mvp.md) concluída.

## Tarefas ordenadas

### Build reprodutível

- [ ] **F8-01** Reproduzir Go 1.25, Node 24.18.0 e npm 11.16.0 já fixados e fixar uma versão GoReleaser v2 exata na CI/release antes do primeiro snapshot.
- [ ] **F8-02** Criar pipeline determinístico: instalar frontend com lockfile, testar, compilar assets e só então compilar Go.
- [ ] **F8-03** Fazer o build falhar claramente quando os assets necessários ao `go:embed` estiverem ausentes.
- [ ] **F8-04** Embutir frontend, migrations e assets sem versionar binários compilados no Git.
- [ ] **F8-05** Injetar version, commit e build date por ldflags e expô-los em CLI/status.
- [ ] **F8-06** Compilar com `CGO_ENABLED=0`, `-trimpath` e flags de tamanho aprovadas.
- [ ] **F8-07** Produzir build snapshot e comparar conteúdo/nome de todos os archives.

### GoReleaser

- [ ] **F8-08** Configurar Linux amd64 e arm64.
- [ ] **F8-09** Configurar macOS amd64 e arm64.
- [ ] **F8-10** Configurar Windows amd64 e validar Windows arm64; excluir arm64 somente com incompatibilidade reproduzida e documentada.
- [ ] **F8-11** Gerar `tar.gz` para Unix e `zip` para Windows com nomes previsíveis.
- [ ] **F8-12** Gerar `checksums.txt` SHA-256 e validar cada entrada.
- [ ] **F8-13** Validar configuração GoReleaser e executar release snapshot antes de criar tag.

### CI de pull request

- [ ] **F8-14** Executar formatação/lint, `go vet`, testes Go e race detector onde suportado.
- [ ] **F8-15** Executar lint, typecheck, testes e build do frontend.
- [ ] **F8-16** Executar testes de integração com SQLite temporário e API Kubernetes controlada.
- [ ] **F8-17** Executar `ginger inspect` e `ginger doctor` na CLI v1.4.4.
- [ ] **F8-18** Compilar o binário embutido e executar smoke test em Linux.
- [ ] **F8-19** Cobrir mudanças de backend, frontend, instaladores, workflows e configuração de release nos filtros do pipeline.

### E2E restritivo

- [ ] **F8-20** Consolidar e recriar do zero o cluster Kind canônico incremental das Fases 4 a 7; K3d permanece apenas alternativa local equivalente.
- [ ] **F8-21** Validar novamente namespace permitido/negado e Role/RoleBinding restritos.
- [ ] **F8-22** Validar novamente Deployment saudável/degradado, pod com restart e logs sintéticos.
- [ ] **F8-23** Validar novamente evento `Warning`, Service, Ingress e recursos necessários a cada ação.
- [ ] **F8-24** Validar seleção/escopos, dashboard, listas, logs, permissions e ações permitidas/negadas.
- [ ] **F8-25** Provar que, sem `list namespaces`, `all` é recusado com fallback manual; com `list`, a interface usa exatamente a resposta da API e continua autorizando recursos separadamente em cada namespace.
- [ ] **F8-26** Executar inspeção do SQLite e logs após o E2E para ausência de credenciais/conteúdo proibido.

### Instaladores

- [ ] **F8-27** Implementar detecção segura de SO/arquitetura em `install.sh`.
- [ ] **F8-28** Implementar instalação no PATH do usuário sem exigir `sudo`.
- [ ] **F8-29** Tornar download e validação SHA-256 obrigatórios; abortar se checksum ou ferramenta segura não estiver disponível.
- [ ] **F8-30** Substituir versão anterior atomicamente e preservar backup para rollback em falha.
- [ ] **F8-31** Implementar o equivalente em PowerShell 5.1+ para Windows.
- [ ] **F8-32** Manter a matriz dos instaladores idêntica à matriz realmente publicada pelo GoReleaser.
- [ ] **F8-33** Verificar `kubePeep version` após instalação e imprimir o próximo comando.
- [ ] **F8-34** Testar checksum válido, inválido, archive ausente, arquitetura não suportada, PATH e upgrade.

### Update e remoção

- [ ] **F8-35** Implementar `kubePeep update` com descoberta de versão, download, checksum obrigatório e troca atômica.
- [ ] **F8-36** Tratar binário em uso no Windows com helper pós-exit/mecanismo aprovado, checksum, rollback e teste nativo; não depender de rename do processo em execução.
- [ ] **F8-37** Implementar exatamente a experiência aprovada na Fase 2: `install.sh --uninstall`/`install.ps1 -Uninstall`, dados preservados por default e purge separado com confirmação/path/lock/reparse validados.
- [ ] **F8-38** Distinguir remoção do binário de remoção opcional dos dados locais; nunca apagar dados sem confirmação explícita.

### Release

- [ ] **F8-39** Separar workflow de validação do workflow com permissão de publicar.
- [ ] **F8-40** Publicar apenas a partir de tag/versionamento aprovado, com permissões mínimas.
- [ ] **F8-41** Executar smoke test dos archives reais em runners Linux, macOS e Windows.
- [ ] **F8-42** Executar instaladores contra uma release candidate e verificar checksum.
- [ ] **F8-43** Atualizar README com instalação, execução, flags, dados locais, segurança, RBAC, update, remoção e troubleshooting.
- [ ] **F8-44** Registrar limitações de Metrics API, plugins `exec`, plataformas e permissões.
- [ ] **F8-45** Revisar licença e avisos de dependências/reuso visual.
- [ ] **F8-46** Preencher as 27 evidências e os gates técnicos complementares da matriz do MVP.
- [ ] **F8-47** Executar `ginger doctor`, suíte completa, GoReleaser e validação final dos artefatos.
- [ ] **F8-48** Testar os comandos publicados de instalação nos caminhos canônicos do GitHub, incluindo nomes/casing dos archives.
- [ ] **F8-49** Fixar actions de terceiros por versão imutável/commit e limitar permissões do token por job.
- [ ] **F8-50** Verificar que nenhum archive contém kubeconfig, banco, logs, cache, credencial ou asset de desenvolvimento indevido.

## Matriz mínima de artefatos

| Sistema | amd64 | arm64 |
| --- | --- | --- |
| Linux | obrigatório | obrigatório |
| macOS | obrigatório | obrigatório |
| Windows | obrigatório | obrigatório se dependências permitirem; decisão documentada caso contrário |

Os instaladores nunca devem solicitar um archive que a release não publica.

## Melhorias deliberadas em relação à referência DWYT

- CI de pull request executa testes antes de qualquer release.
- Workflow de release não publica em todo push comum.
- API do frontend usa caminhos relativos, sem porta localhost hard-coded.
- Checksum Unix é obrigatório, assim como no Windows.
- Filtros de workflow incluem `install.ps1` e bibliotecas auxiliares dos instaladores.
- Matriz de GoReleaser, instaladores e documentação é única e coerente.
- Binários compilados não são mantidos no repositório.

## Testes finais obrigatórios

- unitários Go;
- integração Go/SQLite/Kubernetes;
- frontend unitário e de interação;
- race detector onde aplicável;
- E2E restritivo no Kind canônico;
- cross-build de toda a matriz;
- execução dos archives reais;
- instalação limpa e upgrade em Unix/Windows;
- checksum adulterado bloqueando instalação/update;
- execução sem Node.js;
- cluster offline e Metrics API ausente;
- `ginger inspect` e `ginger doctor`;
- inspeção de banco, logs e artefatos para dados sensíveis.

## Riscos específicos

| Risco | Mitigação |
| --- | --- |
| Release conter frontend antigo | Build encadeado e teste de hash/versão do asset |
| Installer e GoReleaser divergirem | Matriz única validada automaticamente |
| Update corromper binário | Download temporário, checksum, troca atômica e rollback |
| Workflow publicar código não validado | CI obrigatória e release somente por tag |
| Cross-build compilar mas não iniciar | Smoke test do archive em runner nativo |
| Remoção apagar dados do usuário | Escopos separados e confirmação explícita |

## Fora de escopo

- Servidor cloud obrigatório.
- Auto-update silencioso.
- Instalação que exija administrador por padrão.
- Publicar uma plataforma sem smoke test ou incompatibilidade documentada.

## Critério de saída

Todos os critérios `MVP-01` a `MVP-27` e todos os gates técnicos complementares possuem evidência executada; suítes e E2E passam; GoReleaser produz archives coerentes; instaladores e update validam SHA-256; os binários rodam sem Node.js em runtime; e a release pode ser publicada para Linux, macOS e Windows com limitações explicitamente documentadas.
