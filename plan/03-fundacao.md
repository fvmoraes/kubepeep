# Fase 3 — Fundação

**Estado inicial:** pendente

**Dependências:** Fases 1 e 2 concluídas
**Gate seguinte:** a integração Kubernetes começa somente após existir um binário local testado e observável.

## Objetivo

Construir a base executável do produto: CLI Cobra, aplicação Ginger, configuração, SQLite, runtime local, frontend React e embedding em um único binário. Ao final, ainda não haverá dados reais de Kubernetes, mas toda a infraestrutura de execução deverá funcionar.

## Entregáveis

- projeto Go 1.25 com `github.com/fvmoraes/ginger v1.4.4` fixado;
- entrypoint Cobra e lifecycle do serviço Ginger;
- diretórios locais e configuração multiplataforma;
- SQLite sem CGO e migrations iniciais;
- `/health`, `/api/v1/status` e bootstrap local de sessão/CSRF;
- shell React com identidade visual, rotas e estados básicos;
- frontend e migrations embutidos no binário;
- Makefile/scripts de desenvolvimento e testes básicos;
- relatório atualizado de `ginger inspect` e `ginger doctor`.

## Tarefas ordenadas

### Scaffold e dependências

- [ ] **F3-01** Gerar o scaffold `--service` aprovado em diretório temporário com Ginger CLI v1.4.4, manter `project.type: service`, revisar o diff e integrar Cobra manualmente sem sobrescrever arquivos existentes.
- [ ] **F3-02** Criar `go.mod` como `github.com/fvmoraes/kubepeep`, com Go 1.25 e versão exata `github.com/fvmoraes/ginger v1.4.4`.
- [ ] **F3-03** Adicionar somente dependências justificadas: Cobra, SQLite sem CGO integrado manualmente e bibliotecas de suporte aprovadas na Fase 2; não usar `ginger add sqlite`.
- [ ] **F3-04** Executar `go mod tidy`, revisar dependências transitivas e registrar qualquer exceção.
- [ ] **F3-05** Organizar `cmd`, `internal/api`, `internal/ports`, `internal/services`, `internal/adapters`, `internal/config`, `internal/runtime`, `internal/migrations` e `internal/web` conforme a decisão arquitetural.

### CLI e lifecycle local

- [ ] **F3-06** Implementar o comando raiz como equivalente a `start`.
- [ ] **F3-07** Implementar `start`, `stop`, `status`, `version` e `doctor`; `status` e `stop` autenticam o mesmo canal local com o token privado da instância, e `update` será concluído na Fase 8.
- [ ] **F3-08** Implementar flags `--context`, `--kubeconfig`, `--namespace`, `--no-browser` e `--port`, ainda sem conectar ao cluster.
- [ ] **F3-09** Implementar precedência de flags, config Ginger, configuração própria validada e defaults conforme especificação.
- [ ] **F3-10** Criar adapter de diretórios do usuário para Linux, macOS e Windows.
- [ ] **F3-11** Criar exatamente `~/.kubePeep/config.yaml`, `kubePeep.db`, `logs/kubePeep.log`, `runtime/kubePeep.lock`, `runtime/instance.json` e `cache/`; no Windows, usar o diretório de dados aprovado. Nunca criar cópia de kubeconfig nem arquivos PID/porta paralelos.
- [ ] **F3-12** Manter o lock estável aberto durante a instância e publicar `runtime/instance.json` de forma privada e atômica somente depois da prontidão; o schema contém versão, instance ID, PID, fingerprint de início, porta, versão do protocolo e token de controle. PID isolado nunca prova identidade.
- [ ] **F3-13** Adquirir por bind real uma porta a partir de `2748`, sem janela probe→bind, e escutar somente em loopback.
- [ ] **F3-14** Publicar `instance.json`/prontidão somente depois do listener e `/health` responderem; então abrir o navegador, respeitando `--no-browser`.
- [ ] **F3-15** Implementar shutdown com cleanup garantido de HTTP, banco, streams futuros, lock e arquivos de runtime mesmo se o encerramento gracioso expirar.

### Ginger, configuração e observabilidade

