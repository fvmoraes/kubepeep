import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { NavLink, useParams } from 'react-router'
import { getHelmReleases, getStatus } from '../api/client'
import type { HelmRelease } from '../api/types'
import { effectiveNamespaces, useGlobalNamespace } from '../context/GlobalNamespace'
import { helmCollection, type HelmDriver } from '../navigation/helm'
import { Badge, type DataTableColumn } from './ui'
import { ResourcePage } from './resource/ResourcePage'
import { ResourceCollectionTable } from './resource/ResourceCollectionTable'
import { ResourceListControls } from './ResourceListControls'
import { SelectionGate } from './resource/states'
import { useInfiniteCollection } from './resource/useInfiniteCollection'
import { usePreferenceColumnVisibility } from './resource/columns'
import { TableLink } from './resource/TableLink'
import { age } from './resource/format'
import { useResourceWorkspace } from './workspace/ResourceWorkspaceProvider'

export function HelmPage() {
 const status=useQuery({queryKey:['local-status'],queryFn:({signal})=>getStatus(signal),staleTime:15_000})
 const generation=status.data?.selection?.generation
 const params=useParams<{driver?:string;namespace?:string;name?:string}>()
 const driver:HelmDriver=params.driver==='configmaps'?'configmaps':'secrets'
 const collectionID=helmCollection(driver)
 const namespace=useGlobalNamespace()
 const workspace=useResourceWorkspace()
 const [search,setSearch]=useState('')
 const [applied,setApplied]=useState('')
 useEffect(()=>{
  if(!params.name || !params.namespace || !generation)return
  workspace.openFromRoute({collection:collectionID,namespace:params.namespace,name:params.name})
  // eslint-disable-next-line react-hooks/exhaustive-deps -- synchronize explicit resource routes only
 },[collectionID,params.namespace,params.name,generation])
 const collection=useInfiniteCollection<HelmRelease>({
  identity:['resources',collectionID,generation,namespace.value],
  filters:applied,enabled:Boolean(generation),
  fetchPage:(cursor,signal,prefetch,focus)=>getHelmReleases(driver,{...focus,limit:100,search:applied||undefined,continueToken:cursor||undefined,prefetch,namespaces:effectiveNamespaces(namespace.value,[])},signal,generation),
 })
 const visibility=usePreferenceColumnVisibility(collectionID)
 const columns:DataTableColumn<HelmRelease>[]=[
  {key:'namespace',header:'Namespace',cell:item=>item.namespace},
  {key:'name',header:'Release',cell:item=><TableLink primary={item.name} aria-label={`Open Helm release ${item.name}`} onClick={()=>workspace.openResource({collection:collectionID,namespace:item.namespace,name:item.name})}/>},
  {key:'status',header:'Status',cell:item=><Badge variant={item.status==='deployed'?'healthy':item.status==='failed'?'danger':item.status.startsWith('pending-')?'warning':'unknown'}>{item.status||'unknown'}</Badge>},
  {key:'revision',header:'Revision',cell:item=>item.revision},
  {key:'storage',header:'Storage',cell:item=>item.driver==='secrets'?'Secrets':'ConfigMaps'},
  {key:'age',header:'Age',cell:item=>age(item.ageSeconds)},
 ]
 return <ResourcePage title="Helm Releases">
  <nav aria-label="Helm storage" className="flex shrink-0 flex-wrap gap-2 border-b border-kp-overlay-0 pb-2">
   {(['secrets','configmaps'] as const).map(storage=><NavLink key={storage} to={`/helm/releases/${storage}`} className={({isActive})=>`control px-2 py-1 text-content ${isActive?'text-kp-text bg-kp-surface-2':'text-kp-overlay-text'}`}>{storage==='secrets'?'Secrets storage':'ConfigMaps storage'}</NavLink>)}
  </nav>
  <ResourceListControls search={search} appliedSearch={applied} onSearchChange={setSearch} onApply={()=>setApplied(search)}/>
  <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(generation)}>
   <ResourceCollectionTable key={collectionID} collection={collection} columnVisibility={visibility} caption="Latest Helm release revisions" columns={columns} getRowKey={item=>`${item.namespace}/${item.name}`}/>
  </SelectionGate>
 </ResourcePage>
}
