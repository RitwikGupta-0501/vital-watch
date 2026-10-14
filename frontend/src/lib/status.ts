import {
  Calendar,
  CheckCircle2,
  XCircle,
  Building2,
  Video,
  FileSearch,
  AlertCircle,
  ShieldCheck,
  Ban,
  UploadCloud,
  FileSignature,
  Clock,
  Check,
  MinusCircle,
  Sun,
  Sunset,
  Moon,
  BedDouble,
  Pill,
  AlertTriangle,
  Info,
  User,
  Stethoscope,
  Shield,
  Building,
  ShieldAlert,
  type LucideIcon,
} from 'lucide-react';

import type {
  AppointmentStatus,
  AppointmentType,
  PrescriptionStatus,
  PrescriptionSource,
  MedicationLogStatus,
  MedicationTimeOfDay,
  SafetySeverity,
  UserRole,
} from './api/types';

export type StatusTone = 'neutral' | 'info' | 'success' | 'warning' | 'critical';

export interface StatusMeta {
  label: string;
  tone: StatusTone;
  icon: LucideIcon;
  description?: string;
}

export const APPOINTMENT_STATUS_MAP: Record<AppointmentStatus, StatusMeta> = {
  upcoming: {
    label: 'Upcoming',
    tone: 'info',
    icon: Calendar,
    description: 'Scheduled visit',
  },
  completed: {
    label: 'Completed',
    tone: 'success',
    icon: CheckCircle2,
    description: 'Consultation concluded',
  },
  cancelled: {
    label: 'Cancelled',
    tone: 'neutral',
    icon: XCircle,
    description: 'Visit cancelled',
  },
};

export const APPOINTMENT_TYPE_MAP: Record<AppointmentType, StatusMeta> = {
  in_person: {
    label: 'In-Person',
    tone: 'neutral',
    icon: Building2,
    description: 'At clinic location',
  },
  virtual: {
    label: 'Virtual Telehealth',
    tone: 'info',
    icon: Video,
    description: 'Encrypted video call',
  },
};

export const PRESCRIPTION_STATUS_MAP: Record<PrescriptionStatus, StatusMeta> = {
  pending_ocr: {
    label: 'Processing OCR',
    tone: 'warning',
    icon: FileSearch,
    description: 'Extracting medication data via AI pipeline',
  },
  needs_review: {
    label: 'Needs Review',
    tone: 'warning',
    icon: AlertCircle,
    description: 'Awaiting clinician verification',
  },
  approved: {
    label: 'Approved',
    tone: 'success',
    icon: ShieldCheck,
    description: 'Clinically verified & signed',
  },
  rejected: {
    label: 'Rejected',
    tone: 'critical',
    icon: Ban,
    description: 'Disapproved by prescriber',
  },
};

export const PRESCRIPTION_SOURCE_MAP: Record<PrescriptionSource, StatusMeta> = {
  uploaded: {
    label: 'Paper Slip Scan',
    tone: 'neutral',
    icon: UploadCloud,
    description: 'Digitized from physical script',
  },
  digital: {
    label: 'Digital E-Prescription',
    tone: 'info',
    icon: FileSignature,
    description: 'Authored in VitalWatch',
  },
};

export const MEDICATION_LOG_STATUS_MAP: Record<MedicationLogStatus, StatusMeta> = {
  pending: {
    label: 'Due',
    tone: 'neutral',
    icon: Clock,
    description: 'Dose awaiting confirmation',
  },
  taken: {
    label: 'Taken',
    tone: 'success',
    icon: Check,
    description: 'Confirmed administered',
  },
  skipped: {
    label: 'Skipped',
    tone: 'warning',
    icon: MinusCircle,
    description: 'Dose skipped',
  },
};

export const MEDICATION_TIME_OF_DAY_MAP: Record<MedicationTimeOfDay, StatusMeta> = {
  morning: {
    label: 'Morning',
    tone: 'info',
    icon: Sun,
  },
  afternoon: {
    label: 'Afternoon',
    tone: 'warning',
    icon: Sunset,
  },
  evening: {
    label: 'Evening',
    tone: 'info',
    icon: Moon,
  },
  bedtime: {
    label: 'Bedtime',
    tone: 'neutral',
    icon: BedDouble,
  },
  as_needed: {
    label: 'As Needed (PRN)',
    tone: 'neutral',
    icon: Pill,
  },
};

export const SAFETY_SEVERITY_MAP: Record<SafetySeverity, StatusMeta> = {
  HIGH: {
    label: 'High Severity Interrupt',
    tone: 'critical',
    icon: AlertTriangle,
    description: 'Critical interaction requiring clinician override',
  },
  MODERATE: {
    label: 'Moderate Severity Warning',
    tone: 'warning',
    icon: AlertCircle,
    description: 'Clinical caution recommended',
  },
  LOW: {
    label: 'Informational',
    tone: 'info',
    icon: Info,
    description: 'Mild interaction or advisory note',
  },
};

export const USER_ROLE_MAP: Record<UserRole, StatusMeta> = {
  patient: {
    label: 'Patient',
    tone: 'neutral',
    icon: User,
  },
  doctor: {
    label: 'Doctor / Clinician',
    tone: 'info',
    icon: Stethoscope,
  },
  admin: {
    label: 'Administrator',
    tone: 'neutral',
    icon: Shield,
  },
  tenant_admin: {
    label: 'Clinic Admin',
    tone: 'warning',
    icon: Building,
  },
  platform_admin: {
    label: 'Platform Admin',
    tone: 'critical',
    icon: ShieldAlert,
  },
};
