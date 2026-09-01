import logo from '../assets/kubepeep-logo.svg'

interface BrandLogoProps {
  className?: string
  size?: number
  alt?: string
}

export function BrandLogo({ className, size = 34, alt = 'KubePeep' }: BrandLogoProps) {
  return (
    <img
      src={logo}
      alt={alt}
      width={size}
      height={size}
      className={className}
      aria-hidden="true"
    />
  )
}
