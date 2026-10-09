import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { NavLink, useParams } from 'react-router'
import { getGatewayResources, getStatus } from '../api/client'
import type { GatewayResource } from '../api/types'
import { effectiveNamespaces, useGlobalNamespace } from '../context/GlobalNamespace'
import { gatewayCollections, isGatewayCollection } from '../navigation/gateway'
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

export function GatewayPage() {
 const status=useQuery({queryKey:['local-status'],queryFn:({signal})=>getStatus(signal),staleTime:15_000})
 const generation=status.data?.selection?.generation
 const params=useParams<{tab?:string; namespace?:string; name?:string}>()
 const tab=isGatewayCollection(params.tab??'') ? params.tab as keyof typeof gatewayCollections : 'gateways'
 const namespace=useGlobalNamespace()
 const workspace=useResourceWorkspace()
 const [search,setSearch]=useState('')
 const [applied,setApplied]=useState('')
 const cluster=tab==='gateway-classes'
 useEffect(()=>{
  if(!params.name || !generation)return
  workspace.openFromRoute({collection:tab,namespace:cluster?null:params.namespace,name:params.name})
  // eslint-disable-next-line react-hooks/exhaustive-deps -- synchronize explicit resource routes only
 },[tab,params.namespace,params.name,generation,cluster])
 const collection=useInfiniteCollection<GatewayResource>({
  identity:['resources',tab,generation,cluster?'cluster':namespace.value],
  filters:applied, enabled:Boolean(generation),
  fetchPage:(cursor,signal,prefetch,focus)=>getGatewayResources(tab,{...focus,limit:100,search:applied||undefined,continueToken:cursor||undefined,prefetch,namespaces:cluster?undefined:effectiveNamespaces(namespace.value,[])},signal,generation),
 })
 const visibility=usePreferenceColumnVisibility(tab)
 const columns:DataTableColumn<GatewayResource>[]=[
  ...(!cluster?[{key:'namespace',header:'Namespace',cell:(item:GatewayResource)=>item.namespace}]:[]),
  {key:'name',header:'Name',cell:item=><TableLink primary={item.name} aria-label={`Open ${item.kind} ${item.name}`} onClick={()=>workspace.openResource({collection:tab,namespace:item.namespace||null,name:item.name})}/>},
  {key:'status',header:'Status',cell:item=><Badge variant={item.status==='Ready'?'healthy':item.status==='Degraded'?'danger':item.status==='Pending'?'warning':'unknown'}>{item.status}</Badge>},
  ...(cluster?[{key:'controller',header:'Controller',cell:(item:GatewayResource)=>item.controllerName||'—'}]:[]),
  ...(tab==='gateways'?[{key:'class',header:'Class',cell:(item:GatewayResource)=>item.className||'—'}]:[]),
  ...(['http-routes','grpc-routes','tls-routes'].includes(tab)?[{key:'hosts',header:'Hosts',cell:(item:GatewayResource)=>(item.hosts??[]).join(', ')||'—'}]:[]),
  ...(tab==='gateways'?[{key:'addresses',header:'Addresses',cell:(item:GatewayResource)=>(item.addresses??[]).join(', ')||'—'}]:[]),
  ...(['gateways','listener-sets'].includes(tab)?[{key:'listeners',header:'Listeners',cell:(item:GatewayResource)=>item.listeners}]:[]),
  ...(tab.endsWith('-routes')?[{key:'rules',header:'Rules',cell:(item:GatewayResource)=>item.rules}]:[]),
  {key:'version',header:'API version',cell:item=>item.apiVersion},
  {key:'age',header:'Age',cell:item=>age(item.ageSeconds)},
 ]
 return <ResourcePage title="Gateway API">
  <nav aria-label="Gateway API resources" className="flex shrink-0 gap-2 overflow-x-auto border-b border-kp-overlay-0 pb-2">
   {Object.entries(gatewayCollections).map(([key,label])=><NavLink key={key} to={`/network/gateway-api/${key}`} className={({isActive})=>`control shrink-0 px-2 py-1 text-content ${isActive?'text-kp-text bg-kp-surface-2':'text-kp-overlay-text'}`}>{label}</NavLink>)}
  </nav>
  <ResourceListControls search={search} appliedSearch={applied} onSearchChange={setSearch} onApply={()=>setApplied(search)} />
  <SelectionGate pending={status.isPending} error={status.error} selected={Boolean(generation)}>
   <ResourceCollectionTable key={tab} collection={collection} columnVisibility={visibility} caption={`${gatewayCollections[tab]} resources`} columns={columns} getRowKey={item=>`${item.namespace}/${item.name}`} />
  </SelectionGate>
 </ResourcePage>
}
