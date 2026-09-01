import type { ButtonHTMLAttributes, ReactNode } from 'react'

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
  size?: 'default' | 'compact'
  children: ReactNode
}

export function Button({
  variant = 'primary',
  size = 'default',
  className = '',
  children,
  ...props
}: ButtonProps) {
  const base =
    'inline-flex items-center justify-center gap-2 font-bold rounded-md cursor-pointer transition-[filter] disabled:cursor-not-allowed disabled:opacity-48'

  const sizes = {
    default: 'min-h-9 px-3.5 py-2 text-base',
    compact: 'min-h-7.5 px-2 py-1 text-xs',
  }

  const variants = {
    primary:
      'text-kp-crust bg-kp-mauve border border-kp-mauve-hover hover:not-disabled:brightness-108',
    secondary:
      'text-kp-subtext bg-kp-surface-3 border border-kp-overlay-3 hover:not-disabled:bg-kp-surface-4 hover:not-disabled:border-kp-overlay-5',
    danger:
      'text-kp-red bg-kp-red-bg border border-kp-red-border hover:not-disabled:brightness-108',
    ghost:
      'text-kp-subtext bg-transparent border border-transparent hover:not-disabled:bg-kp-surface-2 hover:not-disabled:text-kp-text',
  }

  return (
    <button
      type="button"
      className={`${base} ${sizes[size]} ${variants[variant]} ${className}`}
      {...props}
    >
      {children}
    </button>
  )
}
