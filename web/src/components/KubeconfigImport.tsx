import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { FolderOpen } from 'lucide-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { APIError, getSession, importKubeconfig, type ClusterProfile } from '../api/client'
import { Button, Input } from './ui'

export function KubeconfigImport({ onImported }: { onImported: (profile: ClusterProfile) => void }) {
  const [open, setOpen] = useState(false)
  return <>
    <Button variant="ghost" aria-label="Add kubeconfig" title="Browse or import kubeconfig" onClick={() => setOpen(true)}><FolderOpen size={15} /></Button>
    {open ? <ImportDialog onImported={onImported} onClose={() => setOpen(false)} /> : null}
  </>
}

function ImportDialog({ onImported, onClose }: { onImported: (profile: ClusterProfile) => void; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const file = useRef<HTMLInputElement>(null)
  const queryClient = useQueryClient()
  const [mode, setMode] = useState<'content' | 'path'>('content')
  const [content, setContent] = useState('')
  const [path, setPath] = useState('')
  const [fileError, setFileError] = useState('')
  const [reading, setReading] = useState(false)
  const upload = useMutation({
    mutationFn: async () => {
      const session = await getSession()
      return importKubeconfig(mode === 'path' ? { path: path.trim() } : { content }, session.csrfToken)
    },
    onSuccess: async (profile) => {
      // The next context is selected explicitly with the existing selector.
      setContent('')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['cluster-profiles'] }),
        queryClient.invalidateQueries({ queryKey: ['contexts'] }),
      ])
      onImported(profile)
      onClose()
    },
  })
  useEffect(() => {
    const node = dialog.current!
    node.showModal()
    return () => node.close()
  }, [])
  const busy = upload.isPending || reading
  return createPortal(<dialog ref={dialog} className="kubeconfig-dialog" aria-labelledby="kubeconfig-title" onCancel={(event) => { event.preventDefault(); if (!busy) onClose() }}>
    <form onSubmit={(event) => { event.preventDefault(); upload.mutate() }} className="grid gap-3">
      <h2 id="kubeconfig-title" className="m-0 text-title">Add kubeconfig</h2>
      <div className="flex flex-wrap gap-2">
        <Button variant={mode === 'content' ? 'primary' : 'secondary'} aria-pressed={mode === 'content'} disabled={busy} onClick={() => { setMode('content'); setFileError(''); upload.reset() }}>Import file or text</Button>
        <Button variant={mode === 'path' ? 'primary' : 'secondary'} aria-pressed={mode === 'path'} disabled={busy} onClick={() => { setMode('path'); setFileError(''); upload.reset() }}>Use local path</Button>
      </div>
      {mode === 'content' ? <>
        <p className="m-0 text-content text-kp-subtext">Save to ~/.kube/config in your system home directory. Existing entries are preserved; conflicting names are rejected.</p>
        <input ref={file} type="file" className="sr-only" aria-label="Kubeconfig file" tabIndex={-1} disabled={busy} onChange={async (event) => {
          const selected = event.target.files?.[0]
          if (!selected) return
          setFileError('')
          if (selected.size > 1024 * 1024) { setFileError('Choose a kubeconfig up to 1 MiB.'); event.target.value = ''; return }
          setReading(true)
          try { setContent(await selected.text()) } catch { setFileError('The file could not be read. Choose it again.'); setContent('') } finally { setReading(false); if (file.current) file.current.value = '' }
        }} />
        <Button variant="secondary" disabled={busy} onClick={() => file.current?.click()}>Browse files…</Button>
        <label className="grid gap-1 text-content">Kubeconfig YAML<textarea className="kubeconfig-input" aria-label="Kubeconfig YAML" autoComplete="off" spellCheck={false} value={content} disabled={busy} onChange={(event) => { setContent(event.target.value); setFileError('') }} /></label>
      </> : <label className="grid gap-1 text-content">Local kubeconfig path<Input aria-label="Local kubeconfig path" placeholder="~/.kube/config" value={path} disabled={busy} onChange={(event) => setPath(event.target.value)} /><span className="text-kp-subtext">Keep the file in its current location, including relative certificate references.</span></label>}
      {fileError ? <p role="alert" className="m-0 text-content text-kp-red">{fileError}</p> : null}
      {upload.isError ? <p role="alert" className="m-0 text-content text-kp-red">{upload.error instanceof APIError ? upload.error.message : 'Could not import kubeconfig. Try again.'}</p> : null}
      <div className="flex justify-end gap-2">
        <Button variant="secondary" disabled={busy} onClick={onClose}>Cancel</Button>
        <Button type="submit" disabled={busy || Boolean(fileError) || !(mode === 'content' ? content.trim() : path.trim())}>{upload.isPending ? 'Saving…' : mode === 'content' ? 'Save kubeconfig' : 'Add source'}</Button>
      </div>
    </form>
  </dialog>, document.body)
}
