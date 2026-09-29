# Fase 6 — Protocolo e tuning avançado (condicional a benchmark)

**Prioridade:** P2 — cada item só ativa com ganho comprovado na matriz da F0. **Entrada:** F0 (métricas/baseline) e F2. **Matriz:** D30–D34, T02.

Otimizações de protocolo e adaptação dinâmica. Nada aqui altera defaults sem evidência; tudo atrás de flag/configuração, com fallback garantido. Cada item encerra como aplicado ou avaliado/não ativado com comparação medida; ausência de medição permanece pendente e bloqueia o fechamento (C05).

Contratos transversais de execução e aceite: [C01–C06](05-contratos-e-aceite.md).

## Tarefas

- [x] **F6-01 — PartialObjectMetadata.** Para catálogos, autocomplete, relações, seleção e lookup (Accept `application/json;as=PartialObjectMetadataList;g=meta.k8s.io;v=v1`), quando spec/status não são necessários. Tabelas que exibem status/replicas/ports continuam com objeto completo. Fallback JSON sempre. **Aplicado:** p95 -50,5%, bytes -51,1%; fallback JSON de negociação e fallback explícito 406/415 para catálogos não sensíveis validados. Secrets nunca usam cliente tipado.
- [x] **F6-02 — Protobuf para built-ins.** `application/vnd.kubernetes.protobuf` com fallback JSON para APIs built-in suportadas; CRDs e aggregated APIs permanecem JSON. **Aplicado:** p95 -29,1%, bytes -33,0%; famílias dynamic permanecem JSON.
- [x] **F6-03 — Compressão HTTP.** Medir `APIResponseCompression`/gzip por cenário: localhost (CPU pode custar mais que rede) × cluster remoto (tende a compensar). Ativar apenas onde medido como ganho. **Avaliado, não ativado no default:** p95 local +3,9%; rede modelada p95 -70,7%. Flag preservada para ambiente remoto medido.
- [x] **F6-04 — QPS/Burst do client-go.** Medir 5/10 → 10/20 → 20/40 observando 429, latência e throttle client-side; ajustar somente com evidência do benchmark. Nunca 100/200 "para ficar mais rápido". **Avaliado, default mantido em 10/20:** o Kind pequeno não gerou 429; limites rejeitam QPS >20 ou burst >40.
- [x] **F6-05 — Concorrência adaptativa (AIMD).** Success estável → +1; 429 ou burst de timeout → ÷2; min 2 / default 4 / max 8, atrás de flag; default só muda com benchmark comprovando benefício. **Aplicado:** 429 caiu de 9 para 2 (-77,8%) sem aumento de duração.
- [x] **F6-06 — Prefetch inteligente.** Prioridades no scheduler da F2-09: visible (100), likely next (20), unrelated (0) — ex.: vendo Deployments, prefetch leve de Pods/Events/Logs. Prefetch nunca compete com a requisição visível. **Aplicado:** Pods/Events relacionados limitados a 20 e 3 s; espera visível caiu de 40,152 ms para abaixo de 0,001 ms no ensaio.

## Cenários obrigatórios de aceite

| Cenário | Resultado exigido |
| --- | --- |
| cada otimização avaliada | comparativo antes/depois obrigatório; ganho validado permite aplicação, sem ganho encerra como avaliado/não ativado; sem medição permanece pendente (C05) |
| CRD/aggregated API | continua JSON; sem quebra de leitura |
| 429 no experimento AIMD | medir redução e recuperação gradual sem amplificação; manter desativado se a comparação não demonstrar ganho seguro (C05) |
| cluster antigo/sem suporte | fallback JSON/LIST+WATCH preserva funcionamento |

**Saída:** protocolo mais enxuto onde medido; nenhum default alterado sem evidência. **Rollback:** tudo atrás de flag/configuração; desligar restaura o comportamento da F2.

## Fechamento

**Status: FINALIZADA no commit funcional `148419f`.** A coleta completa, as
condições, os limites e o rollback estão em
[`docs/protocol-tuning.md`](../../docs/protocol-tuning.md). O comando
`make benchmark-protocol` reproduz a matriz sem criar ou remover recursos.
