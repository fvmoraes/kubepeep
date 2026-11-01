// A section chunk has no data dependency, but the Outlet that mounts it sits
// behind the scope and namespace gates in App. Starting the download as soon
// as the route is known lets the gate wait and the chunk fetch overlap,
// without mounting anything early: the component still appears only after the
// gates clear.
//
// Specific paths precede the generic prefixes they live under, because the
// first match wins. /workloads/custom is a dynamic resource view, not a
// workload list, and /network/gateway-api is its own page.
const sectionChunks: Array<{ pattern: RegExp; chunk: string; load: () => Promise<unknown> }> = [
  { pattern: /^\/workloads\/custom(\/|$)/, chunk: 'DynamicResourcePage', load: () => import('../components/DynamicResourcePage') },
  { pattern: /^\/custom-resources(\/|$)/, chunk: 'DynamicResourcePage', load: () => import('../components/DynamicResourcePage') },
  { pattern: /^\/network\/gateway-api(\/|$)/, chunk: 'GatewayPage', load: () => import('../components/GatewayPage') },
  { pattern: /^\/namespaces$/, chunk: 'NamespaceScopeEditor', load: () => import('../components/NamespaceScopeEditor') },
  { pattern: /^\/namespaces\//, chunk: 'FamilyPages', load: () => import('../components/FamilyPages') },
  { pattern: /^\/(pods|workloads|network|config|events|nodes)(\/|$)/, chunk: 'ResourcePages', load: () => import('../components/ResourcePages') },
  { pattern: /^\/(storage|leases)(\/|$)/, chunk: 'FamilyPages', load: () => import('../components/FamilyPages') },
  { pattern: /^\/(configuration|service-accounts)(\/|$)/, chunk: 'ConfigurationPages', load: () => import('../components/ConfigurationPages') },
  { pattern: /^\/(access|administration)(\/|$)/, chunk: 'AccessPages', load: () => import('../components/AccessPages') },
  { pattern: /^\/helm(\/|$)/, chunk: 'HelmPage', load: () => import('../components/HelmPage') },
  { pattern: /^\/permissions$/, chunk: 'PermissionsMatrix', load: () => import('../components/PermissionsMatrix') },
  { pattern: /^\/logs$/, chunk: 'LogsPage', load: () => import('../components/LogsPage') },
  { pattern: /^\/settings$/, chunk: 'SettingsPage', load: () => import('../components/SettingsPage') },
  { pattern: /^\/$/, chunk: 'Dashboard', load: () => import('../components/Dashboard') },
]

/**
 * sectionChunkFor names the chunk a route will mount, without importing it.
 * A wrong answer warms the wrong module: it costs parse and delivers nothing,
 * and the lazy boundary still imports the right one, so the mistake is
 * silent. sectionChunks.test.ts mirrors App's route table to catch it.
 */
export function sectionChunkFor(pathname: string): string | null {
  return sectionChunks.find((entry) => entry.pattern.test(pathname))?.chunk ?? null
}

export function warmSectionChunk(pathname: string): void {
  const entry = sectionChunks.find((candidate) => candidate.pattern.test(pathname))
  // A speculative warm stays silent on failure. The lazy boundary imports
  // again when it mounts and owns the error from there.
  if (entry) void entry.load().catch(() => {})
}
