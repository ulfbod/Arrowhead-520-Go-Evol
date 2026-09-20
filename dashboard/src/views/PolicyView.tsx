import { useEffect, useState } from 'react'
import { pap, type Policy } from '../api'

export default function PolicyView() {
  const [policies, setPolicies] = useState<Policy[]>([])
  const [count, setCount] = useState(0)
  const [newPolicy, setNewPolicy] = useState({ subject: '', resource: '', action: 'consume', effect: 'Permit' })
  const [msg, setMsg] = useState('')

  const refresh = () => {
    pap.list().then(r => { setPolicies(r.policies); setCount(r.count) }).catch(() => {})
  }

  useEffect(() => { refresh() }, [])

  const create = () => {
    if (!newPolicy.subject || !newPolicy.resource) { setMsg('Subject and resource required'); return }
    pap.create(newPolicy)
      .then(() => { setMsg('Policy created'); setNewPolicy({ subject: '', resource: '', action: 'consume', effect: 'Permit' }); refresh() })
      .catch(e => setMsg(`Error: ${e}`))
  }

  return (
    <div>
      <h2>XACML Policies ({count})</h2>

      <div style={{ marginBottom: 16, padding: 12, backgroundColor: '#f5f5f5', borderRadius: 4 }}>
        <strong>Create policy:</strong>
        <div style={{ display: 'flex', gap: 8, marginTop: 8, flexWrap: 'wrap' }}>
          <input placeholder="Subject" value={newPolicy.subject} onChange={e => setNewPolicy(p => ({ ...p, subject: e.target.value }))} style={{ padding: 4 }} />
          <input placeholder="Resource" value={newPolicy.resource} onChange={e => setNewPolicy(p => ({ ...p, resource: e.target.value }))} style={{ padding: 4 }} />
          <select value={newPolicy.action} onChange={e => setNewPolicy(p => ({ ...p, action: e.target.value }))}>
            <option>consume</option>
            <option>orchestrate</option>
            <option>invoke</option>
            <option>subscribe</option>
          </select>
          <select value={newPolicy.effect} onChange={e => setNewPolicy(p => ({ ...p, effect: e.target.value }))}>
            <option>Permit</option>
            <option>Deny</option>
          </select>
          <button onClick={create}>Add</button>
        </div>
        {msg && <p style={{ marginTop: 8, color: msg.startsWith('Error') ? '#f44336' : '#1976d2' }}>{msg}</p>}
      </div>

      <table style={{ borderCollapse: 'collapse', width: '100%', fontSize: 13 }}>
        <thead>
          <tr>
            {['ID', 'Subject', 'Resource', 'Action', 'Effect', 'Created'].map(h => (
              <th key={h} style={{ textAlign: 'left', padding: '6px 8px', borderBottom: '2px solid #ddd' }}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {policies.map(p => (
            <tr key={p.id}>
              <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee', fontFamily: 'monospace' }}>{p.id}</td>
              <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee' }}>{p.subject}</td>
              <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee' }}>{p.resource}</td>
              <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee' }}>{p.action}</td>
              <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee', color: p.effect === 'Permit' ? '#4caf50' : '#f44336' }}>{p.effect}</td>
              <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee' }}>{new Date(p.createdAt).toLocaleString()}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <button onClick={refresh} style={{ marginTop: 8 }}>Refresh</button>
    </div>
  )
}
