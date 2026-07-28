# Especificação de produto e experiência

> **Status:** revisado com as evidências e ADRs da Fase 1
>
> **Fontes:** [prompt inicial](../plan/initial_prompt.md), [plano geral](../plan/README.md), [Fase 2](../plan/02-especificacao.md) e [matriz do MVP](../plan/matriz-aceite-mvp.md).

## 1. Propósito

O Kube Peep é uma interface web local, compacta e orientada a problemas para pessoas desenvolvedoras que usam Kubernetes com permissões restritas. O produto deve reduzir o tempo entre “há algo errado” e “sei qual recurso investigar”, sem representar capacidades que a identidade atual não possui.

Princípio invariável:

> Mostrar somente o que o usuário pode acessar e habilitar somente o que ele pode executar.

O Kube Peep complementa `kubectl` e ferramentas administrativas. Ele não tenta substituir uma console de administração de clusters.

### 1.1 Convenção de nomes

| Uso | Forma canônica |
| --- | --- |
| Produto e textos de marca | **Kube Peep** |
| Executável e comando | `kubePeep` |
| Diretório Unix | `~/.kubePeep/` |
| Repositório e módulo Go | `github.com/fvmoraes/kubepeep` |
| Termos literais da API Kubernetes | Mantidos em inglês e entre crases |
| Navegação | Rótulos em inglês definidos neste documento; uma futura localização não altera as rotas |

O spike preservou esse casing nos cross-builds Linux, macOS e Windows. A Fase 8
repete a verificação executando archives em runners nativos, inclusive
`kubePeep.exe` em Windows.

### 1.2 Distribuição canônica

| Item | Convenção |
| --- | --- |
| Binário Unix | `kubePeep` |
| Binário Windows | `kubePeep.exe` |
| Archive Linux/macOS | `kubePeep_${version}_${goos}_${goarch}.tar.gz` |
| Archive Windows | `kubePeep_${version}_windows_${goarch}.zip` |
| Checksums | `checksums.txt` com SHA-256 |
| Tags | `v${version}`; versão do archive sem prefixo `v` |
| Releases | `https://github.com/fvmoraes/kubepeep/releases/download/v${version}/...` |
| Instalador Unix | `https://github.com/fvmoraes/kubepeep/releases/download/v${version}/install.sh` |
| Instalador Windows | `https://github.com/fvmoraes/kubepeep/releases/download/v${version}/install.ps1` |

GoReleaser, instaladores e documentação devem derivar da mesma matriz; a Fase 8
executa o smoke nativo da convenção antes da primeira publicação. Scripts,
archives e checksums são assets da mesma tag explícita. Documentação e
instaladores não baixam de `raw/main`, branch mutável ou `latest`.

## 2. Pessoas usuárias

### 2.1 Persona primária — pessoa desenvolvedora de aplicação

- Possui acesso a um ou poucos namespaces.
- Precisa inspecionar workloads, pods, eventos e logs.
- Pode ter algumas ações operacionais, como reiniciar um Deployment ou fazer port-forward.
- Não conhece nem controla a política global do cluster.
- Usa um kubeconfig já funcional, inclusive com plugin `exec`.

Objetivo principal: localizar rapidamente um problema dentro do acesso já concedido, sem precisar memorizar muitos comandos ou interpretar uma interface administrativa extensa.

### 2.2 Persona secundária — pessoa de suporte ou SRE com acesso segmentado

- Alterna entre contextos e conjuntos de namespaces.
- Precisa distinguir indisponibilidade do cluster de negação por RBAC.
- Usa detalhes, YAML, eventos, logs, port-forward e `exec` conforme autorização.
- Valoriza rastreabilidade local e encerramento determinístico de sessões.

### 2.3 Não persona do MVP

Uma pessoa administradora buscando governança global, edição arbitrária de recursos, aplicação de YAML, gestão de credenciais, auditoria centralizada ou operação multiusuário deve usar outra ferramenta. Esses problemas não orientam decisões do MVP.

## 3. Problemas a resolver

