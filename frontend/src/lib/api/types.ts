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
