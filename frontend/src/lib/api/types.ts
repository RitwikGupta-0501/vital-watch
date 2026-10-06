export interface User {
  id: string;
  email: string;
  role: 'patient' | 'doctor' | 'admin';
  first_name: string;
  last_name: string;
  created_at: string;
}

// Additional models will be added as we build out features
