import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Palette } from 'lucide-react'
import { getPreferences, type Preferences, type SelectionSummary } from '../api/client'
import { mutatePreferences } from '../api/preferences'
import { TableMenu } from './ui/TableMenu'

export function contextColor(preferences: Preferences | undefined, selection: SelectionSummary | null): string | undefined {
  const color = preferences?.ui?.contextColors?.find((item) => item.clusterProfileId === selection?.clusterProfileId && item.context === selection?.context)?.color
  return color && /^#[0-9a-f]{6}$/i.test(color) ? color : undefined
}

const presets = [
  { label: 'Development — blue', color: '#38bdf8' },
  { label: 'Staging — yellow', color: '#facc15' },
  { label: 'Production — red', color: '#f87171' },
]

export function ContextColorPicker({ selection }: { selection: SelectionSummary | null }) {
  const queryClient = useQueryClient()
  const preferences = useQuery({ queryKey: ['preferences'], queryFn: ({ signal }) => getPreferences(signal), staleTime: 60_000 })
  const save = useMutation({
    mutationFn: ({ color, clusterProfileId, context }: { color?: string; clusterProfileId: number; context: string }) => mutatePreferences((current) => {
      const colors = (current.ui.contextColors ?? []).filter((item) => item.clusterProfileId !== clusterProfileId || item.context !== context)
      if (color) colors.push({ clusterProfileId, context, color })
      return { ...current, ui: { ...current.ui, contextColors: colors } }
    }),
    onSuccess: (saved) => queryClient.setQueryData(['preferences'], saved),
  })
  if (!selection) return null
  const selectedColor = contextColor(preferences.data, selection)
  const choose = (color?: string) => save.mutate({ color, clusterProfileId: selection.clusterProfileId, context: selection.context })
  return <TableMenu label="Context color" icon={<Palette size={14} style={{ color: selectedColor }} />} active={Boolean(selectedColor)}>
    <div className="grid gap-2 p-2 min-w-48">
      <strong className="text-content break-all">{selection.context}</strong>
      {presets.map((preset) => <button key={preset.color} type="button" className="flex items-center gap-2 text-left text-content" aria-pressed={selectedColor === preset.color} disabled={save.isPending} onClick={() => choose(preset.color)}>
        <span className="h-3 w-3 rounded-full" style={{ backgroundColor: preset.color }} aria-hidden="true" />{preset.label}
      </button>)}
      <label className="flex items-center justify-between gap-2 text-content">Custom color<input type="color" aria-label="Custom context color" value={selectedColor ?? '#38bdf8'} disabled={save.isPending} onChange={(event) => choose(event.target.value)} /></label>
      <button type="button" className="text-left text-content text-kp-overlay-text" disabled={save.isPending} onClick={() => choose()}>Clear context color</button>
      {save.isError ? <span role="alert" className="text-kp-red text-content">Could not save the context color. Try again.</span> : null}
    </div>
  </TableMenu>
}
