# Modelo de dados local

> **Status:** revisado com as evidências e ADRs da Fase 1
>
> **Banco:** SQLite com `modernc.org/sqlite v1.54.0`, validado com Go 1.25 e
> cross-build CGO-free.
>
> **Regra central:** o banco guarda configuração local allowlisted; nunca é cache de conteúdo do cluster.

## 1. Convenções

- Nomes de tabela e coluna usam `snake_case`.
- IDs locais usam `INTEGER PRIMARY KEY`.
- Booleanos usam `INTEGER NOT NULL CHECK (valor IN (0, 1))`.
- Instantes usam epoch UTC em milissegundos, `INTEGER NOT NULL`.
- Paths são armazenados como texto, sem conteúdo do arquivo.
- JSON só é usado em preferências com schema e tamanho conhecidos.
- Foreign keys e constraints são habilitadas em toda conexão.
- Exclusões de profile/scope usam cascade apenas dentro do modelo local.
- Conteúdo Kubernetes, logs, credenciais e Secrets não possuem tabela.

## 2. Diagrama

```text
cluster_profiles 1 ─── * cluster_profile_kubeconfig_files
       │
       └────────── 1 ─── * namespace_scopes 1 ─── * namespace_scope_items

preferences (key/value allowlisted, independente)
schema_migrations (controle interno)
```

`cluster_profile_kubeconfig_files` é uma extensão deliberada do modelo sugerido. Um único `kubeconfig_path` não representa corretamente o conjunto ordenado de arquivos aceito por `KUBECONFIG`.

## 3. Schema lógico

### 3.1 `cluster_profiles`

| Coluna | Tipo | Null | Regra |
| --- | --- | --- | --- |
| `id` | INTEGER | não | primary key |
| `name` | TEXT | não | trim; 1–120 caracteres |
| `context_name` | TEXT | sim | nome opaco do kubeconfig; null quando o profile ainda não possui contexto selecionado; quando presente, 1–1024 bytes |
| `is_default` | INTEGER | não | 0 ou 1 |
| `created_at` | INTEGER | não | epoch ms |
| `updated_at` | INTEGER | não | epoch ms, `>= created_at` |

Índices/constraints:

```sql
UNIQUE(name)
CREATE UNIQUE INDEX one_default_cluster_profile
  ON cluster_profiles(is_default)
  WHERE is_default = 1;
```

O nome do profile é único localmente. Contextos iguais podem existir em profiles diferentes quando o conjunto de kubeconfigs for diferente. Um kubeconfig válido sem `current-context` também produz profile: nesse caso `context_name` permanece null até uma seleção explícita.

Trocar o profile default ocorre em uma transação: zerar o anterior, marcar o novo e verificar que existe exatamente um default quando há profiles.

Profiles não são criados nem recebem paths por uma rota web no MVP. No
bootstrap, o adapter resolve o conjunto ordenado de paths pela precedência
canônica, normaliza-o e o reconcilia em uma transação:

1. procurar um profile cujo conjunto ordenado de filhos seja exatamente igual;
2. reutilizá-lo sem persistir fingerprint ou conteúdo;
3. se não existir, criar um profile e todos os seus paths atomicamente;
4. gerar o nome local a partir do contexto selecionado, ou `Default` quando não
   houver contexto, acrescentando sufixo numérico em caso de conflito;
5. tornar o primeiro profile criado o default; alterações posteriores de
   default são sempre explícitas;
6. atualizar `context_name` somente depois de validar que o contexto pertence
   ao conjunto de kubeconfigs.

Duas inicializações concorrentes não podem criar profiles para o mesmo conjunto
ordenado. Essa unicidade, que envolve várias linhas, é garantida pelo serviço
sob `BEGIN IMMEDIATE` e por teste de concorrência.

### 3.2 `cluster_profile_kubeconfig_files`

| Coluna | Tipo | Null | Regra |
| --- | --- | --- | --- |
| `cluster_profile_id` | INTEGER | não | FK para `cluster_profiles(id)` com cascade |
| `position` | INTEGER | não | `>= 0`; preserva precedência |
| `path` | TEXT | não | path não vazio; conteúdo nunca armazenado |
| `created_at` | INTEGER | não | epoch ms |

