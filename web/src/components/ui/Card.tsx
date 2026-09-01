import type { HTMLAttributes, ReactNode } from 'react'

export interface CardProps extends HTMLAttributes<HTMLDivElement> {
  children: ReactNode
  className?: string
}

export function Card({ children, className = '', ...props }: CardProps) {
  return (
    <div
      className={`
        border border-kp-overlay-0 rounded-xl bg-kp-surface-0 shadow-card
        ${className}
      `}
      {...props}
    >
      {children}
    </div>
  )
}

export interface CardHeaderProps {
  children: ReactNode
  className?: string
}

export function CardHeader({ children, className = '' }: CardHeaderProps) {
  return <div className={`flex items-center justify-between gap-5 mb-5 ${className}`}>{children}</div>
}

export interface CardTitleProps {
  children: ReactNode
  className?: string
}

export function CardTitle({ children, className = '' }: CardTitleProps) {
  return <h2 className={`text-xl text-kp-text ${className}`}>{children}</h2>
}

export interface CardContentProps {
  children: ReactNode
  className?: string
}

export function CardContent({ children, className = '' }: CardContentProps) {
  return <div className={className}>{children}</div>
}
