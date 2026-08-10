import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import type { CapabilityMatrix } from '../api/client'
import { PermissionsMatrixView } from './PermissionsMatrix'

afterEach(cleanup)

describe('permission matrix', () => {
  it('renders allowed, denied, and unknown as distinct textual decisions', () => {
    const matrix: CapabilityMatrix = {
      generation: 'gen_42',
      complete: false,
      truncated: false,
      errors: [{ code: 'AUTHORIZATION_UNAVAILABLE', message: 'One review timed out.' }],
      decisions: [
        { capabilityId: 'pods.list', namespace: 'payments', apiGroup: '', resource: 'pods', subresource: '', verb: 'list', resourceName: '', decision: 'allowed', reasonCode: 'SAR_ALLOWED', expiresAt: null },
        { capabilityId: 'pods.delete', namespace: 'payments', apiGroup: '', resource: 'pods', subresource: '', verb: 'delete', resourceName: 'api', decision: 'denied', reasonCode: 'SAR_DENIED', expiresAt: null },
        { capabilityId: 'pods.logs.get', namespace: 'payments', apiGroup: '', resource: 'pods', subresource: 'log', verb: 'get', resourceName: 'api', decision: 'unknown', reasonCode: 'SAR_UNAVAILABLE', expiresAt: null },
      ],
    }

    render(<PermissionsMatrixView matrix={matrix} />)

    expect(screen.getByText('allowed')).toHaveClass('permission-decision--allowed')
    expect(screen.getByText('denied')).toHaveClass('permission-decision--denied')
    expect(screen.getByText('permission could not be verified')).toHaveClass('permission-decision--unknown')
    expect(screen.getByText(/Unknown is not treated as a confirmed denial/)).toBeInTheDocument()
    expect(screen.getByText('One review timed out.')).toBeInTheDocument()
  })
})
