import wordmark from '../assets/kubepeep-name.svg'

interface BrandWordmarkProps {
  className?: string
  height?: number
  alt?: string
}

export function BrandWordmark({ className, height = 20, alt = 'KubePeep' }: BrandWordmarkProps) {
  return (
    <img
      src={wordmark}
      alt={alt}
      height={height}
      className={className}
    />
  )
}
