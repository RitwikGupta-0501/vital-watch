import urllib.request
import urllib.error
import json
import uuid

BASE_URL = "http://localhost:8000/api"

def post(url, payload):
    req = urllib.request.Request(url, data=json.dumps(payload).encode('utf-8'), headers={'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req) as response:
            return response.status, json.loads(response.read().decode('utf-8'))
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read().decode('utf-8'))

# 1. Register a new patient
email = f"test_{uuid.uuid4().hex[:8]}@example.com"
password = "VerySecurePass123!#"

print(f"[*] Registering user: {email}")
status, res = post(f"{BASE_URL}/register", {
    "first_name": "Bug",
    "last_name": "Tester",
    "email": email,
    "password": password,
    "role": "patient"
})
assert status == 201, f"Failed to register: {res}"

# 2. Login to get initial tokens
print("[*] Logging in...")
status, tokens = post(f"{BASE_URL}/login", {
    "email": email,
    "password": password,
    "role": "patient"
})
assert status == 200, f"Failed to login: {tokens}"
refresh_token_1 = tokens["refresh_token"]
print(f"    -> Got initial refresh token: {refresh_token_1[:15]}...")

# 3. Normal rotation (simulates the first request of a retry race)
print("[*] Rotating token (Request A)...")
status, tokens_A = post(f"{BASE_URL}/auth/refresh", {"refresh_token": refresh_token_1})
assert status == 200, f"Failed to rotate token: {tokens_A}"
refresh_token_A = tokens_A["refresh_token"]
print(f"    -> Rotated! Server returned new refresh token A: {refresh_token_A[:15]}...")

# 4. Grace-period retry (simulates dropped connection / concurrent retry)
print("\n[*] Sending ORIGINAL token again within 10s grace window (Request B - the retry)...")
status, tokens_B = post(f"{BASE_URL}/auth/refresh", {"refresh_token": refresh_token_1})
assert status == 200, f"Retry failed: {tokens_B}"
refresh_token_B = tokens_B["refresh_token"]
print(f"    -> Server accepted retry and returned new refresh token B: {refresh_token_B[:15]}...")

# 5. The Moment of Truth
print("\n[*] Attempting to use the new refresh token B from the retry...")
status, res = post(f"{BASE_URL}/auth/refresh", {"refresh_token": refresh_token_B})

if status == 200:
    print("✅ BUG NOT REPRODUCED: Token B worked.")
else:
    print(f"❌ BUG PROVED: Token B failed! Server response: {status} {res}")
    print("   Explanation: The server gave the client `refresh_token_B`, but the database never saved it.")
    print("   The client is now permanently logged out.")

