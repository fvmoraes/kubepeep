# Inventários, detalhes e visões personalizadas

Este guia descreve o comportamento implementado na linha de desenvolvimento
0.12.0. A disponibilidade de dados e ações depende da identidade Kubernetes
selecionada. Consulte também [produto](../reference/product-spec.md),
[RBAC](../reference/rbac-requirements.md) e [API](../reference/api.md).

## Origem, contexto e namespace

Use **Add kubeconfig** no cabeçalho para selecionar um arquivo, colar seu YAML
ou informar um caminho local. Arquivo/texto é mesclado no `.kube/config` do
diretório pessoal do sistema; caminho local mantém a fonte original. Conflitos
preservam o arquivo existente e o texto informado. Para arquivos com caminhos
relativos de certificados/chaves, prefira registrar o caminho original.
Após importar, selecione explicitamente o contexto e o escopo.

O seletor de cor oferece **Blue**, **Yellow**, **Red** e **Green**, além de cor
personalizada ou remoção. A cor fica salva por profile/contexto e aparece nas
divisórias do menu e do cabeçalho, incluindo abaixo do nome e versão. Ela não
altera as cores de saúde dos recursos nem consulta o cluster ao trocar a cor.

O seletor **Namespace scope** no cabeçalho lista os escopos salvos do perfil e
contexto ativos e permite alternar diretamente entre eles. Em **Namespaces**,
criar ou editar um escopo limpa os campos após salvar com sucesso; erros
preservam o formulário para correção.

Ao abrir ou trocar contexto/escopo, o filtro começa no namespace padrão. A
opção **All** é explícita: ela inclui os demais namespaces autorizados,
começando pelo padrão. Scopes antigos sem padrão usam `default`, se presente,
ou o primeiro namespace disponível. A aplicação aguarda essa resolução antes
de consultar o inventário.

## Lista e detalhes

A lista ocupa toda a área útil abaixo dos filtros. Clicar em um objeto abre
seus detalhes sobre a lista, de baixo para cima, com altura fixa de **70% da
janela**. Não há ajuste de tamanho; fechar remove o painel e deixa toda a lista
acessível. Lista e conteúdo da aba têm rolagens independentes.

Um link para objeto associado muda para a família e o namespace desse objeto,
filtra seu nome exato e abre seus detalhes. **Clear object filter** remove o
filtro de identidade. Voltar/avançar no painel percorre as inspeções anteriores.

As abas comuns mantêm a ordem **Overview → Logs → YAML → Metrics → Containers
→ Events → Data / Env**. Cada objeto mostra somente as que suporta. As abas
específicas aparecem depois em ordem alfabética, com **Actions** por último.
Por exemplo, Helm mostra **Overview → History → Manifest → Values**.

## Colunas e seleção

No primeiro uso em telas largas, Pods mostram:

**Namespace → Pod → Status → Ready → Restarts → CPU → Memory → Type → Age.**

As outras famílias seguem a mesma sequência com os campos equivalentes e um
conjunto reduzido de informações relevantes. As demais colunas aparecem depois
das padrão, em ordem alfabética pelo nome exibido, inclusive as de Custom Resources.
Em áreas menores, o conjunto
inicial usa até cinco colunas abaixo de 920 px e três abaixo de 600 px.

**Choose visible columns** permite mostrar/ocultar colunas e movê-las para a
esquerda/direita; também é possível arrastar cabeçalhos. Ordem e visibilidade
ficam salvas por tipo de objeto no banco local, sem expiração, inclusive após
fechar o aplicativo. Preferências existentes prevalecem sobre o padrão inicial
e podem produzir rolagem horizontal. Filtros e ordenação do menu de cada coluna
operam sobre as linhas carregadas.

**Containers** é uma coluna opcional de Pods: exibe quantidade e um indicador
por container, incluindo init/ephemeral. Verde indica running; amarelo,
inicialização; vermelho, problema; cinza, concluído/inativo. Readiness inicial
ou startup em andamento pode aparecer como **Starting**, sem esconder falhas
reais como CrashLoopBackOff ou erro de imagem.

