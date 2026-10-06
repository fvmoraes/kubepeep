import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from 'react'

import '@xterm/xterm/css/xterm.css'

export interface ExecTerminalHandle {
  writeStdout: (text: string) => void
  writeStderr: (text: string) => void
  clear: () => void
}

export interface ExecTerminalProps {
  onStdin?: (text: string) => void
  onResize?: (columns: number, rows: number) => void
  label?: string
}

function cssToken(styles: CSSStyleDeclaration, name: string): string {
  return styles.getPropertyValue(name).trim()
}

function terminalTheme(styles: CSSStyleDeclaration) {
  return {
    background: cssToken(styles, '--color-kp-crust'),
    foreground: cssToken(styles, '--color-kp-text'),
    cursor: cssToken(styles, '--color-kp-mauve'),
    cursorAccent: cssToken(styles, '--color-kp-crust'),
    selectionBackground: cssToken(styles, '--color-kp-mauve-muted'),
    black: cssToken(styles, '--color-kp-crust'),
    red: cssToken(styles, '--color-kp-red'),
    green: cssToken(styles, '--color-kp-green'),
    yellow: cssToken(styles, '--color-kp-yellow'),
    blue: cssToken(styles, '--color-kp-sky'),
    magenta: cssToken(styles, '--color-kp-mauve'),
    cyan: cssToken(styles, '--color-kp-sky'),
    white: cssToken(styles, '--color-kp-text'),
  }
}

// ExecTerminal renders the real bidirectional exec stream with xterm.js.
// Control/status messages stay in the accessible HTML status log; only the
// remote process's stdout/stderr bytes flow through this component.
const ExecTerminal = forwardRef<ExecTerminalHandle, ExecTerminalProps>(function ExecTerminal(
  { onStdin, onResize, label = 'Exec terminal output' },
  ref,
) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const terminalRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const onStdinRef = useRef(onStdin)
  const onResizeRef = useRef(onResize)
  const [degraded, setDegraded] = useState(false)
  onStdinRef.current = onStdin
  onResizeRef.current = onResize

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    // jsdom (unit tests) and some embedded webviews lack canvas APIs; degrade
    // to an inert <pre> instead of crashing the actions panel.
    try {
      const rootStyles = getComputedStyle(document.documentElement)
      const tokenFontSize = Number.parseFloat(getComputedStyle(container).fontSize)
      const terminal = new Terminal({
        convertEol: true,
        cursorBlink: true,
        ...(Number.isFinite(tokenFontSize) ? { fontSize: tokenFontSize } : {}),
        fontFamily: cssToken(rootStyles, '--font-mono'),
        scrollback: 1_000,
        screenReaderMode: true,
        theme: terminalTheme(rootStyles),
      })
      const fit = new FitAddon()
      terminal.loadAddon(fit)
      terminal.onData((data) => onStdinRef.current?.(data))
      terminal.onResize(({ cols, rows }) => onResizeRef.current?.(cols, rows))
      terminal.open(container)
      try {
        fit.fit()
      } catch {
        // The container may be hidden before layout; the ResizeObserver below
        // fits it as soon as it becomes measurable.
      }
      terminalRef.current = terminal
      fitRef.current = fit
      // Fit again when the bundled face replaces the fallback metrics.
      void document.fonts?.ready.then(() => {
        if (terminalRef.current !== terminal) return
        try { fit.fit() } catch { /* A hidden terminal is fitted on resize. */ }
      })
    } catch {
      setDegraded(true)
      return
    }

    if (typeof ResizeObserver === 'undefined') {
      return
    }
    const observer = new ResizeObserver(() => {
      try {
        fitRef.current?.fit()
      } catch {
        // Ignore transient zero-size layouts.
      }
    })
    observer.observe(container)
    return () => {
      observer.disconnect()
      terminalRef.current?.dispose()
      terminalRef.current = null
      fitRef.current = null
    }
  }, [])

  useImperativeHandle(ref, () => ({
    writeStdout: (text: string) => terminalRef.current?.write(text),
    writeStderr: (text: string) => terminalRef.current?.write(`\x1b[31m${text}\x1b[0m`),
    clear: () => {
      terminalRef.current?.clear()
      terminalRef.current?.write('\x1b[2J\x1b[H')
    },
  }))

  return degraded
    ? <pre aria-label={label} className="min-h-32 rounded-md border border-kp-overlay-0 bg-kp-crust p-2.5 text-content text-kp-text" />
    : <div ref={containerRef} role="group" aria-label={label} className="min-h-32 text-content" />
})

export default ExecTerminal
