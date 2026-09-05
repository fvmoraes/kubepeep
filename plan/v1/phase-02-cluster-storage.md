# Fase 2 — Recursos Cluster e Storage

> **Objetivo:** habilitar os grupos **Cluster** (Leases) e **Storage** da navegação, todos **cluster-scoped** (PersistentVolumeClaims é namespaced), reutilizando integralmente o padrão da Fase 1.
> **Prioridade:** P0. **Dependências:** Fase 1. **Complexidade agregada:** L.

## Tarefas

### Cluster

- [ ] **V2-01** `Leases`: adapter/service/handler (`/api/v1/leases` — namespaced, mas útil em escopo All; holder, leaseDuration, renewTime), capabilities `leases.list/get`, página com colunas Name, Namespace, Holder, Renew, Age.

### Storage

- [ ] **V2-02** `PersistentVolumes`: DTO allowlisted (name, phase→StatusBadge, capacity, access modes, reclaim policy, storageClass, claimRef namespace/name, age); capabilities `persistentvolumes.list/get`.
- [ ] **V2-03** `PersistentVolumeClaims`: namespaced (name, namespace, phase, volume, capacity, access modes, storageClass, age); capabilities `persistentvolumeclaims.list/get`.
- [ ] **V2-04** `StorageClasses`: cluster-scoped (name, provisioner, reclaim policy, volumeBindingMode, default annotation, age); capabilities `storageclasses.list/get`.
- [ ] **V2-05** `CSIDrivers` e `CSINodes`: cluster-scoped (driver name, attachRequired, podInfoOnMount; node name, driver names); capabilities `csidrivers.list/get`, `csinodes.list/get`.
- [ ] **V2-06** `VolumeAttachments`: cluster-scoped (name, nodeName, attacher, attachmentMetadata limitado, attached), apenas se listável sem expor PV sensível; caso contrário manter desabilitado com nota na Fase 7.

### Frontend (comum)

- [ ] **V2-07** Páginas via resource framework + detalhes com `Facts`; YAML por recurso (exceto onde o backend já recusa); StatusBadge mapeado em `components/resource/status.ts` (Bound/Available→verde, Released/Failed→âmbar/vermelho, Pending→âmbar).
- [ ] **V2-08** Navegação: habilitar Leases, PersistentVolumes, PersistentVolumeClaims, StorageClasses, CSIDrivers, CSINodes; sem filtro de namespace nas páginas cluster-scoped (§13).
- [ ] **V2-09** Favoritos/Command center para as novas coleções.

## Critérios de aceite

- Grupo Storage 100% navegável conforme §36 da especificação de referência (exceto VolumeAttachments se V2-06 decidir recusar).
- Cluster-scoped sem namespace scope: com escopo "Finance" ativo, PVs/StorageClasses/Nodes listam normalmente; PVCs respeitam o escopo.
- RBAC: 403 por capability; cobertura reflete denied/failed.
- E2E: lista→detalhe→YAML para PV e PVC; screenshots 1280×720/1920×1080 em evidência local.

## Testes e rollback

- Mesmos níveis da Fase 1 (adapter fake, wire handler, DTO allowlist, e2e mockado).
- Rollback: revert por família; rotas e nav são aditivos.
