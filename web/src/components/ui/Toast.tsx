import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'
import { CheckCircle2, Info, TriangleAlert, XCircle, X } from 'lucide-react'

export type ToastTone = 'success' | 'error' | 'info' | 'warning'

export interface Toast {
  id: number
  tone: ToastTone
  title: string
  detail?: string
}

interface ToastContextValue {
  push: (toast: Omit<Toast, 'id'>) => void
  success: (title: string, detail?: string) => void
  error: (title: string, detail?: string) => void
  info: (title: string, detail?: string) => void
  warning: (title: string, detail?: string) => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

const maximumToasts = 5
const toastDuration = { success: 5000, info: 5000, warning: 7000, error: 9000 } as const

const toneIcon = {
  success: CheckCircle2,
  error: XCircle,
  info: Info,
  warning: TriangleAlert,
} as const

const toneColor = {
  success: 'text-kp-green',
  error: 'text-kp-red',
  info: 'text-kp-sky',
  warning: 'text-kp-yellow',
} as const

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const nextID = useRef(1)

  const dismiss = useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id))
  }, [])

  const push = useCallback((toast: Omit<Toast, 'id'>) => {
    const id = nextID.current++
    setToasts((current) => [...current.slice(-(maximumToasts - 1)), { ...toast, id }])
    window.setTimeout(() => dismiss(id), toastDuration[toast.tone])
  }, [dismiss])

  const value = useMemo<ToastContextValue>(() => ({
    push,
    success: (title, detail) => push({ tone: 'success', title, detail }),
    error: (title, detail) => push({ tone: 'error', title, detail }),
    info: (title, detail) => push({ tone: 'info', title, detail }),
    warning: (title, detail) => push({ tone: 'warning', title, detail }),
  }), [push])

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="toast-viewport" aria-live="polite" aria-label="Action feedback">
        {toasts.map((toast) => {
          const Icon = toneIcon[toast.tone]
          return (
            <div key={toast.id} className="toast-item" data-tone={toast.tone} role={toast.tone === 'error' ? 'alert' : 'status'}>
              <div className="flex items-start gap-2">
                <Icon size={15} strokeWidth={1.8} className={`mt-0.5 shrink-0 ${toneColor[toast.tone]}`} aria-hidden="true" />
                <div className="min-w-0 flex-1">
                  <p className="m-0 text-sm font-medium text-kp-text">{toast.title}</p>
                  {toast.detail ? <p className="m-0 text-xs leading-relaxed break-words text-kp-subtext">{toast.detail}</p> : null}
                </div>
                <button type="button" onClick={() => dismiss(toast.id)} aria-label="Dismiss notification" className="-m-1 grid h-6 w-6 shrink-0 place-items-center rounded-md text-kp-overlay-text hover:text-kp-text hover:bg-kp-surface-3">
                  <X size={13} aria-hidden="true" />
                </button>
              </div>
            </div>
          )
        })}
      </div>
    </ToastContext.Provider>
  )
}

export function useToast(): ToastContextValue {
  const context = useContext(ToastContext)
  if (!context) {
    throw new Error('useToast requires ToastProvider.')
  }
  return context
}
