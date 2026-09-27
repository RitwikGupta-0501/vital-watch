import re

with open("internal/middleware/ratelimit.go", "r") as f:
    text = f.read()
if '"encoding/json"' not in text:
    text = text.replace('"time"', '"encoding/json"\n\t"time"', 1)
text = re.sub(r'(Metadata:\s*)(fmt\.Sprintf\(`{"path":%q,"method":%q,"retry_after":%d}`, c\.Request\.URL\.Path, c\.Request\.Method, retryAfter\))', r'\1json.RawMessage(\2)', text)
with open("internal/middleware/ratelimit.go", "w") as f:
    f.write(text)

with open("internal/api/auth_handlers.go", "r") as f:
    text = f.read()
text = re.sub(r'(Metadata:\s*)(fmt\.Sprintf\(.*?\))', r'\1json.RawMessage(\2)', text)
with open("internal/api/auth_handlers.go", "w") as f:
    f.write(text)