Chave e índices:

```sql
PRIMARY KEY (cluster_profile_id, position)
UNIQUE (cluster_profile_id, path)
```

Regras:

- deve existir ao menos um path por profile;
- ordem é significativa;
- persistir forma normalizada aprovada pelo adapter de plataforma;
- não expandir conteúdo, token ou certificado;
- `~` pode ser expandido antes de persistir para evitar ambiguidade;
- o comportamento de separadores, deduplicação e merge reproduz o loader
  oficial do `client-go v0.35.7`: flag explícita seleciona um único arquivo;
  `KUBECONFIG` usa a lista de paths nativa preservando precedência; na ausência
  de ambos, usa o path recomendado da plataforma.

### 3.3 `namespace_scopes`

| Coluna | Tipo | Null | Regra |
| --- | --- | --- | --- |
| `id` | INTEGER | não | primary key |
| `cluster_profile_id` | INTEGER | não | FK com cascade |
| `context_name` | TEXT | não | nome opaco do contexto do profile; 1–1024 bytes |
| `name` | TEXT | não | trim; 1–120 caracteres |
| `mode` | TEXT | não | `single`, `list` ou `all` |
| `default_namespace` | TEXT | sim | null ou nome Kubernetes válido |
| `version` | INTEGER | não | começa em 1; incrementa em cada update |
| `created_at` | INTEGER | não | epoch ms |
| `updated_at` | INTEGER | não | epoch ms, `>= created_at` |

Constraints:

```sql
CHECK (mode IN ('single', 'list', 'all'))
CHECK (mode <> 'all' OR default_namespace IS NULL)
CHECK (version >= 1)
UNIQUE (cluster_profile_id, context_name, name)
CREATE INDEX namespace_scopes_by_profile_context
  ON namespace_scopes(cluster_profile_id, context_name, id);
```

`cluster_profile_id` e `context_name` identificam a origem do scope e são
imutáveis depois da criação. Mover um scope entre profiles/contextos exige criar
outro aggregate; um `PUT` não pode alterar esses campos.

### 3.4 `namespace_scope_items`

| Coluna | Tipo | Null | Regra |
| --- | --- | --- | --- |
| `id` | INTEGER | não | primary key |
| `namespace_scope_id` | INTEGER | não | FK com cascade |
| `namespace` | TEXT | não | nome Kubernetes validado |
| `position` | INTEGER | não | `>= 0`; ordem estável da importação |
| `created_at` | INTEGER | não | epoch ms |

Índices:

```sql
UNIQUE (namespace_scope_id, namespace)
UNIQUE (namespace_scope_id, position)
CREATE INDEX namespace_scope_items_by_scope
  ON namespace_scope_items(namespace_scope_id, position);
```

### 3.5 Invariantes de escopo

| Modo | Itens | Namespace padrão |
| --- | --- | --- |
| `single` | exatamente 1 | null ou igual ao único item |
| `list` | 1 ou mais | null ou um dos itens |
| `all` | exatamente 0 | sempre null |

SQLite não possui constraint diferida simples para contar filhos no commit. O serviço deve:

1. abrir transação;
2. gravar/atualizar scope;
3. substituir itens em lote;
4. validar as invariantes com query na mesma transação;
5. incrementar `version` somente no update que satisfizer a versão esperada;
6. fazer commit apenas se todas forem verdadeiras.

Triggers defensivos impedem inserir item em scope `all` e inserir mais de um item em `single`. Testes de integração verificam que nenhuma saída de serviço consegue quebrar a invariável.

O caractere `*` é rejeitado como item em todos os modos.

### 3.6 `preferences`

| Coluna | Tipo | Null | Regra |
| --- | --- | --- | --- |
| `key` | TEXT | não | primary key e allowlist |
| `value_json` | TEXT | não | JSON canônico válido |
| `schema_version` | INTEGER | não | `>= 1` |
| `updated_at` | INTEGER | não | epoch ms |

Não existe endpoint para criar uma chave arbitrária. Uma atualização recebe um objeto conhecido e o backend traduz para chaves internas.

