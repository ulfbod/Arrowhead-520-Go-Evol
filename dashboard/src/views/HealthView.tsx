import { useEffect, useState } from 'react'
import { getHealth, type HealthResponse } from '../api'

interface ServiceStatus {
  name: string
  path: string
  status: HealthResponse | null
  error: string | null
}

const SERVICES = [
  { name: 'Profile-CA', path: '/api/profile-ca/health' },
  { name: 'PIP (CA-as-PIP)', path: '/api/pip/health' },
  { name: 'PAP', path: '/api/pap/health' },
  { name: 'AuthzForce', path: '/api/authzforce/domains' },
  { name: 'DynamicOrch-XACML', path: '/api/dynamicorch/health' },
  { name: 'kafka-authz', path: '/api/kafka-authz/health' },
  { name: 'pki-rest-authz', path: '/api/pki-rest-authz/health' },
]

function StatusDot({ ok }: { ok: boolean | null }) {
  const color = ok === null ? '#999' : ok ? '#4caf50' : '#f44336'
  return <span style={{ display: 'inline-block', width: 12, height: 12, borderRadius: '50%', backgroundColor: color, marginRight: 8 }} />
}

export default function HealthView() {
  const [services, setServices] = useState<ServiceStatus[]>(
    SERVICES.map(s => ({ ...s, status: null, error: null }))
  )

  useEffect(() => {
    const check = () => {
      SERVICES.forEach((svc, i) => {
        getHealth(svc.path)
          .then(status => setServices(prev => {
            const next = [...prev]
            next[i] = { ...svc, status, error: null }
            return next
          }))
          .catch(err => setServices(prev => {
            const next = [...prev]
            next[i] = { ...svc, status: null, error: String(err) }
            return next
          }))
      })
    }
    check()
    const id = setInterval(check, 5000)
    return () => clearInterval(id)
  }, [])

  return (
    <div>
      <h2>Service Health</h2>
      <table style={{ borderCollapse: 'collapse', width: '100%' }}>
        <thead>
          <tr>
            <th style={{ textAlign: 'left', padding: '8px', borderBottom: '1px solid #ddd' }}>Service</th>
            <th style={{ textAlign: 'left', padding: '8px', borderBottom: '1px solid #ddd' }}>Status</th>
          </tr>
        </thead>
        <tbody>
          {services.map(svc => (
            <tr key={svc.name}>
              <td style={{ padding: '8px', borderBottom: '1px solid #eee' }}>
                <StatusDot ok={svc.error ? false : svc.status ? true : null} />
                {svc.name}
              </td>
              <td style={{ padding: '8px', borderBottom: '1px solid #eee', fontFamily: 'monospace', fontSize: 13 }}>
                {svc.error ? <span style={{ color: '#f44336' }}>DOWN</span> : svc.status?.status ?? 'checking...'}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <p style={{ fontSize: 12, color: '#999', marginTop: 8 }}>Auto-refreshes every 5s</p>
    </div>
  )
}
