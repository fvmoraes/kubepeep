import wordmark from '../assets/kubepeep-name.svg'

interface BrandWordmarkProps {
  className?: string
  height?: number
  alt?: string
}

export function BrandWordmark({ className = '', height = 16, alt = 'KubePeep' }: BrandWordmarkProps) {
  return (
    <img
      src={wordmark}
      alt={alt}
      style={{ height: `${height}px`, width: 'auto' }}
      className={`block ${className}`}
    />
  )
}
