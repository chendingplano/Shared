# mitmproxy Traffic Capture Guide

How to use mitmproxy to capture HTTP/HTTPS traffic, store it to files, and use the logs for analysis and test development.

## Installation

```bash
# macOS
brew install mitmproxy

# or pip
pip install mitmproxy
```

## Basic Usage: Save to Files

### Option 1: Forward Proxy, Direct file capture (simplest)

```bash
# Capture all traffic and save to a file
mitmproxy --save-stream-file traffic.mitm

# Or use mitmdump (headless, good for scripting)
mitmdump --save-stream-file traffic.mitm

# With port specified
mitmdump -p 8080 --save-stream-file traffic.mitm
```

### Option 2: Reverse Proxy

To GUI
```bash 
mitmweb --mode reverse:https://some-company.com --listen-port 8000
```

Example:
```bash
mitmweb --mode reverse:https://api.anthropic.com --listen-port 8000
```

To File    
```bash 
mitmdump --mode reverse:https://some-company.com --listen-port 8000 \
  --save-stream-file traffic.mitm
```

Example 
```bash 
mitmdump --mode reverse:https://api.anthropic.com --listen-port 8000 \
  --save-stream-file traffic.mitm
```

IMPORTANT:
Make sure your app points to http://localhost:8000.

To query_name.filter(d => d.column === 'value')

### Option 2: Save as HAR (JSON, great for analysis)

Use a script to export as HAR format:

```bash
mitmdump -s save_har.py -w traffic.mitm
```

## Custom Script for Structured Logging

Create a script to save traffic in a readable format for test development:

```python
# capture.py
import json
import os
from datetime import datetime
from mitmproxy import http

output_dir = "captured_traffic"
os.makedirs(output_dir, exist_ok=True)

def response(flow: http.HTTPFlow) -> None:
    """Called when a response is received."""
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S_%f")

    # Build a structured record
    record = {
        "timestamp": timestamp,
        "request": {
            "method": flow.request.method,
            "url": str(flow.request.url),
            "headers": dict(flow.request.headers),
            "body": flow.request.get_text(strict=False),
        },
        "response": {
            "status_code": flow.response.status_code,
            "headers": dict(flow.response.headers),
            "body": flow.response.get_text(strict=False),
        }
    }

    # Save each request/response pair as a JSON file
    safe_path = flow.request.path.replace("/", "_").strip("_")[:50]
    filename = f"{output_dir}/{timestamp}_{flow.request.method}_{safe_path}.json"

    with open(filename, "w") as f:
        json.dump(record, f, indent=2, default=str)

    print(f"[{flow.response.status_code}] {flow.request.method} {flow.request.url}")
```

Run it:

```bash
mitmdump -s capture.py -p 8080
```

## Configure Your App to Use the Proxy

```bash
# Set environment variables for your app
export HTTP_PROXY=http://localhost:8080
export HTTPS_PROXY=http://localhost:8080

# Or for curl testing
curl --proxy http://localhost:8080 https://example.com
```

## SSL/HTTPS Interception

For HTTPS traffic, install the mitmproxy CA certificate:

```bash
# Start mitmproxy once to generate the cert
mitmproxy -p 8080

# The cert is at:
~/.mitmproxy/mitmproxy-ca-cert.pem

# Install on macOS system trust
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain \
  ~/.mitmproxy/mitmproxy-ca-cert.pem
```

## Replay and Filter Saved Traffic

```bash
# Read back a saved .mitm file
mitmproxy -r traffic.mitm

# Filter while capturing (e.g., only a specific host)
mitmdump --save-stream-file traffic.mitm "~d example.com"

# Filter by method
mitmdump --save-stream-file traffic.mitm "~m POST"

# Filter by URL pattern
mitmdump --save-stream-file traffic.mitm "~u /api/"
```

## For Test Development

Extend the capture script to generate test fixtures from captured traffic:

```python
# capture_for_tests.py
import json, os, re
from mitmproxy import http

os.makedirs("test_fixtures", exist_ok=True)

def response(flow: http.HTTPFlow) -> None:
    # Only capture API calls
    if "/api/" not in flow.request.path:
        return

    # Create a test fixture name from the endpoint
    name = re.sub(r"[^a-zA-Z0-9]", "_", flow.request.path).strip("_")

    fixture = {
        "request": {
            "method": flow.request.method,
            "path": flow.request.path,
            "query": dict(flow.request.query),
            "body": flow.request.get_text(strict=False),
        },
        "response": {
            "status": flow.response.status_code,
            "body": flow.response.get_text(strict=False),
        }
    }

    path = f"test_fixtures/{name}.json"
    with open(path, "w") as f:
        json.dump(fixture, f, indent=2, default=str)
    print(f"Saved fixture: {path}")
```

## Quick Reference

| Goal | Command |
|------|---------|
| Interactive UI | `mitmproxy -p 8080` |
| Headless capture | `mitmdump -p 8080 -w out.mitm` |
| Custom script | `mitmdump -s capture.py -p 8080` |
| Replay file | `mitmproxy -r traffic.mitm` |
| Filter by host | `mitmdump "~d api.example.com" -w out.mitm` |

The `.mitm` binary format is the most complete (preserves everything), while the JSON script approach gives you human-readable files that are easier to use for building tests.
