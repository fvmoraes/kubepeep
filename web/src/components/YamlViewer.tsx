import { APIError, getYAMLDiff, type YAMLDiff } from '../api/client'
import { Badge, Button, Input } from '../components/ui'
import { PrismLight as SyntaxHighlighter } from 'react-syntax-highlighter'
import yaml from 'react-syntax-highlighter/dist/esm/languages/prism/yaml'
import vscDarkPlus from 'react-syntax-highlighter/dist/esm/styles/prism/vsc-dark-plus'
import { useMemo, useRef, useState } from 'react'

SyntaxHighlighter.registerLanguage('yaml', yaml)

function formatError(error: unknown): string {
  if (error instanceof APIError) return `${error.code}: ${error.message}`
  return error instanceof Error ? error.message : 'The local API could not load this resource.'
}

export interface YamlDiffTarget {
  collection: string
  namespace: string
  name: string
  generation?: string
}

interface YamlViewerProps {
  value?: string
  pending: boolean
  error: unknown
  onLoad: () => void
  diffTarget?: YamlDiffTarget
}

type DiffState =
  | { kind: 'idle' }
  | { kind: 'pending' }
  | { kind: 'ready'; diff: YAMLDiff }
  | { kind: 'error'; message: string }

export function YamlViewer({ value, pending, error, onLoad, diffTarget }: YamlViewerProps) {
  const [diffState, setDiffState] = useState<DiffState>({ kind: 'idle' })
  const [search, setSearch] = useState('')
  const [matchIndex, setMatchIndex] = useState(0)
  const [wrap, setWrap] = useState(false)
  const [copied, setCopied] = useState(false)
  const containerRef = useRef<HTMLDivElement | null>(null)

  // Local, in-memory search over the already-fetched document (V5-08): the
  // document never leaves memory and matches are counted across the whole text.
  const lines = useMemo(() => (value ?? '').split('\n'), [value])
  const matches = useMemo(() => {
    if (search === '') return [] as Array<{ line: number; index: number }>
    const needle = search.toLowerCase()
    const found: Array<{ line: number; index: number }> = []
    lines.forEach((line, lineIndex) => {
      let cursor = line.toLowerCase().indexOf(needle)
      while (cursor !== -1) {
        found.push({ line: lineIndex, index: cursor })
        if (found.length >= 2000) return
        cursor = line.toLowerCase().indexOf(needle, cursor + needle.length)
      }
    })
    return found
  }, [lines, search])

  function jumpTo(match: { line: number; index: number }) {
    const container = containerRef.current
    if (!container) return
    const target = container.querySelectorAll('[data-yaml-line]')[match.line] as HTMLElement | undefined
    target?.scrollIntoView({ block: 'center' })
  }

  function showMatch(step: number) {
    if (matches.length === 0) return
    const next = (matchIndex + step + matches.length) % matches.length
    setMatchIndex(next)
    jumpTo(matches[next])
  }

  async function copyDocument() {
    if (value === undefined) return
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    } catch {
      setCopied(false)
    }
  }

  function downloadDocument() {
    if (value === undefined) return
    const blob = new Blob([value], { type: 'application/yaml' })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = 'resource.yaml'
    anchor.click()
    URL.revokeObjectURL(url)
  }

  async function loadDiff() {
    if (!diffTarget) return
    setDiffState({ kind: 'pending' })
    try {
      const diff = await getYAMLDiff(diffTarget.collection, diffTarget.namespace, diffTarget.name, undefined, diffTarget.generation)
      setDiffState({ kind: 'ready', diff })
    } catch (cause) {
      setDiffState({ kind: 'error', message: formatError(cause) })
    }
  }

  return (
    <section className="mt-3 flex flex-col gap-2 border-t border-kp-overlay-0 pt-3" aria-label="Authorized YAML">
      <Button variant="secondary" size="sm" className="justify-self-start" disabled={pending} onClick={onLoad}>
        {pending ? 'Loading YAML…' : 'Load authorized YAML'}
      </Button>
      {error ? <p className="text-sm text-kp-red">{formatError(error)}</p> : null}
      {value !== undefined ? (
        <>
          <div className="flex flex-wrap items-center gap-2" role="search">
            <Input aria-label="Search in YAML" placeholder="Search in YAML" value={search} maxLength={128} onChange={(event) => { setSearch(event.target.value); setMatchIndex(0) }} onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); showMatch(event.shiftKey ? -1 : 1) } }} className="w-44" />
            <small className="text-xs text-kp-overlay-text" role="status">{search === '' ? '' : matches.length === 0 ? 'no matches' : `${matchIndex + 1} of ${matches.length}`}</small>
            <Button variant="secondary" size="sm" disabled={matches.length === 0} onClick={() => showMatch(-1)} aria-label="Previous match">↑</Button>
            <Button variant="secondary" size="sm" disabled={matches.length === 0} onClick={() => showMatch(1)} aria-label="Next match">↓</Button>
            <Button variant="secondary" size="sm" onClick={() => setWrap((current) => !current)} aria-pressed={wrap}>{wrap ? 'Unwrap' : 'Wrap'}</Button>
            <Button variant="secondary" size="sm" onClick={() => void copyDocument()} aria-label="Copy YAML to clipboard">{copied ? 'Copied' : 'Copy'}</Button>
            <Button variant="secondary" size="sm" onClick={downloadDocument} aria-label="Download YAML">Download</Button>
          </div>
          <div
            ref={containerRef}
            aria-label="YAML document"
            className={`mono overflow-auto rounded-md border border-kp-overlay-0 bg-kp-crust p-3 text-xs leading-relaxed ${wrap ? 'whitespace-pre-wrap' : ''}`}
            role="region"
          >
            {search === '' ? (
              <SyntaxHighlighter
                language="yaml"
                PreTag="div"
                style={vscDarkPlus}
                wrapLongLines={wrap}
                codeTagProps={{ style: { fontFamily: 'inherit' } }}
                customStyle={{ background: 'transparent', margin: 0, padding: 0, fontFamily: 'inherit' }}
              >
                {value}
              </SyntaxHighlighter>
            ) : (
              <div>
                {lines.map((line, lineIndex) => {
                  const needle = search.toLowerCase()
                  const lower = line.toLowerCase()
                  const segments: Array<{ text: string; match: boolean }> = []
                  let cursor = 0
                  let at = needle === '' ? -1 : lower.indexOf(needle)
                  while (at !== -1) {
                    if (at > cursor) segments.push({ text: line.slice(cursor, at), match: false })
                    segments.push({ text: line.slice(at, at + needle.length), match: true })
                    cursor = at + needle.length
                    at = lower.indexOf(needle, cursor)
                  }
                  if (segments.length === 0 || cursor < line.length) segments.push({ text: line.slice(cursor), match: false })
                  return (
                    <div key={lineIndex} data-yaml-line={lineIndex} className={wrap ? 'whitespace-pre-wrap' : 'whitespace-pre'}>
                      {segments.map((segment, partIndex) =>
                        segment.match ? (
                          <mark key={partIndex} className="bg-kp-accent-border text-kp-text">{segment.text}</mark>
                        ) : (
                          <span key={partIndex}>{segment.text}</span>
                        ),
                      )}
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        </>
      ) : (
        <p className="text-xs text-kp-overlay-text">YAML is fetched only after this explicit action and remains in memory.</p>
      )}
      {diffTarget ? (
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="secondary"
            size="sm"
            disabled={value === undefined || diffState.kind === 'pending'}
            onClick={() => void loadDiff()}
          >
            {diffState.kind === 'pending' ? 'Computing diff…' : 'Diff vs last-applied'}
          </Button>
          {diffState.kind === 'ready' && diffState.diff.truncated ? <Badge variant="warning">truncated</Badge> : null}
        </div>
      ) : null}
      {diffState.kind === 'error' ? <p className="text-sm text-kp-red">{diffState.message}</p> : null}
      {diffState.kind === 'ready' ? (
        diffState.diff.absent ? (
          <p role="status">No last-applied baseline was found; the resource was not applied through kubectl.</p>
        ) : (
          <div
            aria-label="YAML diff against last-applied"
            role="region"
            className="overflow-auto rounded-md border border-kp-overlay-0 bg-kp-crust p-3 font-mono text-xs leading-relaxed"
          >
            {diffState.diff.lines.map((line, index) => (
              <div
                key={`${index}-${line.kind}`}
                className={
                  line.kind === 'added'
                    ? 'text-kp-green'
                    : line.kind === 'removed'
                      ? 'text-kp-red'
                      : 'text-kp-overlay-text'
                }
              >
                {line.kind === 'added' ? '+ ' : line.kind === 'removed' ? '- ' : '  '}
                {line.text}
              </div>
            ))}
          </div>
        )
      ) : null}
    </section>
  )
}
