---
description: Transfer files, run remote commands, and control remote screens using ztransfer API
allowed-tools: ["Bash"]
---

Check if the ztransfer API is running locally, list available peers, and help the user with file transfer, remote execution, or computer use.

Steps:
1. Check if ztransfer API is reachable:
   ```bash
   curl -s http://localhost:9877/api/status
   ```
   If connection refused, inform the user to start it with `ztransfer api &`

2. List available peers:
   ```bash
   curl -s http://localhost:9877/api/peers
   ```

3. Ask the user what they want to do:

   **File Transfer:**
   - **List remote files**: `curl -s 'http://localhost:9877/api/ls?peer=PEER&path=/'`
   - **Download a file**: `curl -s -X POST http://localhost:9877/api/get -d '{"peer":"PEER","remote_path":"/file","local_path":"/tmp/"}'`
   - **Upload a file**: `curl -s -X POST http://localhost:9877/api/put -d '{"peer":"PEER","local_path":"/path/to/file","remote_path":"/"}'`
   - **Stream download**: `curl -s 'http://localhost:9877/api/receive?peer=PEER&path=/file' > localfile`
   - **Stream upload**: `curl -s -X POST http://localhost:9877/api/send -F file=@localfile -F peer=PEER -F remote_path=/`

   **Remote Execution (via warp code):**
   - **Run a command**: `curl -s -X POST http://localhost:9877/api/remote/exec -d '{"code":"warp-123-alpha","command":"uname -a"}'`
   - **Host a session**: `curl -s -X POST http://localhost:9877/api/remote/host`
   - **List active sessions**: `curl -s http://localhost:9877/api/remote/sessions`

   **Computer Use (via warp code):**
   - **Start session**: `curl -s -X POST http://localhost:9877/api/remote/computer/start -d '{"code":"warp-123-alpha"}'`
   - **Take screenshot**: `curl -s 'http://localhost:9877/api/remote/computer/screen?session=SESSION_ID&format=jpeg&quality=65'`
   - **Perform action**: `curl -s -X POST http://localhost:9877/api/remote/computer/action -d '{"session":"SESSION_ID","action":{"type":"click","x":500,"y":300}}'`
   - **Get display info**: `curl -s 'http://localhost:9877/api/remote/computer/info?session=SESSION_ID'`
   - **Stop session**: `curl -s -X POST http://localhost:9877/api/remote/computer/stop -d '{"session":"SESSION_ID"}'`

Present the peer list and ask what operation to perform. Use the Bash tool with curl to execute operations.
