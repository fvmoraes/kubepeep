import { useCallback, useState } from 'react'

import type { ResourceStreamProgress, ResourceTopic } from '../ResourceLiveUpdates'

interface Preview<T> {
  identity: string
  snapshotId: string
  items: T[]
  completed: number
  requested: number
}

/** Stream rows are a bounded, non-selectable preview until HTTP confirms the list. */
export function useResourceStreamPreview<T extends { namespace: string }>({
  identity,
  topic,
  namespace,
  isItem,
  itemKey,
  compare,
}: {
  identity: readonly unknown[]
  topic: ResourceTopic
  namespace: string
  isItem: (value: unknown) => value is T
  itemKey: (item: T) => string
  compare?: (left: T, right: T) => number
}) {
  const fingerprint = JSON.stringify(identity)
  const [preview, setPreview] = useState<Preview<T> | null>(null)
  const onProgress = useCallback((progress: ResourceStreamProgress) => {
    if (progress.topic !== topic) return
    setPreview((current) => {
      const previous = current?.identity === fingerprint && current.snapshotId === progress.snapshotId ? current.items : []
      const byKey = new Map(previous.map((item) => [itemKey(item), item]))
      for (const value of progress.items) {
        if (!isItem(value) || namespace && value.namespace !== namespace) continue
        byKey.set(itemKey(value), value)
      }
      const items = [...byKey.values()].slice(0, 500)
      items.sort(compare ?? ((left, right) => itemKey(left).localeCompare(itemKey(right))))
      return { identity: fingerprint, snapshotId: progress.snapshotId, items, completed: progress.completedNamespaces, requested: progress.requestedNamespaces }
    })
  }, [topic, fingerprint, namespace, isItem, itemKey, compare])
  const onReset = useCallback(() => setPreview(null), [])
  return { preview: preview?.identity === fingerprint ? preview : null, onProgress, onReset }
}
