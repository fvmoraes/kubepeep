import { expect, test } from '@playwright/test'

const generation='gen_helm'
const values='replicaCount: 2\nmessage: olá\npassword: c2VjcmV0\n'
const release={name:'checkout',namespace:'payments',driver:'secrets',status:'deployed',revision:4,storageName:'sh.helm.release.v1.checkout.v4',uid:'release-uid',resourceVersion:'25',ageSeconds:60,chart:'checkout-1.2.0',appVersion:'1.2.0',related:[{apiGroup:'',kind:'Service',namespace:'payments',name:'checkout-api'}],history:[{revision:4,status:'deployed',updatedAt:'2026-10-09T00:00:00Z'},{revision:3,status:'superseded',updatedAt:'2026-10-08T00:00:00Z'}]}
const pageMeta={limit:100,next:'',complete:true,truncated:false,filterScope:'collection'}

test.beforeEach(async({context})=>{
 await context.route('**/api/v1/**',async route=>{
  const url=new URL(route.request().url());const path=url.pathname
  let data:unknown=[];const meta={generation,page:pageMeta,coverage:null}
  if(path==='/api/v1/status')data={version:'test',selection:{clusterProfileId:1,context:'development',cluster:'dev',scopeId:1,scopeMode:'list',scopeName:'Payments',scopeSource:'saved',defaultNamespace:'payments',namespaceCount:1,generation},components:Object.fromEntries(['application','sqlite','kubeconfig','context','cluster','metrics'].map(key=>[key,{status:'healthy'}]))}
  else if(path==='/api/v1/session')data={csrfToken:'csrf_helm',generation}
  else if(path==='/api/v1/preferences')data={version:1,ui:{language:'en'},logs:{wrap:false,timestamps:true,tailLines:200},dashboard:{logScanWindow:'15m',sectionOrder:['summary'],hiddenSections:[]},filters:Object.fromEntries(['pods','workloads','events','logs'].map(name=>[name,{version:1,items:[]}]))}
  else if(path==='/api/v1/namespace-scopes/1')data={namespaces:['payments']}
  else if(path==='/api/v1/stream'){await route.fulfill({status:503,json:{code:'CLUSTER_UNAVAILABLE',message:'Mock stream unavailable.'}});return}
  else if(path.startsWith('/api/v1/helm/releases/')){
   const parts=path.split('/')
   const driver=parts[5]
   if(parts.length===6)data=[{...release,driver}]
   else if(parts.length===8)data={...release,driver}
   else if(parts.length===9)data={document:parts[8]==='values'?values:'apiVersion: v1\nkind: Service\nmetadata:\n  name: checkout-api\n',format:parts[8],revision:4,uid:release.uid,resourceVersion:release.resourceVersion}
  }else if(path==='/api/v1/services')data=[{namespace:'payments',name:'checkout-api',type:'ClusterIP',clusterIPs:['10.0.0.1'],ports:[],ageSeconds:10}]
  else if(path==='/api/v1/services/payments/checkout-api')data={metadata:{name:'checkout-api',namespace:'payments'},summary:{name:'checkout-api',namespace:'payments',type:'ClusterIP',clusterIPs:['10.0.0.1'],ports:[]}}
  await route.fulfill({json:{data,meta}})
 })
})

for(const driver of ['secrets','configmaps']){
 test(`Helm ${driver}: metadata list, fixed details, related resource and no bulk delete`,async({page})=>{
  const documents:string[]=[]
  page.on('request',request=>{if(/\/(values|manifest)$/.test(new URL(request.url()).pathname))documents.push(request.url())})
  await page.goto(`/helm/releases/${driver}`)
  await expect(page.getByRole('heading',{name:'Helm Releases'})).toBeVisible()
  await expect(page.getByRole('combobox',{name:'Global namespace'})).toHaveValue('payments')
  await page.getByRole('checkbox',{name:'Select all loaded rows'}).check()
  await expect(page.getByRole('button',{name:'Delete selected'})).toHaveCount(0)
  const initial=await page.locator('.resource-list-pane').boundingBox()
  await page.getByRole('button',{name:'Open Helm release checkout'}).click()
  await expect(page.getByRole('region',{name:'HelmRelease checkout'})).toBeVisible()
  await expect(page.getByText('checkout-1.2.0',{exact:true})).toBeVisible()
  const detail=await page.locator('.resource-detail-slot').boundingBox()
  expect(detail!.height/page.viewportSize()!.height).toBeCloseTo(.7,2)
  expect((await page.locator('.resource-list-pane').boundingBox())!.height).toBeCloseTo(initial!.height,0)
  expect(documents).toHaveLength(0)
  await page.getByRole('button',{name:'Open Service checkout-api'}).click()
  await expect(page).toHaveURL(/network\/services\/payments\/checkout-api\?focus=checkout-api&namespace=payments/)
  await expect(page.getByText('Exact name: checkout-api')).toBeVisible()
  await page.getByRole('button',{name:'Close resource workspace'}).click()
  await expect(page.locator('.resource-detail-slot')).not.toBeVisible()
  await expect.poll(async()=>{
   const list=(await page.locator('.resource-collection').boundingBox())!
   const pane=(await page.locator('.resource-list-pane').boundingBox())!
   return Math.abs(list.y+list.height-pane.y-pane.height)
  }).toBeLessThan(1)
 })
}

