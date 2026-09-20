export default function ExternalToolsView() {
  const tools = [
    { name: 'RabbitMQ Management', url: 'http://localhost:15672', description: 'Broker admin UI (admin/admin). View queues, exchanges, connections, and channels.', port: 15672 },
    { name: 'Kafdrop', url: 'http://localhost:9000', description: 'Kafka topic browser. View topics, partitions, messages, and consumer groups.', port: 9000 },
  ]

  return (
    <div>
      <h2>External Tools</h2>
      <p style={{ fontSize: 13, color: '#666' }}>Built-in UIs from infrastructure services.</p>
      {tools.map(t => (
        <div key={t.name} style={{ marginBottom: 12, padding: 12, border: '1px solid #ddd', borderRadius: 4 }}>
          <strong>{t.name}</strong> — port {t.port}
          <p style={{ fontSize: 13, color: '#555', margin: '4px 0' }}>{t.description}</p>
          <a href={t.url} target="_blank" rel="noopener noreferrer" style={{ color: '#1976d2' }}>
            Open {t.name} &#8599;
          </a>
        </div>
      ))}
    </div>
  )
}
