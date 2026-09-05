# Benchmark de experiência operacional — Aptakube

**Data da revisão:** 2026-08-25

**Escopo:** facilitadores de uso documentados publicamente pelo Aptakube que
podem orientar requisitos próprios do Kube Peep.

**Resultado:** referência funcional aprovada com limites explícitos de
identidade, licença, segurança e arquitetura.

## 1. Método e limites

Esta análise usa apenas materiais oficiais mantidos pelo Aptakube. Ela não
autoriza copiar código, marca, textos, screenshots, ícones, composição visual,
trade dress ou identidade proprietária. O Kube Peep mantém design system,
arquitetura, contratos, linguagem e implementação próprios.

As fontes descrevem comportamentos de produto; não são evidência de que uma
implementação específica seja adequada aos limites de RBAC, retenção e
privacidade do Kube Peep. Quando há conflito, prevalecem o threat model, os
ADRs, o princípio metadata-only de Secret e a autoridade final do API Server.

## 2. Fontes oficiais consultadas

| Fonte | Conteúdo usado como benchmark |
| --- | --- |
| [Repositório oficial](https://github.com/aptakube/aptakube) | visão geral, operação em múltiplos clusters, logs agregados, diff, seleção de namespaces, visualização humana e operação local |
| [Site oficial](https://aptakube.com/) | visão de saúde, métricas opcionais, port-forward, ações rápidas, YAML e fluxo sem configuração adicional no cluster |
| [Multi-Cluster](https://aptakube.com/multi-cluster) | agregação entre contextos sem exigir interconexão entre clusters |
| [Aggregated Logs](https://aptakube.com/aggregated-logs) | seleção de fontes, live/terminated, filtro, destaque, wrap, cópia e download |
| [Metrics](https://aptakube.com/metrics) | uso da Metrics API, CPU/memória, requests, limits e capacidade |
| [YAML Editor](https://aptakube.com/yaml-editor) | syntax highlighting, busca, seções recolhíveis e edição condicionada a RBAC |
| [Quick Actions](https://aptakube.com/quick-actions) | ações contextuais em listas e detalhes |
| [Resource View](https://aptakube.com/resource-view) | colunas por tipo, detalhes humanos, relações, filtros e status |
| [Security](https://aptakube.com/docs/security) | conexão direta, dados locais, kubeconfig e RBAC como fronteiras de confiança |
| [Command palette](https://aptakube.com/blog/aptakube-1.4) | `Ctrl/Cmd+K`, busca e navegação rápida |
| [Keyboard/navigation](https://aptakube.com/blog/aptakube-1.5) | navegação por teclado, seletor de contexto e fluxo de port-forward |
| [Changelog](https://aptakube.com/changelog) | favoritos, filtros persistentes, ordenação natural, colunas, estado de conexão e refinamentos de navegação |

## 3. Facilitadores adotados como requisitos próprios

| Capacidade | Benefício esperado no Kube Peep | Adaptação obrigatória |
| --- | --- | --- |
| Entrada sem configuração adicional | aproveitar kubeconfig já funcional | kubeconfig somente leitura; nenhum componente obrigatório no cluster |
| Contextos e namespaces acessíveis | reduzir alternância repetitiva | toda origem permanece explícita e cada contexto mantém autorização isolada |
| Estado de conexão | distinguir vazio, offline, proibido, parcial e obsoleto | erro sempre ligado à origem; retry com cancelamento/backoff |
| Visão humana e orientada a problemas | reduzir leitura manual de YAML | não inventar diagnóstico; cada resumo liga à evidência Kubernetes |
| Listas consistentes | busca, filtro, ordenação natural e colunas úteis | paginação honesta, alta cardinalidade e nenhum campo sensível configurável |
| Paleta e teclado | acesso rápido sem memorizar rotas | foco contido, leitor de tela, ajuda de atalhos e nenhuma mutação pela paleta inicial |
| Favoritos e recentes | reduzir navegação repetida | somente identificadores allowlisted; nunca endpoint, token, path de kubeconfig ou corpo do objeto |
| Ações rápidas | aproximar ação do alvo | capability na UI é informativa; backend repete SAR e confirmação proporcional ao risco |
| Logs agregados | correlacionar réplicas/containers | origem em cada linha, budgets, backpressure, cancelamento e zero persistência automática |
| Métricas opcionais | mostrar pressão e dimensionamento | ausência/proibição degrada somente o bloco afetado |
| YAML legível | investigar campos avançados | somente leitura no primeiro gate; Secret mascarado/metadata-only |
| Diff | comparar origens ou revisões | origens explícitas, normalização opt-in e proibição absoluta para Secret |
| Port-forward | reduzir parâmetros manuais | bind loopback, sessão com dono, limite e encerramento individual/coletivo |
| Edição YAML futura | correção controlada | dry-run server-side, diff, `resourceVersion`, RBAC, confirmação e recusa de Secret |

## 4. Desvios deliberados de segurança

O Kube Peep **não** reproduz automaticamente expansão de referências de Secret
descrita na página oficial de Resource View. Também não oferece rota YAML,
diff, coluna configurável, favorito, pesquisa, cópia ou download que revele
`Secret.data` ou `Secret.stringData`.

Regras permanentes para toda facilidade nova:

- kubeconfig e plugins `exec` permanecem responsabilidade do ambiente e nunca
  são copiados ou modificados;
- token, certificado, chave, header de autorização, endpoint privado, path
  local, conteúdo de Secret, log, YAML ou comando de `exec` não entram em
  SQLite, storage do browser, telemetria ou diagnóstico;
- dados remotos usam `Cache-Control: no-store` e estado somente em memória;
- preferências persistentes passam por allowlist, versão e limite de tamanho;
- resultados multi-contexto identificam `clusterProfileId`, contexto,
  namespace, recurso e geração sem misturar permissões;
- uma permissão concedida em um contexto nunca habilita ação em outro;
- toda mutação é reautorizada no backend imediatamente antes do acesso real;
- filtros, atalhos e affordances não são fronteira de autorização;
- exportação de logs/YAML é explícita, efêmera e nunca cria cópia interna;
- falhas publicadas usam códigos estáveis e campos allowlisted, não mensagens,
  bodies, stack traces ou paths brutos.

## 5. Escopo de entrega

### 5.1 Núcleo da experiência operacional

O primeiro gate estende a arquitetura de contexto ativo já existente:

1. paleta somente de navegação com `Ctrl/Cmd+K`;
2. ajuda de atalhos e navegação completa por teclado;
3. menu de ações rápidas baseado nas capabilities existentes;
4. filtros explícitos, clear/reset e ordenação natural;
5. favoritos/recentes com persistência allowlisted;
6. estados de conexão/retry/stale associados ao contexto;
7. visualizador YAML com busca e recolhimento, mantendo Secret metadata-only;
8. leitor de logs com fontes, busca, pause/follow, wrap e encerramento seguro;
9. gerenciador de port-forwards com parar individual e parar todos.

### 5.2 Agregação somente leitura

O segundo gate permite selecionar mais de um contexto para consultas, nunca
para mutação em massa. Cada linha/erro mantém sua origem, os requests são
canceláveis e limitados, e a falha de um contexto não derruba os demais.

### 5.3 Produtividade avançada

Diff, colunas configuráveis, filtros compostos, logs multi-pod e visualização
genérica de CRDs só entram após os gates de privacidade, RBAC, alta
cardinalidade e falha parcial. Edição/criação de YAML permanece uma capacidade
separada e mais restrita; não é consequência automática de visualizar YAML.

## 6. Evidência obrigatória

Cada requisito da matriz UX (histórico Git, `plan/matriz-aceite-ux.md` no
commit `5ac7320^`) precisa registrar:

- fonte oficial e decisão de adaptação;
- superfície de UI e contrato de API usado;
- recurso/subrecurso/verbo Kubernetes;
- perfis RBAC permitido, negado e desconhecido;
- classificação, retenção, exportação e redaction de dados;
- loading, vazio, offline, proibido, parcial, cancelado, stale e truncado;
- teste unitário, integração, frontend e Kind/E2E conforme aplicável;
- inspeção negativa de browser storage, SQLite, arquivos e logs;
- acessibilidade por teclado e sem dependência exclusiva de cor;
- evidência versionada e run de CI do commit que realmente contém a mudança.

## 7. Conclusão

O benchmark é aceito como fonte de requisitos de conveniência, não como
blueprint visual ou autorização para ampliar privilégios. O ganho pretendido é
uma operação diária mais rápida e compreensível, sem transformar o Kube Peep em
uma console administrativa, persistir dados do cluster ou reduzir os controles
de RBAC e Secret já aprovados.
