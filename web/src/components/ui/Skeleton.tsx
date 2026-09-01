export interface SkeletonProps {
  className?: string
}

export function Skeleton({ className = '' }: SkeletonProps) {
  return (
    <div
      className={`
        animate-pulse rounded-md bg-kp-overlay-1
        ${className}
      `}
      aria-hidden="true"
    />
  )
}