### 3.7 `schema_migrations`

| Coluna | Tipo | Null | Regra |
| --- | --- | --- | --- |
| `version` | INTEGER | não | primary key |
| `name` | TEXT | não | nome estável |
| `checksum` | TEXT | não | hash da migration embutida |
| `applied_at` | INTEGER | não | epoch ms |

Uma migration já aplicada com checksum diferente é erro fatal e não é reaplicada.

## 4. Allowlist de preferências

### 4.1 Chaves

| Chave | Schema v1 | Default | Limite |
| --- | --- | --- | --- |
| `ui.language` | `"en"` ou `"pt-BR"` | `"en"` | enum |
| `logs.wrap` | boolean | `false` | — |
| `logs.timestamps` | boolean | `true` | — |
| `logs.tail_lines` | integer | `200` | 1–2000 |
| `dashboard.log_scan_window` | `"15m"`, `"30m"`, `"1h"`, `"4h"` | `"15m"` | enum |
| `dashboard.section_order` | array de IDs conhecidos | ordem padrão | cada ID uma vez |
| `dashboard.hidden_sections` | array de IDs conhecidos | `[]` | subset; métricas podem ser ocultas automaticamente |
| `filters.workloads` | objeto `SavedFilterSet` | vazio | até 50 filtros |
| `filters.pods` | objeto `SavedFilterSet` | vazio | até 50 filtros |
| `filters.events` | objeto `SavedFilterSet` | vazio | até 50 filtros |
| `filters.logs` | objeto `SavedFilterSet` | vazio | até 50 filtros |

IDs de seção conhecidos:

```text
summary
problems
restarts
workloads
events
logScan
metrics
```

### 4.2 `SavedFilterSet`

```json
{
  "version": 1,
  "items": [
    {
      "id": "local-id",
      "name": "Problemáticos em payments",
      "query": {
        "namespace": "payments",
        "status": ["problematic"],
        "search": ""
      }
    }
  ]
}
```

Regras:

- `id` é local e opaco;
- `name` tem 1–80 caracteres;
- somente campos de filtro documentados para a tela;
- strings individuais até 256 bytes;
- JSON total por chave até 64 KiB;
- sem headers, cookies, kubeconfig, token, certificado, Secret, YAML, log, comando ou saída de `exec`;
- filtros que referenciam contexto/namespace não concedem acesso e são revalidados ao usar.

Antes de persistir, o backend aplica o mesmo detector de valores sensíveis usado na redaction. Um match rejeita a preferência com `PREFERENCE_SENSITIVE_VALUE`; não grava uma versão mascarada que poderia ser confundida com o filtro original.

### 4.3 Evolução

- `schema_version` acompanha o decoder da chave.
- Leitura de versão desconhecida falha de forma isolada e usa default; não derruba o banco.
- Migração de preferência é pura, testada e nunca transforma string arbitrária em campo sensível.
- Downgrade não sobrescreve valor de versão futura sem backup/aviso.

## 5. Dados deliberadamente não modelados

Não criar tabelas ou colunas para:

- conteúdo de kubeconfig;
- `rest.Config`;
- tokens, certificados, client keys ou senhas;
- conteúdo ou chaves de Secret;
- objetos Kubernetes serializados;
- YAML de recurso;
- logs, scan de logs ou downloads;
- eventos/watch cache;
- métricas;
- stdin/stdout/stderr ou command argv de `exec`;
- tráfego de port-forward;
- sessões históricas;
- CSRF token;
- cache RBAC/clientset;
- cursor de paginação.

Esses dados são transitórios em memória ou não entram no processo.

## 6. Transações

### 6.1 Reconciliar profile no bootstrap

```text
resolver e normalizar paths fora da transação
BEGIN IMMEDIATE
  → procurar igualdade exata de quantidade, posição e path
  → criar profile + lote de paths somente se não existir
  → validar contexto escolhido no kubeconfig resolvido
  → atualizar context_name quando aplicável
  → garantir exatamente um default quando houver profiles
COMMIT
```

Falha de path, parse ou contexto ocorre antes do commit. Fingerprints de
modificação são estado transitório de invalidação e nunca participam dessa
identidade persistida.

