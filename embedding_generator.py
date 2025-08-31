#!/usr/bin/env python3
"""
ARO-HCP Pull Request Embedding Generator (PR-only)

This module processes merged pull requests from the Azure/ARO-HCP repository,
performs LangChain-based diff analysis via Ollama, and stores embeddings in
PostgreSQL using SQLModel. Commit-centric diff analysis has been removed.
"""

from __future__ import annotations

import asyncio
import logging
import os
import sys
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Any, Callable, Dict, List, Optional, Union

import numpy as np  # type: ignore
import psycopg  # type: ignore
from github import Auth, Github
from dotenv import load_dotenv  # type: ignore
from langchain_ollama import OllamaEmbeddings
from sqlmodel import SQLModel, Session, create_engine, select, text

from database_models import PREmbedding, ProcessingState
from pr_diff_analyzer import PRAnalysis, PRAnalysisData, PRDiffAnalyzer
from repo_manager import RepoManager

# Ensure site-packages lookup when executed directly
PROJECT_ROOT = os.path.dirname(os.path.abspath(__file__))
CONFIG_PATH = os.path.join(PROJECT_ROOT, "manifests", "config.env")
load_dotenv(CONFIG_PATH)

# Logging
logging.basicConfig(format="%(asctime)s - %(levelname)s - %(message)s")
logger = logging.getLogger(__name__)
_log_level = os.getenv("LOG_LEVEL", "INFO").upper()
try:
    logger.setLevel(_log_level)
except ValueError:
    logger.setLevel(logging.INFO)

# Environment configuration
CONTEXT_TOKENS = int(os.getenv("PR_DIFF_CONTEXT_TOKENS", "4096"))
EMBEDDING_MODEL_NAME = os.getenv("EMBEDDING_MODEL_NAME", "nomic-embed-text").strip()


def _parse_start_date(value: Optional[str]) -> Optional[datetime]:
    if not value or not value.strip():
        return None
    normalized = value.strip()
    if normalized.endswith("Z"):
        normalized = normalized[:-1] + "+00:00"
    try:
        parsed = datetime.fromisoformat(normalized)
    except ValueError:
        logger.error("Invalid PR_START_DATE '%s'; ignoring", value)
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed


PR_START_DATE = _parse_start_date(os.getenv("PR_START_DATE"))
if PR_START_DATE:
    logger.info("Configured PR start date for ingestion: %s", PR_START_DATE.isoformat())

INGESTION_MODE = os.getenv("INGESTION_MODE", "INCREMENTAL").strip().upper()
if INGESTION_MODE not in {"INCREMENTAL", "BATCH"}:
    logger.warning("Invalid INGESTION_MODE '%s'; defaulting to INCREMENTAL", INGESTION_MODE)
    INGESTION_MODE = "INCREMENTAL"


@dataclass
class PRChange:
    """Lightweight representation of a pull request stored in the database."""

    pr_number: int
    title: str
    body: str
    author: str
    created_at: datetime
    merged_at: Optional[datetime]
    state: str
    head_commit_sha: Optional[str] = None
    base_ref: Optional[str] = None
    github_base_sha: Optional[str] = None
    merge_commit_sha: Optional[str] = None
    base_merge_base_sha: Optional[str] = None

@dataclass
class SearchResult:
    """Common search result structure used by the MCP server."""

    content: str
    similarity_score: float
    metadata: Dict[str, Any]
    timestamp: datetime