- [ ] **F3-16** Construir a aplicação com `pkg/app` e registrar rotas com `pkg/router`.
- [ ] **F3-17** Usar `pkg/config` para o bootstrap suportado e uma camada própria estrita para opções do Kube Peep que o struct fechado do Ginger não representa.
- [ ] **F3-18** Integrar `pkg/logger`/`log/slog` ao sink local aprovado, com rotação, sanitização por conteúdo e os campos `timestamp`, `level`, `component`, `operation`, `request_id`, `context`, `namespace`, `resource`, `duration` e `error_code`.
- [ ] **F3-19** Instalar middlewares de request ID, recuperação, logging, Host/Origin, CORS desabilitado, headers/CSP e CSRF antes da primeira rota mutável; implementar `GET /api/v1/session`, nonce em memória com TTL, rotação no restart e mecanismo de rebootstrap, criando cadeia separada que preserve interfaces para futuras rotas raw.
- [ ] **F3-20** Padronizar sucesso e erros com `pkg/response` e `pkg/errors`, complementados por cursor e decoder estrito próprios.
- [ ] **F3-21** Garantir que logs de startup não revelem valores de configuração sensíveis.

### SQLite e migrations

- [ ] **F3-22** Integrar `modernc.org/sqlite` ou o driver sem CGO aprovado.
- [ ] **F3-23** Criar runner de migrations versionadas e embutidas.
- [ ] **F3-24** Criar as tabelas e índices definidos em `docs/data-model.md`.
- [ ] **F3-25** Configurar foreign keys e busy timeout por conexão, pool coerente, transações, modo de journal aprovado e encerramento limpo de DB/WAL/SHM.
- [ ] **F3-26** Testar primeira inicialização, reinicialização idempotente, migration inválida e banco temporário.

### Health e status

- [ ] **F3-27** Implementar `/health` com `pkg/health` para checks locais aplicáveis e a composição aprovada para estados degradados, usando deadline interno e erro público sanitizado em cada checker.
- [ ] **F3-28** Preparar campos separados para kubeconfig, contexto e cluster sem fazer a disponibilidade local depender deles.
- [ ] **F3-29** Implementar `/api/v1/status` com versão, commit, build date, porta e as seis chaves obrigatórias de componentes: aplicação, SQLite, kubeconfig, contexto, cluster e Metrics API.
- [ ] **F3-30** Testar status saudável, SQLite indisponível e dependência externa degradada conforme o ADR.

### Frontend e embedding

- [ ] **F3-31** Criar React + TypeScript + Vite + Tailwind + React Router + TanStack Query + Lucide React.
- [ ] **F3-32** Reproduzir apenas a linguagem visual aprovada do DWYT: Catppuccin Mocha, mauve como acento, cards compactos, bordas discretas e fonte monoespaçada.
- [ ] **F3-33** Criar layout responsivo, menu principal, cabeçalho, componentes básicos e rotas placeholder sem dados fictícios.
- [ ] **F3-34** Criar cliente HTTP tipado com URLs relativas para envelopes Ginger, erros, cancelamento e estados parciais.
- [ ] **F3-35** Criar estados acessíveis de loading, vazio, erro e offline.
- [ ] **F3-36** Compilar assets, embuti-los com `go:embed` e implementar fallback SPA que não capture `/api/v1` nem `/health`.
- [ ] **F3-37** Garantir cache headers corretos para assets versionados e `index.html`.

### Ferramentas e qualidade

