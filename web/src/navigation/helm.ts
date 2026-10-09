export type HelmDriver = 'secrets' | 'configmaps'
export function helmCollection(driver: HelmDriver) { return driver === 'secrets' ? 'helm-releases' : 'helm-configmap-releases' }
export function helmDriver(collection: string): HelmDriver { return collection === 'helm-configmap-releases' ? 'configmaps' : 'secrets' }
export function isHelmCollection(collection: string) { return collection === 'helm-releases' || collection === 'helm-configmap-releases' }