### 6.2 Criar/atualizar escopo

```text
BEGIN IMMEDIATE
  → validar profile e context_name
  → upsert scope
  → apagar itens antigos no update
  → inserir lote com posições
  → validar contagem/default/mode
COMMIT
```

Qualquer erro faz rollback integral. A API recebe a lista inteira; não há uma requisição por namespace.

### 6.3 Selecionar contexto/profile default

```text
BEGIN IMMEDIATE
  → validar profile alvo
  → validar contexto no conjunto ordenado do profile
  → atualizar context_name do profile
  → se setDefault=true, is_default = 0 nos demais
  → se setDefault=true, is_default = 1 no alvo
  → verificar cardinalidade
COMMIT
```

A geração da seleção, os contexts canceláveis e o nonce CSRF vivem somente em
memória. Depois do commit local bem-sucedido, o serviço cria a nova geração,
cancela a anterior e rotaciona o nonce. Falha anterior ao commit preserva tanto
o banco quanto a geração corrente.

### 6.4 Preferências

Uma atualização de Settings valida o objeto inteiro e grava todas as chaves afetadas em uma transação. Chave inválida não produz gravação parcial.

## 7. Configuração SQLite

Configuração inicial a validar na Fase 3:

```text
foreign_keys = ON                 por conexão
busy_timeout = 5000 ms            por conexão
journal_mode = WAL                ao abrir o banco
synchronous = NORMAL              com teste de recuperação
MaxOpenConns = 4
MaxIdleConns = 4
```

Justificativa:

- WAL permite leituras enquanto uma curta gravação de preferência/escopo ocorre.
- busy timeout evita falha imediata em concorrência local curta.
- pool é pequeno porque existe um único processo e carga local.

Esses valores são contrato inicial, não evidência de comportamento sob carga.
A matriz F1 confirmou o driver, Go 1.25 e cross-build sem CGO; a Fase 3 deve
testar WAL/SHM, corrupção simulada, lock, shutdown, concorrência e filesystem
nativo. Se um valor mudar, registrar decisão e atualizar este documento antes
do merge.

## 8. Migrations

- Arquivos SQL têm número crescente e nome estável.
- Migrations são embutidas no binário.
- Runner valida ordem, checksum e versão atual.
- Cada migration transacional roda em uma transação quando SQLite permitir.
- Nenhuma migration destrutiva roda sem backup verificável.
- Não editar migration já lançada; criar uma nova.
- Falha mantém versão anterior e impede startup de escrita.
- O binário não abre frontend como “pronto” antes de migrations concluírem.

O spike F1 comprovou frontend e migration embutidos no mesmo binário e lidos no
artefato executável. A Fase 3 transforma essa prova no runner de migrations de
produção.

## 9. Backup, update e rollback

Antes de migration destrutiva ou incompatível:

1. impedir novas gravações;
2. aguardar transações ativas;
3. executar checkpoint WAL;
4. criar backup consistente pela API SQLite aprovada, não cópia crua concorrente;
5. validar abertura/integridade do backup;
6. aplicar migration;
7. em falha, restaurar de forma atômica;
8. preservar o arquivo falho para diagnóstico sem exibir seu conteúdo.

Backup contém apenas dados permitidos, mas ainda recebe permissões restritas. Update de binário não apaga banco. Remoção de dados exige confirmação separada.

## 10. Lock e instância única

- Um processo adquire lock de instância antes de abrir o banco em modo de escrita.
- PID e porta são estado informativo; PID isolado não prova identidade.
- Lock obsoleto é recuperado somente após validação específica da plataforma.
- `stop` não envia sinal a PID sem provar que pertence ao Kube Peep esperado.
- Windows e Unix possuem adapters e testes próprios.

O mecanismo e a identidade foram fixados no ADR 0004. A Fase 3 implementa os
adapters `flock`/`LockFileEx` e o canal de controle; o smoke nativo Windows
do probe isolado F1 valida o desenho antes do scaffold, e a implementação
completa repete essa matriz antes do gate de distribuição.

## 11. Repositórios e DTOs

Repositórios locais retornam modelos de domínio, não `sql.Row` nem JSON bruto.

