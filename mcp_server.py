#!/usr/bin/env python3
"""
ARO-HCP MCP HTTP Server (Refactored with fastapi-mcp)

Provides Model Context Protocol (MCP) access to ARO-HCP embeddings using the fastapi-mcp library.
This approach converts FastAPI endpoints automatically into MCP tools with zero configuration.

Usage:
    python mcp_server.py
    
    # Or via Docker/Kubernetes
    uvicorn mcp_server:app --host 0.0.0.0 --port 8000
"""

import os
import sys
import signal
import logging
from contextlib import asynccontextmanager
from typing import List, Optional
from dataclasses import dataclass, asdict

import uvicorn
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi_mcp import FastApiMCP

# --- Imports de tu lógica de negocio (sin cambios) ---
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from embedding_generator import EmbeddingService, DatabaseManager
from repo_manager import RepoManager
from aro_hcp_image_tracer import AROHCPImageTracer, ComponentInfo as TracerComponentInfo

# Response dataclasses (moved from deleted original server file)
@dataclass
class PRResult:
    pr_number: int
    title: str
    body: str
    author: str
    state: str
    created_at: str
    merged_at: Optional[str]
    github_url: str
    similarity_score: Optional[float] = None

@dataclass
class SearchPrsResponse:
    query: str
    results: List[PRResult]
    total_found: int

@dataclass
class GetPrDetailsResponse:
    pr: PRResult

@dataclass
class ComponentTraceInfo:
    """Information about a traced component"""
    name: str
    registry: str
    repository: str
    digest: str
    source_sha: Optional[str]
    source_repo_url: Optional[str]
    error: Optional[str]

@dataclass
class TraceImagesResponse:
    """Response for tracing images back to source commits"""
    commit_sha: str
    environment: str
    components: List[ComponentTraceInfo]
    errors: List[str]

