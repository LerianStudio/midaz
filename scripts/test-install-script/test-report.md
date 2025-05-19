# Midaz Install Script Test Report

Generated: Mon May 19 07:26:39 -03 2025

## Summary

| Distribution | Status | Log |
|-------------|--------|-----|
| alpine | ❌ FAILED | [View Log](./logs/alpine.log) |
| arm | ❌ FAILED | [View Log](./logs/arm.log) |
| debian | ❌ FAILED | [View Log](./logs/debian.log) |
| fedora | ❌ FAILED | [View Log](./logs/fedora.log) |
| ubuntu | ❌ FAILED | [View Log](./logs/ubuntu.log) |
| wsl | ❌ FAILED | [View Log](./logs/wsl.log) |

## Statistics

- **Total Tests:** 6
- **Passed:** 0
- **Failed:** 6
- **Pass Rate:** 0%

## Failure Details

### alpine Failure

```
[MIDAZ] WARNING: Cannot connect to Docker Hub. Container images may fail to download.
[MIDAZ] ERROR: Cannot install Docker. Please install Docker manually and try again.
```

### arm Failure

```
./install.sh: 83: set: Illegal option -o pipefail
```

### debian Failure

```
./install.sh: 83: set: Illegal option -o pipefail
```

### fedora Failure

```
[MIDAZ] WARNING: Cannot connect to Docker Hub. Container images may fail to download.
Failed to connect to bus: Host is down
Public key for nodejs-20.19.2-1nodesource.aarch64.rpm is not installed. Failing package is: nodejs-2:20.19.2-1nodesource.aarch64
Error: GPG check FAILED
[MIDAZ] WARNING: Failed to install node or it's not in PATH
[MIDAZ] WARNING: Node.js installation may have failed. Please check manually.
make[1]: *** [Makefile:155: up] Error 1
make: *** [Makefile:342: up] Error 1
[MIDAZ] ERROR: Failed to start services. Collecting diagnostic information...
[MIDAZ] Recently exited containers (may show startup failures):
[MIDAZ] ERROR: Failed to start services. See diagnostic information above.
```

### ubuntu Failure

```
./install.sh: 83: set: Illegal option -o pipefail
```

### wsl Failure

```
./install.sh: 83: set: Illegal option -o pipefail
```