1. Identificar qual contexto e escopo estão ativos antes de interpretar dados.
2. Descobrir pods e workloads degradados sem carregar todo o cluster.
3. Encontrar restarts, eventos `Warning` e possíveis erros em logs com limites previsíveis.
4. Navegar entre resumo, lista, detalhe, YAML e logs preservando o filtro de origem.
5. Entender se uma ausência é “zero resultados”, “acesso negado”, “indisponível” ou “ainda carregando”.
6. Executar apenas ações autorizadas, com confirmação e consequência explícitas.
7. Encerrar streams e sessões ao mudar de contexto, sair da tela ou parar o processo.

## 4. Objetivos e não objetivos

### 4.1 Objetivos do MVP

- Executar localmente como um único binário, sem Node.js em runtime.
- Usar o kubeconfig já usado pelo `kubectl`, sem copiar credenciais.
- Suportar contexto e escopo de namespaces `single`, `list` e `all`.
- Oferecer overview progressivo de saúde e problemas.
- Expor recursos somente leitura, logs e YAML permitido.
- Expor restart, scale, delete de Pod, port-forward e `exec`, cada um condicionado a RBAC.
- Continuar útil sem Metrics API e durante falhas parciais.
- Instalar, atualizar e remover em Linux, macOS e Windows.

### 4.2 Não objetivos do MVP

- Ser um servidor cloud, produto multiusuário ou sistema com login próprio.
- Substituir Rancher, Lens, OpenShift Console ou uma plataforma de observabilidade.
- Fazer impersonation, pedir novas credenciais ou contornar RBAC.
- Exibir valores de Secret.
- Editar ou aplicar YAML genericamente.
- Persistir snapshots do cluster, logs de aplicação ou sessões.
- Fazer scan ilimitado de logs ou watch indiscriminado.
- Oferecer gráficos decorativos.
- Escutar fora de loopback.
- Instalar ou gerenciar dependências exigidas por plugins `exec` do kubeconfig.

## 5. Fronteira funcional do MVP

| Área | Incluído | Explicitamente fora do MVP |
| --- | --- | --- |
| Execução local | `start`, comando raiz equivalente, `stop`, `status`, `version`, `doctor`, `update`; frontend embutido | Serviço remoto obrigatório |
| Contextos | listar, selecionar e iniciar por flag | editar credenciais do kubeconfig |
| Escopos | CRUD de `single`, `list`, `all`; importação em massa | inferir namespaces não retornados pela API |
| Overview | resumo, problemas, restarts, workloads, `Warning`, scan limitado, métricas opcionais | varredura irrestrita ou gráficos sem ação |
| Recursos | Workloads, Pods, Events, Services, Ingresses, EndpointSlices, ConfigMaps, metadados de Secrets | edição genérica e valores de Secrets |
| Logs | atuais, anteriores, follow, busca local, cópia e download explícito | persistência automática |
| Ações | restart de Deployment; scale de Deployment/StatefulSet; delete de Pod; port-forward; `exec` | bypass de RBAC e ações administrativas genéricas |
| Preferências | chaves allowlisted para UI, filtros e dashboard | dados arbitrários ou sensíveis |
| Distribuição | artefatos e instaladores com SHA-256 | auto-update silencioso |

## 6. Arquitetura de informação

O menu lateral permanece compacto e nesta ordem:

1. **Overview**
2. **Workloads**
3. **Pods**
4. **Logs**
5. **Events**
6. **Network**
7. **Config**
8. **Namespaces**
9. **Permissions**
10. **Settings**

### 6.1 Responsabilidade de cada área

| Área | Responsabilidade |
| --- | --- |
| Overview | responder rapidamente se o cluster está acessível e onde investigar |
| Workloads | listar e detalhar Deployments, StatefulSets, DaemonSets, Jobs e CronJobs |
| Pods | listar, filtrar, detalhar e navegar para YAML/logs |
| Logs | selecionar namespace/pod/container e ler logs atuais, anteriores ou follow |
| Events | lista cronológica filtrável, preservando dados reais do Kubernetes |
| Network | Services, Ingresses, EndpointSlices e sessões de port-forward |
| Config | ConfigMaps e metadados allowlisted de Secrets |
| Namespaces | contextos e escopos `single`, `list`, `all` |
| Permissions | capabilities por namespace, recurso, sub-recurso e ação |
| Settings | preferências allowlisted; nunca credenciais nem dados do cluster |

