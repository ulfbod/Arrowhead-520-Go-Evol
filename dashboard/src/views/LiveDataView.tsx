import { useEffect, useState } from 'react'
import { orch, kafkaAuthz, pkiRestAuthz } from '../api'

interface EnforcementStatus {
  name: string
  status: string
  details: Record<string, unknown>
  error: string | null
}

export default function LiveDataView() {
  const [statuses, setStatuses] = useState<EnforcementStatus[]>([])

  useEffect(() => {
    const check = async () => {
      const results: EnforcementStatus[] = []

      try {
        const o = await orch.status()
        results.push({ name: 'DynamicOrch-XACML', status: 'UP', details: o, error: null })
      } catch (e) {
        results.push({ name: 'DynamicOrch-XACML', status: 'DOWN', details: {}, error: String(e) })
      }

      try {
        const k = await kafkaAuthz.health()
        results.push({ name: 'kafka-authz', status: k.status === 'ok' ? 'UP' : 'DEGRADED', details: k as unknown as Record<string, unknown>, error: null })
      } catch (e) {
        results.push({ name: 'kafka-authz', status: 'DOWN', details: {}, error: String(e) })
      }

      try {
        const p = await pkiRestAuthz.health()
        results.push({ name: 'pki-rest-authz', status: p.status === 'ok' ? 'UP' : 'DEGRADED', details: p as unknown as Record<string, unknown>, error: null })
      } catch (e) {
        results.push({ name: 'pki-rest-authz', status: 'DOWN', details: {}, error: String(e) })
      }

      setStatuses(results)
    }

    check()
    const id = setInterval(check, 5000)
    return () => clearInterval(id)
  }, [])

  return (
    <div>
      <h2>Enforcement Status</h2>
      <p style={{ fontSize: 13, color: '#666' }}>PEP and orchestration service status, auto-refreshes every 5s.</p>
      {statuses.map(s => (
        <div key={s.name} style={{ marginBottom: 12, padding: 12, border: '1px solid #ddd', borderRadius: 4, borderLeft: `4px solid ${s.status === 'UP' ? '#4caf50' : '#f44336'}` }}>
          <strong>{s.name}</strong> — <span style={{ color: s.status === 'UP' ? '#4caf50' : '#f44336' }}>{s.status}</span>
          {s.error && <p style={{ color: '#f44336', fontSize: 13, margin: '4px 0 0' }}>{s.error}</p>}
          {Object.keys(s.details).length > 0 && (
            <pre style={{ fontSize: 12, marginTop: 4, color: '#555' }}>{JSON.stringify(s.details, null, 2)}</pre>
          )}
        </div>
      ))}
    </div>
  )
}
