import { Card } from '../../components/ui/Card';
import { AlertTriangle, CheckSquare } from 'lucide-react';

export function TermsOfService() {
  return (
    <div className="flex-1 bg-surface py-12 px-6">
      <div className="max-w-4xl mx-auto space-y-8">
        <div className="border-b border-border pb-6">
          <h1 className="text-3xl md:text-4xl font-bold tracking-tight text-slate-900">
            Terms of Clinical Service
          </h1>
          <p className="mt-2 text-sm text-slate-500">
            Last Updated: October 2026 • VitalWatch Clinical Terms
          </p>
        </div>

        <Card className="p-8 space-y-6 text-slate-700 leading-relaxed bg-white border border-border rounded-2xl">
          <section className="space-y-3">
            <h2 className="text-xl font-bold text-slate-900 flex items-center gap-2">
              <AlertTriangle className="w-5 h-5 text-amber-600" />
              1. Not for Acute Medical Emergencies
            </h2>
            <div className="p-4 bg-amber-50 border border-amber-200 rounded-xl text-amber-900 text-sm">
              If you are experiencing a life-threatening medical emergency, call 911 (or your local emergency
              services) immediately. VitalWatch telehealth, vitals tracking, and messaging services are not intended
              for urgent, real-time crisis dispatch.
            </div>
          </section>

          <section className="space-y-3">
            <h2 className="text-xl font-bold text-slate-900 flex items-center gap-2">
              <CheckSquare className="w-5 h-5 text-brand-600" />
              2. Clinician Autonomy and Responsibility
            </h2>
            <p className="text-sm">
              All digital prescriptions, OCR extractions, and drug interaction screenings generated or supported
              by VitalWatch are decision-support tools. Final prescribing authority, clinical verification, and
              therapeutic decisions reside strictly with licensed, credentialed medical practitioners.
            </p>
          </section>

          <section className="space-y-3">
            <h2 className="text-xl font-bold text-slate-900">
              3. Telehealth & Account Integrity
            </h2>
            <p className="text-sm">
              Users agree to provide accurate medical information. Telehealth rooms are end-to-end encrypted
              and accessible only within designated consult time windows. Session authentication tokens are
              single-use rotating tokens; account sharing or scraping is strictly prohibited.
            </p>
          </section>
        </Card>
      </div>
    </div>
  );
}
