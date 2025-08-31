#!/usr/bin/env python3
"""
ARO-HCP Image Tracer

This tool traces back from deployed ARO-HCP configurations to source code commits by:
1. Extracting image digests from ARO-HCP configuration overlays at a specific commit
2. Inspecting image metadata using the skopeo CLI to get vcs-ref labels
3. Mapping components to their source repositories
"""

import json
import os
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

import yaml

from repo_manager import RepoManager

@dataclass
class ComponentInfo:
    """Information about a component and its source"""
    name: str
    registry: str
    repository: str
    digest: str
    source_sha: Optional[str] = None
    source_repo_url: Optional[str] = None
    image_creation_time: Optional[str] = None
    error: Optional[str] = None


@dataclass
class ImageTraceResult:
    """Result of tracing images back to source commits"""
    commit_sha: str
    environment: str
    components: List[ComponentInfo]
    errors: List[str]


# Component mapping - based on service-status hardcoded knowledge
COMPONENT_MAPPINGS = {
    "Backend": {
        "registry": "arohcpsvcdev.azurecr.io",
        "repository": "arohcpbackend",
        "source_repo": "https://github.com/Azure/ARO-HCP"
    },
    "Frontend": {
        "registry": "arohcpsvcdev.azurecr.io", 
        "repository": "arohcpfrontend",
        "source_repo": "https://github.com/Azure/ARO-HCP"
    },
    "Cluster Service": {
        "registry": "quay.io",
        "repository": "app-sre/uhc-clusters-service",
        "source_repo": "https://gitlab.cee.redhat.com/service/uhc-clusters-service"
    },
    "Maestro": {
        "registry": "quay.io",
        "repository": "redhat-user-workloads/maestro-rhtap-tenant/maestro/maestro",
        "source_repo": "https://github.com/openshift-online/maestro/"
    },
    "Hypershift": {
        "registry": "quay.io",
        "repository": "acm-d/rhtap-hypershift-operator",
        "source_repo": "https://github.com/openshift/hypershift"
    },
    "ACM Operator": {
        "registry": "quay.io",
        "repository": "redhat-user-workloads/crt-redhat-acm-tenant/acm-operator-bundle-acm-214",
        "source_repo": "https://github.com/stolostron/acm-operator-bundle"
    },
    "MCE": {
        "registry": "quay.io",
        "repository": "redhat-user-workloads/crt-redhat-acm-tenant/mce-operator-bundle-mce-29",
        "source_repo": "https://github.com/stolostron/mce-operator-bundle"
    },
    "OcMirror": {
        "registry": "arohcpsvcdev.azurecr.io",
        "repository": "image-sync/oc-mirror",
        "source_repo": "https://github.com/openshift/oc-mirror"
    },
}

ENVIRONMENT_CONFIG_PATHS = {
    "dev": {
        "file": "config/rendered/dev/dev/westus3.yaml",
        "base_path": ["defaults"],
    },
    "int": {
        "file": "config/config.msft.clouds-overlay.yaml",
        "base_path": ["clouds", "public", "environments", "int", "defaults"],
    },
    "stg": {
        "file": "config/config.msft.clouds-overlay.yaml",
        "base_path": ["clouds", "public", "environments", "stg", "defaults"],
    },
    "prod": {
        "file": "config/config.msft.clouds-overlay.yaml",
        "base_path": ["clouds", "public", "environments", "prod", "defaults"],
    },
}

# Paths (relative to defaults blocks) for locating image configuration per component
IMAGE_CONFIG_PATHS = {
    "Backend": ["backend", "image"],
    "Frontend": ["frontend", "image"],
    "Cluster Service": ["clustersService", "image"],
    "Maestro": ["maestro", "image"],
    "Hypershift": ["hypershift", "image"],
    "ACM Operator": ["acm", "operator", "bundle"],
    "MCE": ["acm", "mce", "bundle"],
    "OcMirror": ["imageSync", "ocMirror", "image"],
    "Package Operator Package": ["pko", "imagePackage"],
    "Package Operator Manager": ["pko", "imageManager"],
    "Package Operator Remote Phase Manager": ["pko", "remotePhaseManager"],
}


