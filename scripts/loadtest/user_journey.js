import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { randomString } from 'https://jslib.k6.io/k6-utils/1.2.0/index.js';

const targetVUs = __ENV.TARGET_VUS ? parseInt(__ENV.TARGET_VUS) : 20;

export const options = {
  stages: [
    { duration: '15s', target: targetVUs }, 
    { duration: '30s', target: targetVUs },
    { duration: '15s', target: 0 },
  ],
  thresholds: {
    'http_req_duration': ['p(95)<500'], // 500ms for complex business logic
    'http_req_failed': ['rate<0.05'],   // Max 5% error rate
  },
};

const BASE_URL = 'http://localhost:8000/api';

export default function () {
  // 1. Patient Registration
  const email = `test_${randomString(8)}@example.com`;
  const password = 'VerySecurePass123!#';
  
  const registerPayload = JSON.stringify({
    first_name: 'Test',
    last_name: 'Patient',
    email: email,
    password: password,
    role: 'patient',
  });

  const registerRes = http.post(`${BASE_URL}/register`, registerPayload, {
    headers: { 'Content-Type': 'application/json' },
  });

  check(registerRes, {
    'registered successfully': (r) => r.status === 201,
  });

  sleep(1);

  // 2. Patient Login
  const loginPayload = JSON.stringify({
    role: 'patient',
    email: email,
    password: password,
  });

  const loginRes = http.post(`${BASE_URL}/login`, loginPayload, {
    headers: { 'Content-Type': 'application/json' },
  });

  check(loginRes, {
    'logged in successfully': (r) => r.status === 200,
  });

  const token = loginRes.json('token');
  if (!token) {
    return; // Exit iteration if login failed
  }

  const authHeaders = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
  };

  sleep(1);

  // 3. View Profile
  const profileRes = http.get(`${BASE_URL}/profile`, authHeaders);
  check(profileRes, {
    'profile loaded': (r) => r.status === 200,
  });

  sleep(1);

  // 4. View Doctors
  const doctorsRes = http.get(`${BASE_URL}/doctors`, authHeaders);
  check(doctorsRes, {
    'doctors loaded': (r) => r.status === 200,
  });

  sleep(1);
}
