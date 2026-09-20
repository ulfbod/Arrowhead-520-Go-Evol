import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api/profile-ca': { target: 'http://localhost:8787', rewrite: p => p.replace('/api/profile-ca', '') },
      '/api/pip': { target: 'http://localhost:8787', rewrite: p => p.replace('/api/pip', '/pip') },
      '/api/kafka-authz': { target: 'http://localhost:9101', rewrite: p => p.replace('/api/kafka-authz', '') },
      '/api/topic-auth-xacml': { target: 'http://localhost:9090', rewrite: p => p.replace('/api/topic-auth-xacml', '') },
      '/api/pki-rest-authz': { target: 'http://localhost:9209', rewrite: p => p.replace('/api/pki-rest-authz', '') },
      '/api/pap': { target: 'http://localhost:9505', rewrite: p => p.replace('/api/pap', '') },
      '/api/authzforce': { target: 'http://localhost:8096', rewrite: p => p.replace('/api/authzforce', '/authzforce-ce') },
      '/api/authz-pdp': { target: 'http://localhost:9550', rewrite: p => p.replace('/api/authz-pdp', '') },
      '/api/dynamicorch': { target: 'http://localhost:8083', rewrite: p => p.replace('/api/dynamicorch', '') },
    },
  },
})
