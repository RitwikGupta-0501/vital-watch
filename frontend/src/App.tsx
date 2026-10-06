import { Routes, Route } from 'react-router-dom'
import { PublicLayout } from './components/layout/PublicLayout'
import { PatientLayout } from './components/layout/PatientLayout'
import { DoctorLayout } from './components/layout/DoctorLayout'
import { Landing } from './pages/Landing'
import { Login } from './pages/Login'
import { PatientDashboard } from './pages/patient/Dashboard'
import { DoctorDashboard } from './pages/doctor/Dashboard'

function App() {
  return (
    <Routes>
      {/* Public Routes */}
      <Route element={<PublicLayout />}>
        <Route path="/" element={<Landing />} />
        <Route path="/login" element={<Login />} />
      </Route>

      {/* Patient Routes */}
      <Route path="/patient" element={<PatientLayout />}>
        <Route path="dashboard" element={<PatientDashboard />} />
      </Route>

      {/* Doctor Routes */}
      <Route path="/doctor" element={<DoctorLayout />}>
        <Route path="dashboard" element={<DoctorDashboard />} />
      </Route>
    </Routes>
  )
}

export default App
