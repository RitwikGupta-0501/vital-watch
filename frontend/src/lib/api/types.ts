// Enums directly mirrored from backend/internal/models/models.go:10-60

export type AppointmentStatus = 'upcoming' | 'completed' | 'cancelled';

export type AppointmentType = 'in_person' | 'virtual';

export type PrescriptionStatus = 'pending_ocr' | 'needs_review' | 'approved' | 'rejected';

export type PrescriptionSource = 'uploaded' | 'digital';

export type MedicationLogStatus = 'pending' | 'taken' | 'skipped';

export type MedicationTimeOfDay = 'morning' | 'afternoon' | 'evening' | 'bedtime' | 'as_needed';

export type SafetySeverity = 'HIGH' | 'MODERATE' | 'LOW';

export type UserRole = 'patient' | 'doctor' | 'admin' | 'tenant_admin' | 'platform_admin';

// User Profiles
export interface BaseUserProfile {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  role: UserRole;
  tenant_id: string;
  created_at: string;
  updated_at?: string;
}

export interface PatientProfile extends BaseUserProfile {
  role: 'patient';
}

export interface DoctorProfile extends BaseUserProfile {
  role: 'doctor';
  specialty: string;
  experience: number;
  available: boolean;
}

export interface AdminProfile extends BaseUserProfile {
  role: 'admin' | 'tenant_admin' | 'platform_admin';
  department?: string;
}

export type UserProfile = PatientProfile | DoctorProfile | AdminProfile;

// Backward-compatibility alias
export type User = UserProfile;

// Authentication Request / Response Types
export interface LoginRequest {
  role: string;
  email: string;
  password: string;
}

export interface LoginResponse {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expires_in: number;
  token: string;
  role: string;
}

export interface RegisterPatientRequest {
  role: 'patient';
  first_name: string;
  last_name: string;
  email: string;
  password: string;
}

export interface RegisterDoctorRequest {
  role: 'doctor';
  first_name: string;
  last_name: string;
  email: string;
  password: string;
  specialty: string;
  experience: number;
  invite_code: string;
}

export interface RegisterAdminRequest {
  role: 'admin' | 'platform_admin';
  first_name: string;
  last_name: string;
  email: string;
  password: string;
  invite_code: string;
}

export type RegisterRequest = RegisterPatientRequest | RegisterDoctorRequest | RegisterAdminRequest;

export interface RegisterResponse {
  id: string;
}

export interface LogoutRequest {
  all_devices?: boolean;
  refresh_token?: string;
}

export interface LogoutResponse {
  message: string;
}

// Envelope & Error Types
export interface ApiErrorResponse {
  error?: string;
  message?: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  limit: number;
  offset: number;
}

// Doctor Directory & Profiles
export interface Doctor {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  specialty: string;
  experience_years?: number;
  experience?: number;
  available: boolean;
  tenant_id?: string;
  created_at?: string;
}

// Slot Generation
export interface TimeSlot {
  start_time: string;
  end_time: string;
  available: boolean;
}

export interface DoctorAvailableSlotsResponse {
  date: string;
  day_of_week: number;
  slot_duration: number;
  timezone: string;
  slots: TimeSlot[];
  message?: string;
}

// Clinical Appointments
export interface Appointment {
  id: string;
  doctor_id: string;
  patient_id: string;
  start_time: string;
  end_time: string;
  status: AppointmentStatus;
  type: AppointmentType;
  meeting_link?: string;
  meeting_id?: string;
  doctor_name?: string;
  doctor_specialty?: string;
  patient_name?: string;
  created_at: string;
  updated_at?: string;
}

export interface CreateAppointmentRequest {
  doctor_id: string;
  start_time: string;
  end_time: string;
  type: AppointmentType;
}

export interface CreateAppointmentResponse {
  id: string;
  meeting_link?: string;
  meeting_id?: string;
}

// Clinical Prescriptions
export interface PrescriptionItem {
  id: string;
  prescription_id: string;
  medication_name: string;
  dosage?: string;
  frequency?: string;
  duration?: string;
  timing?: string;
  instructions?: string;
  created_at: string;
}

export interface Prescription {
  id: string;
  patient_id: string;
  doctor_id: string;
  source: PrescriptionSource;
  status: PrescriptionStatus;
  file_name?: string;
  notes: string;
  ocr_provider?: string;
  items: PrescriptionItem[];
  expires_at?: string;
  created_at: string;
  updated_at: string;
  doctor_name?: string;
  patient_name?: string;
}

export interface PrescriptionDownloadResponse {
  download_url: string;
  file_name: string;
  expires_in: number;
}

// Public Authenticity Verification (A4)
export interface PublicRxVerification {
  id: string;
  status: PrescriptionStatus;
  created_at: string;
  doctor_id: string;
  doctor_name?: string;
  doctor_specialty?: string;
  masked_patient_id: string;
  notes?: string;
  items: PrescriptionItem[];
  verified?: boolean;
}

// Doctor Workspace & Clinical Roster (M3)
export interface DoctorPatient {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  role: 'patient';
  tenant_id: string;
  created_at: string;
  updated_at?: string;
}

export interface DoctorSchedule {
  id: string;
  doctor_id: string;
  day_of_week: number; // 0=Sunday, 1=Monday, ..., 6=Saturday
  start_time: string; // "HH:MM" e.g. "09:00"
  end_time: string; // "HH:MM" e.g. "17:00"
  slot_duration: number; // 15 | 20 | 30 | 45 | 60
  timezone: string;
  is_active: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface DoctorScheduleInput {
  day_of_week: number;
  start_time: string;
  end_time: string;
  slot_duration: number;
  timezone: string;
  is_active?: boolean;
}

export interface InteractionAlert {
  drug1: string;
  drug2: string;
  severity: SafetySeverity;
  description: string;
  mechanism?: string;
}

export interface SafetyReport {
  has_high_severity_alerts: boolean;
  interaction_alerts: InteractionAlert[];
  allergy_alerts: unknown[];
  service_degraded: boolean;
  degraded_reason?: string;
  unchecked_drugs?: string[];
}

export interface CreateDigitalPrescriptionRequest {
  patient_id: string;
  notes?: string;
  override_safety?: boolean;
  override_reason?: string;
  items: {
    medication_name: string;
    dosage?: string;
    frequency?: string;
    duration?: string;
    timing?: string;
    instructions?: string;
  }[];
}

export interface CreateDigitalPrescriptionResponse {
  id: string;
  source: 'digital';
  status: 'approved';
}

export interface PatientVital {
  id: string;
  patient_id: string;
  recorder_id: string;
  recorder_role: UserRole;
  recorded_at: string;
  systolic_bp?: number;
  diastolic_bp?: number;
  heart_rate?: number;
  blood_glucose?: number;
  oxygen_saturation?: number;
  temperature?: number;
  weight_kg?: number;
  notes?: string;
  is_critical?: boolean;
  critical_alerts?: string[];
  created_at: string;
}

export interface VitalInput {
  patient_id: string;
  recorded_at?: string;
  systolic_bp?: number;
  diastolic_bp?: number;
  heart_rate?: number;
  blood_glucose?: number;
  oxygen_saturation?: number;
  temperature?: number;
  weight_kg?: number;
  notes?: string;
}

