# Fase 3 — Fundação

**Estado:** concluída; gates locais e nativos Linux/macOS/Windows aprovados

**Dependências:** Fases 1 e 2 concluídas
**Gate seguinte:** a integração Kubernetes começa somente após existir um binário local testado e observável.

## Objetivo

Construir a base executável do produto: CLI Cobra, aplicação Ginger, configuração, SQLite, runtime local, frontend React e embedding em um único binário. Ao final, ainda não haverá dados reais de Kubernetes, mas toda a infraestrutura de execução deverá funcionar.

## Entregáveis

- projeto Go 1.25 com `github.com/fvmoraes/ginger v1.4.4` e `github.com/spf13/cobra v1.10.2` fixados;
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

- [x] **F3-01** Gerar o scaffold `--service` aprovado em diretório temporário com Ginger CLI v1.4.4, manter `project.type: service`, revisar o diff e integrar Cobra manualmente sem sobrescrever arquivos existentes.
- [x] **F3-02** Criar `go.mod` como `github.com/fvmoraes/kubepeep`, com Go 1.25 e versões exatas `github.com/fvmoraes/ginger v1.4.4` e `github.com/spf13/cobra v1.10.2`.
- [x] **F3-03** Adicionar somente dependências justificadas: Cobra v1.10.2, SQLite sem CGO integrado manualmente e bibliotecas de suporte aprovadas na Fase 2; não usar `ginger add sqlite`.
- [x] **F3-04** Executar `go mod tidy`, revisar dependências transitivas e registrar qualquer exceção.
- [x] **F3-05** Organizar `cmd`, `internal/api`, `internal/ports`, `internal/services`, `internal/adapters`, `internal/config`, `internal/runtime`, `internal/migrations` e `internal/web` conforme a decisão arquitetural.

### CLI e lifecycle local