- [ ] **F3-38** Criar comandos únicos para formatar, lintar, testar, compilar frontend e compilar o binário.
- [ ] **F3-39** Adicionar testes Go usando `pkg/testhelper` para health/status, incluindo 200/500/503 e todos os componentes, envelopes, `GET /api/v1/session`, expiração/rotação do nonce, Host/Origin/CSRF e fallback.
- [ ] **F3-40** Adicionar testes frontend para layout, rotas e estados comuns.
- [ ] **F3-41** Executar um smoke test do binário em diretório de dados temporário e ambiente sem Node.js.
- [ ] **F3-42** Executar cross-build inicial sem CGO para a matriz aprovada.
- [ ] **F3-43** Executar `ginger inspect`, `ginger doctor`, testes, linters e build; corrigir ou documentar todo diagnóstico.
- [ ] **F3-44** Testar a infraestrutura raw com `Flusher`/`Hijacker`, Origin/Host guards e deadline superior a 15 segundos, sem expor endpoint de teste no build final.
- [ ] **F3-45** Verificar que logs operacionais chegam ao arquivo esperado, rotacionam e removem valores sensíveis mesmo quando a chave se chama apenas `error`.
- [ ] **F3-46** Aplicar permissões restritivas aos diretórios/arquivos em Unix e ACL equivalente possível no Windows.
- [ ] **F3-47** Garantir que OpenTelemetry não seja inicializado, não abra exporter/rede nem seja exigido por padrão; qualquer modo opt-in permanece separado.
- [ ] **F3-48** Criar CI mínima de pull request com formatação, testes Go, typecheck/testes frontend e build do binário embutido.
- [ ] **F3-49** Testar o modo de processo aprovado, incluindo retorno, publicação pós-prontidão, lock, identidade completa de `instance.json`, token privado, logs e canal autenticado de `status`/`stop` em Unix e Windows.
- [ ] **F3-50** Implementar `doctor` local para runtime, paths, permissões, banco, porta e integridade do frontend, deixando diagnósticos Kubernetes para a Fase 4.
- [ ] **F3-51** Testar presença, tipo e ausência segura de cada campo de observabilidade, inclusive em sucesso, erro e request sem contexto Kubernetes.
- [ ] **F3-52** Testar shutdown com rota raw ativa, timeout forçado e falha de hook, comprovando cleanup e código de saída.
- [ ] **F3-53** Inspecionar DB, journal/WAL/SHM e backups temporários para garantir ausência de conteúdo proibido.
- [ ] **F3-54** Verificar que logs reais do middleware HTTP, e não apenas chamadas manuais, chegam ao sink local e passam pela sanitização.

## Cenários mínimos de teste

- comando raiz e `start` produzem o mesmo resultado;
- `--port` válido é respeitado e porta ocupada tem comportamento especificado;
- `--no-browser` impede a abertura do navegador;
- duas inicializações concorrentes não corrompem o lock nem `instance.json`;
- duas inicializações concorrentes são impedidas pelo lock, não apenas por PID;
- `status` e `stop` autenticam a instância e tratam processo ativo, estado
  obsoleto e PID reutilizado sem atingir outro processo;
- primeira execução cria apenas arquivos permitidos;
- nomes e localização dos arquivos locais correspondem exatamente à especificação;
- migration é idempotente;
- `/health`, `/api/v1/status`, `/api/v1/session`, `/api/v1/*` inexistente e rota SPA retornam conteúdos corretos;
- nonce vencido/reiniciado é rejeitado e recuperado por novo bootstrap, sem
  repetição automática da mutação;
- binário executa com frontend embutido sem Node.js;
- telemetria permanece desativada sem configuração explícita;
- sinais encerram servidor e SQLite e removem arquivos transitórios.
- timeout de shutdown ainda executa hooks/cleanup e retorna erro observável.

## Riscos específicos

| Risco | Mitigação |
| --- | --- |
| Scaffold apagar conteúdo existente | Gerar fora do repositório e aplicar diff revisado |
| Duplo tratamento de sinais | Um único coordenador de lifecycle definido pelo ADR |
| PID representar outro processo | Validar identidade/estado e tratar arquivo como dica, não autoridade cega |
| Browser abrir antes do serviço | Health probe local antes de chamar o adapter |
| Rota SPA capturar API | Testes de precedência e not-found |
| Assets não entrarem no release | Teste do binário compilado, não apenas do dev server |

## Fora de escopo

- Conectar a um cluster real.
- Contextos, namespaces e RBAC.
- Dashboard com dados reais.
- Update funcional, GoReleaser final e instaladores.

## Critério de saída

O binário inicia por `kubePeep`, cria o banco, serve API e frontend embutido em loopback, escolhe porta, abre o navegador quando permitido e encerra de forma limpa. Testes e cross-builds passam, e `ginger doctor` não possui problema não documentado.
