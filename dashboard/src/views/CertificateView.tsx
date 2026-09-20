import { useEffect, useState } from 'react'
import { pip, ca, type PIPSubject, type PIPAttributes } from '../api'

export default function CertificateView() {
  const [subjects, setSubjects] = useState<PIPSubject[]>([])
  const [count, setCount] = useState(0)
  const [lookupCN, setLookupCN] = useState('')
  const [lookupResult, setLookupResult] = useState<PIPAttributes | null>(null)
  const [lookupError, setLookupError] = useState('')
  const [actionMsg, setActionMsg] = useState('')

  const refresh = () => {
    pip.subjects().then(r => { setSubjects(r.subjects); setCount(r.count) }).catch(() => {})
  }

  useEffect(() => { refresh() }, [])

  const lookup = () => {
    if (!lookupCN) return
    setLookupError('')
    setLookupResult(null)
    pip.attributes(lookupCN)
      .then(setLookupResult)
      .catch(e => setLookupError(String(e)))
  }

  const revoke = (cn: string) => {
    ca.revoke(cn).then(() => { setActionMsg(`Revoked: ${cn}`); refresh() }).catch(e => setActionMsg(`Error: ${e}`))
  }

  const reissue = (cn: string) => {
    ca.reissue(cn).then(() => { setActionMsg(`Reissued: ${cn}`); refresh() }).catch(e => setActionMsg(`Error: ${e}`))
  }

  return (
    <div>
      <h2>Certificate Registry ({count} total)</h2>

      <div style={{ marginBottom: 16, padding: 12, backgroundColor: '#f5f5f5', borderRadius: 4 }}>
        <strong>Lookup attributes:</strong>{' '}
        <input value={lookupCN} onChange={e => setLookupCN(e.target.value)} placeholder="Common Name" style={{ marginRight: 8, padding: 4 }} />
        <button onClick={lookup}>Query PIP</button>
        {lookupResult && (
          <pre style={{ marginTop: 8, fontSize: 13 }}>{JSON.stringify(lookupResult, null, 2)}</pre>
        )}
        {lookupError && <p style={{ color: '#f44336', marginTop: 8 }}>{lookupError}</p>}
      </div>

      {actionMsg && <p style={{ color: '#1976d2', marginBottom: 8 }}>{actionMsg}</p>}

      <table style={{ borderCollapse: 'collapse', width: '100%', fontSize: 13 }}>
        <thead>
          <tr>
            {['CN', 'Profile', 'Valid', 'Revoked', 'Issued', 'Expires', 'Actions'].map(h => (
              <th key={h} style={{ textAlign: 'left', padding: '6px 8px', borderBottom: '2px solid #ddd' }}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {subjects.map(s => (
            <tr key={s.cn} style={{ backgroundColor: s.revoked ? '#fff3f3' : undefined }}>
              <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee', fontFamily: 'monospace' }}>{s.cn}</td>
              <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee' }}>{s.ou}</td>
              <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee' }}>
                <span style={{ color: s.valid ? '#4caf50' : '#f44336' }}>{s.valid ? 'YES' : 'NO'}</span>
              </td>
              <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee' }}>{s.revoked ? 'YES' : ''}</td>
              <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee' }}>{new Date(s.issuedAt).toLocaleString()}</td>
              <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee' }}>{new Date(s.expiresAt).toLocaleString()}</td>
              <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee' }}>
                {s.revoked
                  ? <button onClick={() => reissue(s.cn)} style={{ fontSize: 12 }}>Reissue</button>
                  : <button onClick={() => revoke(s.cn)} style={{ fontSize: 12, color: '#f44336' }}>Revoke</button>
                }
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <button onClick={refresh} style={{ marginTop: 8 }}>Refresh</button>
    </div>
  )
}
