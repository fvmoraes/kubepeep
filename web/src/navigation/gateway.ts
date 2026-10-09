export const gatewayCollections = {
  'gateway-classes': 'GatewayClass',
  'gateways': 'Gateway',
  'http-routes': 'HTTPRoute',
  'grpc-routes': 'GRPCRoute',
  'tcp-routes': 'TCPRoute',
  'tls-routes': 'TLSRoute',
  'udp-routes': 'UDPRoute',
  'reference-grants': 'ReferenceGrant',
  'backend-tls-policies': 'BackendTLSPolicy',
  'listener-sets': 'ListenerSet',
} as const
export type GatewayCollection = keyof typeof gatewayCollections
export function isGatewayCollection(value: string): value is GatewayCollection { return Object.hasOwn(gatewayCollections, value) }
