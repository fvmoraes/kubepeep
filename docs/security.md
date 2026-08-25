# Segurança e threat model

> **Status:** revisado com as evidências e ADRs da Fase 1
>
> **Escopo:** processo local, browser, kubeconfig, Kubernetes, SQLite, logs, streams e distribuição.
>
> **Princípio:** o Kube Peep representa e revalida autorização Kubernetes; não cria autorização própria.

## 1. Objetivos de segurança

1. Não ampliar as capacidades da identidade do kubeconfig.
2. Não copiar nem persistir credenciais Kubernetes; transmiti-las ao API
   server somente pelo `client-go` e pelo transporte TLS configurado, nunca
   devolvê-las à UI ou registrá-las.
3. Impedir que páginas externas usem a API local como ponte para o cluster.
4. Falhar fechado quando uma permissão não puder ser determinada.
5. Não expor valores de Secret em nenhuma camada do MVP.
6. Limitar carga, memória e duração de listas, scans, streams e sessões.
7. Encerrar recursos quando contexto, escopo, página ou processo mudar.
8. Instalar e atualizar somente artefatos cuja integridade foi validada.

### 1.1 Repositório e cadeia de desenvolvimento

A ausência de dados sensíveis no Git é uma premissa inegociável do projeto.
Nenhum commit, branch, tag, release, artefato ou log de CI pode conter:

- credenciais, tokens, senhas, kubeconfigs, chaves privadas ou arquivos de ambiente;
- dados de clientes, endpoints internos, PII privada ou e-mails corporativos;
- paths absolutos da máquina de desenvolvimento;
- bancos, logs, dumps, caches, binários ou outros artefatos gerados.

Fixtures precisam ser sintéticas e deliberadamente inválidas. Uma exceção do
scanner exige valor público/sintético exato, path exato e justificativa em
`.gitleaks.toml`, ou fingerprint histórico exato em `.gitleaksignore`;
allowlists amplas são proibidas. Autor, committer e tagger usam somente endereço
noreply oficial do GitHub.

Todo clone deve ativar `.githooks/pre-commit` e `.githooks/pre-push` com
`git config --local core.hooksPath .githooks`. O gate
`scripts/security_check.sh HEAD` valida o index staged e o histórico completo,
identidades, mensagens de commit/tag, paths e nomes de arquivos de risco, e
executa Gitleaks com segredos redigidos. As CIs de verificação e release
executam o mesmo gate com checkout completo.

Se um dado sensível alcançar o remoto, a resposta é: interromper novos pushes,
revogar ou rotacionar a credencial primeiro, reescrever todos os refs afetados
com lease explícito, validar novamente e solicitar ao provedor a remoção de
objetos órfãos, caches e logs que continuem acessíveis. O valor descoberto nunca
deve ser copiado para issue, commit, log ou relatório de auditoria.

## 2. Premissas e limites

- O dispositivo e a conta local do usuário não são considerados comprometidos.
- O kubeconfig já existe e é responsabilidade do usuário.
- Plugins `exec` referenciados pelo kubeconfig podem ser necessários; o Kube Peep não os instala.
- A API Kubernetes e seus authorizers são a autoridade.
- Loopback reduz exposição de rede, mas não impede DNS rebinding, CSRF ou abuso por outro processo local.
- Redaction reduz risco, mas não transforma logs de aplicações em dados confiáveis.
- O MVP não oferece confidencialidade contra outro processo com os mesmos privilégios de usuário e acesso aos arquivos locais.

## 3. Ativos protegidos

| Ativo | Impacto se exposto/alterado |
| --- | --- |
| tokens, certificados, client keys e senhas | acesso ao cluster ou sistemas relacionados |
| conteúdo e erros brutos do kubeconfig/plugin `exec` | credenciais e detalhes do ambiente |
| valores de Secrets | credenciais e dados de aplicação |
| conteúdo de logs Kubernetes | PII, tokens, connection strings e dados de negócio |
| comandos e saída de `exec` | segredos e capacidade operacional |
| port-forwards/sessões ativas | acesso indireto a serviços internos |
| contexto/namespace selecionado | risco de operar no alvo errado |
| banco/preferências | privacidade e integridade da experiência |
| binário/update | execução de código arbitrário |

## 4. Atores e ameaças

