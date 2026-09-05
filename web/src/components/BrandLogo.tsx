import logo from '../assets/kubepeep-logo.svg'

interface BrandLogoProps {
  className?: string
  size?: number
  alt?: string
}

export function BrandLogo({ className = '', size = 28, alt = 'KubePeep' }: BrandLogoProps) {
  return (
    <img
      src={logo}
      alt={alt}
      style={{ width: `${size}px`, height: `${size}px` }}
      className={`block shrink-0 ${className}`}
      aria-hidden="true"
    />
  )
}
