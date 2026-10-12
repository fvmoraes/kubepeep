import { useEffect, useLayoutEffect, useRef } from 'react'
import { Compartment, EditorState } from '@codemirror/state'
import { EditorView, highlightSpecialChars, keymap } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { HighlightStyle, indentOnInput, syntaxHighlighting } from '@codemirror/language'
import { yaml } from '@codemirror/lang-yaml'
import { tags } from '@lezer/highlight'

const colors = HighlightStyle.define([
  { tag: tags.propertyName, color: '#ce9178' },
  { tag: [tags.string, tags.special(tags.string)], color: '#ce9178' },
  { tag: [tags.bool, tags.null, tags.keyword], color: '#569cd6' },
  { tag: tags.number, color: '#b5cea8' },
  { tag: tags.comment, color: '#6a9955' },
  { tag: tags.content, color: '#569cd6' },
  { tag: [tags.atom, tags.labelName], color: '#9cdcfe' },
  { tag: [tags.punctuation, tags.operator], color: '#d4d4d4' },
])
const theme = EditorView.theme({
  '&': { height: '100%', color: 'var(--color-kp-text)', backgroundColor: 'var(--color-kp-crust)' },
  '.cm-scroller': { overflow: 'auto', fontFamily: 'var(--font-mono)', fontSize: 'var(--text-content)', lineHeight: '1.6' },
  '.cm-content': { padding: '16px 0', caretColor: 'var(--color-kp-text)' },
  '.cm-line': { padding: '0 16px' },
  '&.cm-focused': { outline: 'none' },
  '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': { backgroundColor: '#61508080' },
}, { dark: true })

/** A viewport-rendered editor; documents stay in memory and are never reformatted. */
export function YamlCodeEditor({ id, value, disabled = false, onChange, describedBy }: {
  id: string; value: string; disabled?: boolean; onChange: (value: string) => void; describedBy?: string
}) {
  const host = useRef<HTMLDivElement>(null)
  const editor = useRef<EditorView | null>(null)
  const callback = useRef(onChange)
  const editable = useRef(new Compartment())
  const attributes = useRef(new Compartment())
  // Keep callbacks current without recreating the editor, losing selection or undo.
  useLayoutEffect(() => { callback.current = onChange }, [onChange])
  useEffect(() => {
    const view = new EditorView({ parent: host.current!, state: EditorState.create({ doc: value, extensions: [
      yaml(), syntaxHighlighting(colors), theme, history(), highlightSpecialChars(), indentOnInput(),
      keymap.of([...defaultKeymap, ...historyKeymap]), EditorState.tabSize.of(2), EditorState.lineSeparator.of(value.includes('\r\n') ? '\r\n' : '\n'),
      editable.current.of(EditorState.readOnly.of(disabled)),
      attributes.current.of(EditorView.contentAttributes.of({ id, 'aria-labelledby': `${id}-label`, 'aria-describedby': describedBy ?? '', 'aria-multiline': 'true', spellcheck: 'false', autocapitalize: 'off', autocomplete: 'off' })),
      EditorView.updateListener.of(update => { if (update.docChanged) callback.current(update.state.sliceDoc()) }),
    ] }) })
    editor.current = view
    return () => { editor.current = null; view.destroy() }
    // Initial content is synchronized below; editing must preserve undo history.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  useEffect(() => {
    const view = editor.current
    if (view && view.state.sliceDoc() !== value) view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } })
  }, [value])
  useEffect(() => {
    editor.current?.dispatch({ effects: [editable.current.reconfigure(EditorState.readOnly.of(disabled)), attributes.current.reconfigure(EditorView.contentAttributes.of({ id, 'aria-labelledby': `${id}-label`, 'aria-describedby': describedBy ?? '', 'aria-multiline': 'true', 'aria-readonly': String(disabled), spellcheck: 'false', autocapitalize: 'off', autocomplete: 'off' }))] })
  }, [disabled, describedBy, id])
  return <div className="yaml-editor" ref={host} />
}