class DatabaseManager:
    """SQLModel-powered database access focused on PR embeddings."""

    def __init__(self, db_config: Dict[str, Union[str, int]], recreate: str = "no") -> None:
        connection_string = (
            f"postgresql+psycopg://{db_config['user']}:{db_config['password']}"
            f"@{db_config['host']}:{db_config['port']}/{db_config['dbname']}"
        )
        self.engine = create_engine(connection_string)
        self._bootstrap_database(recreate)

    def _bootstrap_database(self, recreate_mode: str) -> None:
        reset_performed = False
        if recreate_mode == "all":
            logger.info("Bootstrapping database with full recreation")
            SQLModel.metadata.drop_all(self.engine)
            reset_performed = True
        elif recreate_mode == "prs":
            logger.info("Bootstrapping database with PR-only recreation")
            PREmbedding.__table__.drop(self.engine, checkfirst=True)
            with Session(self.engine) as session:
                state = session.exec(
                    select(ProcessingState).where(
                        ProcessingState.repository_url == "https://github.com/Azure/ARO-HCP"
                    )
                ).first()
                if state:
                    state.last_processed_pr_number = None
                    state.last_processed_pr_date = None
                    state.updated_at = datetime.now(timezone.utc)
                    session.add(state)
                    session.commit()
            reset_performed = True
        else:
            logger.info("Ensuring database schema and extensions are present")

        with Session(self.engine) as session:
            session.execute(text("CREATE EXTENSION IF NOT EXISTS vector;"))
            session.commit()

        SQLModel.metadata.create_all(self.engine)

        if reset_performed:
            logger.info("Database recreation complete")
        else:
            logger.info("Database schema verification complete")

    def connect(self) -> None:
        with Session(self.engine) as session:
            session.execute(text("SELECT 1"))
        logger.info("Connected to PostgreSQL database")

    def disconnect(self) -> None:
        logger.info("Disconnected from database")

    def get_or_create_processing_state(self, repo_url: str) -> ProcessingState:
        with Session(self.engine) as session:
            state = session.exec(select(ProcessingState).where(ProcessingState.repository_url == repo_url)).first()
            if not state:
                state = ProcessingState(repository_url=repo_url)
                session.add(state)
                session.commit()
                session.refresh(state)
            return state

    def update_pr_state(self, state: ProcessingState, pr_number: int, pr_date: Optional[datetime]) -> ProcessingState:
        with Session(self.engine) as session:
            state = session.merge(state)

            if state.last_processed_pr_number is None or pr_number > state.last_processed_pr_number:
                state.last_processed_pr_number = pr_number
            if pr_date and (state.last_processed_pr_date is None or pr_date > state.last_processed_pr_date):
                state.last_processed_pr_date = pr_date

            if state.earliest_processed_pr_number is None or pr_number < state.earliest_processed_pr_number:
                state.earliest_processed_pr_number = pr_number
            if pr_date and (state.earliest_processed_pr_date is None or pr_date < state.earliest_processed_pr_date):
                state.earliest_processed_pr_date = pr_date

            state.updated_at = datetime.now(timezone.utc)
            session.add(state)
            session.commit()
            session.refresh(state)
            return state

    def has_pr(self, pr_number: int) -> bool:
        with Session(self.engine) as session:
            exists = session.exec(
                select(PREmbedding.id).where(PREmbedding.pr_number == pr_number)
            ).first()
            return exists is not None

    def store_pr(self, pr: PREmbedding) -> None:
        with Session(self.engine) as session:
            existing = session.exec(select(PREmbedding).where(PREmbedding.pr_number == pr.pr_number)).first()
            if existing:
                logger.info("PR #%s already stored; skipping", pr.pr_number)
                return
            session.add(pr)
            session.commit()
            logger.info("Stored PR embedding for #%s", pr.pr_number)

    def get_pr_by_number(self, pr_number: int) -> Optional[PRChange]:
        with Session(self.engine) as session:
            record = session.exec(select(PREmbedding).where(PREmbedding.pr_number == pr_number)).first()
            if not record:
                return None
            return PRChange(
                pr_number=record.pr_number,
                title=record.pr_title,
                body=record.pr_body,
                author=record.author,
                created_at=record.created_at,
                merged_at=record.merged_at,
                state=record.state,
                head_commit_sha=record.head_commit_sha,
                base_ref=record.base_ref,
                github_base_sha=record.github_base_sha,
                merge_commit_sha=record.merge_commit_sha,
                base_merge_base_sha=record.base_merge_base_sha,
            )

    def search_pr_embeddings(self, query_embedding: np.ndarray, limit: int = 10) -> List[SearchResult]:
        with Session(self.engine) as session:
            records = session.exec(
                select(PREmbedding).order_by(PREmbedding.embedding.l2_distance(query_embedding)).limit(limit)
            ).all()
            results: List[SearchResult] = []
            for record in records:
                distance = session.exec(
                    select(PREmbedding.embedding.l2_distance(query_embedding)).where(PREmbedding.id == record.id)
                ).one()
                pr_change = PRChange(
                    pr_number=record.pr_number,
                    title=record.pr_title,
                    body=record.pr_body,
                    author=record.author,
                    created_at=record.created_at,
                    merged_at=record.merged_at,
                    state=record.state,
                    head_commit_sha=record.head_commit_sha,
                    base_ref=record.base_ref,
                    github_base_sha=record.github_base_sha,
                    merge_commit_sha=record.merge_commit_sha,
                    base_merge_base_sha=record.base_merge_base_sha,
                )
                results.append(
                    SearchResult(
                        content=(
                            f"PR: {record.pr_title}\n"
                            f"PR #{record.pr_number}\n"
                            f"GitHub Link: https://github.com/Azure/ARO-HCP/pull/{record.pr_number}"
                        ),
                        similarity_score=1 - (distance / 2.0),
                        metadata={"pr": pr_change},
                        timestamp=record.created_at,
                    )
                )
            return results

    def get_recent_prs(self, since_date: datetime, limit: int = 20) -> List[PRChange]:
        with Session(self.engine) as session:
            records = session.exec(
                select(PREmbedding)
                .where(PREmbedding.created_at >= since_date)
                .order_by(PREmbedding.created_at.desc())
                .limit(limit)
            ).all()
            return [
                PRChange(
                    pr_number=r.pr_number,
                    title=r.pr_title,
                    body=r.pr_body,
                    author=r.author,
                    created_at=r.created_at,
                    merged_at=r.merged_at,
                    state=r.state,
                    head_commit_sha=r.head_commit_sha,
                    base_ref=r.base_ref,
                    github_base_sha=r.github_base_sha,
                    merge_commit_sha=r.merge_commit_sha,
                    base_merge_base_sha=r.base_merge_base_sha,
                )
                for r in records
            ]


