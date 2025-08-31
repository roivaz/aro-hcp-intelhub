#!/usr/bin/env python3
"""
Test script for ARO-HCP Image Tracer

This script tests the image tracing functionality without requiring pull secrets
by testing the configuration parsing portion.
"""

import os
import sys
from pathlib import Path

# Add current directory to path for imports
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from aro_hcp_image_tracer import AROHCPConfigParser, AROHCPImageTracer, COMPONENT_MAPPINGS
from repo_manager import RepoManager


def test_config_parser():
    """Test the configuration parser"""
    print("Testing ARO-HCP Configuration Parser...")
    
    # Use the ARO-HCP repo in ignore directory
    repo_path = Path(__file__).parent / "ignore" / "aro-hcp-repo"
    manager = RepoManager("https://github.com/Azure/ARO-HCP", repo_path)
    try:
        repo_path = Path(manager.ensure_ready())
    except Exception as exc:
        print(f"❌ Failed to prepare repository: {exc}")
        return False
    
    try:
        parser = AROHCPConfigParser(str(repo_path))
        print(f"✅ Successfully initialized parser with repo at {repo_path}")
        
        # Test with a recent commit (you may need to update this)
        test_commit = "HEAD"  # Use HEAD for testing
        test_env = "int"
        
        print(f"Testing with commit: {test_commit}, environment: {test_env}")
        
        # Extract images
        images = parser.extract_images_for_commit_and_env(test_commit, test_env)
        
        print(f"✅ Successfully extracted {len(images)} component images:")
        for component_name, image_info in images.items():
            print(f"  - {component_name}:")
            print(f"    Registry: {image_info['registry']}")
            print(f"    Repository: {image_info['repository']}")
            print(f"    Digest: {image_info['digest'][:20]}...")
            
            # Check if we have mapping for this component
            if component_name in COMPONENT_MAPPINGS:
                expected_registry = COMPONENT_MAPPINGS[component_name]['registry']
                expected_repo = COMPONENT_MAPPINGS[component_name]['repository']
                if (image_info['registry'] == expected_registry and 
                    image_info['repository'] == expected_repo):
                    print(f"    ✅ Mapping matches expected values")
                else:
                    print(f"    ⚠️  Mapping mismatch - expected {expected_registry}/{expected_repo}")
            else:
                print(f"    ⚠️  No component mapping found")
        
        return True
        
    except Exception as e:
        print(f"❌ Configuration parser test failed: {e}")
        return False


def test_component_mappings():
    """Test component mappings"""
    print("\nTesting Component Mappings...")
    
    print(f"✅ Found mappings for {len(COMPONENT_MAPPINGS)} components:")
    for name, info in COMPONENT_MAPPINGS.items():
        print(f"  - {name}: {info['registry']}/{info['repository']}")
        if info['source_repo']:
            print(f"    Source: {info['source_repo']}")
        else:
            print(f"    Source: External (no repository)")


def test_image_inspector_mock():
    """Test image inspector with mock data (no network calls)"""
    print("\nTesting Image Inspector (mock mode)...")
    
    # Mock pull secret for testing  
    from aro_hcp_image_tracer import ImageInspector
    
    try:
        inspector = ImageInspector()
        print("✅ Successfully initialized ImageInspector with mock credentials")
        
        # Note: We don't actually call get_image_labels here since it would make network calls
        # In a real test environment, you would need valid credentials and network access
        print("ℹ️  Image inspection requires network access and valid credentials")
        print("   To test fully, set PULL_SECRET environment variable with valid credentials")
        print("✅ Image inspector initialization test passed")
        
        return True
        
    except Exception as e:
        print(f"❌ Image inspector initialization failed: {e}")
        return False


def main():
    """Run all tests"""
    print("🧪 Running ARO-HCP Image Tracer Tests\n")
    
    tests = [
        test_config_parser,
        test_component_mappings, 
        test_image_inspector_mock
    ]
    
    passed = 0
    total = len(tests)
    
    for test in tests:
        try:
            if test():
                passed += 1
        except Exception as e:
            print(f"❌ Test {test.__name__} failed with exception: {e}")
    
    print(f"\n📊 Test Results: {passed}/{total} tests passed")
    
    if passed == total:
        print("🎉 All tests passed!")
        return 0
    else:
        print("💥 Some tests failed!")
        return 1


if __name__ == "__main__":
    sys.exit(main())
