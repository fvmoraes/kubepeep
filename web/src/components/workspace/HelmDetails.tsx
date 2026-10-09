import { useEffect, useId, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { parseDocument } from 'yaml'
import { APIError, applyHelmRelease, getHelmDocument } from '../../api/client'
import type { HelmDocument, HelmRelease, SelectionSummary } from '../../api/types'
import type { HelmDriver } from '../../navigation/helm'
import { csrfForGeneration } from '../../actions/csrf'
import { errorMessage } from '../resource/errors'
import { Button } from '../ui'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { YamlViewer } from '../YamlViewer'

function helmError(error: unknown): string {
 if (error instanceof APIError && error.status === 409) return 'The release changed. Your draft is preserved; check the current release state before retrying.'
 return errorMessage(error)
}

export function HelmDocumentEditor({driver,namespace,name,format,selection}:{driver:HelmDriver;namespace:string;name:string;format:'values'|'manifest';selection:SelectionSummary}) {
 const client=useQueryClient()
 const id=useId()
 const controller=useRef<AbortController|null>(null)
 const [document,setDocument]=useState<HelmDocument>()
 const [draft,setDraft]=useState('')
 const [editing,setEditing]=useState(false)
 const [confirming,setConfirming]=useState(false)
 const [pending,setPending]=useState(false)
 const [error,setError]=useState<unknown>()
 const [saved,setSaved]=useState<number>()
 useEffect(()=>()=>controller.current?.abort(),[])
 async function load() {
  controller.current?.abort()
  const request=new AbortController();controller.current=request
  setPending(true);setError(undefined);setSaved(undefined)
  try {
   const value=await getHelmDocument(driver,namespace,name,format,request.signal,selection.generation)
   if(!request.signal.aborted){setDocument(value);setEditing(false);setDraft('')}
  } catch(error){if(!request.signal.aborted)setError(error)}
  finally{if(!request.signal.aborted)setPending(false)}
 }
 function review(){
  try {
   if(new TextEncoder().encode(draft).length>2*1024*1024)throw new Error('Helm values exceed the 2 MiB editor limit.')
   const parsed=parseDocument(draft,{uniqueKeys:true})
   if(parsed.errors.length)throw new Error('Invalid YAML. Check indentation and duplicate keys.')
   const values=parsed.toJS({maxAliasCount:50})
   if(!values || Array.isArray(values) || typeof values!=='object')throw new Error('Helm values must be a YAML mapping.')
   setError(undefined);setConfirming(true)
  } catch(error){setError(error)}
 }
 async function save(){
  if(!document || pending)return
  const request=new AbortController();controller.current=request
  setPending(true);setError(undefined)
  try {
   const csrf=await csrfForGeneration(selection.generation,request.signal)
   const result=await applyHelmRelease(driver,namespace,name,{confirmed:true,action:'values',expectedGeneration:selection.generation,expectedRevision:document.revision,expectedUid:document.uid,expectedResourceVersion:document.resourceVersion,values:draft},csrf,request.signal)
   if(request.signal.aborted)return
   setSaved(result.revision);setDocument(undefined);setDraft('');setEditing(false)
   void client.invalidateQueries({queryKey:['resources']})
   void client.invalidateQueries({queryKey:['workspace-detail',selection.generation]})
  }catch(error){if(!request.signal.aborted)setError(error)}
  finally{if(!request.signal.aborted){setPending(false);setConfirming(false)}}
 }
 return <section className="resource-yaml-editor gap-3" aria-label={`Helm ${format}`}>
  <p className="m-0 text-content text-kp-subtext">{format==='values'?'Stored YAML values; Base64 strings stay encoded. Saving upgrades the current chart and runs its hooks.':'Manifest stored by Helm; it may differ from live changes. Open a related resource to edit its live YAML.'}</p>
  {!document?<p className="m-0 text-content text-kp-overlay-text">Load YAML to reveal this document, which may contain sensitive values. It is kept only while this tab is open.</p>:null}
  {saved?<p role="status" className="text-content text-kp-green">Helm revision {saved} applied. Load YAML to inspect the new values.</p>:null}
  {!editing?<YamlViewer value={document?.document} pending={pending} error={error?new Error(helmError(error)):null} onLoad={()=>void load()} actions={document&&format==='values'?<Button disabled={pending} onClick={()=>{setDraft(document.document);setEditing(true);setError(undefined)}}>Edit values</Button>:undefined}/>:<>
   <div className="flex flex-wrap gap-2"><Button disabled={pending||draft===document?.document} onClick={review}>Review and apply values</Button><Button variant="secondary" disabled={pending} onClick={()=>{setEditing(false);setDraft('');setError(undefined)}}>Cancel editing</Button></div>
   <label className="text-content font-bold" htmlFor={id}>Helm values YAML · revision {document?.revision}</label>
   <textarea id={id} className="yaml-editor" spellCheck={false} autoComplete="off" autoCapitalize="off" value={draft} disabled={pending} onChange={event=>setDraft(event.target.value)}/>
   {error?<p role="alert" className="text-content text-kp-red">{helmError(error)}</p>:null}
  </>}
  <ConfirmDialog open={confirming} severity="warning" title="Apply Helm values" description={`Context ${selection.context} · namespace ${namespace}. Helm will apply the chart resources and run its upgrade hooks, creating a new revision after ${document?.revision}.`} resources={[{kind:'HelmRelease',namespace,name}]} acknowledgementLabel="I understand Helm will apply the release resources and hooks." confirmLabel="Apply values" pending={pending} onCancel={()=>{if(!pending)setConfirming(false)}} onConfirm={()=>void save()}/>
 </section>
}

export function HelmHistory({release,selection}:{release:HelmRelease;selection:SelectionSummary}){
 const client=useQueryClient()
 const controller=useRef<AbortController|null>(null)
 const [target,setTarget]=useState<{release:HelmRelease;revision:number}>()
 const [pending,setPending]=useState(false)
 const [error,setError]=useState<unknown>()
 const [saved,setSaved]=useState<number>()
 useEffect(()=>()=>controller.current?.abort(),[])
 async function rollback(){
  if(!target||pending)return
  const request=new AbortController();controller.current=request;setPending(true);setError(undefined)
  try{
   const csrf=await csrfForGeneration(selection.generation,request.signal)
   const current=target.release
   const result=await applyHelmRelease(current.driver,current.namespace,current.name,{confirmed:true,action:'rollback',expectedGeneration:selection.generation,expectedRevision:current.revision,expectedUid:current.uid,expectedResourceVersion:current.resourceVersion,revision:target.revision},csrf,request.signal)
   if(request.signal.aborted)return
   setSaved(result.revision)
   void client.invalidateQueries({queryKey:['resources']})
   void client.invalidateQueries({queryKey:['workspace-detail',selection.generation]})
  }catch(error){if(!request.signal.aborted)setError(error)}
  finally{if(!request.signal.aborted){setPending(false);setTarget(undefined)}}
 }
 return <section className="grid gap-3" aria-label="Helm release history">
  <p className="m-0 text-content text-kp-subtext">A rollback applies a saved revision as a new release revision.</p>
  {saved?<p role="status" className="text-content text-kp-green">Rollback applied as revision {saved}.</p>:null}
  {error?<p role="alert" className="text-content text-kp-red">{helmError(error)}</p>:null}
  <div className="overflow-x-auto rounded-lg border border-kp-overlay-0"><table className="w-full text-left text-content"><thead><tr className="border-b border-kp-overlay-0 text-kp-overlay-text"><th className="p-2">Revision</th><th className="p-2">Status</th><th className="p-2">Created</th><th className="p-2">Action</th></tr></thead>
   <tbody>{(release.history??[]).map(item=><tr key={item.revision} className="border-b border-kp-overlay-0"><td className="p-2">{item.revision}</td><td className="p-2">{item.status}</td><td className="p-2">{item.updatedAt}</td><td className="p-2">{item.revision<release.revision?<Button variant="secondary" disabled={pending} onClick={()=>setTarget({release,revision:item.revision})}>Rollback to revision {item.revision}</Button>:'Current'}</td></tr>)}</tbody>
  </table></div>
  <ConfirmDialog open={Boolean(target)} severity="warning" title="Rollback Helm release" description={`Context ${selection.context} · namespace ${release.namespace}. Apply revision ${target?.revision} as a new release revision. Helm will update its resources and run rollback hooks.`} resources={[{kind:'HelmRelease',namespace:release.namespace,name:release.name}]} acknowledgementLabel="I understand Helm will apply the release resources and hooks." confirmLabel="Apply rollback" pending={pending} onCancel={()=>{if(!pending)setTarget(undefined)}} onConfirm={()=>void rollback()}/>
 </section>
}
