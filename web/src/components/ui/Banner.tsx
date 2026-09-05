import { AlertTriangle, CheckCircle2, Info, OctagonAlert } from 'lucide-react'
import type { ReactElement, ReactNode } from 'react'

export type BannerVariant = 'error' | 'warning' | 'info' | 'success'

const styles: Record<BannerVariant, { box: string; icon: ReactElement }> = {
  error: { box: 'border-kp-red-border bg-kp-red-bg', icon: <OctagonAlert size={15} className="text-kp-red" /> },
  warning: { box: 'border-kp-yellow-border bg-kp-yellow-bg', icon: <AlertTriangle size={15} className="text-kp-yellow" /> },
  info: { box: 'border-kp-blue-border bg-kp-blue-bg', icon: <Info size={15} className="text-kp-sky" /> },
  success: { box: 'border-kp-green-border bg-kp-green-bg', icon: <CheckCircle2 size={15} className="text-kp-green" /> },
}

export interface BannerProps {
  variant?: BannerVariant
  title?: ReactNode
  children: ReactNode
  details?: ReactNode
  className?: string
  role?: 'alert' | 'status'
}

/** Inline banner for errors, warnings, info and success feedback. */
export function Banner({ variant = 'info', title, children, details, className = '', role }: BannerProps) {
  const style = styles[variant]
  return (
    <div role={role} className={`flex flex-col gap-1 px-3 py-2 border-l-2 rounded-r-md text-sm leading-relaxed ${style.box} ${className}`}>
      <div className="flex items-start gap-2">
        <span className="mt-0.5 shrink-0" aria-hidden="true">{style.icon}</span>
        <div className="min-w-0">
          {title ? <strong className="block text-kp-text">{title}</strong> : null}
          <div className="text-kp-subtext">{children}</div>
        </div>
      </div>
      {details ? <details className="ml-6"><summary className="cursor-pointer text-xs text-kp-overlay-text hover:text-kp-subtext">Technical details</summary><div className="mt-1 mono text-xs text-kp-overlay-text break-words">{details}</div></details> : null}
    </div>
  )
}

export function ErrorBanner(props: Omit<BannerProps, 'variant'>) {
  return <Banner variant="error" role="alert" {...props} />
}

export function WarningBanner(props: Omit<BannerProps, 'variant'>) {
  return <Banner variant="warning" {...props} />
}

export function InfoBanner(props: Omit<BannerProps, 'variant'>) {
  return <Banner variant="info" {...props} />
}

export function SuccessBanner(props: Omit<BannerProps, 'variant'>) {
  return <Banner variant="success" role="status" {...props} />
}
