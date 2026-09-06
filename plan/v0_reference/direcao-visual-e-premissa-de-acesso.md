# Direção visual aprovada e premissa de acesso

Diretriz explícita do usuário que complementa a [especificação original](KubePeep_UI_UX_Design_System_e_Recursos_Kubernetes.md). A imagem **KubePeep.png** enviada pelo usuário é a referência de estilo desejada para a v1. O original permanece em `references/KubePeep.png` no Project Brain privado, junto da decisão e da imagem visualizável no Obsidian; caminhos da estação e dados ilustrados não são versionados.

## Estilo desejado

- Fundo escuro em azul profundo/grafite, superfícies discretamente elevadas, bordas finas, cantos suaves e contraste legível para uso prolongado.
- Sidebar compacta e hierárquica, com ícones, grupos Kubernetes recolhíveis e seleção destacada em roxo; marca KubePeep preservada.
- Barra superior organizada com origem, contexto, escopo, conexão e busca; controles de altura consistente e configuração facilmente acessível.
- Overview com cards compactos de indicadores, ícones e hierarquia clara entre título, número e informação secundária; distribuição equilibrada em grade.
- Gráficos de status e distribuição, eventos recentes e tabelas de problemas/restarts integrados ao mesmo layout. Usar barras/proporções somente com significado e cobertura definidos.
- Roxo como identidade/seleção, azul nas ações principais e verde/âmbar/vermelho nos estados; texto e ícones complementam as cores.
- Tabelas densas e legíveis, divisores sutis, alinhamento consistente e ações contextuais discretas. Inter continua a fonte principal; mono fica no conteúdo técnico.

A implementação deve seguir essa composição e linguagem visual usando os tokens e componentes existentes. Comparar shell, Overview e páginas de recursos à imagem na revisão visual da F7; diferenças necessárias por responsividade, acessibilidade ou RBAC devem ter motivo registrado. A aparência da imagem é uma meta planejada, não uma afirmação de que o redesign atual já a reproduz.

Nomes, números, tendências, timestamps, versões e permissões desenhados são ilustrativos. Não copiar esses dados para o produto ou para fixtures. Cards refletem o escopo consultado, indicam cobertura parcial e não apresentam totais globais do cluster sem evidência. Destinos futuros continuam condicionados ao [escopo v1](../v1/01-matriz-de-entregas.md).

## Premissa básica do KubePeep — acesso restrito desde o início

**O público principal inclui operadores que conhecem seus namespaces e podem acessar recursos neles, mas não podem listar namespaces nem administrar o cluster.** Essa condição é um fluxo principal do produto, não uma exceção de onboarding.

**Cadastrar namespaces em lote é essencial e obrigatório na v1.** Aqui, cadastrar significa salvar nomes em um escopo local do KubePeep; não significa criar objetos Namespace no Kubernetes. O usuário deve colar uma lista, revisar o conjunto e salvá-lo em uma operação, sem repetir o cadastro nome por nome.

- A entrada manual/em lote permanece disponível sem `list namespaces`, `get namespace`, `create namespaces` ou `cluster-admin`. Descoberta automática é uma conveniência quando autorizada.
- Validação sintática da lista é separada da verificação de acesso. Um 403, timeout ou revisão de autorização inconclusiva não prova que o namespace não existe nem deve eliminar seu nome do escopo salvo.
- Consultas e ações respeitam RBAC por namespace, recurso e verbo. Falha em um namespace ou recurso não bloqueia os demais autorizados; mostrar cobertura e motivos sem ampliar acesso.
- O seletor **All namespaces** da imagem não define o padrão de acesso. Sem permissão de descoberta, oferecer seleção explícita dos nomes cadastrados, sem fazer uma consulta global como alternativa.
- Nodes, Storage e outros recursos cluster-scoped são independentes: sua indisponibilidade não torna inútil o fluxo de Pods, workloads, eventos ou logs permitidos nos namespaces conhecidos.

A [Fase 0](../v1/phase-00-acesso-restrito-e-lote.md) transforma essa premissa em tarefas e cenários bloqueantes. O requisito transversal **U12** da matriz acompanha sua preservação até a release.
