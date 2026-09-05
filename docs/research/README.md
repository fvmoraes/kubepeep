# Pesquisa

Pesquisa registra fontes, método e limitações na data indicada em cada texto.
Para comportamento atual, usar os [contratos](../README.md); para execução,
usar o [plano v1](../../plan/README.md).

| Documento | Contexto |
| --- | --- |
| [Benchmark Aptakube](aptakube-ux-benchmark.md) | Referência de experiência operacional; requisitos não implicam implementação |
| [Ginger v1.4.4](ginger-v1.4.4.md) | Pesquisa histórica de bootstrap, transportes e scaffolds |
| [DWYT](dwyt.md) | Pesquisa histórica da ferramenta e padrões aproveitados; não é a configuração local do projeto |
| [Matriz F1](compatibility-matrix.md) | Compatibilidade observada no spike, sem substituir CI ou smoke de release atuais |
| [Método Ginger](evidence/ginger-v1.4.4/README.md) | Reprodução e síntese sanitizada dos diagnósticos |
| [Método do controle F1](evidence/f1-control/README.md) | Reprodução da validação nativa do lifecycle |

`evidence/` contém somente métodos e análises em Markdown. Saídas completas de
comandos e binários ficam no projeto privado sob `~/.dwyt/projects/`, ou são
regenerados em diretórios ignorados. Não adicionar transcripts, logs ou
capturas ao Git para atualizar uma pesquisa.