- [x] **F3-06** Implementar o comando raiz como equivalente a `start`.
- [x] **F3-07** Implementar `start`, `stop`, `status`, `version` e `doctor`; `status`/`stop` usam exatamente os métodos, paths, header, guards, timeout e `ControlIdentityDTO` de `docs/api.md`, e `update` será concluído na Fase 8.
- [x] **F3-08** Implementar flags `--context`, `--kubeconfig`, `--namespace`, `--no-browser` e `--port`, ainda sem conectar ao cluster.
- [x] **F3-09** Implementar precedência de flags, config Ginger, configuração própria validada e defaults conforme especificação.
- [x] **F3-10** Criar adapter de diretórios do usuário para Linux, macOS e Windows.
- [x] **F3-11** Criar exatamente `config.yaml`, `kubePeep.db`, `logs/kubePeep.log`, `runtime/kubePeep.lock`, `runtime/instance.json` e `cache/` sob o root canônico: `~/.kubePeep/` em Unix e `%LOCALAPPDATA%\kubePeep\`, resolvido via `FOLDERID_LocalAppData`, em Windows. Nunca criar cópia de kubeconfig nem arquivos PID/porta paralelos.
- [x] **F3-12** Manter o lock estável aberto durante a instância e publicar `runtime/instance.json` de forma privada e atômica somente depois da prontidão; o schema contém versão, instance ID, PID, fingerprint de início, porta, versão do protocolo e token de controle. PID isolado nunca prova identidade.
- [x] **F3-13** Escutar somente em `127.0.0.1` e adquirir por bind real: sem porta explícita, tentar exatamente 2748–2797; com `--port N`/config, validar 1024–65535 e tentar somente N; avançar apenas em `address in use`, sem janela probe→bind.
- [x] **F3-14** Publicar `instance.json`/prontidão somente depois do listener e `/health` responderem; então abrir o navegador, respeitando `--no-browser`.
- [x] **F3-15** Implementar shutdown com cleanup garantido de HTTP, banco, streams futuros, lock e arquivos de runtime mesmo se o encerramento gracioso expirar.

### Ginger, configuração e observabilidade

- [x] **F3-16** Construir a aplicação com `pkg/app` e registrar rotas com `pkg/router`.
- [x] **F3-17** Usar `pkg/config` para o bootstrap suportado e uma camada própria estrita para opções do Kube Peep que o struct fechado do Ginger não representa.
- [x] **F3-18** Integrar `pkg/logger`/`log/slog` ao sink JSONL local aprovado, com limite de 10 MiB, cinco backups, retenção de 14 dias, falha segura, sanitização por conteúdo e os campos `timestamp`, `level`, `component`, `operation`, `request_id`, `context`, `namespace`, `resource`, `duration` e `error_code`.
- [x] **F3-19** Instalar middlewares de request ID, recuperação, logging, Host/Origin, CORS desabilitado, headers/CSP e CSRF antes da primeira rota mutável; implementar `GET /api/v1/session`, nonce em memória com TTL, rotação no restart e mecanismo de rebootstrap, criando cadeia separada que preserve interfaces para futuras rotas raw.
- [x] **F3-20** Padronizar sucesso e erros com `pkg/response` e `pkg/errors`, complementados por cursor e decoder estrito próprios.
- [x] **F3-21** Garantir que logs de startup não revelem valores de configuração sensíveis.

### SQLite e migrations

- [x] **F3-22** Integrar exatamente `modernc.org/sqlite v1.54.0`, sem CGO, conforme a decisão aprovada na Fase 2.
- [x] **F3-23** Criar runner de migrations versionadas e embutidas com checksum; antes de migration destrutiva, executar checkpoint, backup pela SQLite Backup API, verificação de integridade e restore atômico em falha.
- [x] **F3-24** Criar as tabelas e índices definidos em `docs/data-model.md`.
- [x] **F3-25** Configurar foreign keys e busy timeout por conexão, pool coerente, transações, modo de journal aprovado e encerramento limpo de DB/WAL/SHM.
- [x] **F3-26** Testar primeira inicialização, reinicialização idempotente, checksum/migration inválida, checkpoint WAL, criação/verificação de backup, falha no meio da migration e restore íntegro/atômico em banco temporário.

### Health e status

- [x] **F3-27** Implementar `/health` com `pkg/health` para checks locais aplicáveis e a composição aprovada para estados degradados, usando deadline interno e erro público sanitizado em cada checker.
- [x] **F3-28** Preparar campos separados para kubeconfig, contexto e cluster sem fazer a disponibilidade local depender deles.
- [x] **F3-29** Implementar `/api/v1/status` com versão, commit, build date, porta e as seis chaves obrigatórias de componentes: aplicação, SQLite, kubeconfig, contexto, cluster e Metrics API.
- [x] **F3-30** Testar status saudável, SQLite indisponível e dependência externa degradada conforme o ADR.

### Frontend e embedding

- [x] **F3-31** Criar React + TypeScript + Vite + Tailwind + React Router + TanStack Query + Lucide React usando npm/package-lock, Node de build e versões exatas da baseline em `docs/implementation-plan.md`, sem ranges semver.
- [x] **F3-32** Reproduzir apenas a linguagem visual aprovada do DWYT: Catppuccin Mocha, mauve como acento, cards compactos, bordas discretas e fonte monoespaçada.
- [x] **F3-33** Criar layout responsivo, menu principal, cabeçalho, componentes básicos e rotas placeholder sem dados fictícios.
- [x] **F3-34** Criar cliente HTTP tipado com URLs relativas para envelopes Ginger, erros, cancelamento e estados parciais.
- [x] **F3-35** Criar estados acessíveis de loading, vazio, erro e offline.
- [x] **F3-36** Compilar assets, embuti-los com `go:embed` e implementar fallback SPA que não capture `/api/v1` nem `/health`.
- [x] **F3-37** Garantir cache longo somente para assets versionados, `no-store` para `index.html`, `/health`, sessão e toda API, e provar que o frontend não registra Service Worker nem usa Cache Storage, IndexedDB ou persister do TanStack Query.

### Ferramentas e qualidade

- [x] **F3-38** Criar comandos únicos para formatar, lintar, testar, compilar frontend e compilar o binário.
- [x] **F3-39** Adicionar testes Go usando `pkg/testhelper` para health/status, incluindo 200/500/503 e todos os componentes, envelopes, `GET /api/v1/session`, expiração/rotação do nonce, Host/Origin/CSRF, `Cache-Control: no-store` em cada classe de rota e fallback.
- [x] **F3-40** Adicionar testes frontend para layout, rotas, estados comuns e ausência de Service Worker/Cache Storage/IndexedDB/persister.
- [x] **F3-41** Executar um smoke test do binário em diretório de dados temporário e ambiente sem Node.js.
- [x] **F3-42** Executar cross-build inicial sem CGO para a matriz aprovada.
- [x] **F3-43** Executar `ginger inspect`, `ginger doctor`, testes, linters e build; corrigir ou documentar todo diagnóstico.
- [x] **F3-44** Testar a infraestrutura raw com `Flusher`/`Hijacker`, Origin/Host guards e deadline superior a 15 segundos, sem expor endpoint de teste no build final.
- [x] **F3-45** Forçar ao menos duas rotações e verificar path, JSONL, 10 MiB, cinco backups, retenção, permissões, degradação/fallback stdout em falha e remoção de valores sensíveis no arquivo atual e backups, inclusive sob chave `error`.
- [x] **F3-46** Aplicar `0700`/`0600` em Unix e DACL protegida e herdável limitada ao SID do usuário atual em Windows; no teste nativo elevado, distinguir esse SID de `TOKEN_OWNER`, proteger e validar temporários antes do replace e filhos após herança, rejeitar reparse points/objetos adulterados por handle quando a API permitir e falhar fechado.
- [x] **F3-47** Garantir que OpenTelemetry não seja inicializado, não abra exporter/rede nem seja exigido por padrão; qualquer modo opt-in permanece separado.
- [x] **F3-48** Criar CI mínima de pull request com formatação, testes Go, typecheck/testes frontend e build do binário embutido.
- [x] **F3-49** Testar o modo de processo aprovado, incluindo retorno, publicação pós-prontidão, lock, identidade completa de `instance.json`, token privado, métodos/paths/guards/códigos do canal `status`/`stop`, prova estrita/trailing JSON e lifecycle em Unix e Windows.
- [x] **F3-50** Implementar `doctor` local para runtime, paths, permissões, banco, porta e integridade do frontend, deixando diagnósticos Kubernetes para a Fase 4.
- [x] **F3-51** Testar presença, tipo e ausência segura de cada campo de observabilidade, inclusive em sucesso, erro e request sem contexto Kubernetes.
- [x] **F3-52** Testar shutdown com rota raw ativa, timeout forçado e falha de hook, comprovando cleanup e código de saída.
- [x] **F3-53** Inspecionar DB, journal/WAL/SHM e backups temporários para garantir ausência de conteúdo proibido.
- [x] **F3-54** Verificar que logs reais do middleware HTTP, e não apenas chamadas manuais, chegam ao sink local e passam pela sanitização.

## Cenários mínimos de teste

- comando raiz e `start` produzem o mesmo resultado;
- default ocupado avança dentro de 2748–2797; `--port` explícito ocupado falha sem fallback;
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
