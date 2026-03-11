---
description: Transfer files between machines using ztransfer API
allowed-tools: ["Bash"]
---

Check if the ztransfer API is running locally, list available peers, and help the user transfer files.

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
   - **List remote files**: `curl -s 'http://localhost:9877/api/ls?peer=PEER&path=/'`
   - **Download a file**: `curl -s -X POST http://localhost:9877/api/get -d '{"peer":"PEER","remote_path":"/file","local_path":"/tmp/"}'`
   - **Upload a file**: `curl -s -X POST http://localhost:9877/api/put -d '{"peer":"PEER","local_path":"/path/to/file","remote_path":"/"}'`
   - **Stream download**: `curl -s 'http://localhost:9877/api/receive?peer=PEER&path=/file' > localfile`
   - **Stream upload**: `curl -s -X POST http://localhost:9877/api/send -F file=@localfile -F peer=PEER -F remote_path=/`

Present the peer list and ask what operation to perform. Use the Bash tool with curl to execute transfers.