class AROHCPConfigParser:
    """Parses ARO-HCP configuration files to extract image information"""
    
    def __init__(self, aro_hcp_repo_path: str, repo_url: str = "https://github.com/Azure/ARO-HCP"):
        """
        Initialize parser with ARO-HCP repository path
        
        Args:
            aro_hcp_repo_path: Path to ARO-HCP repository clone
            repo_url: Remote repository URL (default: Azure ARO-HCP)
        """
        self.repo_manager = RepoManager(repo_url, aro_hcp_repo_path)
        self.repo_path = Path(self.repo_manager.ensure_ready())
    
    def extract_images_for_commit_and_env(self, commit_sha: str, environment: str) -> Dict[str, Dict[str, str]]:
        """Extract image references for a specific commit and environment"""
        if environment not in ENVIRONMENT_CONFIG_PATHS:
            raise ValueError(f"Unsupported environment: {environment}")

        with self.repo_manager.temporary_checkout(commit_sha):
            env_section, base_defaults = self._load_environment_section(environment)
            return self._extract_images_from_config(env_section, base_defaults)
    
    def _load_environment_section(self, environment: str) -> Tuple[Dict[str, Any], Dict[str, Any]]:
        """Load the configuration section containing image definitions plus base defaults"""
        spec = ENVIRONMENT_CONFIG_PATHS[environment]
        config_file = self.repo_path / spec["file"]
        if not config_file.exists():
            raise Exception(f"Configuration file not found: {config_file}")

        with open(config_file, "r") as f:
            content = f.read()
            data = yaml.safe_load(content)

        env_section = self._get_nested(data, spec["base_path"])
        if env_section is None:
            raise Exception(
                f"Base path {'.'.join(spec['base_path'])} not found in configuration for environment '{environment}'"
            )

        base_defaults = self._load_base_defaults()
        return env_section, base_defaults

    def _load_base_defaults(self) -> Dict[str, Any]:
        """Load defaults from config/config.yaml"""
        config_dir = self.repo_path / "config"
        base_config_path = config_dir / "config.yaml"
        if not base_config_path.exists():
            raise Exception(f"Base config file not found: {base_config_path}")
        
        with open(base_config_path, "r") as f:
            base_content = f.read()
            base_content = base_content.replace("{{ .ev2.availabilityZoneCount }}", "2")
            base_config = yaml.safe_load(base_content)
        
        return base_config.get("defaults", {})

    def _extract_images_from_config(
        self,
        env_config: Dict[str, Any],
        base_defaults: Dict[str, Any],
    ) -> Dict[str, Dict[str, str]]:
        """Extract image references using configured paths with fallback"""
        images: Dict[str, Dict[str, str]] = {}

        for component, path in IMAGE_CONFIG_PATHS.items():
            env_cfg = self._get_nested(env_config, path) or {}
            base_cfg = self._get_nested(base_defaults, path) or {}

            if not base_cfg and not env_cfg:
                raise Exception(
                    f"Image configuration for '{component}' not found at path {'.'.join(path)}"
                )

            merged_cfg: Dict[str, str] = {}
            merged_cfg.update(base_cfg)
            merged_cfg.update(env_cfg)

            registry = merged_cfg.get("registry", "")
            repository = merged_cfg.get("repository", "")
            digest = merged_cfg.get("digest", "")

            if not registry or not repository:
                raise Exception(
                    f"Incomplete image configuration for '{component}' at path {'.'.join(path)}"
                )

            images[component] = {
                "registry": registry,
                "repository": repository,
                "digest": digest,
            }

        return images

    @staticmethod
    def _get_nested(source: Optional[Dict[str, Any]], path: List[str]) -> Optional[Dict[str, Any]]:
        """Walk nested dictionaries following the provided path"""
        if not source:
            return None
        if not path:
            return source if isinstance(source, dict) else None

        current: Any = source
        for key in path:
            if not isinstance(current, dict):
                return None
            current = current.get(key)
            if current is None:
                return None

        return current if isinstance(current, dict) else None