- Página web externa aberta no browser.
- Site controlando DNS e tentando rebinding para loopback.
- Extensão de browser ou script injetado.
- Usuário Kubernetes com RBAC limitado.
- Resposta malformada ou maliciosa da API Kubernetes.
- Plugin `exec` que escreve segredo no erro/stdout/stderr.
- Log de aplicação contendo token ou sequência adversarial.
- Outro processo local concorrendo por porta, PID, arquivo ou banco.
- Artefato de release adulterado.
- Cliente lento ou desconectado mantendo stream/sessão.

## 5. Fronteiras de confiança

```text
Browser não confiável
  │ Host/Origin/CSRF/CSP
  ▼
API loopback
  │ DTO + autorização + limites
  ├──────────────► Kubernetes API / plugin exec
  │
  ├──────────────► SQLite / logs / runtime files
  │
  └──────────────► browser adapter / updater
```

Cada seta exige validação e sanitização nos dois sentidos pertinentes.

## 6. Classificação de dados

| Classe | Exemplos | Memória | SQLite | Log local | Resposta HTTP |
| --- | --- | --- | --- | --- | --- |
| Pública | versão do Kube Peep, nomes de campos | sim | opcional | sim | sim |
| Operacional allowlisted | nome de contexto, paths de kubeconfig, nome de escopo, preferências | sim | sim | somente quando necessário | sim |
| Cluster não sensível | nomes/status de recursos autorizados | sim, com limite | não | somente identificadores allowlisted | sim, `no-store` |
| Sensível transitória | logs, YAML permitido, saída de `exec` | sim, pelo menor tempo | nunca | nunca como payload | sim, `no-store` |
| Credencial Kubernetes | token, certificado, chave, senha, Authorization header | apenas dentro de `client-go`/plugin que precisa | nunca | nunca | nunca para o browser |
| Proibida no MVP | valor de Secret, kubeconfig completo, comando/saída de `exec` persistido | não deve entrar no modelo de produto | nunca | nunca | nunca |

Paths de kubeconfig são sensíveis de baixa criticidade: podem ser persistidos por requisito, mas são omitidos de respostas desnecessárias e sanitizados em logs.

### 6.1 Tokens locais não são credenciais Kubernetes

| Artefato | Persistência | Transporte permitido | Exposição |
| --- | --- | --- | --- |
| nonce CSRF da sessão web | somente memória, TTL máximo inicial de 8 h | `SessionDTO` same-origin e header `X-KubePeep-CSRF` | nunca em URL/log |
| token de controle do processo | `runtime/instance.json` protegido, durante a instância | request CLI local de `status`/`stop` | nunca em API/HTML/JavaScript |
| ticket WebSocket | somente memória, one-shot e TTL curto | resposta do POST protegido e oferta em `Sec-WebSocket-Protocol` | nunca em query/log/protocolo selecionado |

Esses artefatos limitam canais locais; não concedem nem ampliam RBAC
Kubernetes. “Token” sem qualificador em uma política deve ser interpretado
conforme esta taxonomia, não como uma categoria única.

## 7. API local e browser

### 7.1 Bind

- Escutar/publicar somente `127.0.0.1` no MVP.
- Falhar fechado se configuração pedir endereço não loopback.
- Sem porta explícita, adquirir por bind real uma das 50 portas consecutivas
  `2748–2797`; avançar somente em `address in use`. `--port N`/config explícito,
  entre 1024 e 65535, exige exatamente N e não faz fallback. Nunca usar uma
  checagem separada de “porta livre”.
- Port-forward também escuta somente em loopback.

O listener e a prontidão seguem o lifecycle aceito no ADR 0004: o processo
mantém o listener adquirido, inicia HTTP e primeiro comprova `/health` na
própria instância. Só então grava PID, porta e identidade em estado temporário
privado, publica `instance.json` por substituição atômica e abre o browser.

### 7.2 Host e DNS rebinding

- Aceitar somente o Host exato `127.0.0.1:<porta>` da origem publicada; não
  tratar `localhost`, outro alias ou IPv6 como equivalente implícito.
- Rejeitar IP não loopback, hostname arbitrário, host vazio fora do protocolo permitido e porta divergente.
- Não usar wildcard CORS.
- Não refletir `Origin` ou `Host` em headers.
- Testar hostname controlado apontando para loopback.

### 7.3 Origin, CORS e CSRF

