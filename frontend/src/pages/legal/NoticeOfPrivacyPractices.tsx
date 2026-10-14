import { ShieldCheck, Lock, FileText } from 'lucide-react';
import { Card } from '../../components/ui/Card';

export function NoticeOfPrivacyPractices() {
  return (
    <div className="flex-1 bg-surface py-12 px-6">
      <div className="max-w-4xl mx-auto space-y-8">
        <div className="border-b border-border pb-6">
          <div className="inline-flex items-center gap-2 text-xs font-bold text-brand-700 bg-brand-50 px-3 py-1 rounded-full uppercase tracking-wider mb-3">
            <ShieldCheck className="w-4 h-4" />
            <span>HIPAA Compliance Document</span>
          </div>
          <h1 className="text-3xl md:text-4xl font-bold tracking-tight text-slate-900">
            Notice of Privacy Practices (NPP)
          </h1>
          <p className="mt-2 text-sm text-slate-500">
            Effective Date: October 2026 • VitalWatch Health Data Governance
          </p>
        </div>

        <Card className="p-8 space-y-6 text-slate-700 leading-relaxed bg-white border border-border rounded-2xl">
          <section className="space-y-3">
            <h2 className="text-xl font-bold text-slate-900 flex items-center gap-2">
              <Lock className="w-5 h-5 text-brand-600" />
              1. Our Pledge Regarding Medical Information
            </h2>
            <p className="text-sm">
              VitalWatch is committed to protecting medical information about you. We create an immutable,
              cryptographically secured record of the care and services you receive to provide quality care
              and comply with federal and state legal requirements.
            </p>
          </section>

          <section className="space-y-3">
            <h2 className="text-xl font-bold text-slate-900 flex items-center gap-2">
              <FileText className="w-5 h-5 text-brand-600" />
              2. How We May Use and Disclose Protected Health Information (PHI)
            </h2>
            <ul className="list-disc pl-5 space-y-2 text-sm">
              <li>
                <strong>Treatment:</strong> Providing, coordinating, or managing your healthcare with doctors,
                pharmacists, and clinical specialists.
              </li>
              <li>
                <strong>Clinical Safety Interrupts:</strong> Cross-referencing prescription drugs against OpenFDA
                drug-drug interaction (DDI) databases using deterministic, rule-based screening.
              </li>
              <li>
                <strong>Human-in-the-Loop OCR Verification:</strong> Prescription images processed via OCR are
                segregated, transiently analyzed, and strictly subject to mandatory clinician review and sign-off.
              </li>
              <li>
                <strong>Audit & Compliance:</strong> Every access to patient records is written to an append-only,
                tamper-evident audit log recording timestamp, actor identity, IP address, and resource accessed.
              </li>
            </ul>
          </section>

          <section className="space-y-3">
            <h2 className="text-xl font-bold text-slate-900">
              3. Your Rights Regarding Your PHI
            </h2>
            <p className="text-sm">
              Under 45 CFR § 164.524 and § 164.526, you have the right to inspect and receive an electronic copy
              of your electronic health records, request restrictions on disclosures, and view verified digital
              prescriptions issued to you.
            </p>
          </section>
        </Card>
      </div>
    </div>
  );
}