class ImageInspector:
    """Inspects container images using skopeo"""

    def __init__(self, skopeo_path: str = "skopeo", pull_secret: Optional[str] = None):
        self.skopeo_path = skopeo_path
        self.credentials: Dict[str, Dict[str, str]] = {}
        self.pull_secret_path: Optional[str] = None
        if pull_secret:
            if not os.path.isfile(pull_secret):
                raise ValueError("Pull secret must be a file path")
            self.pull_secret_path = pull_secret
            try:
                with open(pull_secret, "r", encoding="utf-8") as f:
                    pull_secret_payload = f.read()
            except OSError as exc:
                raise ValueError(f"Failed to read pull secret file '{pull_secret}': {exc}")
            try:
                config = json.loads(pull_secret_payload)
                self.credentials = config.get("auths", {})
            except json.JSONDecodeError as exc:
                raise ValueError(f"Invalid pull secret JSON: {exc}")

    def get_image_labels(self, registry: str, repository: str, digest: str) -> Dict[str, str]:
        image_ref = f"{registry}/{repository}@{digest}"
        auth_env = os.environ.copy()
        authfile = self._get_authfile(registry)
        inspect_cmd = [self.skopeo_path, "inspect"]
        if authfile:
            inspect_cmd.extend(["--authfile", authfile])
        inspect_cmd.extend(["--raw", f"docker://{image_ref}"])
        try:
            raw_output = subprocess.check_output(inspect_cmd, env=auth_env, stderr=subprocess.PIPE)
        except subprocess.CalledProcessError as exc:
            stderr = exc.stderr.decode("utf-8", errors="ignore") if exc.stderr else str(exc)
            raise Exception(f"skopeo inspect --raw failed for {image_ref}: {stderr.strip()}")
        manifest = json.loads(raw_output)
        config_ref = self._resolve_config_reference(manifest, registry, repository, digest)
        config_cmd = [self.skopeo_path, "inspect"]
        if authfile:
            config_cmd.extend(["--authfile", authfile])
        config_cmd.extend(["--config", config_ref])
        try:
            config_output = subprocess.check_output(config_cmd, env=auth_env, stderr=subprocess.PIPE)
        except subprocess.CalledProcessError as exc:
            stderr = exc.stderr.decode("utf-8", errors="ignore") if exc.stderr else str(exc)
            raise Exception(f"skopeo inspect --config failed for {config_ref}: {stderr.strip()}")
        config_data = json.loads(config_output)
        return config_data.get("config", {}).get("Labels", {})

    def _get_authfile(self, registry: str) -> Optional[str]:
        if self.pull_secret_path:
            return self.pull_secret_path
        return None

    @staticmethod
    def _resolve_config_reference(
        manifest: Dict[str, Any],
        registry: str,
        repository: str,
        original_digest: str,
    ) -> str:
        media_type = manifest.get("mediaType")
        if media_type in {
            "application/vnd.docker.distribution.manifest.v2+json",
            "application/vnd.oci.image.manifest.v1+json",
        }:
            return f"docker://{registry}/{repository}@{original_digest}"
        if media_type in {
            "application/vnd.docker.distribution.manifest.list.v2+json",
            "application/vnd.oci.image.index.v1+json",
        }:
            manifests = manifest.get("manifests", [])
            chosen = None
            for entry in manifests:
                platform = entry.get("platform", {})
                if platform.get("architecture") == "amd64" and platform.get("os") == "linux":
                    chosen = entry
                    break
            if not chosen and manifests:
                chosen = manifests[0]
            if not chosen:
                raise Exception("No manifests available in manifest list")
            child_digest = chosen.get("digest")
            if not child_digest:
                raise Exception("Manifest list entry missing digest")
            return f"docker://{registry}/{repository}@{child_digest}"
        return f"docker://{registry}/{repository}@{original_digest}"