class GitHubAPIAnalyzer:
    """Utility for fetching merged PRs and their commit metadata."""

    def __init__(self, repo_url: str, github_token: Optional[str] = None) -> None:
        self.repo_url = repo_url
        self.github_token = github_token or os.getenv("GITHUB_TOKEN")
        parts = repo_url.rstrip("/").split("/")
        self.owner = parts[-2]
        self.repo_name = parts[-1]
        page_size = 100
        if self.github_token:
            auth = Auth.Token(self.github_token)
            self.github = Github(auth=auth, per_page=page_size)
            logger.info("Using authenticated GitHub client")
        else:
            self.github = Github(per_page=page_size)
            logger.info("Using unauthenticated GitHub client (60 req/hr)")
        logger.debug("Configured GitHub client page size: %s", page_size)
        self.repository = self.github.get_repo(f"{self.owner}/{self.repo_name}")
        self.executor = ThreadPoolExecutor(max_workers=4)

    async def fetch_prs(
        self,
        mode: str,
        high_watermark_date: Optional[datetime] = None,
        high_watermark_number: Optional[int] = None,
        low_watermark_date: Optional[datetime] = None,
        low_watermark_number: Optional[int] = None,
        max_items: Optional[int] = None,
    ) -> List[PRChange]:
        return await asyncio.get_event_loop().run_in_executor(
            self.executor,
            self._fetch_prs_sync,
            mode,
            high_watermark_date,
            high_watermark_number,
            low_watermark_date,
            low_watermark_number,
            max_items,
        )

    def _fetch_prs_sync(
        self,
        mode: str,
        high_watermark_date: Optional[datetime],
        high_watermark_number: Optional[int],
        low_watermark_date: Optional[datetime],
        low_watermark_number: Optional[int],
        max_items: Optional[int],
    ) -> List[PRChange]:
        max_items = max_items or int(os.getenv("MAX_NEW_PRS_PER_RUN", "100"))

        if mode == "INCREMENTAL":
            logger.info(
                "Incremental ingestion: looking for PRs newer than (%s, #%s)",
                high_watermark_date,
                high_watermark_number,
            )
            return self._fetch_incremental(
                high_watermark_date,
                high_watermark_number,
                max_items,
            )

        logger.info(
            "Batch ingestion: looking for PRs older than (%s, #%s)",
            low_watermark_date,
            low_watermark_number,
        )
        return self._fetch_batch(
            low_watermark_date,
            low_watermark_number,
            max_items,
        )

    def _fetch_incremental(
        self,
        high_watermark_date: Optional[datetime],
        high_watermark_number: Optional[int],
        max_items: int,
    ) -> List[PRChange]:
        prs: List[PRChange] = []
        try:
            github_prs = self.repository.get_pulls(
                state="closed",
                base="main",
                sort="created",
                direction="asc",
            )
            for pr in github_prs:
                merged_at = pr.merged_at
                if not merged_at:
                    continue
                merged_at_utc = (
                    merged_at.astimezone(timezone.utc)
                    if merged_at.tzinfo
                    else merged_at.replace(tzinfo=timezone.utc)
                )
                if PR_START_DATE and merged_at_utc < PR_START_DATE:
                    continue
                if high_watermark_date and merged_at_utc <= high_watermark_date:
                    continue
                if high_watermark_number and pr.number <= high_watermark_number:
                    continue
                prs.append(self._to_pr_change(pr, merged_at_utc))
                if len(prs) >= max_items:
                    break
        except Exception as exc:
            logger.error("Failed to fetch incremental PRs: %s", exc)
        prs.sort(key=lambda pr: pr.pr_number)
        return prs

    def _fetch_batch(
        self,
        low_watermark_date: Optional[datetime],
        low_watermark_number: Optional[int],
        max_items: int,
    ) -> List[PRChange]:
        prs: List[PRChange] = []
        try:
            github_prs = self.repository.get_pulls(
                state="closed",
                base="main",
                sort="created",
                direction="desc",
            )
            for pr in github_prs:
                merged_at = pr.merged_at
                if not merged_at:
                    continue
                merged_at_utc = (
                    merged_at.astimezone(timezone.utc)
                    if merged_at.tzinfo
                    else merged_at.replace(tzinfo=timezone.utc)
                )
                if PR_START_DATE and merged_at_utc < PR_START_DATE:
                    break
                if low_watermark_date and merged_at_utc >= low_watermark_date:
                    continue
                if low_watermark_number and pr.number >= low_watermark_number:
                    continue
                prs.append(self._to_pr_change(pr, merged_at_utc))
                if len(prs) >= max_items:
                    break
        except Exception as exc:
            logger.error("Failed to fetch batch PRs: %s", exc)
        prs.sort(key=lambda pr: pr.pr_number, reverse=True)
        return prs

    def _to_pr_change(self, pr: Any, merged_at_utc: datetime) -> PRChange:
        base_commit_sha: Optional[str] = getattr(pr.base, "sha", None)
        base_ref = getattr(pr.base, "ref", None) or "main"
        head_commit_sha: Optional[str] = getattr(pr.head, "sha", None)
        merge_commit_sha: Optional[str] = pr.merge_commit_sha

        created_at = pr.created_at
        if created_at.tzinfo is None:
            created_at = created_at.replace(tzinfo=timezone.utc)
        else:
            created_at = created_at.astimezone(timezone.utc)

        return PRChange(
            pr_number=pr.number,
            title=pr.title,
            body=pr.body or "",
            author=pr.user.login,
            created_at=created_at,
            merged_at=merged_at_utc,
            state=pr.state,
            head_commit_sha=head_commit_sha,
            base_ref=base_ref,
            github_base_sha=base_commit_sha,
            merge_commit_sha=merge_commit_sha,
        )

    def close(self) -> None:
        if hasattr(self, "github"):
            self.github.close()
        if hasattr(self, "executor"):
            self.executor.shutdown(wait=True)


