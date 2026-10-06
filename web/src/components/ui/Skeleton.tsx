export interface SkeletonProps {
  className?: string
}

export function Skeleton({ className = '' }: SkeletonProps) {
  return <div className={`loading-skeleton rounded-md bg-kp-surface-3 ${className}`} aria-hidden="true" />
}