A seleção geral seleciona as linhas carregadas que passam pelos filtros da
tabela. Pods oferecem exclusão em massa e reinício quando cada alvo possui
permissão e controlador compatível. Reinício remove o Pod para seu controlador
recriá-lo. Outras famílias não oferecem exclusão em massa; ações compatíveis
continuam disponíveis. Cada alvo é revalidado no backend.

## Histórico de Jobs na lista de Pods

Pods mostra a execução mais recente de cada CronJob, seja sucesso, falha ou
execução em andamento. Tentativas e Pods paralelos da mesma execução continuam
visíveis. Jobs independentes permanecem na lista porque não têm um CronJob para
agrupar. Jobs conserva todas as execuções ainda existentes no cluster; CronJobs
mostra os agendamentos. A aplicação não altera políticas de retenção Kubernetes.
Navegar diretamente para um Pod antigo ou pelos detalhes/logs de um Job continua
funcionando. Sem autorização/histórico completo de Jobs, os Pods são preservados
e a lista informa que pode incluir execuções antigas.

## Port forwarding em Actions

Pods e Services mostram seus encaminhamentos em **Actions**. Escolha a porta
remota e, opcionalmente, a porta local (1.024–65.535). A porta local efetivamente
atribuída é salva mesmo quando escolhida automaticamente. **Open in browser**
abre o endereço loopback no navegador, usando o padrão do sistema no desktop;
protocolos que não são HTTP continuam disponíveis pelo endereço informado.

Cada objeto tem no máximo uma conexão ativa. **Start port-forward** substitui a
anterior após validar o pedido e a autorização. **Stop** encerra a conexão e
preserva as portas; **Delete** encerra e remove a configuração salva. Ao fechar a
aplicação ou trocar a geração, as conexões são encerradas. Ao reabrir, a receita
aparece como **Inactive** e **Re-enable** reconecta nas mesmas portas, passando
novamente pelas verificações de contexto, escopo e RBAC. Porta ocupada resulta
em erro, sem escolher silenciosamente outra porta. Se a nova conexão falhar
após o encerramento da anterior, a configuração permanece disponível para retry.

As receitas são locais, isoladas por profile/contexto/namespace/kind/nome, sem
credenciais ou sessões persistidas. As receitas inativas também aparecem em
**Network → Port Forwarding**. Nenhuma conexão é reaberta automaticamente.

## Atualização e desempenho

**Auto · 5s** começa ligado e controla a atualização de inventários e métricas.
O intervalo é fixo; o botão liga/desliga, sem escolher outro valor. Logs possuem
controles próprios. As cinco últimas consultas de inventário ficam em memória,
com atualização sequencial em segundo plano quando Auto está ligado e a
aplicação está visível. Mudar geração/contexto invalida os dados anteriores.
Cada consulta retém até cinco páginas; a lista virtualizada limita o DOM.

A troca de contexto publica a seleção confirmada pelo servidor imediatamente,
sem esperar as consultas auxiliares de saúde, perfis e contextos. Resultados
atrasados de uma geração anterior não substituem uma seleção mais recente.

Falhas transitórias têm tentativas limitadas e nova revalidação automática.
**FORBIDDEN** identifica negação explícita; falha de revisão de autorização,
autenticação, rede e timeout têm causas distintas. A cobertura parcial informa
quais namespaces concluíram, falharam ou foram negados. Retry não concede RBAC.
**Performance**, no rodapé, leva aos diagnósticos em Settings para avaliar
latência, volume de consultas, cache e renderização. Barras de CPU/memória e
HPA distinguem valor medido de informação indisponível; ausência não vira zero.

## YAML, Secrets e ConfigMaps

Nas telas nativas, abra **YAML → Load authorized YAML → Edit YAML**. A leitura
exige get e a gravação exige update no objeto exato, além de confirmação.
Identidade, UID e resourceVersion impedem gravar sobre outro alvo ou versão.
Conflitos/validação preservam o rascunho; copie as mudanças antes de carregar
novamente. Não há force update ou repetição automática de escrita.

