import { Routes, Route } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AuthProvider } from './context/AuthContext';
import { useAuth } from './context/useAuth';
import { useNotificationStream } from './hooks/useNotificationStream';
import { ReconnectingBanner } from './components/ui/ReconnectingBanner';
import { ProtectedRoute } from './components/auth/ProtectedRoute';

import { PublicLayout } from './components/layout/PublicLayout';
import { PatientLayout } from './components/layout/PatientLayout';
import { DoctorLayout } from './components/layout/DoctorLayout';

import { Landing } from './pages/Landing';
import { Login } from './pages/Login';
import { Register } from './pages/Register';
import { NoticeOfPrivacyPractices } from './pages/legal/NoticeOfPrivacyPractices';
import { TermsOfService } from './pages/legal/TermsOfService';
import { SecurityPosture } from './pages/legal/SecurityPosture';
import { VerifyRx } from './pages/VerifyRx';

import { PatientDashboard } from './pages/patient/Dashboard';
import { DoctorDirectory } from './pages/patient/DoctorDirectory';
import { DoctorDetail } from './pages/patient/DoctorDetail';
import { BookingConfirm } from './pages/patient/BookingConfirm';
import { AppointmentsList } from './pages/patient/AppointmentsList';
import { PrescriptionsList } from './pages/patient/PrescriptionsList';
import { PrescriptionDetail } from './pages/patient/PrescriptionDetail';

import { DoctorDashboard } from './pages/doctor/Dashboard';
import { TodayTimeline } from './pages/doctor/TodayTimeline';
import { DoctorAppointmentsList } from './pages/doctor/AppointmentsList';
import { DoctorPatientRoster } from './pages/doctor/PatientRoster';
import { DoctorPatientChart } from './pages/doctor/PatientChart';
import { EmptyState } from './components/ui/EmptyState';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 0, // Invalidate PHI queries according to HIPAA data hygiene
      retry: 1,
    },
  },
});

function GlobalStreamBanner() {
  const { isAuthenticated } = useAuth();
  const { status, reconnect } = useNotificationStream({ enabled: isAuthenticated });

  if (!isAuthenticated) return null;
  return <ReconnectingBanner status={status} onRetry={reconnect} />;
}

function NotFound() {
  return (
    <div className="flex-1 flex items-center justify-center p-8 bg-surface">
      <EmptyState
        variant="error"
        title="Page Not Found (404)"
        description="The clinical surface or resource you are looking for does not exist or has been moved."
        actionLabel="Back to Home"
        onAction={() => (window.location.href = '/')}
      />
    </div>
  );
}

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <GlobalStreamBanner />
        <Routes>
          {/* Public Routes with standard chrome */}
          <Route element={<PublicLayout />}>
            <Route path="/" element={<Landing />} />
            <Route path="/login" element={<Login />} />
            <Route path="/register" element={<Register />} />
            <Route path="/notice-of-privacy-practices" element={<NoticeOfPrivacyPractices />} />
            <Route path="/terms" element={<TermsOfService />} />
            <Route path="/security" element={<SecurityPosture />} />
          </Route>

          {/* Standalone Public Verification (Bare layout for QR authenticity) */}
          <Route path="/verify/rx/:id" element={<VerifyRx />} />

          {/* Patient Routes */}
          <Route
            path="/patient"
            element={
              <ProtectedRoute allowedRoles={['patient']}>
                <PatientLayout />
              </ProtectedRoute>
            }
          >
            <Route path="dashboard" element={<PatientDashboard />} />
            <Route path="doctors" element={<DoctorDirectory />} />
            <Route path="doctors/:id" element={<DoctorDetail />} />
            <Route path="doctors/:id/book" element={<BookingConfirm />} />
            <Route path="appointments" element={<AppointmentsList />} />
            <Route path="prescriptions" element={<PrescriptionsList />} />
            <Route path="prescriptions/:id" element={<PrescriptionDetail />} />
          </Route>

          {/* Doctor Routes */}
          <Route
            path="/doctor"
            element={
              <ProtectedRoute allowedRoles={['doctor']}>
                <DoctorLayout />
              </ProtectedRoute>
            }
          >
            <Route index element={<DoctorDashboard />} />
            <Route path="dashboard" element={<DoctorDashboard />} />
            <Route path="today" element={<TodayTimeline />} />
            <Route path="appointments" element={<DoctorAppointmentsList />} />
            <Route path="patients" element={<DoctorPatientRoster />} />
            <Route path="patients/:id" element={<DoctorPatientChart />} />
            <Route path="patients/:id/record-vitals" element={<DoctorPatientChart autoOpenVitals />} />
          </Route>

          {/* 404 Catch-All */}
          <Route path="*" element={<NotFound />} />
        </Routes>
      </AuthProvider>
    </QueryClientProvider>
  );
}

export default App;