class EmbeddingService:
    """Coordinates PR diff analysis and embedding generation."""

    def __init__(
        self,
        db_manager: DatabaseManager,
        mode: str = "read_write",
        ollama_url: str = "http://localhost:11434",
        repo_url: str = "https://github.com/Azure/ARO-HCP",
        local_repo_path: str = "./ignore/aro-hcp-repo",
    ) -> None:
        self.db_manager = db_manager
        self.mode = mode
        self.ollama_url = ollama_url
        self.repo_url = repo_url
        self.local_repo_path = local_repo_path
        self.repo_manager: Optional[RepoManager] = None
        if mode == "read_write":
            self._setup_repository()
        self.model: Optional[OllamaEmbeddings] = None
        if mode in {"read_write", "read_query"}:
            self._load_model()
        self.pr_diff_analyzer: Optional[PRDiffAnalyzer] = None
        if mode == "read_write":
            try:
                model_name = os.getenv("EXECUTION_MODEL_NAME", os.getenv("OLLAMA_MODEL_NAME", "phi3")).strip()
                self.pr_diff_analyzer = PRDiffAnalyzer(
                    ollama_model_name=model_name,
                    ollama_url=self.ollama_url,
                    repo_path=self.local_repo_path,
                    max_context_tokens=CONTEXT_TOKENS,
                )
                logger.info("Initialized PRDiffAnalyzer with model '%s'", model_name)
            except Exception as exc:
                logger.warning("Failed to initialize PRDiffAnalyzer: %s", exc)
                self.pr_diff_analyzer = None

    def _setup_repository(self) -> None:
        self.repo_manager = RepoManager(self.repo_url, self.local_repo_path)
        self.repo_manager.ensure_ready()
        try:
            self.repo_manager.checkout("main")
        except Exception as exc:
            logger.warning("Failed to checkout main branch: %s", exc)

    def _load_model(self) -> None:
        model_name = EMBEDDING_MODEL_NAME
        try:
            logger.info("Loading Ollama embedding model '%s'", model_name)
            self.model = OllamaEmbeddings(model=model_name, base_url=self.ollama_url)
        except Exception as exc:
            logger.error("Failed to load embedding model '%s': %s", model_name, exc)
            logger.error("Ensure Ollama is running and the model is available")
            raise

    async def store_pr_embedding(self, pr_data: PRAnalysisData) -> None:
        if self.mode != "read_write":
            raise PermissionError(f"Cannot store embeddings in mode '{self.mode}'")
        if not self.model:
            raise RuntimeError("Embedding model not loaded")
        analysis: Optional[PRAnalysis] = None
        if self.pr_diff_analyzer:
            logger.info("Analyzing PR #%s with LangChain", pr_data.pr_number)
            analysis = await self.pr_diff_analyzer.analyze_pr_diff(pr_data)
        text_content = (
            f"PR Title: {pr_data.title}\n\n"
            f"PR Description: {pr_data.body[:2000]}\n\n"
        )
        if analysis and analysis.analysis_successful:
            text_content += f"AI Analysis: {analysis.rich_description[:3000]}"
        embedding_vector = np.array(self.model.embed_documents([text_content])[0])
        record = PREmbedding(
            pr_number=pr_data.pr_number,
            pr_title=pr_data.title,
            pr_body=pr_data.body,
            author=pr_data.author,
            created_at=pr_data.created_at,
            merged_at=pr_data.merged_at,
            state="merged",
            base_ref=pr_data.base_ref or "main",
            github_base_sha=pr_data.github_base_sha,
            head_commit_sha=pr_data.head_commit_sha,
            merge_commit_sha=pr_data.merge_commit_sha,
            embedding=embedding_vector.tolist(),
            rich_description=analysis.rich_description if analysis else None,
            analysis_successful=analysis.analysis_successful if analysis else False,
            failure_reason=analysis.failure_reason if analysis else None,
        )
        self.db_manager.store_pr(record)

    def search_prs_semantic(self, query: str, limit: int = 10) -> List[SearchResult]:
        if self.mode not in {"read_write", "read_query"}:
            raise PermissionError(f"Query operations not allowed in mode '{self.mode}'")
        query_embedding = self._generate_query_embedding(query)
        return self.db_manager.search_pr_embeddings(query_embedding, limit)

    def get_pr_details(self, pr_number: int) -> Optional[PRChange]:
        return self.db_manager.get_pr_by_number(pr_number)

    def get_recent_prs(self, days: int = 7, limit: int = 20) -> List[PRChange]:
        if days <= 0:
            days = 7
        since = datetime.now(timezone.utc) - timedelta(days=days)
        return self.db_manager.get_recent_prs(since, limit)

    def _generate_query_embedding(self, query: str) -> np.ndarray:
        if not self.model:
            raise RuntimeError("Embedding model not loaded")
        formatted = f"PR Title: {query}\n\nPR Description: "
        vector = self.model.embed_query(formatted)
        return np.array(vector)


