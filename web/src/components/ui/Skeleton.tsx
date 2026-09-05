export interface SkeletonProps {
  className?: string
}

export function Skeleton({ className = '' }: SkeletonProps) {
  return <div className={`animate-pulse rounded-md bg-kp-surface-3 ${className}`} aria-hidden="true" />
}