test('Helm values remain encoded and require confirmation; conflicts preserve the draft',async({page},testInfo)=>{
 const requests:Record<string,unknown>[]=[]
 let conflict=true
 await page.route('**/api/v1/helm/releases/secrets/payments/checkout',async route=>{
  if(route.request().method()!=='POST'){await route.fallback();return}
  expect(route.request().headers()['x-kubepeep-csrf']).toBe('csrf_helm')
  requests.push(route.request().postDataJSON())
  await route.fulfill(conflict?{status:409,json:{code:'CONFLICT',message:'Changed release.'}}:{json:{data:{accepted:true,revision:5,status:'deployed'}}})
 })
 await page.goto('/helm/releases/secrets/payments/checkout')
 await page.getByRole('tab',{name:'Values',exact:true}).click()
 await page.getByRole('button',{name:'Load authorized YAML',exact:true}).click()
 await page.getByRole('button',{name:'Edit values',exact:true}).click()
 const editor=page.getByRole('textbox',{name:'Helm values YAML · revision 4'})
 await expect(editor).toHaveValue(values)
 const edited=values.replace('replicaCount: 2','replicaCount: 3')+'# editor scroll fixture\n'.repeat(100)
 await editor.fill(edited)
 for(const viewport of [{width:1440,height:900},{width:390,height:844}]){
  await page.setViewportSize(viewport)
  await expect(editor).toBeInViewport({ratio:.99})
  await expect(page.getByRole('button',{name:'Review and apply values'})).toBeInViewport()
 }
 await page.screenshot({path:testInfo.outputPath('helm-values-mobile.png')})
 await page.getByRole('button',{name:'Review and apply values'}).click()
 expect(requests).toHaveLength(0)
 const confirm=page.getByRole('alertdialog',{name:'Apply Helm values'})
 await confirm.getByRole('checkbox').check()
 await confirm.getByRole('button',{name:'Apply values',exact:true}).click()
 await expect(page.getByRole('alert')).toContainText('draft is preserved')
 await expect(editor).toHaveValue(edited)
 expect(requests[0]).toEqual({confirmed:true,action:'values',expectedGeneration:generation,expectedRevision:4,expectedUid:release.uid,expectedResourceVersion:release.resourceVersion,values:edited})
 conflict=false
 await page.getByRole('button',{name:'Review and apply values'}).click()
 await confirm.getByRole('checkbox').check()
 await confirm.getByRole('button',{name:'Apply values',exact:true}).click()
 await expect(page.getByRole('status').filter({hasText:'Helm revision 5 applied'})).toBeVisible()
 await expect(page.getByRole('textbox',{name:/Helm values YAML/})).toHaveCount(0)
})

test('Helm manifest is explicit and leaving the tab clears the document; history rollback binds the current revision',async({page})=>{
 const documents:string[]=[]
 const writes:Record<string,unknown>[]=[]
 page.on('request',request=>{if(/\/(values|manifest)$/.test(new URL(request.url()).pathname))documents.push(request.url())})
 await page.route('**/api/v1/helm/releases/secrets/payments/checkout',async route=>{
  if(route.request().method()!=='POST'){await route.fallback();return}
  writes.push(route.request().postDataJSON())
  await route.fulfill({json:{data:{accepted:true,revision:5,status:'deployed'}}})
 })
 await page.goto('/helm/releases/secrets/payments/checkout')
 await page.getByRole('tab',{name:'Manifest',exact:true}).click()
 expect(documents).toHaveLength(0)
 await page.getByRole('button',{name:'Load authorized YAML',exact:true}).click()
 await expect(page.getByRole('button',{name:'Edit values'})).toHaveCount(0)
 await expect(page.getByText('kind: Service',{exact:false})).toBeVisible()
 await page.getByRole('tab',{name:'Overview',exact:true}).click()
 await page.getByRole('tab',{name:'Manifest',exact:true}).click()
 await expect(page.getByText('Load YAML to reveal this document',{exact:false})).toBeVisible()
 await page.getByRole('tab',{name:'History',exact:true}).click()
 await page.getByRole('button',{name:'Rollback to revision 3'}).click()
 const confirm=page.getByRole('alertdialog',{name:'Rollback Helm release'})
 await confirm.getByRole('checkbox').check()
 await confirm.getByRole('button',{name:'Apply rollback'}).click()
 await expect(page.getByRole('status').filter({hasText:'Rollback applied as revision 5'})).toBeVisible()
 expect(writes).toEqual([{confirmed:true,action:'rollback',expectedGeneration:generation,expectedRevision:4,expectedUid:release.uid,expectedResourceVersion:release.resourceVersion,revision:3}])
})
