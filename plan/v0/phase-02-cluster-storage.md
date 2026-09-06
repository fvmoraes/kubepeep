# Fase 2 — Cluster e Storage

**Prioridade:** P0. **Entrada:** contrato e Nodes da F1. **Matriz:** R04–R05, R23–R27, U03–U04, U08–U09.

Cada família deve seguir o guia da F1 com API/DTO, capabilities, frontend, navegação e testes. **Leases e PVCs são namespaced**; as demais famílias desta fase são cluster-scoped.

## Fatias por família

| ID | Entrega | Campos úteis e cuidado específico |
| --- | --- | --- |
| V2-01 | Namespaces: inspeção do recurso | name/status/age, condições e metadados aprovados; preservar lista usada pelo editor de scopes e adicionar detalhe/get separado sem exigir scope |
| V2-02 | Leases (`coordination.k8s.io`) | name/namespace/holder/duração/renewTime; respeitar scope e autorização por namespace |
| V2-03 | PersistentVolumes | phase/capacity/access modes/reclaim policy/storageClass/claimRef; referências navegáveis somente quando acessíveis |
| V2-04 | PersistentVolumeClaims | namespace/phase/volume/capacity/access modes/storageClass; ausência de capacidade diferente de zero |
| V2-05 | StorageClasses (`storage.k8s.io`) | provisioner/default/reclaim policy/binding mode; omitir parâmetros livres e referências de credenciais |
| V2-06 | CSIDrivers e CSINodes (`storage.k8s.io`) | driver/attachRequired/podInfoOnMount e drivers por node; explicitar limites e campos ausentes |
| V2-07 | VolumeAttachments (`storage.k8s.io`) | node/attacher/PV/attached e condição sanitizada; omitir `attachmentMetadata`, erros crus do driver e atributos arbitrários |

- [ ] Concluir **V2-01** ponta a ponta, com link claro entre inspeção de Namespaces e gestão de scopes. O cadastro em lote da F0 deve continuar acessível quando `list/get namespaces` é negado; não colocá-lo atrás do gate de inspeção do objeto. Não duplicar CRUD de scopes nem mudar seus endpoints por acidente.
- [ ] Concluir **V2-02** com capabilities `list/get` no namespace correto; nenhum acesso automático a `kube-node-lease` sem autorização.
- [ ] Concluir **V2-03** e **V2-04**, incluindo relação PV ↔ PVC validada por namespace/nome/UID quando disponível, sem prefetch proibido.
- [ ] Concluir **V2-05**, **V2-06** e **V2-07**, mantendo VolumeAttachments habilitada quando autorizada. Revisão de campos limita conteúdo, não elimina a família.
- [ ] **V2-08 — Detalhes e YAML.** Para cada família, registrar política de campos e YAML antes de expor endpoints. Uma visualização deve recusar campos/formatos que não possam cumprir a política. Web não deve oferecer YAML que o backend recusa. VolumeAttachments tem detalhe tipado útil mesmo com YAML indisponível; nenhuma expansão de Secret refs.
- [ ] **V2-09 — UI e relações.** Habilitar itens da árvore, rotas/deep links, paleta e catálogo de referências. Status com texto/ícone/cor (Bound/Available, Pending, Released/Failed); PVC preserva scope, páginas C não têm filtro namespace. Criar links Nodes/PVs/PVCs/StorageClasses quando o destino existir e for autorizado.
- [ ] **V2-10 — Contratos e fixture.** Atualizar API/RBAC e matriz da fase; fixtures sintéticas de PV livre, PVC Pending/Bound, driver ausente e attachment com dados proibidos em campos descartados. Não criar dependência de storage real para unitários.

## Aceite

- Storage tem suas seis entradas úteis: PVs, PVCs, VolumeAttachments, StorageClasses, CSINodes e CSIDrivers. Falta legítima de CSI/objetos resulta em estado vazio, não erro global.
- Scope restrito ou ausente não impede recursos C; Leases/PVCs usam apenas namespaces permitidos. `list` e `get` negados são exercitados separadamente.
- Sem descoberta/inspeção de Namespace, o operador mantém cadastro e seleção em lote de scopes locais; permissões de Nodes/Storage não são pré-requisito das operações namespaced autorizadas.
- PV/StorageClass/Attachment com campos sentinela não vazam conteúdo proibido no JSON, YAML, log interno, preferências ou relatório versionado.
- E2E cobre PV → PVC, scope trocado, acesso negado e Namespace → gestão de scopes; contratos/testes por família cobrem os restantes. Rodar gate integrado do [plano](../README.md).

**Saída:** matriz R04–R05/R23–R27 preenchida. **Rollback:** por família, revertendo também rota/nav/capability correspondente; dados locais anteriores permanecem legíveis.