### 6.2 Roteamento do frontend

O frontend usa React Router sobre a **History API**, sem `HashRouter`. URLs de
telas permanecem legíveis e podem ser recarregadas ou abertas diretamente.

O servidor resolve na seguinte ordem:

1. assets estáticos reais;
2. rotas `/api/v1`, `/health` e endpoints internos, que nunca recebem HTML;
3. fallback para `index.html` somente em `GET`/`HEAD` que aceite HTML;
4. `404`/`405` para os demais casos.

O fallback aprovado no spike impede que uma API desconhecida pareça uma página
válida. Chamadas do frontend usam caminhos relativos de mesma origem sob
`/api/v1`; porta dinâmica não é codificada no bundle.

## 7. Jornadas

### 7.1 Primeiro uso e inicialização

1. A pessoa executa `kubePeep` ou `kubePeep start`.
2. A aplicação resolve paths locais, kubeconfig e configuração.
3. O SQLite e a API local iniciam.
4. O frontend embutido fica pronto.
5. O navegador abre, salvo `--no-browser`.
6. A tela distingue aplicação local saudável de kubeconfig/contexto/cluster degradado.

Estados obrigatórios: primeira execução, instância já ativa, porta pedida ocupada, kubeconfig ausente, contexto inválido, cluster offline e falha local crítica.

Critérios relacionados: **MVP-01–04**, **MVP-22**, **MVP-24**.

O comando raiz e `start` permanecem em foreground. A porta é adquirida por bind
real a partir de 2748, o browser só abre depois da prontidão e `status`/`stop`
usam o canal de controle autenticado da instância, não PID como autoridade. O
cleanup continua mesmo após timeout de shutdown, conforme ADR 0004.

### 7.2 Seleção de contexto

1. A pessoa abre o seletor no cabeçalho ou em Namespaces.
2. A aplicação mostra o profile explícito e somente nomes/metadados não
   sensíveis dos contextos desse profile.
3. A seleção cancela requisições, streams e sessões ligados à geração anterior.
4. O status passa a mostrar carregamento e depois acessível, offline ou inválido.
5. A escolha pode ser persistida como profile padrão.

Uma resposta atrasada do contexto anterior nunca pode substituir a seleção atual.

Critério relacionado: **MVP-05**.

### 7.3 Criação de escopo `single`

1. A pessoa informa nome, contexto, modo e um namespace.
2. O backend valida a sintaxe e, quando autorizado, a existência.
3. Falta de `list namespaces` não impede salvar um nome sintaticamente válido.
4. A gravação ocorre em uma transação.

Critérios relacionados: **MVP-06**, **MVP-09**, **MVP-10**.

### 7.4 Importação de escopo `list`

1. A pessoa informa nome, contexto, modo e namespace padrão opcional.
2. Cola uma lista por linhas, vírgulas, ponto e vírgula, espaço não ambíguo, YAML simples ou JSON simples.
3. Seleciona **Validar**.
4. A tela mostra separadamente válidos, duplicados descartados, vazios descartados e inválidos.
5. A pessoa pode remover chips ou limpar a entrada.
6. **Salvar** envia uma única requisição.
7. O backend repete a validação e grava toda a lista em uma única transação.

Se um item ou constraint falhar, nenhum item do escopo é persistido.

Critérios relacionados: **MVP-07–09**, **MVP-22**.

### 7.5 Escopo `all`

1. A pessoa seleciona **All namespaces**.
2. O backend verifica `list namespaces`.
3. Se a coleção puder ser listada, o produto usa exatamente a resposta da API.
4. Se não puder, mostra explicação e oferece voltar a uma lista manual.
5. O banco persiste o modo `all`, nunca `*`.

O RBAC nativo autoriza a operação de coleção; o produto não promete filtrar objetos `Namespace` depois de um `list` autorizado. A autorização de recursos continua sendo avaliada separadamente.

