#!/usr/bin/env python3
"""
Test script for PR-specific LangChain-based diff analyzer using Ollama
"""
import sys
import os
import logging
import asyncio
from datetime import datetime, timezone
from typing import Optional
from pathlib import Path

# Cursor environment workaround - manually add venv site-packages to path
venv_site_packages = os.path.join(os.path.dirname(__file__), '.venv', 'lib', 'python3.12', 'site-packages')
if os.path.exists(venv_site_packages):
    sys.path.insert(0, venv_site_packages)
    print(f"Added venv site-packages to path: {venv_site_packages}")

# Load environment variables from manifests/config.env if present
config_path = Path(__file__).resolve().parent / "manifests" / "config.env"
if config_path.exists():
    from dotenv import load_dotenv

    load_dotenv(config_path)

# Import required modules
try:
    from github import Github, Auth
    from pr_diff_analyzer import PRDiffAnalyzer, PRAnalysisData, PRAnalysis
    from repo_manager import RepoManager
    print("✅ All imports successful!")
except ImportError as e:
    print(f"❌ Import error: {e}")
    print("Make sure you've activated your virtual environment and installed all dependencies.")
    sys.exit(1)

# Set up basic logging
logging.basicConfig(level=logging.INFO, format='%(levelname)s: %(message)s')
logger = logging.getLogger("test_pr_analysis")
test_log_level = os.getenv("LOG_LEVEL", "INFO").upper()
logger.setLevel(test_log_level)

CONTEXT_TOKENS = int(os.getenv("PR_DIFF_CONTEXT_TOKENS", "4096"))


def setup_local_repo(repo_url: str = "https://github.com/Azure/ARO-HCP", local_path: str = "./ignore/aro-hcp-repo") -> str:
    """Ensure the local repository exists and is up to date."""
    try:
        manager = RepoManager(repo_url, local_path)
        repo_path = manager.ensure_ready()
        manager.checkout("main")
        logger.info(f"✅ Repository ready at: {repo_path}")
        return str(repo_path)
    except Exception as e:
        logger.error(f"❌ Error setting up repository: {e}")
        raise

def get_pr_data_from_github(pr_number: int) -> Optional[PRAnalysisData]:
    """Fetch PR data from GitHub API"""
    try:
        # Initialize GitHub client
        github_token = os.getenv('GITHUB_TOKEN')
        if github_token:
            auth = Auth.Token(github_token)
            github_client = Github(auth=auth)
            logger.info("Using authenticated GitHub API")
        else:
            github_client = Github()
            logger.info("Using unauthenticated GitHub API (60 requests/hour limit)")
        
        # Get the repository
        repo = github_client.get_repo("Azure/ARO-HCP")
        pr = repo.get_pull(pr_number)
        pr_head_sha = pr.head.sha
        
        # Check if PR is merged
        if pr.state != 'closed' or not pr.merged:
            logger.warning(f"PR #{pr_number} is not merged (state: {pr.state}, merged: {pr.merged})")
        
        # Get commits in the PR
        # Create PRData object
        pr_data = PRAnalysisData(
            pr_number=pr.number,
            title=pr.title,
            body=pr.body or "",
            author=pr.user.login,
            created_at=pr.created_at.replace(tzinfo=timezone.utc),
            merged_at=pr.merged_at.replace(tzinfo=timezone.utc) if pr.merged_at else None,
            base_ref=pr.base.ref,
            github_base_sha=pr.base.sha,
            head_commit_sha=pr_head_sha,
            merge_commit_sha=pr.merge_commit_sha,
        )
        
        logger.info(f"✅ Loaded PR #{pr_number}: {pr.title}")
        logger.info(f"   Author: {pr.user.login}")
        logger.info(f"   State: {pr.state} (merged: {pr.merged})")
        logger.info(f"   Base ref: {pr.base.ref}")
        logger.info(f"   Base SHA: {pr.base.sha[:8]}")
        logger.info(f"   Head SHA: {pr_head_sha[:8] if pr_head_sha else 'unknown'}")
        logger.info(f"   Description length: {len(pr.body or '')} chars")
        
        return pr_data
        
    except Exception as e:
        logger.error(f"❌ Error fetching PR #{pr_number}: {e}")
        return None

async def main():
    if len(sys.argv) != 2:
        print("Usage: python test_pr_analysis.py <pr_number>")
        print("       python test_pr_analysis.py 1234")
        sys.exit(1)
    
    try:
        pr_number = int(sys.argv[1])
    except ValueError:
        print("❌ PR number must be an integer")
        sys.exit(1)
    
    logger.info(f"Testing PR analysis for PR #{pr_number}")
    
    try:
        # Setup local repository
        local_repo_path = "./ignore/aro-hcp-repo"
        repo_path = setup_local_repo(local_path=local_repo_path)
        
        # Get PR data from GitHub API
        logger.info("Fetching PR data from GitHub API...")
        pr_data = get_pr_data_from_github(pr_number)
        if not pr_data:
            sys.exit(1)
        
        # Initialize the PR analyzer with Ollama
        logger.info("Initializing PRDiffAnalyzer with Ollama...")
        ollama_url = os.getenv('OLLAMA_URL', 'http://localhost:11434')
        MODEL_NAME = os.getenv("EXECUTION_MODEL_NAME", os.getenv("OLLAMA_MODEL_NAME", "phi3")).strip()
        analyzer = PRDiffAnalyzer(
            ollama_model_name=MODEL_NAME,
            ollama_url=ollama_url,
            repo_path=repo_path,
            max_context_tokens=CONTEXT_TOKENS,
        )
        
        # Analyze the PR diff
        logger.info("Running PR diff analysis...")
        analysis_result = await analyzer.analyze_pr_diff(pr_data)
        
        # Print results
        print("\n" + "="*80)
        print("PR ANALYSIS RESULT")
        print("="*80)
        print(f"PR Number: #{pr_data.pr_number}")
        print(f"Title: {pr_data.title}")
        print(f"Author: {pr_data.author}")
        print(f"Created: {pr_data.created_at.strftime('%Y-%m-%d %H:%M:%S UTC')}")
        if pr_data.merged_at:
            print(f"Merged: {pr_data.merged_at.strftime('%Y-%m-%d %H:%M:%S UTC')}")
        print(f"Base Ref: {pr_data.base_ref}")
        if pr_data.github_base_sha:
            print(f"Base SHA (GitHub): {pr_data.github_base_sha[:8]}")
        if pr_data.head_commit_sha:
            print(f"Head SHA: {pr_data.head_commit_sha[:8]}")
        print(f"Analysis Successful: {analysis_result.analysis_successful}")
        
        if pr_data.body:
            print(f"\n📝 Original PR Description:")
            print("-" * 40)
            # Truncate very long descriptions for display
            description = pr_data.body[:500]
            if len(pr_data.body) > 500:
                description += "... [truncated]"
            print(description)
        
        if analysis_result.analysis_successful:
            print("\n🤖 AI-Generated PR Analysis:")
            print("-" * 40)
            print(analysis_result.rich_description)
        else:
            print(f"\n❌ Analysis Failed: {analysis_result.failure_reason}")
        
        print("="*80)
        
    except Exception as e:
        logger.error(f"Test failed: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)

if __name__ == "__main__":
    asyncio.run(main())
