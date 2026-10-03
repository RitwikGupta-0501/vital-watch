import subprocess
import os
import json
from datetime import datetime

def run_k6(target_vus):
    print(f"[*] Testing capacity with {target_vus} VUs...")
    
    cmd = [
        "docker", "run", "--rm",
        "-v", f"{os.getcwd()}/scripts/loadtest:/loadtest",
        "--network", "host",
        "-e", f"TARGET_VUS={target_vus}",
        "-e", "K6_PROMETHEUS_RW_SERVER_URL=http://localhost:9090/api/v1/write",
        "-e", "K6_PROMETHEUS_RW_TREND_AS_NATIVE_HISTOGRAM=true",
        "grafana/k6", "run", 
        "--out", "experimental-prometheus-rw",
        "/loadtest/user_journey.js"
    ]
    
    result = subprocess.run(cmd, capture_output=True, text=True)
    
    # K6 exits with 0 if all thresholds pass, 99 (or non-zero) if any fail
    passed = result.returncode == 0
    print(f"    -> {'PASSED' if passed else 'FAILED'} (Exit code: {result.returncode})")
    
    # Parse output to extract p95 and error rate for reporting (rudimentary parsing)
    p95 = "N/A"
    error_rate = "N/A"
    for line in result.stdout.split('\n') + result.stderr.split('\n'):
        if "http_req_duration" in line and "p(95)=" in line:
            parts = line.split("p(95)=")
            if len(parts) > 1:
                p95 = parts[1].split()[0]
        if "http_req_failed" in line and "rate=" in line:
            parts = line.split("rate=")
            if len(parts) > 1:
                error_rate = parts[1].split()[0]
                
    return passed, p95, error_rate, result.stdout + result.stderr

def main():
    min_vus = 1
    max_vus = 200
    best_vus = 0
    best_p95 = "N/A"
    
    results = []
    
    print("==================================================")
    print(" Starting Binary Search Capacity Test (User Journey)")
    print(f" Range: {min_vus} to {max_vus} VUs")
    print("==================================================\n")
    
    while min_vus <= max_vus:
        mid_vus = (min_vus + max_vus) // 2
        
        passed, p95, err_rate, full_output = run_k6(mid_vus)
        
        results.append({
            "vus": mid_vus,
            "passed": passed,
            "p95": p95,
            "error_rate": err_rate
        })
        
        if passed:
            best_vus = mid_vus
            best_p95 = p95
            min_vus = mid_vus + 1
        else:
            max_vus = mid_vus - 1
            
    print("\n==================================================")
    print(f" Binary Search Complete!")
    print(f" Maximum Supported Capacity: {best_vus} VUs (p95: {best_p95})")
    print("==================================================")
    
    # Generate Markdown Report
    report = f"# Capacity Benchmark Report (User Journey)\n\n"
    report += f"**Date**: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n"
    report += f"**Target Endpoint**: Full User Journey (Register, Login, View Profile/Doctors)\n"
    report += f"**Maximum Verified Capacity**: {best_vus} Concurrent Virtual Users\n\n"
    
    report += "## Test Configuration\n"
    report += "- **Ramp-up**: 15 seconds\n"
    report += "- **Sustain**: 30 seconds\n"
    report += "- **Ramp-down**: 15 seconds\n"
    report += "- **Success Criteria**: p(95) latency < 200ms AND Error rate < 1%\n\n"
    
    report += "## Binary Search Execution Log\n"
    report += "| Virtual Users | Result | p95 Latency | Error Rate |\n"
    report += "|---------------|--------|-------------|------------|\n"
    
    # Sort results by VUs for a cleaner report
    results.sort(key=lambda x: x["vus"])
    for r in results:
        status = "✅ PASS" if r["passed"] else "❌ FAIL"
        report += f"| {r['vus']} | {status} | {r['p95']} | {r['error_rate']} |\n"
        
    with open("user_journey_report.md", "w") as f:
        f.write(report)
        
    print("\nReport written to user_journey_report.md")

if __name__ == "__main__":
    main()
