import { APIError, getYAMLDiff, type YAMLDiff } from '../api/client'
import { Badge, Button } from '../components/ui'
import { PrismLight as SyntaxHighlighter } from 'react-syntax-highlighter'
import yaml from 'react-syntax-highlighter/dist/esm/languages/prism/yaml'
import vscDarkPlus from 'react-syntax-highlighter/dist/esm/styles/prism/vsc-dark-plus'
import { useState } from 'react'

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
        <div
          aria-label="YAML document"
          className="mono overflow-auto rounded-md border border-kp-overlay-0 bg-kp-crust p-3 text-xs leading-relaxed"
          role="region"
        >
          <SyntaxHighlighter
            language="yaml"
            PreTag="div"
            style={vscDarkPlus}
            customStyle={{ background: 'transparent', margin: 0, padding: 0, fontFamily: 'inherit' }}
          >
            {value}
          </SyntaxHighlighter>
        </div>
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
