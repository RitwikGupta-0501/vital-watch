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

import { PatientDashboard } from './pages/patient/Dashboard';
import { DoctorDashboard } from './pages/doctor/Dashboard';
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
          {/* Public Routes */}
          <Route element={<PublicLayout />}>
            <Route path="/" element={<Landing />} />
            <Route path="/login" element={<Login />} />
            <Route path="/register" element={<Register />} />
            <Route path="/notice-of-privacy-practices" element={<NoticeOfPrivacyPractices />} />
            <Route path="/terms" element={<TermsOfService />} />
            <Route path="/security" element={<SecurityPosture />} />
          </Route>

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
            <Route path="dashboard" element={<DoctorDashboard />} />
          </Route>

          {/* 404 Catch-All */}
          <Route path="*" element={<NotFound />} />
        </Routes>
      </AuthProvider>
    </QueryClientProvider>
  );
}

export default App;