- CORS permanece desabilitado por padrão.
- Requests mutáveis (`POST`, `PUT`, `PATCH`, `DELETE`) exigem:
  - `Content-Type: application/json`, salvo upgrades explicitamente definidos;
  - `Origin` exatamente igual à origem local ativa;
  - header customizado `X-KubePeep-CSRF`;
  - token de sessão aleatório, mantido apenas em memória e obtido por bootstrap
    same-origin;
  - JSON estrito e body limitado.
- Streams via `fetch` validam `Origin`, header CSRF e geração.
- WebSocket usa um ticket one-shot criado por POST protegido; o browser o apresenta no subprotocolo do upgrade, nunca em query string.
- `Sec-Fetch-Site` diferente de `same-origin` pode ser usado como defesa adicional, nunca como única defesa.
- Ausência ou erro produz `CSRF_REJECTED` sem revelar o token esperado.

Essa regra cobre rotas da API/browser. O canal CLI separado de ADR 0004 é o
carve-out explícito: `POST /_kubepeep/control/v1/stop` proíbe body/query e
Origin, não usa CSRF/Content-Type JSON e exige peer loopback, Host exato e
`X-KubePeep-Control-Token`.

O endpoint interno `GET /api/v1/session` fornece o token de sessão ao frontend
com `Cache-Control: no-store`; ele não é persistido nem logado. Esse token é
distinto do token de controle do processo descrito no ADR 0004. O token de
controle existe somente em `runtime/instance.json`, nunca é incluído em HTML,
JavaScript, DTO ou resposta acessível ao browser e autentica apenas o canal
local de `status`/`stop`. Tanto `status` quanto `stop` precisam apresentar o
token; o fato de `status` ser uma leitura não o torna público.

### 7.4 Headers de segurança

Resposta HTML:

```text
Content-Security-Policy:
  default-src 'self';
  script-src 'self';
  style-src 'self';
  img-src 'self' data:;
  connect-src 'self' ws://127.0.0.1:<porta-publicada>;
  object-src 'none';
  base-uri 'none';
  frame-ancestors 'none'
X-Content-Type-Options: nosniff
Referrer-Policy: no-referrer
Cross-Origin-Opener-Policy: same-origin
```

Na resposta efetiva, `connect-src` é construído para a única origem e porta
publicadas. O bloco acima expressa a política, não um wildcard a ser enviado em
produção.

APIs com dados Kubernetes, permissões, status detalhado, logs e sessão usam:

```text
Cache-Control: no-store
Pragma: no-cache
```

Assets estáticos com hash podem usar cache longo; `index.html` não.

Service worker, Cache Storage, IndexedDB e persistência TanStack Query não podem armazenar dados do cluster.

## 8. Kubeconfig e plugins `exec`

- Precedência de source: flag explícita, `KUBECONFIG` como lista de paths com o
  separador da plataforma, profile persistido `is_default` e, por fim, path
  recomendado. Precedência de contexto: `--context`, `context_name` persistido
  e `current-context` somente no primeiro reconcile.
- Fonte/contexto escolhido e inválido nunca cai silenciosamente para o próximo;
  a aplicação local continua em HTTP 200 degradado e expõe erro sanitizado.
- Persistir somente conjunto ordenado de paths e contexto; nunca bytes do arquivo.
- Normalizar paths sem seguir uma troca inesperada de alvo entre validação e leitura.
- Usar loaders oficiais do client-go para certificados, tokens referenciados e plugins `exec`.
- Não imprimir `rest.Config`, headers, stdout/stderr bruto do plugin ou comando do plugin.
- Sanitizar erros antes de qualquer log ou resposta.
- Invalidar o clientset em mudança de arquivo ou erro de autenticação reconstruível.
- Aplicar timeout ao carregamento/autenticação.
- Não fazer impersonation nem aceitar headers de impersonation da UI.
- Não solicitar credencial adicional.

A composição e precedência serão implementadas com os loaders oficiais do
`client-go`. A matriz da Fase 1 confirmou a API necessária e o cross-build
CGO-free; fixtures de plugin `exec` e smoke tests nativos permanecem evidências
de implementação das fases 3 e 8.

## 9. RBAC

### 9.1 Chave de autorização

```text
context generation
+ namespace
+ apiGroup
+ resource
+ subresource
+ verb
+ resourceName (para alvo específico)
```

Consultas de coleção não inventam `resourceName`. Ações sobre objeto incluem seu nome na revisão quando a API permitir essa precisão.