Selecionar qualquer scope salvo cria uma nova geração e cancela queries,
streams e sessões da anterior. Para `all`, a seleção revalida `list namespaces`;
se a permissão foi removida, mantém o scope/generation anterior e oferece a
lista manual.

Critério relacionado: **MVP-10**, **MVP-21**.

### 7.6 Overview

O cabeçalho exibe produto, contexto, cluster, escopo, quantidade de namespaces, conexão, última atualização e atalhos. Os blocos carregam de forma independente:

- summary;
- pods problemáticos;
- top restarting pods;
- workloads degradados;
- eventos `Warning`;
- possíveis erros em logs;
- métricas opcionais.

Cada card navega para a lista correspondente com filtro preservado. Uma falha em eventos, logs ou métricas não derruba os demais blocos.

Critérios relacionados: **MVP-11–14**, **MVP-17–18**.

### 7.7 Recursos e logs

1. A pessoa filtra uma lista paginada.
2. Abre detalhe ou YAML sob demanda.
3. Em Pods, pode abrir logs se `get pods/log` estiver permitido.
4. Logs follow encerram ao sair, trocar seleção, trocar geração ou perder a sessão.
5. Download ocorre somente por ação explícita e não cria persistência interna.

Secret não possui rota YAML no MVP. Sua tela usa somente os campos allowlisted definidos na [especificação de segurança](security.md).

Critérios relacionados: **MVP-15–16**, **MVP-21**.

### 7.8 Ações

1. A interface consulta capabilities e decide ocultar ou desabilitar.
2. Ao iniciar uma ação, mostra contexto, namespace, tipo, nome, ação e consequência.
3. Operações destrutivas exigem confirmação explícita.
4. O backend revalida autorização sobre o alvo imediatamente antes da operação.
5. Sucesso não implica resultado assíncrono concluído; por exemplo, restart iniciado não significa rollout saudável.
6. Erro, cancelamento e alvo alterado são mostrados sem inventar diagnóstico.

Critérios relacionados: **MVP-19–20**.

### 7.9 Settings

Settings permite apenas chaves versionadas e allowlisted, incluindo:

- quebra de linha em logs;
- timestamps em logs;
- `tailLines` dentro do limite;
- janela do scan entre os valores aceitos;
- filtros salvos sem conteúdo do cluster;
- visibilidade/ordem de blocos do dashboard, quando suportada.

Não aceita tokens, certificados, headers, paths arbitrários fora do contrato, conteúdo de logs, comandos de `exec` ou objetos Kubernetes.

## 8. Estados de interface

| Estado | Significado | Representação | Ação disponível |
| --- | --- | --- | --- |
| Carregando | ainda não há resposta para a geração atual | skeleton compacto e rótulo acessível | cancelar quando aplicável |
| Vazio | chamada permitida e concluída com zero itens | mensagem específica, nunca “sem acesso” | ajustar filtro ou escopo |
| Offline | API local está acessível, dependência Kubernetes não | bloco degradado com motivo sanitizado | tentar novamente, trocar contexto |
| Proibido | negação explícita do Kubernetes ou operação real | mensagem `FORBIDDEN`, sem sugerir falha técnica | abrir Permissions |
| Desconhecido | revisão de autorização falhou ou ficou incompleta | capacidade indisponível, fail-closed | atualizar permissões |
| Parcial | alguns namespaces/blocos carregaram e outros falharam | dados válidos mais lista de falhas parciais | repetir somente o bloco |
| Cancelado | geração/seleção mudou ou pessoa cancelou | sem toast de erro; estado anterior não reaparece | iniciar nova consulta |
| Truncado | budget/limite encerrou a coleta | indicador `truncated` e cobertura | refinar filtro ou solicitar próxima página |
| Obsoleto | há dado anterior durante refresh | dado marcado com instante, sem misturar gerações | aguardar ou cancelar |

HTTP 403 é reservado a negação explícita. Falha de `SelfSubjectAccessReview`, timeout ou resposta incompleta produz capacidade `unknown` e falha fechada.

### 8.1 Cobertura por fluxo

