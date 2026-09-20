import { useState } from 'react'
import HealthView from './views/HealthView'
import CertificateView from './views/CertificateView'
import PolicyView from './views/PolicyView'
import LiveDataView from './views/LiveDataView'
import ExternalToolsView from './views/ExternalToolsView'

type Tab = 'health' | 'certificates' | 'policy' | 'livedata' | 'tools'

const TABS: { key: Tab; label: string }[] = [
  { key: 'health', label: 'Health' },
  { key: 'certificates', label: 'Certificates' },
  { key: 'policy', label: 'Policies' },
  { key: 'livedata', label: 'Enforcement' },
  { key: 'tools', label: 'Tools' },
]

export default function App() {
  const [tab, setTab] = useState<Tab>('health')

  return (
    <div style={{ fontFamily: 'system-ui, -apple-system, sans-serif', maxWidth: 1000, margin: '0 auto', padding: '1rem' }}>
      <h1 style={{ fontSize: 22, marginBottom: 4 }}>Arrowhead 5.2 ADAPI Dashboard</h1>
      <p style={{ fontSize: 13, color: '#666', marginTop: 0 }}>gRPC PDP + CA-as-PIP + Multi-Transport Enforcement</p>
      <nav style={{ display: 'flex', gap: 4, marginBottom: 16, borderBottom: '2px solid #eee', paddingBottom: 8 }}>
        {TABS.map(t => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            style={{
              padding: '6px 16px',
              border: 'none',
              borderRadius: '4px 4px 0 0',
              backgroundColor: tab === t.key ? '#1976d2' : '#f5f5f5',
              color: tab === t.key ? '#fff' : '#333',
              fontWeight: tab === t.key ? 600 : 400,
              cursor: 'pointer',
              fontSize: 14,
            }}
          >
            {t.label}
          </button>
        ))}
      </nav>
      <main>
        {tab === 'health' && <HealthView />}
        {tab === 'certificates' && <CertificateView />}
        {tab === 'policy' && <PolicyView />}
        {tab === 'livedata' && <LiveDataView />}
        {tab === 'tools' && <ExternalToolsView />}
      </main>
    </div>
  )
}