A edição mantém as cores de sintaxe, seleção e desfazer/refazer. O editor
renderiza as linhas visíveis de documentos extensos e salva o documento
completo, preservando seu conteúdo. Campos imutáveis de Pods continuam sujeitos
às regras do Kubernetes; para CPU/memória de Pods gerenciados, edite o template
do workload controlador quando a alteração direta não for permitida.

Secrets e ConfigMaps usam esse mesmo editor. Data identifica `data` UTF-8 e
campos binários/Base64 conforme o recurso; Secret `data` continua Base64.
Não há decodificação automática que transforme silenciosamente o valor a
editar. Secret Data e YAML exigem leitura explícita e não entram em caches,
preferências ou logs. O documento fica somente no estado da aba aberta.

## Helm e Gateway API

**Helm Releases** permite selecionar armazenamento Secrets ou ConfigMaps,
inspecionar revisão, chart, histórico e recursos associados, e carregar values
ou manifest explicitamente. É possível atualizar values com o chart atual ou
voltar a uma revisão anterior, com confirmação e verificação da revisão atual.
O manifesto é o armazenado pelo Helm; os links abrem os objetos vivos.
Uma operação pode ter efeitos parciais se falhar: revise o estado antes de
repeti-la. Armazenamento SQL e instalação de novos charts não fazem parte desse
fluxo.

**Gateway API** inclui GatewayClass, Gateway, HTTPRoute, GRPCRoute, TCPRoute,
TLSRoute, UDPRoute, ReferenceGrant, BackendTLSPolicy e ListenerSet. Depende das
APIs instaladas e de RBAC; ausência de CRD é diferenciada de negação. Listas,
detalhes, relacionamentos e editor seguem o padrão das outras famílias.

## Custom Resource Views

No menu próprio **Custom Resources**, entre **Configuration** e **Storage**,
use **Add Custom Resource**, sempre a primeira opção. As visões salvas aparecem
abaixo dela e pertencem ao perfil/contexto/cluster ativo.
Pesquise APIs do contexto ativo por nome, Kind, short name ou API group. Fixar
um recurso adiciona uma visão local, sem criar nada no cluster. O menu de cada
visão permite reordenar/remover; até 32 visões ficam salvas por
profile/contexto/cluster. **Refresh catalog** renova a descoberta.

A tabela usa Namespace/Name/Age e as colunas Table fornecidas pelo Kubernetes,
incluindo `additionalPrinterColumns` de CRDs. Na ausência de Table, usa
metadata. Paginação, namespace, Auto, cache recente e detalhes em 70% são
compartilhados com os inventários nativos. Catálogos parciais e limites são
informados; ausência em resultado parcial não prova inexistência.

O YAML dessas visões é **somente leitura**, carregado explicitamente. Ações e
edição continuam nas telas nativas suportadas. Remover uma visão não apaga
objetos Kubernetes.

## Events

O objeto referenciado em cada evento abre os detalhes correspondentes. Recursos
customizados são resolvidos pelo catálogo do contexto, mesmo sem uma visão
salva. Referências indisponíveis ou ambíguas não abrem um objeto diferente.

**Acknowledge** oculta uma ocorrência; selecionar linhas e usar **Acknowledge
selected** faz o mesmo em lote. O reconhecimento vale para a sessão, não remove
Events do Kubernetes e não altera o objeto. Nova contagem ou horário faz a
ocorrência reaparecer. **Show acknowledged** restaura as linhas ocultas.

## Interface e novas versões

**Settings → Interface** mantém somente **English** disponível. Português
(Brasil), espanhol, italiano, francês e alemão aparecem desabilitados com
**(future)**; não há tradução parcial ativada.

Enquanto a aplicação estiver aberta, uma consulta em segundo plano verifica
GitHub Releases a cada duas horas. Uma release estável com versão maior que a
instalada mostra um aviso discreto no rodapé direito, com link para
[download](https://kubepeep.online/download). Falhas de rede não interrompem o
uso. A verificação não instala a versão automaticamente.
