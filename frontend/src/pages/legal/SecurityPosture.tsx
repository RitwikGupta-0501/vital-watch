import { Card } from '../../components/ui/Card';
import { ShieldCheck, Key, Database, RefreshCw, Server } from 'lucide-react';

export function SecurityPosture() {
  return (
    <div className="flex-1 bg-surface py-12 px-6">
      <div className="max-w-4xl mx-auto space-y-8">
        <div className="border-b border-border pb-6">
          <div className="inline-flex items-center gap-2 text-xs font-bold text-brand-700 bg-brand-50 px-3 py-1 rounded-full uppercase tracking-wider mb-3">
            <ShieldCheck className="w-4 h-4" />
            <span>Architecture & Encryption</span>
          </div>
          <h1 className="text-3xl md:text-4xl font-bold tracking-tight text-slate-900">
            VitalWatch Security Architecture
          </h1>
          <p className="mt-2 text-sm text-slate-500">
            Defense-in-depth security model adhering to HIPAA Security Rule 45 CFR Part 164 Subpart C
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <Card className="p-6 bg-white border border-border rounded-xl space-y-3">
            <div className="w-10 h-10 rounded-lg bg-brand-50 text-brand-700 flex items-center justify-center">
              <Key className="w-5 h-5" />
            </div>
            <h3 className="font-bold text-slate-900">Session Authentication & Token Rotation</h3>
            <p className="text-xs text-slate-600 leading-relaxed">
              15-minute ephemeral access JWTs coupled with 7-day single-use rotating refresh tokens.
              Any duplicate presentation of a rotated token immediately revokes all active sessions across all devices
              and records a high-severity security alert.
            </p>
          </Card>

          <Card className="p-6 bg-white border border-border rounded-xl space-y-3">
            <div className="w-10 h-10 rounded-lg bg-brand-50 text-brand-700 flex items-center justify-center">
              <Database className="w-5 h-5" />
            </div>
            <h3 className="font-bold text-slate-900">Envelope Encryption & BYOK Vault</h3>
            <p className="text-xs text-slate-600 leading-relaxed">
              Clinic AI provider keys (OpenAI, Claude, Gemini) are stored using AES-256-GCM envelope encryption
              with Additional Authenticated Data (AAD) bound to the tenant ID. Keys are never transmitted in plaintext.
            </p>
          </Card>

          <Card className="p-6 bg-white border border-border rounded-xl space-y-3">
            <div className="w-10 h-10 rounded-lg bg-brand-50 text-brand-700 flex items-center justify-center">
              <RefreshCw className="w-5 h-5" />
            </div>
            <h3 className="font-bold text-slate-900">Presigned Storage & Short TTL</h3>
            <p className="text-xs text-slate-600 leading-relaxed">
              Prescription scans and clinical PDFs are served via cryptographically signed presigned URLs
              with a strict 300-second time-to-live (TTL). Raw medical files are never public or cached permanently.
            </p>
          </Card>

          <Card className="p-6 bg-white border border-border rounded-xl space-y-3">
            <div className="w-10 h-10 rounded-lg bg-brand-50 text-brand-700 flex items-center justify-center">
              <Server className="w-5 h-5" />
            </div>
            <h3 className="font-bold text-slate-900">PostgreSQL Row-Level Security (RLS)</h3>
            <p className="text-xs text-slate-600 leading-relaxed">
              Multi-tenancy isolation is enforced at both the database level via PostgreSQL Row-Level Security (RLS)
              and application middleware, guaranteeing that queries cannot leak data across tenant boundaries.
            </p>
          </Card>
        </div>
      </div>
    </div>
  );
}
