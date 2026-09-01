import { APIError } from '../api/client'
import { Button } from '../components/ui'
import { PrismLight as SyntaxHighlighter } from 'react-syntax-highlighter'
import yaml from 'react-syntax-highlighter/dist/esm/languages/prism/yaml'
import vscDarkPlus from 'react-syntax-highlighter/dist/esm/styles/prism/vsc-dark-plus'

SyntaxHighlighter.registerLanguage('yaml', yaml)

function formatError(error: unknown): string {
  if (error instanceof APIError) return `${error.code}: ${error.message}`
  return error instanceof Error ? error.message : 'The local API could not load this resource.'
}

interface YamlViewerProps {
  value?: string
  pending: boolean
  error: unknown
  onLoad: () => void
}

export function YamlViewer({ value, pending, error, onLoad }: YamlViewerProps) {
  return (
    <section className="yaml-viewer" aria-label="Authorized YAML">
      <Button variant="secondary" size="compact" disabled={pending} onClick={onLoad}>
        {pending ? 'Loading YAML…' : 'Load authorized YAML'}
      </Button>
      {error ? <p className="field-error">{formatError(error)}</p> : null}
      {value !== undefined ? (
        <div
          aria-label="YAML document"
          className="yaml-document overflow-auto rounded-md border border-kp-overlay-0 bg-kp-crust p-3"
          role="region"
        >
          <SyntaxHighlighter
            language="yaml"
            PreTag="div"
            style={vscDarkPlus}
            customStyle={{ background: 'transparent', margin: 0, padding: 0 }}
          >
            {value}
          </SyntaxHighlighter>
        </div>
      ) : (
        <p>YAML is fetched only after this explicit action and remains in memory.</p>
      )}
    </section>
  )
}
