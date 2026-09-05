import type { ButtonHTMLAttributes, ReactNode } from 'react'

export type ButtonVariant =
  | 'primary'    // blue — normal/primary actions (apply, refresh, open, connect…)
  | 'secondary'  // neutral surface — alternative actions
  | 'success'    // green — positive confirmation
  | 'danger'     // red — delete, stop, disconnect
  | 'warning'    // amber — risky-but-required confirmations
  | 'ghost'      // transparent — inline, table and toolbar actions
  | 'icon'       // ghost square — icon-only controls

export type ButtonSize = 'sm' | 'md' | 'lg'

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: ButtonSize
  children?: ReactNode
}

const sizes: Record<ButtonSize, string> = {
  sm: 'h-7 gap-1.5 px-2.5 text-xs rounded-md',
  md: 'h-8 gap-2 px-3 text-sm rounded-md',
  lg: 'h-9 gap-2 px-4 text-base rounded-lg',
}

const variants: Record<ButtonVariant, string> = {
  primary:
    'text-white bg-kp-blue border border-kp-blue hover:not-disabled:bg-kp-blue-hover hover:not-disabled:border-kp-blue-hover',
  secondary:
    'text-kp-text bg-kp-surface-3 border border-kp-overlay-1 hover:not-disabled:bg-kp-overlay-0 hover:not-disabled:border-kp-overlay-3',
  success:
    'text-white bg-kp-green-solid border border-kp-green-solid hover:not-disabled:bg-kp-green-solid-hover hover:not-disabled:border-kp-green-solid-hover',
  danger:
    'text-white bg-kp-red-solid border border-kp-red-solid hover:not-disabled:bg-kp-red-solid-hover hover:not-disabled:border-kp-red-solid-hover',
  warning:
    'text-kp-base bg-kp-amber border border-kp-amber hover:not-disabled:bg-kp-amber-hover hover:not-disabled:border-kp-amber-hover',
  ghost:
    'text-kp-subtext bg-transparent border border-transparent hover:not-disabled:bg-kp-surface-3 hover:not-disabled:text-kp-text',
  icon:
    'h-7 w-7 gap-0 p-0 justify-center text-kp-overlay-text bg-transparent border border-transparent rounded-md hover:not-disabled:bg-kp-surface-3 hover:not-disabled:text-kp-text',
}

export function Button({ variant = 'primary', size = 'md', className = '', type, children, ...props }: ButtonProps) {
  const sizeClass = variant === 'icon' ? '' : sizes[size]
  return (
    <button
      type={type ?? 'button'}
      className={`inline-flex items-center justify-center font-medium whitespace-nowrap cursor-pointer transition-colors duration-100 disabled:cursor-not-allowed disabled:opacity-45 focus-visible:outline-2 focus-visible:outline-kp-mauve focus-visible:outline-offset-1 ${sizeClass} ${variants[variant]} ${className}`}
      {...props}
    >
      {children}
    </button>
  )
}