# --- Configuración y Lifespan (sin cambios) ---
logging.basicConfig(
    level=logging.INFO, 
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

embedding_service: Optional[EmbeddingService] = None

@asynccontextmanager
async def lifespan(app: FastAPI):
    global embedding_service
    logger.info("Initializing ARO-HCP embedding service...")
    
    try:
        # Initialize database configuration
        db_config = {
            'host': os.getenv('POSTGRES_HOST', 'localhost'),
            'port': int(os.getenv('POSTGRES_PORT', '5432')),
            'dbname': os.getenv('POSTGRES_DB', 'aro_hcp_embeddings'),
            'user': os.getenv('POSTGRES_USER', 'postgres'),
            'password': os.getenv('POSTGRES_PASSWORD', 'password')
        }
        
        # Create database manager and embedding service in read_query mode
        db_manager = DatabaseManager(db_config)
        db_manager.connect()
        db_manager._bootstrap_database("no")
        
        # Get Ollama URL from environment or use default
        ollama_url = os.getenv('OLLAMA_URL', 'http://localhost:11434')
        
        embedding_service = EmbeddingService(db_manager, mode="read_query", ollama_url=ollama_url)
        logger.info(f"ARO-HCP MCP server initialized successfully with EmbeddingService (Ollama: {ollama_url})")
    except Exception as e:
        logger.error(f"Failed to initialize MCP server: {e}")
        raise
    
    yield
    
    # Cleanup: Close database connections and resources
    logger.info("Shutting down ARO-HCP server...")
    try:
        if embedding_service and hasattr(embedding_service, 'db_manager'):
            logger.info("Disconnecting from database...")
            embedding_service.db_manager.disconnect()
        logger.info("✅ Server shutdown completed")
    except Exception as e:
        logger.error(f"Error during shutdown: {e}")
    finally:
        embedding_service = None

# --- Creación de la App FastAPI ---
app = FastAPI(
    title="ARO-HCP Tools API",
    description="API de herramientas para los embeddings de ARO-HCP",
    version="0.1.0",
    lifespan=lifespan
)

app.add_middleware(
    CORSMiddleware, 
    allow_origins=["*"], 
    allow_credentials=True, 
    allow_methods=["*"], 
    allow_headers=["*"]
)

# --- ¡LA MAGIA! DEFINE TUS HERRAMIENTAS COMO ENDPOINTS DE API NORMALES ---

@app.get("/tools/search_prs", response_model=SearchPrsResponse, summary="Search PRs",
         description="Search PR embeddings for context and rationale behind changes",
         operation_id="search_prs")
async def search_prs(query: str, limit: int = 10) -> SearchPrsResponse:
    """
    Busca en los embeddings de PRs para encontrar contexto y explicaciones de cambios.
    
    Args:
        query: Search query describing the feature or change to understand
        limit: Maximum number of results to return (default: 10)
    
    Returns:
        SearchPrsResponse: Query results with PR details and similarity scores
    """
    if not embedding_service:
        raise HTTPException(status_code=503, detail="Server not initialized")
    
    try:
        results = embedding_service.search_prs_semantic(query, limit)
        pr_results = []
        for result in results:
            # Extract PR data from PRChange object
            pr = result.metadata.get('pr')
            if not pr:
                continue
                
            pr_results.append(PRResult(
                pr_number=pr.pr_number,
                title=pr.title,
                body=pr.body,
                author=pr.author,
                state=pr.state,
                created_at=str(pr.created_at),
                merged_at=str(pr.merged_at) if pr.merged_at else None,
                github_url=f"https://github.com/Azure/ARO-HCP/pull/{pr.pr_number}",
                similarity_score=result.similarity_score
            ))
        
        return SearchPrsResponse(
            query=query,
            results=pr_results,
            total_found=len(pr_results)
        )
    except Exception as e:
        logger.error(f"Error searching PRs: {e}")
        raise HTTPException(status_code=500, detail=f"Error searching PRs: {str(e)}")

@app.get("/tools/get_pr_details", response_model=GetPrDetailsResponse, summary="Get PR Details",
         description="Get detailed information about a specific PR",
         operation_id="get_pr_details")
async def get_pr_details(pr_number: int) -> GetPrDetailsResponse:
    """
    Obtiene información detallada sobre un PR específico incluyendo título, descripción, autor, estado y cronología.
    
    Args:
        pr_number: The PR number to get details for
    
    Returns:
        GetPrDetailsResponse: Detailed PR information with metadata
    """
    if not embedding_service:
        raise HTTPException(status_code=503, detail="Server not initialized")
    
    try:
        pr = embedding_service.get_pr_details(pr_number)
        if not pr:
            raise HTTPException(status_code=404, detail=f"No PR found with number: {pr_number}")
        
        pr_result = PRResult(
            pr_number=pr.pr_number,
            title=pr.title,
            body=pr.body,
            author=pr.author,
            state=pr.state,
            created_at=str(pr.created_at),
            merged_at=str(pr.merged_at) if pr.merged_at else None,
            github_url=f"https://github.com/Azure/ARO-HCP/pull/{pr.pr_number}",
        )
        
        return GetPrDetailsResponse(pr=pr_result)
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error getting PR details: {e}")
        raise HTTPException(status_code=500, detail=f"Error getting PR details: {str(e)}")

@app.post("/tools/trace-images", response_model=TraceImagesResponse, summary="Trace Images",
         description="Trace images to source commits",
         operation_id="trace_images")
async def trace_images(
    commit_sha: str,
    environment: str
) -> TraceImagesResponse:
    """
    Trace Images to Source Commits
    
    Given an ARO-HCP repository commit and environment, extracts all image digests
    from the configuration overlay and traces each image back to its source repository
    and commit by inspecting image metadata.
    
    Args:
        commit_sha: ARO-HCP repository commit SHA to analyze
        environment: Environment of interest (int, stg, prod)
        
    Returns:
        Information about each component image and its source commit
    """
    logger.info(f"Tracing images for commit {commit_sha} in environment {environment}")
    
    try:
        # Get ARO-HCP repository path from environment and ensure it's ready
        aro_hcp_repo_path = os.getenv("ARO_HCP_REPO_PATH", "/app/ignore/aro-hcp-repo")
        repo_manager = RepoManager("https://github.com/Azure/ARO-HCP", aro_hcp_repo_path)
        repo_path = repo_manager.ensure_ready()
        
        # Get pull secret from environment
        pull_secret = os.getenv("PULL_SECRET")
        
        # Initialize tracer
        tracer = AROHCPImageTracer(str(repo_path), pull_secret)
        
        # Trace images
        result = tracer.trace_images(commit_sha, environment)
        
        # Convert to response format
        components = []
        for component in result.components:
            components.append(ComponentTraceInfo(
                name=component.name,
                registry=component.registry,
                repository=component.repository,
                digest=component.digest,
                source_sha=component.source_sha,
                source_repo_url=component.source_repo_url,
                error=component.error
            ))
        
        return TraceImagesResponse(
            commit_sha=result.commit_sha,
            environment=result.environment,
            components=components,
            errors=result.errors
        )
        
    except Exception as e:
        logger.error(f"Error tracing images: {e}")
        raise HTTPException(status_code=500, detail=f"Error tracing images: {str(e)}")

# --- MONTA EL SERVIDOR MCP CON UNA SOLA LÍNEA ---
# fastapi-mcp inspeccionará tu 'app' de FastAPI y convertirá los endpoints anteriores en herramientas MCP
FastApiMCP(app).mount()

# --- Endpoints adicionales (opcional) ---
@app.get("/")
def root():
    return {"status": "healthy", "service": "aro-hcp-mcp-server", "version": "1.0.0", "mcp_endpoint": "/mcp"}

@app.get("/health")
async def health_check():
    """Detailed health check"""
    global embedding_service
    
    status = {
        "status": "healthy" if embedding_service else "unhealthy",
        "embedding_service_initialized": embedding_service is not None,
        "service": "aro-hcp-mcp-server",
        "version": "1.0.0"
    }
    
    if embedding_service:
        try:
            # Test database connection
            test_results = embedding_service.search_prs_semantic("test", limit=1)
            status["database_connected"] = True
        except Exception as e:
            status["database_connected"] = False
            status["database_error"] = str(e)
    
    return status

# --- Ejecución del servidor (sin cambios) ---
def signal_handler(signum, frame):
    """Handle graceful shutdown on SIGINT/SIGTERM"""
    logger.info(f"🛑 Received signal {signum}, shutting down gracefully...")
    sys.exit(0)

if __name__ == "__main__":
    # Set up signal handlers for graceful shutdown
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)
    
    port = int(os.getenv("PORT", "8000"))
    host = os.getenv("HOST", "0.0.0.0")
    ollama_url = os.getenv("OLLAMA_URL", "http://localhost:11434")
    
    logger.info(f"Starting ARO-HCP MCP server on {host}:{port}")
    logger.info(f"Using Ollama server at: {ollama_url}")
    logger.info("💡 Press CTRL+C to stop the server")
    
    try:
        uvicorn.run(
            "mcp_server:app", 
            host=host, 
            port=port, 
            reload=False,
            log_level="info",
            access_log=False,  # Reduce verbosity
            timeout_keep_alive=5,  # Shorter keep-alive timeout
            timeout_graceful_shutdown=5  # Faster graceful shutdown
        )
    except KeyboardInterrupt:
        logger.info("🛑 Server interrupted by user (CTRL+C)")
    except Exception as e:
        logger.error(f"❌ Server error: {e}")
    finally:
        logger.info("📴 Server stopped")