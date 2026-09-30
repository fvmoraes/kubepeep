import { Component, type ReactNode } from 'react'

import { Button } from './ui'

interface Props {
  name: string
  children: ReactNode
  onRetry?: () => void
}

interface State {
  error: Error | null
}

/** Keeps a rendering failure inside one panel, without losing the shell. */
export class PanelErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  private retry = () => {
    this.setState({ error: null })
    this.props.onRetry?.()
  }

  render() {
    if (!this.state.error) return this.props.children
    return (
      <div role="alert" className="grid gap-2 rounded-lg border border-kp-red-border bg-kp-red-bg/50 p-3">
        <strong className="text-sm text-kp-text">{this.props.name} could not be displayed</strong>
        <p className="m-0 text-xs text-kp-subtext">Other panels remain available. Retry this panel; if it fails again, use Refresh after checking the local service.</p>
        <Button variant="secondary" size="sm" className="justify-self-start" onClick={this.retry}>Retry {this.props.name}</Button>
      </div>
    )
  }
}