| Fluxo | Carregando | Vazio | Offline | Proibido/desconhecido | Parcial | Cancelado |
| --- | --- | --- | --- | --- | --- | --- |
| Primeiro uso | progresso local por componente | nenhum contexto configurado | shell local com cluster degradado | não aplicável à saúde local | aplicação/SQLite saudáveis e Kubernetes degradado | shutdown limpa progresso |
| Contextos | skeleton no seletor | kubeconfig sem contextos | contextos locais visíveis, conexão falha | autenticação indisponível não vira 403 | profiles válidos preservados | seleção anterior descartada |
| Escopos | validação/salvamento distintos | nenhuma lista salva | edição local permanece utilizável | existência não verificável não bloqueia lista manual | relatório separa validação local/remota | validação anterior descartada |
| Overview | skeleton por bloco | zero real por card | blocos remotos degradados | bloco negado ou capability unknown | outros blocos continuam | refresh anterior substituído |
| Recursos | tabela preserva filtro | lista permitida sem itens | erro da origem atual | namespace/recurso negado | namespaces permitidos continuam | query/watch encerrado |
| Logs | indicador por leitura | zero linhas permitido | Pod/cluster indisponível | ação ausente/desabilitada e API segura | containers permitidos continuam | leitura/stream encerrado |
| Ações | progresso não promete conclusão | alvo não existe | operação não iniciada | desabilitada ou 403 real | não se declara sucesso parcial oculto | estado reconciliado antes de repetir |
| Settings | formulário/salvamento | defaults sem preferências | operação local independe do cluster | chave desconhecida é validação, não RBAC | update é transacional, sem parcial | formulário preserva estado local |

## 9. Política de visibilidade de ações

| Situação | Tratamento |
| --- | --- |
| Ação irrelevante ao recurso ou nunca suportada no MVP | ocultar |
| Negação explícita e a descoberta da limitação ajuda a pessoa | mostrar desabilitada com texto/tooltip |
| Negação explícita sem valor informativo ou que polui a tela | ocultar; Permissions continua sendo a fonte explicativa |
| Capability desconhecida | mostrar desabilitada como “permissão não pôde ser verificada” |
| Capability permitida | habilitar, mas revalidar no backend |
| Operação real retorna 403 após capability permitida | fechar ação, mostrar negação e invalidar cache |

Ícones nunca são a única forma de comunicar estado.

## 10. Identidade visual e acessibilidade

### 10.1 Tokens base

```css
--ctp-mauve: #cba6f7;
--ctp-red: #f38ba8;
--ctp-peach: #fab387;
--ctp-yellow: #f9e2af;
--ctp-green: #a6e3a1;
--ctp-sky: #89dceb;
--ctp-text: #cdd6f4;
--ctp-overlay1: #7f849c;
--ctp-base: #1e1e2e;
--ctp-mantle: #181825;
--ctp-crust: #11111b;

--accent: var(--ctp-mauve);
--success: var(--ctp-green);
--warning: var(--ctp-yellow);
--danger: var(--ctp-red);
--info: var(--ctp-sky);
```

### 10.2 Regras

- Fundo escuro, cards compactos, bordas e sombras discretas.
- Fonte monoespaçada; fallback local deve manter legibilidade sem download externo.
- Densidade alta sem reduzir alvo interativo abaixo de 40 px em telas de toque.
- Foco visível, navegação por teclado e ordem de tabulação coerente.
- Contraste mínimo WCAG AA para texto e controles.
- Cor sempre acompanhada de texto, ícone com nome acessível ou padrão adicional.
- Tabelas possuem cabeçalhos semânticos; atualização não rouba foco.
- Streams oferecem pausa visual e respeitam `prefers-reduced-motion`.
- Tooltips também são acessíveis por teclado; informação crítica não existe apenas em hover.
- Layout funcional a partir de 320 px; tabelas podem rolar horizontalmente sem ocultar ações essenciais.

A direção visual usa o inventário reproduzido em
[docs/research/dwyt.md](research/dwyt.md): densidade, hierarquia, navegação e
estados úteis podem ser reinterpretados, sem copiar identidade, assets ou regras
de negócio do projeto de referência.