class AROHCPEmbedder:
    """Main orchestration entry point for PR embedding generation."""

    def __init__(self) -> None:
        self.repo_url = "https://github.com/Azure/ARO-HCP"
        self.local_repo_path = "./ignore/aro-hcp-repo"
        self.db_config = {
            "host": os.getenv("POSTGRES_HOST", "localhost"),
            "port": int(os.getenv("POSTGRES_PORT", "5432")),
            "dbname": os.getenv("POSTGRES_DB", "aro_hcp_embeddings"),
            "user": os.getenv("POSTGRES_USER", "postgres"),
            "password": os.getenv("POSTGRES_PASSWORD", "postgres"),
        }
        recreate = os.getenv("RECREATE", "no")
        self.db_manager = DatabaseManager(self.db_config, recreate=recreate)
        self.github_analyzer = GitHubAPIAnalyzer(self.repo_url)
        self.embedding_service: Optional[EmbeddingService] = None

    async def run(self) -> None:
        try:
            logger.info("Starting PR embedding generation pipeline")
            self.db_manager.connect()
            ollama_url = os.getenv("OLLAMA_URL", "http://localhost:11434")
            self.embedding_service = EmbeddingService(
                db_manager=self.db_manager,
                ollama_url=ollama_url,
            )
            state = self.db_manager.get_or_create_processing_state(self.repo_url)
            logger.info("Last processed PR #: %s", state.last_processed_pr_number or "None")
            await self._process_merged_prs(state)
        except Exception as exc:
            logger.error("Embedding generation failed: %s", exc)
            raise
        finally:
            self.db_manager.disconnect()
            self.github_analyzer.close()

    def _convert_pr_to_pr_data(self, pr: PRChange, base_commit_sha: Optional[str]) -> PRAnalysisData:
        return PRAnalysisData(
            pr_number=pr.pr_number,
            title=pr.title,
            body=pr.body,
            author=pr.author,
            created_at=pr.created_at,
            merged_at=pr.merged_at,
            base_ref=pr.base_ref,
            github_base_sha=pr.github_base_sha or base_commit_sha,
            head_commit_sha=pr.head_commit_sha,
            merge_commit_sha=pr.merge_commit_sha,
        )

    async def _process_merged_prs(self, state: ProcessingState) -> None:
        if not self.embedding_service:
            raise RuntimeError("EmbeddingService not initialized")
        batch_size = int(os.getenv("MAX_NEW_PRS_PER_RUN", "100"))
        mode = INGESTION_MODE
        if mode == "INCREMENTAL":
            prs = await self.github_analyzer.fetch_prs(
                mode=mode,
                high_watermark_date=state.last_processed_pr_date,
                high_watermark_number=state.last_processed_pr_number,
                max_items=batch_size,
            )
        else:
            prs = await self.github_analyzer.fetch_prs(
                mode=mode,
                low_watermark_date=state.earliest_processed_pr_date,
                low_watermark_number=state.earliest_processed_pr_number,
                max_items=batch_size,
            )
        if not prs:
            logger.info("No PRs to process in %s mode", mode.lower())
            return
        prs.sort(key=lambda pr: pr.pr_number)
        processed = 0
        for idx, pr in enumerate(prs, start=1):
            logger.info("\n")
            logger.info("Processing PR %s/%s (#%s)", idx, len(prs), pr.pr_number)
            try:
                pr_data = self._convert_pr_to_pr_data(pr, pr.github_base_sha)
                await self.embedding_service.store_pr_embedding(pr_data)
                state = self.db_manager.update_pr_state(state, pr.pr_number, pr.merged_at)
                processed += 1
                if processed >= batch_size:
                    logger.info("Reached batch size limit (%s)", batch_size)
                    break
            except Exception as exc:
                logger.error("Failed to process PR #%s: %s", pr.pr_number, exc)
                continue
        logger.info("Processed %s PRs in %s mode", processed, mode.lower())


async def async_main() -> None:
    embedder = AROHCPEmbedder()
    await embedder.run()
    logger.info("PR embedding generation completed successfully")


def main() -> None:
    try:
        asyncio.run(async_main())
    except Exception as exc:
        logger.error("Embedding generator failed: %s", exc)
        sys.exit(1)


if __name__ == "__main__":
    main()
