# ARO-HCP Image Tracer Tool

## Overview

This tool provides **full traceability** from deployed ARO-HCP configurations back to source code commits. Given an ARO-HCP repository commit and environment, it:

1. **Extracts image digests** from the appropriate configuration overlay
2. **Inspects each image's metadata** using container registry APIs to get the `vcs-ref` label  
3. **Maps components to repositories** to provide complete traceability

This solves the critical observability question: **"What version of the source code is currently running in this environment?"**

## Architecture

### Core Components

1. **`aro_hcp_image_tracer.py`** - Main implementation
   - `AROHCPConfigParser` - Extracts image info from config overlays
   - `ImageInspector` - Queries container registries via HTTP API  
   - `AROHCPImageTracer` - Orchestrates the full tracing process

2. **`mcp_server.py`** - Integration with MCP server
   - New `/trace-images` endpoint that accepts commit SHA + environment
   - Automatically exposed as MCP tool for AI assistants

3. **Component Mapping** - Hardcoded knowledge of:
   - Which registry/repository each component uses
   - Source repository URLs for each component

### Key Features

- **No Docker-in-Docker**: Uses container registry HTTP APIs instead of pulling images
- **Environment-aware**: Handles int/stg/prod configuration overlays correctly
- **Credential support**: Supports pull secrets for private registries
- **Comprehensive**: Traces 13+ ARO-HCP components including Backend, Frontend, Cluster Service, Maestro, etc.

## Usage

### As MCP Tool

When using via MCP (e.g., with Claude or other AI assistants):

```bash
# The AI can call this tool directly
trace_images(
    commit_sha="abc123def456",
    environment="prod"
)
```

### Direct API Call

```bash
curl -X POST "http://localhost:8000/trace-images" \
  -H "Content-Type: application/json" \
  -d '{
    "commit_sha": "abc123def456", 
    "environment": "prod"
  }'
```

### Command Line

```bash
python aro_hcp_image_tracer.py \
  --aro-hcp-dir ./ignore/aro-hcp-repo \
  --commit abc123def456 \
  --environment prod \
  --pull-secret '{"auths":{"registry.com":{"auth":"..."}}}'
```

## Configuration

### Environment Variables

- `ARO_HCP_REPO_PATH` - Path to ARO-HCP repository clone (default: `/app/ignore/aro-hcp-repo`)
- `PULL_SECRET` - JSON string with docker registry credentials (optional for public images)

### Pull Secret Format

```json
{
  "auths": {
    "arohcpsvcdev.azurecr.io": {
      "auth": "base64_encoded_username:password"
    },
    "quay.io": {
      "auth": "base64_encoded_username:password"  
    }
  }
}
```

## Example Response

```json
{
  "commit_sha": "abc123def456",
  "environment": "prod",
  "components": [
    {
      "name": "Backend",
      "registry": "arohcpsvcdev.azurecr.io",
      "repository": "arohcpbackend", 
      "digest": "sha256:bad87c9fac8a8...",
      "source_sha": "def789abc123",
      "source_repo_url": "https://github.com/Azure/ARO-HCP",
      "error": null
    },
    {
      "name": "Maestro",
      "registry": "quay.io",
      "repository": "redhat-user-workloads/maestro-rhtap-tenant/maestro/maestro",
      "digest": "sha256:90daeea3b4191...",
      "source_sha": "xyz456def789", 
      "source_repo_url": "https://github.com/openshift-online/maestro/",
      "error": null
    }
  ],
  "errors": []
}
```

## Supported Components

The tool currently traces these ARO-HCP components:

| Component | Registry | Source Repository |
|-----------|----------|-------------------|
| Backend | arohcpsvcdev.azurecr.io | https://github.com/Azure/ARO-HCP |
| Frontend | arohcpsvcdev.azurecr.io | https://github.com/Azure/ARO-HCP |
| Cluster Service | quay.io | https://gitlab.cee.redhat.com/service/uhc-clusters-service |
| Backplane | quay.io | https://gitlab.cee.redhat.com/service/backplane-api |
| Maestro | quay.io | https://github.com/openshift-online/maestro/ |
| Hypershift | quay.io | https://github.com/openshift/hypershift |
| ACM Operator | quay.io | https://github.com/stolostron/acm-operator-bundle |
| MCE | quay.io | https://github.com/stolostron/mce-operator-bundle |
| OcMirror | arohcpsvcdev.azurecr.io | https://github.com/openshift/oc-mirror |
| Package Operator (3 components) | quay.io | https://github.com/package-operator/package-operator |
| ACR Pull | mcr.microsoft.com | External (Microsoft) |

## How It Works

### 1. Configuration Parsing

The tool checks out the specific commit in the ARO-HCP repository and:

1. Loads `config/config.yaml` (base configuration)
2. Loads `config/config.msft.clouds-overlay.yaml` (environment overlays) 
3. Merges base + environment-specific overlay (int/stg/prod)
4. Extracts image digests for each component

### 2. Image Inspection

For each image digest, the tool:

1. Authenticates with the container registry using pull secrets
2. Fetches the image manifest using the digest
3. Retrieves the image config blob  
4. Extracts the `vcs-ref` label containing the source commit SHA

### 3. Traceability

The tool provides complete traceability by linking:

- **ARO-HCP commit** → **Environment config** → **Image digest** → **Source commit** → **Source repository**

## Testing

Run the test suite:

```bash
python test_image_tracer.py
```

The test verifies:
- Configuration parsing works correctly
- Component mappings are accurate
- Image inspector initializes properly

## Deployment

The tool is integrated into the existing MCP server deployment:

1. **Environment variables** are configured in `manifests/config.env`
2. **ARO-HCP repository** should be mounted at `/app/ignore/aro-hcp-repo` 
3. **Pull secrets** should be provided via `PULL_SECRET` environment variable

## Credits

This tool is based on the methodology from the [service-status](../ignore/service-status) Go tool, reimplemented in Python for MCP integration.
