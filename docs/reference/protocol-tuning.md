# Protocolo e tuning avançado — Fase 6 v0.7

Este documento registra as decisões condicionais da Fase 6. A coleta final
mediu o commit funcional `148419f672877a042d6745fba3bfc99df5488208` com
árvore limpa, Go 1.26.7, Linux/amd64, 12 CPUs, `GOMAXPROCS=12`, 41.716.428.800
bytes de memória e Kind/Kubernetes v1.35.0. O endpoint real continha 23 Pods;
cada variante de transporte teve duas execuções de aquecimento e 30 amostras.

O relatório JSON sanitizado é reproduzível por `make benchmark-protocol` e
fica em `test/kind/.state/performance-protocol.json`, fora do Git. Ele contém
contagens, versões, tempos e tamanhos, sem contexto, namespace, nome, UID,
selector ou payload Kubernetes.

## Resultado por otimização

| Entrega | JSON base p95 / bytes | Variante p95 / bytes | Decisão |
| --- | ---: | ---: | --- |
| PartialObjectMetadata | 7,767 ms / 142.202 | 3,841 ms / 69.584 | **Aplicado:** 50,5% menor p95 e 51,1% menos bytes nos catálogos metadata-only. ServiceAccount e ConfigMap refazem 406/415 em JSON; o cliente metadata inclui JSON na própria negociação. |
| Protobuf built-in | 7,767 ms / 142.202 | 5,504 ms / 95.261 | **Aplicado:** 29,1% menor p95 e 33,0% menos bytes. O `Accept` mantém JSON como fallback; dynamic, CRD e APIs agregadas são forçados a JSON. |
| gzip no Kind local | 7,767 ms / 142.202 | 8,070 ms / 20.746 | **Avaliado, não ativado:** reduziu 85,4% dos bytes, mas aumentou o p95 em 3,9%. |
| gzip em rede modelada | 140,091 ms / 142.202 | 41,071 ms / 12.282 | **Disponível por configuração:** a rede modelada de 30 ms/10 Mbit/s reduziu p95 em 70,7% e bytes em 91,4%. Não é apresentada como cluster remoto real; o default continua desligado. |
| QPS/Burst 5/10 | 5.602,510 ms p95; 92.996,9 ms agregados de throttle | — | **Avaliado, não ativado.** |
| QPS/Burst 10/20 | 1.803,443 ms p95; 21.010,8 ms agregados de throttle | — | **Mantido como default.** |
| QPS/Burst 20/40 | 11,803 ms p95; 0,4 ms agregados de throttle | — | **Avaliado, não ativado:** o Kind pequeno não produziu 429 em nenhuma faixa e não prova segurança sob API Server compartilhado. Os limites de configuração impedem 100/200. |
| AIMD | fixo 4: 9 respostas 429 / 231,463 ms | AIMD: 2 respostas 429 / 231,668 ms | **Aplicado:** 77,8% menos 429 sem amplificação material de tempo; começa em 4, reduz à metade até 2 e recupera `+1` após sucesso estável, no máximo 8. |
| Prefetch | sem reserva: requisição visível esperou 40,152 ms | scheduler: abaixo da resolução de 0,001 ms; 1 prefetch adiado | **Aplicado:** Pods e Events relacionados usam página de 20 itens, prazo de 3 s e prioridade baixa; trabalho visível preserva capacidade. |

Os ensaios de QPS/Burst executaram 40 LISTs por faixa e observaram latência,
tempo no rate limiter, erros e HTTP 429. Todos terminaram sem erro ou 429; por
isso o resultado rápido de 20/40 não autoriza elevar o default em um cluster
maior ou compartilhado.

O experimento AIMD usa endpoint determinístico que responde 429 acima de três
requisições concorrentes. A rede remota é modelada com o payload JSON real,
30 ms de RTT e 10 Mbit/s. Esses dois cenários são sintéticos e aparecem como
tal no relatório.

## Configuração e rollback

```yaml
resources:
  collectionTimeout: 30s
  protocol:
    partialMetadata: true
    protobuf: true
    compression: false
    qps: 10
    burst: 20
    adaptiveConcurrency: true
    intelligentPrefetch: true
```

- `partialMetadata=false` usa objetos JSON completos nos catálogos não
  sensíveis afetados; Secrets continuam obrigatoriamente no cliente metadata.
- `protobuf=false` restaura JSON para clientes built-in.
- `compression=true` deve ser usado somente em um cenário remoto medido.
- `adaptiveConcurrency=false` mantém concorrência fixa em 4.
- `intelligentPrefetch=false` remove o aquecimento relacionado; o prefetch da
  próxima página continua protegido pelo scheduler existente.
- `qps` aceita 1–20; `burst` deve ficar entre `qps` e 40. A combinação 10/20 é
  o default validado.

Secrets continuam metadata-only mesmo durante fallback: o cliente metadata
negocia JSON como segunda opção e decodifica apenas `ObjectMeta`; o cliente
tipado de Secrets nunca é usado, e conteúdo não entra em DTO, cache de página,
log ou telemetria.
Os trabalhos de prefetch carregam geração e autorização existentes, têm prazo
próprio, são cancelados no fechamento e não compartilham dados entre seleções.
Logs não são abertos especulativamente: antes de um Pod/container escolhido,
isso ampliaria leitura de conteúdo sensível. A investigação inicia logs somente
após a ação explícita do usuário.

## Reprodução

```sh
make benchmark-protocol

PROTOCOL_BENCHMARK_OUTPUT=/tmp/protocol.json make benchmark-protocol
```

O comando usa o contexto Kubernetes corrente e não cria nem remove recursos.
Compare coletas somente quando versão, dataset, máquina e forma de autorização
forem equivalentes; diferenças devem permanecer explícitas.
