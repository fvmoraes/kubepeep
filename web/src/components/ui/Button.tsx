import type { ButtonHTMLAttributes, ReactNode } from 'react'

export type ButtonVariant =
  | 'primary'    // blue — normal/primary actions (apply, refresh, open, connect…)
  | 'secondary'  // neutral surface — alternative actions
  | 'success'    // green — positive confirmation
  | 'danger'     // red — delete, stop, disconnect
  | 'warning'    // amber — risky-but-required confirmations
  | 'ghost'      // transparent — inline, table and toolbar actions
  | 'icon'       // ghost square — icon-only controls

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  children?: ReactNode
  disabledReason?: string
}

const variants: Record<ButtonVariant, string> = {
  primary:
    'text-kp-sky bg-kp-blue-bg border-kp-blue-border hover:not-disabled:border-kp-sky hover:not-disabled:bg-kp-surface-3',
  secondary:
    'text-kp-subtext bg-kp-surface-1 border-kp-overlay-1 hover:not-disabled:bg-kp-surface-3 hover:not-disabled:text-kp-text hover:not-disabled:border-kp-overlay-3',
  success:
    'text-kp-green bg-kp-green-bg border-kp-green-border hover:not-disabled:border-kp-green hover:not-disabled:bg-kp-surface-3',
  danger:
    'text-kp-red bg-kp-red-bg border-kp-red-border hover:not-disabled:border-kp-red hover:not-disabled:bg-kp-surface-3',
  warning:
    'text-kp-yellow bg-kp-yellow-bg border-kp-yellow-border hover:not-disabled:border-kp-yellow hover:not-disabled:bg-kp-surface-3',
  ghost:
    'text-kp-subtext bg-transparent border-transparent hover:not-disabled:bg-kp-surface-3 hover:not-disabled:text-kp-text aria-pressed:font-bold aria-pressed:bg-kp-accent-bg aria-pressed:text-kp-mauve aria-pressed:border-kp-accent-border',
  icon:
    'text-kp-overlay-text bg-transparent border-transparent hover:not-disabled:bg-kp-surface-3 hover:not-disabled:text-kp-text aria-pressed:font-bold aria-pressed:bg-kp-accent-bg aria-pressed:text-kp-mauve aria-pressed:border-kp-accent-border',
}

export function Button({ variant = 'primary', className = '', type, children, disabled, disabledReason, title, ...props }: ButtonProps) {
  const tooltip = disabled
    ? disabledReason ?? title ?? 'This action is unavailable until its current requirements are satisfied.'
    : title
  return (
    <button
      type={type ?? 'button'}
      disabled={disabled}
      title={tooltip}
      className={`control inline-flex items-center justify-center border text-content leading-5 font-normal whitespace-nowrap cursor-pointer transition-colors duration-100 disabled:cursor-not-allowed disabled:opacity-45 focus-visible:outline-2 focus-visible:outline-kp-mauve focus-visible:outline-offset-1 ${variant === 'icon' ? 'control-icon p-0' : 'gap-1.5 px-2 py-0'} ${variants[variant]} ${className}`}
      {...props}
    >
      {children}
    </button>
  )
}