class AROHCPImageTracer:
    """Main class for tracing ARO-HCP images back to source commits"""
    
    def __init__(self, aro_hcp_repo_path: str, pull_secret: Optional[str] = None, skopeo_path: str = "skopeo"):
        """
        Initialize tracer
        
        Args:
            aro_hcp_repo_path: Path to ARO-HCP repository clone
            pull_secret: Path to docker registry pull secret JSON
        """
        self.config_parser = AROHCPConfigParser(aro_hcp_repo_path)
        self.image_inspector = ImageInspector(skopeo_path, pull_secret)
    
    def trace_images(self, commit_sha: str, environment: str) -> ImageTraceResult:
        """
        Trace images for a specific commit and environment back to source commits
        
        Args:
            commit_sha: ARO-HCP repository commit SHA
            environment: Environment name (int, stg, prod)
            
        Returns:
            ImageTraceResult with component information and source commits
        """
        errors = []
        components = []
        
        try:
            # Extract image information from config
            images = self.config_parser.extract_images_for_commit_and_env(commit_sha, environment)
            
            for component_name, image_info in images.items():
                component = ComponentInfo(
                    name=component_name,
                    registry=image_info['registry'],
                    repository=image_info['repository'],
                    digest=image_info['digest']
                )
                
                # Add source repository URL if known
                if component_name in COMPONENT_MAPPINGS:
                    component.source_repo_url = COMPONENT_MAPPINGS[component_name]['source_repo']
                
                # Try to get source commit from image labels
                try:
                    labels = self.image_inspector.get_image_labels(
                        component.registry,
                        component.repository,
                        component.digest
                    )
                    
                    # Extract vcs-ref label (source commit SHA)
                    component.source_sha = labels.get('vcs-ref')
                    
                    # Also get creation time if available  
                    # Note: This would be in the config blob, not labels
                    
                except Exception as e:
                    component.error = str(e)
                    errors.append(f"Failed to inspect {component_name}: {e}")
                
                components.append(component)
                
        except Exception as e:
            errors.append(f"Failed to extract configuration: {e}")
        
        return ImageTraceResult(
            commit_sha=commit_sha,
            environment=environment,
            components=components,
            errors=errors
        )


# CLI interface for testing
if __name__ == "__main__":
    import argparse
    
    parser = argparse.ArgumentParser(description="Trace ARO-HCP images to source commits")
    parser.add_argument("--aro-hcp-dir", required=True, help="Path to ARO-HCP repository")
    parser.add_argument("--commit", required=True, help="ARO-HCP commit SHA")
    parser.add_argument("--environment", required=True, choices=['int', 'stg', 'prod'], help="Environment")
    parser.add_argument("--pull-secret", help="Path to docker registry pull secret JSON")
    parser.add_argument("--skopeo", default="skopeo", help="Path to skopeo binary")
    
    args = parser.parse_args()
    
    # Get pull secret path from environment if not provided
    pull_secret = args.pull_secret
    if not pull_secret:
        pull_secret = os.getenv('PULL_SECRET')
    
    tracer = AROHCPImageTracer(args.aro_hcp_dir, pull_secret, skopeo_path=args.skopeo)
    result = tracer.trace_images(args.commit, args.environment)
    
    print(json.dumps({
        'commit_sha': result.commit_sha,
        'environment': result.environment,
        'components': [
            {
                'name': c.name,
                'registry': c.registry,
                'repository': c.repository,
                'digest': c.digest,
                'source_sha': c.source_sha,
                'source_repo_url': c.source_repo_url,
                'error': c.error
            }
            for c in result.components
        ],
        'errors': result.errors
    }, indent=2))
