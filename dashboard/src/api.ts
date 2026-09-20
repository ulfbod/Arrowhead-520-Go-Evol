const BASE = ''

async function get<T>(url: string): Promise<T> {
  const resp = await fetch(BASE + url)
  if (!resp.ok) throw new Error(`${resp.status} ${resp.statusText}`)
  return resp.json()
}

async function post<T>(url: string, body: unknown): Promise<T> {
  const resp = await fetch(BASE + url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!resp.ok) throw new Error(`${resp.status} ${resp.statusText}`)
  return resp.json()
}

async function del(url: string): Promise<void> {
  const resp = await fetch(BASE + url, { method: 'DELETE' })
  if (!resp.ok) throw new Error(`${resp.status} ${resp.statusText}`)
}

// Health checks
export interface HealthResponse { status: string; system?: string }

export async function getHealth(path: string): Promise<HealthResponse> {
  const resp = await fetch(BASE + path)
  if (!resp.ok) throw new Error(`${resp.status} ${resp.statusText}`)
  const contentType = resp.headers.get('content-type') ?? ''
  if (contentType.includes('json')) {
    return resp.json()
  }
  // Non-JSON 200 (e.g. AuthzForce returns XML) — treat as healthy
  return { status: 'ok' }
}

// Profile-CA / PIP
export interface PIPAttributes { systemName: string; certLevel: string; valid: boolean }
export interface PIPSubject { cn: string; ou: string; issuedAt: string; expiresAt: string; revoked: boolean; valid: boolean }
export interface PIPSubjectsList { subjects: PIPSubject[]; count: number }
export interface PIPStatus { subjects: number }

export const pip = {
  attributes: (cn: string) => get<PIPAttributes>(`/api/pip/attributes/${cn}`),
  subjects: () => get<PIPSubjectsList>('/api/pip/subjects'),
  status: () => get<PIPStatus>('/api/pip/status'),
}

export const ca = {
  info: () => get<{ commonName: string; certificate: string }>('/api/profile-ca/ca/info'),
  revoke: (cn: string) => del(`/api/profile-ca/ca/certificates/${cn}`),
  reissue: (cn: string) => post<void>(`/api/profile-ca/ca/certificates/${cn}/reissue`, null),
}

// PAP
export interface Policy { id: string; subject: string; resource: string; action: string; effect: string; createdAt: string }
export interface PoliciesResponse { count: number; policies: Policy[] }

export const pap = {
  list: () => get<PoliciesResponse>('/api/pap/policies'),
  create: (p: Omit<Policy, 'id' | 'createdAt'>) => post<Policy>('/api/pap/policies', p),
}

// DynamicOrch
export const orch = {
  status: () => get<{ status: string; authBackend: string; enableAuth: boolean }>('/api/dynamicorch/status'),
}

// Kafka-authz
export const kafkaAuthz = {
  health: () => getHealth('/api/kafka-authz/health'),
}

// PKI-rest-authz
export const pkiRestAuthz = {
  health: () => getHealth('/api/pki-rest-authz/health'),
}