## 11. Metas de experiência e performance

- A API de status responde sem esperar coleta do cluster.
- O shell local aparece antes dos blocos remotos.
- Nenhum scan de logs bloqueia a renderização do overview.
- Listas grandes usam paginação e resultado truncado explícito.
- Nenhum componente React cria watcher próprio.
- Refresh manual substitui o anterior e cancela trabalho obsoleto.
- Métricas ausentes não geram banner global de erro.
- Conteúdo Kubernetes usa `Cache-Control: no-store`.

Valores exatos de latência serão medidos no scaffold e registrados com a
evidência de performance; os budgets de carga e stream já definidos em
[security.md](security.md) são limites obrigatórios desde a implementação.

## 12. Critérios de aceite por jornada

| Jornada | Critério objetivo | IDs |
| --- | --- | --- |
| Inicialização | artefato inicia, serve frontend embutido e funciona sem Node.js | MVP-01–04 |
| Contexto | seleção substitui a geração anterior e atualiza status | MVP-05 |
| Escopo | `single`, importação em massa, deduplicação, inválidos e `all` funcionam | MVP-06–10 |
| Overview | problemas, restarts, workloads e `Warning` aparecem com fixtures controladas | MVP-11–14 |
| Logs | caminho permitido retorna dados; caminho negado não retorna conteúdo | MVP-15–17 |
| Métricas | ausência de `metrics.k8s.io` degrada somente o bloco | MVP-18 |
| Ações | UI representa capabilities e backend revalida cada operação | MVP-19–20 |
| RBAC restrito | cenário E2E funciona sem `cluster-admin` | MVP-21 |
| Persistência | inspeção do SQLite/WAL/backup não encontra credenciais | MVP-22 |
| Qualidade e release | testes, Ginger, GoReleaser, checksums e plataformas têm evidência | MVP-23–27 |

Os testes futuros e seus níveis estão detalhados em [implementation-plan.md](implementation-plan.md).

## 13. Decisões incorporadas da Fase 1

| Tema | Resultado aplicado ao produto |
| --- | --- |
| Linguagem visual | inventário DWYT reinterpretado com identidade Catppuccin e acessibilidade próprias |
| Casing | `kubePeep` preservado nos seis cross-builds; smoke nativo permanece gate de release |
| Roteamento | History API com deep links e fallback SPA que exclui API/health |
| `start`/`stop` | foreground, instância única e canal autenticado; probe F1 passou nativamente em Linux/Windows, produção repete em F3/F8 |
| Health degradado | cluster/kubeconfig não tornam a aplicação local 503, conforme ADR 0002 |
| Streams | SSE endurecido e `coder/websocket` para `exec`, conforme ADR 0003 |
| plugin `exec` | loader oficial `client-go` e sanitização de erro; nenhuma credencial adicional |

As provas futuras de UI, contraste, locks e plataformas validam a implementação
dessas decisões; não representam requisitos ainda “a decidir”.

## 14. Rastreabilidade F2

| Tarefas | Cobertura |
| --- | --- |
| F2-01 | personas, problemas, objetivos e fronteira do MVP |
| F2-02 | jornadas da seção 7 |
| F2-03 | estados da seção 8 |
| F2-04 | política de visibilidade da seção 9 |
| F2-05 | arquitetura de informação da seção 6 |
| F2-06 | tokens e acessibilidade da seção 10 |
| F2-07 | critérios das seções 12 e implementação futura |
| F2-41 | convenções de nomes e distribuição das seções 1.1–1.2 |
| F2-43 | fronteira funcional e Settings; schema completo em `data-model.md` |

## 15. Estado da revisão

- [x] Achados reproduzidos do DWYT incorporados sem copiar regras de negócio.
- [x] ADRs 0001–0004 incorporados depois dos spikes.
- [x] Critérios MVP possuem fase e caminho de evidência planejado.
- [ ] Linguagem visual e contraste passam no protótipo real.
- [ ] Caminhos planejados tornam-se testes executáveis junto de cada fatia.