### 9.2 Capability tri-state

| Estado | Origem | Comportamento |
| --- | --- | --- |
| `allowed` | SAR permitido | UI pode habilitar; backend revalida ação |
| `denied` | SAR/operação negada explicitamente | ocultar/desabilitar; HTTP 403 |
| `unknown` | timeout, erro ou review incompleto | fail-closed; não afirmar negação |

`SelfSubjectRulesReview` serve apenas como resumo/otimização e pode ser incompleto.

### 9.3 Revalidação

Antes de restart, scale, delete, port-forward e `exec`:

1. validar CSRF/origem;
2. validar seleção/generation;
3. validar alvo e preconditions;
4. executar SAR sem confiar no cache da UI;
5. executar a operação Kubernetes;
6. tratar 403 da operação real como autoridade;
7. invalidar capability relevante após divergência.

Não criar um segundo serviço de autorização na Fase 7.

### 9.4 Modo `all`

- Verificar `list namespaces`.
- Sem permissão: modo indisponível e fallback manual.
- Com permissão: usar exatamente os objetos retornados.
- Nunca persistir `*`.
- Não alegar que RBAC filtra individualmente uma coleção autorizada.
- Autorizar os recursos separadamente por namespace/escopo.

## 10. Secrets e YAML

### 10.1 Política de Secret

Secret não possui rota YAML nem detalhe genérico no MVP. O DTO metadata-only usa allowlist:

```text
apiVersion
kind
metadata.name
metadata.namespace
metadata.uid
metadata.creationTimestamp
metadata.deletionTimestamp
```

O DTO não inclui:

- `data`;
- `stringData`;
- chaves de `data`;
- labels ou annotations arbitrárias;
- `managedFields`;
- `resourceVersion`;
- `ownerReferences` sem revisão específica;
- objeto bruto ou `unstructured`;
- valores derivados, hashes ou tamanhos de cada entrada.

O adapter solicita `PartialObjectMetadata` à API Kubernetes e converte diretamente para o DTO allowlisted. Se o servidor não suportar uma resposta metadata-only, a funcionalidade fica indisponível em vez de buscar o objeto Secret completo. Um conversor YAML genérico nunca recebe Secret.

### 10.2 Outros YAMLs

- Somente para recurso cujo `get` seja permitido.
- Gerado sob demanda.
- `Cache-Control: no-store`.
- Sem persistência ou log de payload.
- Campos gerenciados ou excessivos podem ser omitidos se o contrato declarar.

## 11. Logs Kubernetes

### 11.1 Limites

O backend aplica simultaneamente:

- janela temporal allowlisted;
- `tailLines`;
- máximo de pods/containers;
- concorrência;
- timeout por consulta;
- bytes máximos por linha;
- bytes máximos por container;
- bytes máximos por resposta/scan;
- duração e buffer máximos de follow.

Defaults de produto:

```text
janela: 15 minutos
tailLines: 200
pods: 20
containers concorrentes: 4
timeout por consulta: 8 segundos
```

Os valores abaixo são limites conservadores iniciais e constituem o contrato a testar na Fase 5.

Contrato inicial de máximos:

| Dimensão | Máximo |
| --- | --- |
| `tailLines` por request | 2.000 |
| pods por scan | 50 |
| containers concorrentes | 8 |
| bytes de uma linha após leitura | 64 KiB |
| bytes analisados por container | 2 MiB |
| bytes retornados por leitura/scan | 10 MiB |
| buffer por stream SSE | menor entre 1 MiB e 1.000 eventos |
| streams SSE simultâneos por processo | 8 |
| duração absoluta de logs follow | 4 h |
| sessões port-forward simultâneas | 8 |
| sessões `exec` simultâneas | 2 |
| frame de dados `exec` | 64 KiB |
| frame de controle/resize | 4 KiB |
| idle de `exec` | 30 min |
| duração absoluta de `exec` | 4 h |
| duração absoluta de port-forward | 8 h |

Ao atingir limite de linha/container/resultado, a resposta marca `truncated`. Ao encher o buffer de um cliente lento, o backend envia erro final quando possível e encerra a conexão. Valores podem ser reduzidos por evidência de carga, mas não ampliados sem revisão de segurança e atualização deste documento.

### 11.2 Redaction

Redaction ocorre antes de DTO, resposta parcial ou log operacional. Detectores cobrem, no mínimo:

- `Bearer` e `Authorization`;
- JWT longo;
- senha em texto/chave JSON;
- chave privada PEM;
- connection string com credencial;
- tokens comuns de provedores;
- headers sensíveis.

O resultado é apresentado como “possível erro”, não diagnóstico confirmado. A interface avisa que redaction não oferece garantia absoluta e que copiar/baixar transfere responsabilidade ao usuário.

### 11.3 Persistência

Linhas, scan, download e stream não entram no SQLite, logs operacionais, tracing ou telemetry. Download é resposta explícita para o browser e não cria arquivo interno.

## 12. Logging e observabilidade

Campos allowlisted:

```text
timestamp
level
component
operation
request_id
context
namespace
resource
duration
error_code
```

Regras:

- usar identificadores, nunca payloads;
- não registrar token CSRF, cookies ou headers completos;
- não registrar kubeconfig, certificados ou tokens;
- não registrar valores de Secret;
- não registrar logs Kubernetes;
- não registrar argv digitado, stdin, stdout ou stderr de `exec`;
- sanitizar o campo `error` por conteúdo, mesmo quando sua chave não denuncia sensibilidade;
- limitar tamanho e remover quebras/control characters de dados externos;
- rotação e permissões restritas do arquivo local.

O projeto construirá `logger.Logger` com um `*slog.Logger` e handler próprios,
pois o tipo do Ginger permite essa composição. O handler aplica allowlist,
sanitização por conteúdo, stdout e arquivo rotativo; `logger.New`, que fixa
stdout e seu formato, não será usado no runtime de produção.

Política inicial fechada do arquivo local:

- `logs/kubePeep.log` e stdout recebem JSON Lines UTF-8 somente depois da mesma
  sanitização central;
- rotacionar ao atingir 10 MiB, mantendo no máximo cinco backups e removendo
  qualquer backup com mais de 14 dias; backups não são comprimidos;
- nomear backup com timestamp UTC e sequência, criar/abrir sempre no mesmo
  diretório privado e aplicar `0600` ou DACL equivalente antes de escrever;
- criação segura do sink é gate de startup: falha de path, owner ou permissão
  impede prontidão com erro local sanitizado;
- falha de rotação depois da prontidão mantém stdout, marca logging/aplicação
  como degradado, emite aviso sanitizado com rate limit e tenta novamente com
  backoff; nunca desabilita redaction nem grava em path alternativo;
- teste de carga força duas rotações, verifica limite/retention/permissões e
  inspeciona arquivo atual e backups contra o corpus sensível.

OpenTelemetry:

- desativado por padrão;
- sem exporter, socket ou tráfego quando não configurado;
- payloads sensíveis não viram atributo/evento;
- falha do exporter nunca afeta o core;
- ativação explícita somente pelo schema fechado de
  [architecture.md §7.1](architecture.md#71-configuração-operacional-local),
  sem headers/tokens configuráveis.

## 13. SQLite e filesystem

- Diretórios e arquivos têm permissões mínimas permitidas pela plataforma.
- Unix usa `~/.kubePeep/`; Windows usa
  `%LOCALAPPDATA%\kubePeep\`, resolvido pelo adapter via
  `FOLDERID_LocalAppData`, nunca pelo diretório Roaming.
- Uma instância possui lock; PID é evidência publicada, nunca autoridade
  suficiente para sinalizar ou encerrar processo.
- `status` e `stop` usam o mesmo token privado, comparação em tempo constante,
  Host/Origin fechados e prova completa de instance ID, PID, fingerprint, porta
  e protocolo.
- SQLite habilita foreign keys por conexão.
- WAL/journal, backups e temporários recebem a mesma inspeção de dados proibidos.
- Migrations e update fazem backup/rollback sem copiar credenciais que não deveriam existir.
- Preferências usam chaves e schemas allowlisted.
- `config.yaml`, tree e `InstanceStateV1` seguem os schemas estritos de
  [architecture.md §7](architecture.md#7-contrato-operacional-cli); estado
  desconhecido/adulterado falha fechado.
- Erro de path não inclui conteúdo do arquivo.
- O estado temporário recebe proteção e validação antes da publicação; PID e
  porta só se tornam visíveis depois do health e da substituição atômica.
- Estado transitório é removido e o lock é liberado no cleanup; o arquivo
  estável que ancora o lock pode permanecer sem representar uma instância viva.

O probe F1 fechou o desenho com execução nativa em Linux e Windows. Duas
limitações observadas não reabrem F1-44, mas são requisitos explícitos do
adapter F3:

- a política de owner do Windows deve funcionar também sob token elevado,
  distinguindo o SID do usuário de `TOKEN_OWNER`, e possuir teste nativo desse
  cenário;
- validação e alteração de DACL por nome têm janela TOCTOU. F3 deve usar uma
  raiz per-user confiável e, onde a API permitir, handles que não sigam reparse
  points para ligar inspeção, proteção e uso ao mesmo objeto.

F3 também protege e valida o arquivo temporário antes do replace, testa
herança efetiva nos filhos e falha fechado. F8 repete o lifecycle usando o
artefato empacotado, não o executável descartável de F1.

## 14. Ações e sessões

### 14.1 Confirmação

Operações destrutivas mostram:

- contexto;
- namespace;
- tipo;
- nome;
- ação;
- consequência;
- owner/precondition quando pertinente.

O request repete esses campos e inclui `confirmed: true`. O backend não confia na descrição visual; valida o alvo real.

### 14.2 Replay e concorrência

- Restart e criação de port-forward exigem `Idempotency-Key` por tentativa.
- A chave é ligada a método, path/alvo canônico, profile, geração e hash do body,
  com TTL terminal de 10 minutos somente em memória.
- Duplicata concorrente idêntica aguarda/reproduz a única execução; reuso com
  qualquer identidade diferente retorna `IDEMPOTENCY_CONFLICT`.
- Scale usa versão/precondition e PUT idempotente para o mesmo alvo/valor.
- Delete usa UID/resourceVersion precondition quando suportado.
- `exec` não é retomado/reproduzido; uma desconexão exige nova autorização e sessão.

### 14.3 Port-forward

- autorização `create pods/portforward`;
- alvo é Pod; container pode apenas sugerir portas, não é parâmetro de autorização;
- bind local exclusivo em loopback;
- limites de sessões e portas;
- cleanup em ação do usuário, troca de geração, Pod encerrado ou shutdown;
- nenhum tráfego de aplicação é logado.

### 14.4 Exec

- autorização `create pods/exec` imediatamente antes do upgrade;
- command enviado como array de argumentos, sem concatenação de shell;
- container e estado do Pod validados;
- POST inicial protegido por CSRF cria ticket curto em memória;
- upgrade valida Origin, ticket one-shot, geração, payload, frame e heartbeat;
- timeout idle, backpressure e limite de sessões;
- nenhum conteúdo registrado.

O transporte browser-backend será `github.com/coder/websocket v1.8.15`, com as
proteções acima. O stream remoto continuará nas bibliotecas oficiais
Kubernetes. `pkg/ws` do Ginger foi rejeitado para `exec` pelas lacunas
comprovadas no ADR 0003.

## 15. Atualização e supply chain

- GoReleaser produz checksums SHA-256.
- Instaladores e `update` abortam sem checksum válido.
- Download ocorre em arquivo temporário.
- Troca de binário é atômica com rollback.
- Workflow de publicação usa permissões mínimas e somente tags aprovadas.
- Artefato real é executado em runner nativo.
- Dependências e reuso visual têm licença/avisos revisados.

Assinatura de artefatos além de SHA-256 é melhoria futura, salvo decisão posterior.

## 16. Matriz de ameaças

| Ameaça | Controle | Evidência futura |
| --- | --- | --- |
| DNS rebinding | allowlist de Host + loopback | teste HTTP adversarial |
| CSRF | Origin + token em memória + JSON/header customizado | teste cross-origin |
| XSS | React escaping + CSP + sem HTML não confiável | testes de payload |
| vazamento de plugin `exec` | sanitização central | fixture com token sintético |
| cache RBAC obsoleto | TTL + revalidação + operação real | integração com mudança de regra |
| Secret por conversor genérico | DTO allowlist e ausência de rota YAML | testes JSON/YAML/memória |
| log com credencial | redaction antes do DTO/log | corpus adversarial |
| scan DoS | budgets e limites de bytes | teste de carga/cancelamento |
| stream órfão | registry + contextos hierárquicos | leak/cleanup test |
| port-forward exposto | bind loopback | teste de endereço |
| replay de ação | idempotência + precondition | repetição concorrente |
| update adulterado | SHA-256 obrigatório | checksum negativo |
| dados no browser | `no-store`, sem persister/service worker | teste de headers/storage |

## 17. Decisões fechadas e riscos residuais

| Tema | Decisão incorporada |
| --- | --- |
| SSE | `pkg/sse` somente em rota raw endurecida, com budgets e cancelamento |
| WebSocket de `exec` | `coder/websocket` v1.8.15; `pkg/ws` rejeitado |
| kubeconfig | loaders oficiais, flag > `KUBECONFIG` > profile default > path recomendado |
| bind e lifecycle | listener real, foreground, controle autenticado e cleanup LIFO |
| logging | handler `slog` próprio envolvido por `logger.Logger` |
| health | composição própria; somente falha local crítica controla HTTP 503 |
| SQLite | `modernc.org/sqlite` v1.54.0, sem CGO |

Estas decisões removem ambiguidade para a Fase 3. O probe F1 já comprovou
nativamente o canal de controle e os locks em Linux e Windows; ele não substitui
a evidência da implementação definitiva. Os adapters F3, fixtures reais de
plugin `exec`, rotação sob carga e testes adversariais continuam obrigatórios,
e os archives reais voltam a passar por smoke nativo na Fase 8.

Riscos aceitos no MVP:

- um usuário local com os mesmos privilégios pode observar o processo/arquivos;
- redaction de logs não garante detectar todo segredo;
- nomes de recursos/contextos são metadados operacionais persistíveis;
- ações autorizadas podem ter consequências no cluster e exigem confirmação, não sandbox.

## 18. Rastreabilidade

| Tarefas F2 | Cobertura |
| --- | --- |
| F2-16–17 | modelo de ameaças, loopback, Host, Origin e CSRF |
| F2-18 | classificação de dados |
| F2-19 | redaction, logging e sink condicionado |
| F2-20–21 | RBAC tri-state e fail-closed |
| F2-22 | confirmação e logs operacionais |
| F2-23 | política de Secrets |
| F2-44 | proibição de impersonation/credenciais/autorização paralela |
| F2-45 | `no-store` e browser storage |
| F2-46 | OpenTelemetry opt-in |
| F2-48 | `resourceName` |
| F2-51 | schema observável |
| F2-55 | limites de bytes |

Critérios MVP diretamente protegidos: **MVP-10**, **MVP-15–22**, **MVP-24**, **MVP-26**.

## 19. Checklist do gate de segurança

Decisões da especificação:

- [x] Bind, Host, Origin, CSRF e canal de controle têm políticas distintas e
  explícitas.
- [x] SSE, WebSocket, kubeconfig, logging, health, SQLite e lifecycle têm
  tecnologia e ownership definidos.
- [x] Toda ação usa o mesmo serviço de autorização e revalida o alvo.
- [x] Secret possui DTO metadata-only allowlisted e não entra no conversor
  genérico.
- [x] Nenhuma decisão de segurança necessária ao início da Fase 3 permanece “a
  decidir”.
- [x] O probe F1 de controle, lock, fingerprint, DACL e cleanup passou
  nativamente em Linux e Windows.

Evidências ainda exigidas da implementação:

- [ ] A API rejeita bind, Host e Origin não permitidos.
- [ ] Toda rota mutável comprova CSRF, body limit e JSON estrito.
- [ ] Banco, WAL/journal, backup e logs passam por inspeção automatizada.
- [ ] Streams comprovam limites, backpressure e cleanup.
- [ ] OpenTelemetry não gera tráfego por padrão.
- [ ] O adapter F3 repete canal autenticado, lock, owner/DACL, TOCTOU e cleanup
  em Unix e Windows nativos.
- [ ] Os archives F8 repetem `start`/`status`/`stop` nos runners de release.

## 20. Delta de segurança da experiência operacional

A Fase 9 introduz superfícies que aceleram navegação e troubleshooting, mas
não altera as fronteiras de confiança. O benchmark funcional está documentado
em [research/aptakube-ux-benchmark.md](research/aptakube-ux-benchmark.md); a
implementação e a identidade continuam sendo próprias do Kube Peep.

### 20.1 Paleta, busca, favoritos e recentes

- A paleta inicial contém apenas destinos estáticos, contextos/escopos já
  visíveis e comandos de navegação. Ela não executa action mutável.
- O índice de busca vive em memória e não incorpora logs, YAML, terminal,
  valores de ConfigMap/Secret, bodies de objetos ou erros brutos.
- Favoritos/recentes usam schema versionado, limite e allowlist. São proibidos
  endpoint, path de kubeconfig, token, certificado, header, UID desnecessário,
  corpo do recurso, YAML, log e comando de `exec`.
- URL/query string não recebe conteúdo remoto ou identificador classificado
  como sensível. Ao mudar autorização/origem, referências inacessíveis somem
  sem revelar metadados adicionais.
- UI preferences nunca viram mecanismo de autorização; o backend ignora
  affordances antigas e revalida operações.

### 20.2 YAML e diff

- YAML continua sob `get`, `no-store`, tamanho/timeout e memória efêmera.
- Secret não possui viewer, busca, cópia, download, favorito, coluna ou diff;
  `data` e `stringData` nunca chegam ao frontend.
- Diff exige leitura autorizada independente dos dois lados e mostra suas
  origens. A normalização de `managedFields`/status é opt-in e não pode ocultar
  que campos foram removidos da comparação.
- Renderização, syntax highlighting e busca não enviam conteúdo a worker,
  serviço, CDN ou telemetria externos.
- Edição/aplicação genérica não faz parte deste gate. Uma fase futura precisará
  de server-side dry-run, preview, `resourceVersion`, confirmação, RBAC e
  recusa de Secret antes de qualquer request mutável.

### 20.3 Logs agregados

- Cada fonte passa por autorização e budget próprios; permissão em um
  namespace/pod não se propaga a outro.
- Toda linha mantém origem, e o merge não remove a distinção entre containers,
  pods, namespaces ou contextos.
- Fontes, linhas, bytes, janela, concorrência e duração têm limites globais e
  por origem. Cliente lento aciona backpressure/truncamento explícito.
- Troca de rota, escopo, geração ou conjunto de contextos cancela leitores e
  fecha streams. Reconexão nunca muda silenciosamente de alvo.
- Busca/destaque/estatística usam apenas o buffer em memória. Conteúdo não entra
  em SQLite, arquivo de aplicação, storage do browser, telemetria, erro ou
  diagnóstico.
- Copiar/baixar é gesto explícito e transfere dados diretamente ao browser; a
  aplicação não cria cópia persistente intermediária.

### 20.4 Multi-contexto

- O modo agregado da Fase 9 é somente leitura e usa fan-out limitado,
  cancelável e ligado a uma geração.
- Todo item carrega proveniência mínima: profile, contexto, cluster,
  namespace, tipo e geração. Essa proveniência é exibida, não apenas interna.
- RBAC/capability são calculados separadamente por contexto, namespace,
  resource, subresource, verb e resourceName. Não existe união de permissões.
- Falha, timeout, autenticação, `403`, stale, truncamento e retry permanecem
  associados à origem. Um contexto não apaga nem torna confiáveis dados de
  outro.
- Toda mutação exige um único alvo/origem explícito e reautorização imediata;
  não há restart/scale/delete/exec/port-forward em massa implícito.
- Kubeconfigs continuam somente leitura e clusters não precisam se conectar
  entre si nem receber componente do Kube Peep.

### 20.5 Port-forward e conexão

- Port-forward usa loopback por padrão e nunca escolhe `0.0.0.0` como fallback
  de colisão.
- Sessões possuem dono, limite, contexto/escopo/generation e cleanup em stop,
  stop-all, troca de origem e shutdown.
- Estado de conexão usa retry limitado com backoff/jitter e cancelamento. Erro
  publicado é código/estado sanitizado, nunca body, plugin stderr, path,
  endpoint, stack trace ou credencial.

### 20.6 Gate negativo

Antes de fechar qualquer critério `UX-M`, o E2E injeta sentinelas sintéticas em
Secret, log, YAML e erro e inspeciona:

- localStorage, sessionStorage, IndexedDB, Cache API e service workers;
- SQLite, WAL/journal, backups e arquivos temporários controlados;
- logs, diagnósticos, traces e outputs de CI;
- archives, installers e assets embutidos;
- staged content e todo o histórico Git alcançável.

A inspeção nunca publica o valor da sentinela. Ela registra somente
pass/fail, categoria e caminho relativo allowlisted. Qualquer achado reabre o
gate, remove o dado do estado versionado/alcançável e exige rotação/revogação
quando o valor puder ser uma credencial real.
