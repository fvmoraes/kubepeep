/** No artificial delay to data: only the visual feedback waits to avoid flashes. */
export function LoadingState({ label = 'Loading…', layout = 'block' }: { label?: string; layout?: 'inline' | 'block' | 'table' }) {
  return <div className={`loading-state loading-state--${layout}`} role="status" aria-live="polite" aria-busy="true" aria-label={label}>
    <div className="loading-feedback"><span className="loading-dot" aria-hidden="true" /><span>{label}</span></div>
    {layout === 'table' ? <div className="loading-rows" aria-hidden="true">{Array.from({ length: 5 }, (_, index) => <div key={index}><span /><span /><span /></div>)}</div> : null}
  </div>
}