| Repositório | Operações |
| --- | --- |
| `ClusterProfileRepository` | list, get, find by exact ordered paths, create from bootstrap, update selected context, select default |
| `NamespaceScopeRepository` | list/get, save aggregate transactionally, delete |
| `PreferenceRepository` | get allowlisted snapshot, replace validated keys |
| `MigrationRepository` | current version/checksum, record apply |

DTO HTTP nunca expõe paths de kubeconfig salvo em uma tela de configuração que precise explicitamente deles; mesmo ali, o contrato pode mascarar diretórios pessoais.

## 12. Inspeção de dados proibidos

Testes criam somente marcadores sintéticos, por exemplo:

```text
TOKEN_SHOULD_NOT_PERSIST_7f3...
PRIVATE_KEY_SHOULD_NOT_PERSIST_...
LOG_LINE_SHOULD_NOT_PERSIST_...
```

Após cada cenário, a inspeção procura os marcadores em:

- arquivo principal;
- `-wal` e `-shm`;
- journal rollback, quando houver;
- backup;
- logs locais;
- diretório cache/runtime;
- archive final de release.

Também inspeciona schema (`sqlite_master`) para impedir novas colunas/tabelas de snapshots, Secrets ou logs.

Nenhum teste usa credencial real.

## 13. Retenção e exclusão

- Profiles reconciliados pelo bootstrap persistem até a remoção explícita de
  todos os dados locais; não existe exclusão individual de profile pela API web
  no MVP.
- A remoção integral dos dados locais elimina profiles, paths e scopes; qualquer
  ferramenta interna futura que remova um profile deve usar cascade e
  confirmação.
- Deletar scope remove itens por cascade.
- Sessões e cache não possuem retenção.
- Remoção do binário não remove dados.
- Remoção de dados locais é ação separada, com caminho e consequência mostrados.

## 14. Critérios de aceite do modelo

- Um profile representa um conjunto ordenado de kubeconfig sem armazenar conteúdo.
- Há no máximo um profile default.
- O bootstrap reutiliza exatamente um profile para o mesmo conjunto ordenado de
  paths, inclusive sob concorrência.
- Um scope pertence a exatamente um `(cluster_profile_id, context_name)` e não
  pode ser movido por update.
- `single`, `list` e `all` satisfazem cardinalidades após todo commit.
- Nenhum item duplicado existe por scope.
- `all` não cria item `*`.
- Default namespace pertence ao scope quando não nulo.
- Falha no meio da importação não deixa dados parciais.
- Foreign keys estão ativas em cada conexão do pool.
- Primeira inicialização e reinicialização são idempotentes.
- Checksum divergente de migration falha claramente.
- Preferência fora da allowlist é rejeitada sem gravação.
- DB, WAL/journal e backup não contêm marcadores proibidos.

Critérios MVP relacionados: **MVP-06–10**, **MVP-22–23**.

## 15. Rastreabilidade F2

| Tarefa | Cobertura |
| --- | --- |
| F2-24 | tabelas, tipos, chaves, índices e FKs |
| F2-25 | migrations, backup e rollback |
| F2-26–27 | invariantes de modos e proibição de `*` |
| F2-28 | dados proibidos e inspeção |
| F2-43 | preferências versionadas e allowlisted |
| F2-56 | PRAGMAs, pool, lock, WAL e backup |
| F2-58 | conjunto ordenado de kubeconfig |

## 16. Decisões fechadas e evidências de implementação

- [x] F1-25 confirmou a semântica de múltiplos kubeconfigs no loader oficial.
- [x] F1-35 confirmou o driver sem CGO no Go 1.25 e nos seis cross-builds.
- [x] F1-40 comprovou frontend e migration embutidos no artefato do spike.
- [x] O ADR 0004 definiu lock, identidade e canal de controle sem depender de
  PID como autoridade.
- [ ] A Fase 3 valida PRAGMAs em toda conexão, migrations, backup, WAL/SHM,
  recuperação e inspeção de marcadores proibidos.
- [ ] Os adapters de lock e `stop` passam em runners Unix, macOS e Windows
  nativos antes da distribuição.
