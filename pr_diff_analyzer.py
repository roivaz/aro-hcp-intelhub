#!/usr/bin/env python3
"""
PR-specific LangChain-based Diff Analyzer using Map-Reduce Strategy via Ollama

This analyzer is specialized for Pull Request analysis and uses LangChain's MapReduceDocumentsChain
with an Ollama-served model to implement a two-stage approach for PR analysis:
1. Map: Extract structured facts from each diff chunk with PR context
2. Reduce: Synthesize facts, PR title, and PR description into a coherent structured description
3. Enhanced Context: Leverages PR metadata (title, description) for richer analysis

Note: Unlike commit analysis, this analyzer processes ALL PRs without auto-generation detection,
as PRs often contain mixed auto-generated and human-written code that provides valuable context.
"""

import logging
import os
import re
from dataclasses import dataclass
from datetime import datetime
from typing import Dict, List, Optional, Tuple

import git

# LangChain imports
from langchain.chains import LLMChain
from langchain.docstore.document import Document
from langchain.prompts import PromptTemplate
from langchain.text_splitter import RecursiveCharacterTextSplitter
from langchain_ollama import ChatOllama

try:
    import tiktoken  # type: ignore
except ImportError:  # pragma: no cover - optional dependency
    tiktoken = None

logger = logging.getLogger(__name__)

_FILLER_MARKERS = {
    "here are the observed changes:",
    "observed changes:",
    "note that i've",
    "i'll stop here",
}

@dataclass
class PRAnalysisData:
    """Represents a Pull Request with all relevant data for analysis"""
    pr_number: int
    title: str
    body: str
    author: str
    created_at: datetime
    merged_at: Optional[datetime]
    base_ref: Optional[str] = None
    github_base_sha: Optional[str] = None
    head_commit_sha: Optional[str] = None
    merge_commit_sha: Optional[str] = None

@dataclass
class PRAnalysis:
    """Result of PR diff analysis with structured description"""
    rich_description: str  # Structured markdown text with sections
    analysis_successful: bool
    failure_reason: Optional[str] = None

class PRDiffAnalyzer:
    """
    Specialized diff analyzer for Pull Requests using LangChain map-reduce strategy
    with enhanced context from PR title and description
    """
    
    def __init__(
        self,
        ollama_model_name: str,
        ollama_url: str = "http://localhost:11434",
        repo_path: str | None = None,
        max_context_tokens: int = 4096,
    ):
        """
        Initialize PR-specialized analyzer with Ollama model.

        Args:
            ollama_model_name (str): The name of the model to use from Ollama (e.g., "phi3").
            ollama_url (str): URL of the Ollama server (default: http://localhost:11434).
            repo_path (str): Path to the git repository for diff calculation.
        """
        logger.info(
            "Initializing PRDiffAnalyzer with Ollama model '%s' at %s...",
            ollama_model_name,
            ollama_url,
        )

        self.repo_path = repo_path
        self.repo: Optional[git.Repo] = None
        if repo_path:
            self.repo = git.Repo(repo_path)
        else:
            logger.warning(
                "PRDiffAnalyzer instantiated without repo_path; diff generation will fail until set."
            )

        try:
            # Configure stateless LLM to prevent context bleeding between PRs
            self.llm = ChatOllama(
                model=ollama_model_name,
                temperature=0.1,
                base_url=ollama_url,
                options={"num_ctx": max_context_tokens},
            )

            self._approx_chars_per_token = 4
            self.max_tokens_per_chunk = int(max_context_tokens * 0.75)
            self.max_chunk_chars = (
                self.max_tokens_per_chunk * self._approx_chars_per_token
            )

            self.large_file_splitter = RecursiveCharacterTextSplitter(
                chunk_size=self.max_chunk_chars,
                chunk_overlap=400,
                separators=["\n@@", "\ndiff --git", "\n"],
            )

            self._token_encoder = self._load_token_encoder(ollama_model_name)
            self._ignore_patterns = self._build_ignore_patterns()
            self._diff_header_regex = re.compile(
                r"^diff --git a/(?P<old>.*?) b/(?P<new>.*?)$",
                re.MULTILINE,
            )

            self._setup_pr_prompts_and_chain()

            log_level_name = os.getenv("LOG_LEVEL", "INFO").upper()
            try:
                logger.setLevel(log_level_name)
            except ValueError:
                logger.warning(
                    "Invalid LOG_LEVEL '%s'; defaulting to INFO", log_level_name
                )
                logger.setLevel(logging.INFO)

            logger.info(
                "✅ PRDiffAnalyzer initialized successfully with Ollama model '%s' (context tokens: %s)",
                ollama_model_name,
                max_context_tokens,
            )

        except Exception as e:
            logger.error("Failed to initialize PRDiffAnalyzer with Ollama: %s", e)
            raise RuntimeError(f"PR analyzer initialization failed: {e}")
    
    def _setup_pr_prompts_and_chain(self) -> None:
        """Setup specialized prompts for PR analysis with enhanced context"""
        
        # Map stage prompt - enhanced for PR context
        map_template = """
You are a code analysis tool. Analyze the diff chunk below and report concrete, observable code changes.

Context:
- Pull request title: {pr_title}
- File path: {file_path}

Rules:
- Only report facts directly visible in the diff (lines starting with '+' or '-').
- Never speculate or use words like "likely", "suggests", "appears", or "possibly".
- Each bullet must include a quoted snippet from the diff showing the change.
- Output exactly one bullet per distinct change, using the format:
  - [FILE: {file_path}] <concise description> — "<diff snippet>"
- Maximum 4 bullets; each under 20 words.

<diff>
{text}
</diff>

**Observed Changes:**
- [FILE: {file_path}] ...
- [FILE: {file_path}] ...
- [FILE: {file_path}] ...
- [FILE: {file_path}] ...
"""

        self.map_prompt = PromptTemplate(
            template=map_template,
            input_variables=["text", "pr_title", "file_path"],
        )
        self.map_chain = LLMChain(llm=self.llm, prompt=self.map_prompt)
        
        # Reduce stage prompt - synthesizes into structured PR analysis
        reduce_template = """
You are a technical summarizer. Your task is to analyze the provided Pull Request context and create a factual, concise, and structured summary of the changes.

## Rules:
1.  **Extract, Don't Infer:** Only report on changes explicitly mentioned in the context. Do not invent goals or risks.
2.  **Be Direct and Factual:** Use clear, technical language. Avoid buzzwords.
3.  **Use the Provided Structure:** Fill in the sections below.

**CONTEXT:**

**PR Title:**
{pr_title}

**PR Description:**
{pr_description}

**Summaries of Code Changes:**
{text}

---
**FACTUAL CHANGE SUMMARY:**

### 1. Stated Purpose
(Summarize the goal from the PR Title and Description in 1-2 sentences.)

### 2. Observed Code Changes
(Create a bulleted list of the most significant technical modifications based *only* on the provided code change summaries.)
- 
- 
- 
"""
        
        self.reduce_prompt = PromptTemplate(
            template=reduce_template,
            input_variables=[
                "text",
                "pr_title",
                "pr_description",
                "pr_author",
            ],
        )
        self.reduce_chain = LLMChain(llm=self.llm, prompt=self.reduce_prompt)

    def get_pr_consolidated_diff(self, pr_data: PRAnalysisData) -> str:
        """
        Calculate consolidated diff for the entire PR using git.
        
        Args:
            pr_data: PR data including base reference and head SHA
            
        Returns:
            Consolidated diff string from base to head of PR
        """
        if not self.repo:
            raise RuntimeError("Repository not available for diff calculation")
        
        try:
            base_ref = pr_data.base_ref or "main"
            head_commit = pr_data.head_commit_sha

            if not head_commit:
                raise RuntimeError(
                    f"PR #{pr_data.pr_number} missing head commit SHA"
                )

            tracked_base_ref = base_ref if base_ref.startswith("origin/") else f"origin/{base_ref}"

            try:
                self.repo.git.fetch("origin", base_ref)
            except git.CommandError as fetch_err:
                logger.warning(
                    "Failed to fetch base ref %s: %s",
                    base_ref,
                    fetch_err,
                )

            self._ensure_commit_present(head_commit)

            merge_commit = pr_data.merge_commit_sha
            if merge_commit and merge_commit != head_commit:
                self._ensure_commit_present(merge_commit)

            head_commit_obj = self.repo.commit(head_commit)

            if head_commit_obj.parents:
                base_commit_sha = head_commit_obj.parents[0].hexsha
                self._ensure_commit_present(base_commit_sha)
            else:
                try:
                    base_commit_sha = self.repo.git.merge_base(tracked_base_ref, head_commit)
                    logger.debug(
                        "Computed merge base between %s and %s: %s",
                        tracked_base_ref,
                        head_commit,
                        base_commit_sha,
                    )
                except git.CommandError:
                    logger.debug(
                        "Merge base lookup failed; refetching %s and retrying",
                        base_ref,
                    )
                    self.repo.git.fetch("origin", base_ref)
                    base_commit_sha = self.repo.git.merge_base(tracked_base_ref, head_commit)
                    logger.debug(
                        "Recomputed merge base after fetch between %s and %s: %s",
                        tracked_base_ref,
                        head_commit,
                        base_commit_sha,
                    )

            diff_text = self.repo.git.diff(
                f"{base_commit_sha}..{head_commit}",
                unified=3
            )

            logger.info(
                "Generated consolidated diff for PR #%s: %s characters",
                pr_data.pr_number,
                len(diff_text),
            )
            return diff_text

        except Exception as e:
            logger.error(f"Failed to generate consolidated diff for PR #{pr_data.pr_number}: {e}")
            raise RuntimeError(f"Diff calculation failed: {e}")

    def _ensure_commit_present(self, commit_sha: str) -> None:
        assert self.repo is not None
        try:
            self.repo.git.cat_file("-t", commit_sha)
        except git.CommandError:
            logger.info(
                "Commit %s not present locally; attempting to fetch from origin",
                commit_sha,
            )
            self.repo.git.fetch("origin", commit_sha)
            self.repo.git.cat_file("-t", commit_sha)

    async def analyze_pr_diff(self, pr_data: PRAnalysisData) -> PRAnalysis:
        """
        Analyze consolidated PR diff using LangChain map-reduce strategy with PR context.
        
        Args:
            pr_data: Complete PR data including metadata and commit information
            
        Returns:
            PRAnalysis with structured rich_description
        """
        try:
            logger.info(f"Starting PR analysis for #{pr_data.pr_number}: {pr_data.title}")
            
            # Get consolidated diff
            consolidated_diff = self.get_pr_consolidated_diff(pr_data)
            
            if not consolidated_diff or not consolidated_diff.strip():
                return PRAnalysis(
                    rich_description=f"## Pull Request Analysis: {pr_data.title}\n\nNo diff content available for analysis.",
                    analysis_successful=False,
                    failure_reason="No diff content available"
                )
            
            diff_chunks = self._split_diff_into_files(consolidated_diff)
            
            filtered_chunks, skipped_chunks = self._filter_generated_files(diff_chunks)

            if skipped_chunks:
                logger.info(
                    "Filtered %d generated files from PR #%s: %s",
                    len(skipped_chunks),
                    pr_data.pr_number,
                    ", ".join(f"{path} ({reason})" for path, reason in skipped_chunks),
                )

            if not filtered_chunks:
                return PRAnalysis(
                    rich_description=f"## Pull Request Analysis: {pr_data.title}\n\nAll diff files were filtered as generated content.",
                    analysis_successful=False,
                    failure_reason="All files filtered as generated",
                )

            documents, doc_stats = self._build_documents(
                filtered_chunks,
                files_filtered=len(skipped_chunks),
            )

            logger.info(
                (
                    "Prepared %d diff chunks across %d files for PR #%s (max tokens %.0f, max chunk tokens %.0f)"
                ),
                len(documents),
                doc_stats["files_included"],
                pr_data.pr_number,
                self.max_tokens_per_chunk,
                doc_stats["max_tokens"],
            )

            logger.info("-" * 80)
            logger.info("📈 DIFF PREPARATION SUMMARY")
            logger.info("Files included: %s of %s", doc_stats['files_included'], doc_stats['files_total'])
            logger.info("Files filtered: %s", doc_stats['files_filtered'])
            if skipped_chunks:
                formatted_skips = ", ".join(
                    f"{path} [{reason}]" for path, reason in skipped_chunks
                )
            else:
                formatted_skips = "None"
            logger.info("Generated file filters: %s", formatted_skips)
            logger.info("Total chunks: %s (median tokens ~%s)", len(documents), doc_stats['median_tokens'])
            logger.info("Max chunk tokens: %s", doc_stats['max_tokens'])

            map_results: List[str] = []
            file_coverage = {}

            logger.info("%s", "-" * 80)
            logger.info("MAP PHASE - PR #%s", pr_data.pr_number)

            for idx, doc in enumerate(documents):
                metadata = doc.metadata or {}
                file_path = metadata.get("file_path", "<unknown>")
                est_tokens = metadata.get("estimated_tokens", "?")
                if file_path not in file_coverage:
                    file_coverage[file_path] = {"chunks": 0, "has_findings": False}
                file_coverage[file_path]["chunks"] = int(file_coverage[file_path]["chunks"]) + 1
                logger.info(
                    "MAP CHUNK %s/%s | File: %s | Tokens≈%s | Chars=%s",
                    idx + 1,
                    len(documents),
                    file_path,
                    est_tokens,
                    metadata.get("char_length", len(doc.page_content)),
                )
                preview = doc.page_content[:500]
                logger.debug(
                    "Preview:\n%s%s",
                    preview,
                    "..." if len(doc.page_content) > 500 else "",
                )

                map_response = self.map_chain.invoke(
                    {
                        "text": doc.page_content,
                        "pr_title": pr_data.title,
                        "file_path": file_path,
                    }
                )

                raw_response = map_response.get("text", "").strip()
                response_text = self._sanitize_map_response(raw_response, file_path)
                map_results.append(response_text)

                if response_text.strip():
                    file_coverage[file_path]["has_findings"] = True

                logger.debug("MAP RESPONSE %s:\n%s", idx + 1, response_text)


            logger.info("%s", "-" * 80)
            logger.info("REDUCE PHASE - Synthesizing %s map results", len(map_results))
            combined_map_summary = self._deduplicate_map_results(map_results)
            combined_map_summary = self._augment_with_uncovered_files(
                combined_map_summary,
                file_coverage,
            )
            reduce_result = self.reduce_chain.invoke(
                {
                    "text": combined_map_summary,
                    "pr_title": pr_data.title,
                    "pr_description": pr_data.body[:2000],
                    "pr_author": pr_data.author,
                }
            )

            rich_description = self._post_process_reduce_output(
                reduce_result.get("text", "").strip()
            )
            rich_description = self._limit_reduce_output_by_file(rich_description)
            
            logger.info("Rich description:\n%s", rich_description)
            logger.info(f"✅ PR analysis completed successfully for #{pr_data.pr_number}")
            logger.info("%s", "-" * 80)

            
            return PRAnalysis(
                rich_description=rich_description,
                analysis_successful=True
            )
            
        except Exception as e:
            logger.error(f"PR analysis failed for #{pr_data.pr_number}: {e}", exc_info=True)
            
            return PRAnalysis(
                rich_description=f"## Pull Request Analysis: {pr_data.title}\n\nAnalysis failed: {str(e)}",
                analysis_successful=False,
                failure_reason=str(e)
            )

    def _load_token_encoder(self, model_name: str):
        """Attempt to load a tiktoken encoder for better token estimation."""

        if not tiktoken:
            logger.warning(
                "tiktoken not installed; falling back to character-based token estimates"
            )
            return None

        try:
            return tiktoken.encoding_for_model(model_name)
        except KeyError:
            logger.debug(
                "No specific encoding for model '%s'; using cl100k_base fallback",
                model_name,
            )
            try:
                return tiktoken.get_encoding("cl100k_base")
            except Exception as exc:  # pragma: no cover - defensive
                logger.warning("Failed to load tiktoken encoding: %s", exc)
                return None

    def _estimate_tokens(self, text: str) -> int:
        """Estimate token count for a given text."""

        if self._token_encoder:
            try:
                return len(self._token_encoder.encode(text))
            except Exception as exc:  # pragma: no cover - defensive
                logger.debug("Token encoding failed (%s). Falling back to heuristic.", exc)

        return max(1, len(text) // self._approx_chars_per_token)

    def _build_ignore_patterns(self) -> Dict[str, re.Pattern[str]]:
        """Define patterns for generated or low-value files to skip in PR analysis."""

        pattern_map: Dict[str, str] = {
            "package-lock": r"package-lock\.json$",
            "yarn-lock": r"yarn\.lock$",
            "pnpm-lock": r"pnpm-lock\.yaml$",
            "npm-shrinkwrap": r"npm-shrinkwrap\.json$",
            "go-sum": r"go\.sum$",
            "go-work-sum": r"go\.work\.sum$",
            "gomodcache": r"(^|/)vendor/",
            "node_modules": r"(^|/)node_modules/",
            "generated-go": r"\.(?:pb|pb\.gw|pb\.json|pb\.grpc)\.go$",
            "generated-client": r"\.generated\.(?:ts|js|py|go|rs|java)$",
            "typescript-snapshots": r"\.snap$",
            "openapi-json": r"api/common-types/.*\.json$",
            "rendered-config": r"config/rendered/.*",
            "digests": r"config/.*\.digests\.yaml$",
            "bicep-cache": r"dev-infrastructure/.+\.bicepparam$",
            "helm-render": r".*chart\.lock$",
            "lockfiles": r"\.lock$",
            "generated-json": r".*\.swagger\.json$",
        }

        compiled: Dict[str, re.Pattern[str]] = {}
        for name, pattern in pattern_map.items():
            compiled[name] = re.compile(pattern)

        return compiled

    def _should_ignore_file(self, file_path: str) -> Optional[str]:
        for reason, pattern in self._ignore_patterns.items():
            if pattern.search(file_path):
                return reason
        return None

    def _split_diff_into_files(self, diff_text: str) -> List[Tuple[str, str]]:
        """Split a unified diff into per-file chunks."""

        if not diff_text:
            return []

        chunks = re.split(r"(?=^diff --git )", diff_text, flags=re.MULTILINE)
        file_chunks: List[Tuple[str, str]] = []

        for chunk in chunks:
            if not chunk or not chunk.strip():
                continue
            if not chunk.startswith("diff --git"):
                chunk = "diff --git " + chunk

            header_match = self._diff_header_regex.search(chunk)
            if not header_match:
                logger.debug("Skipping diff chunk without header: %s", chunk[:120])
                continue

            new_path = header_match.group("new")
            old_path = header_match.group("old")
            file_path = new_path if new_path != "/dev/null" else old_path
            file_path = file_path.strip()

            file_chunks.append((file_path, chunk))

        return file_chunks

    def _filter_generated_files(
        self, file_chunks: List[Tuple[str, str]]
    ) -> Tuple[List[Tuple[str, str]], List[Tuple[str, str]]]:
        """Filter out generated files based on ignore patterns."""

        included: List[Tuple[str, str]] = []
        skipped: List[Tuple[str, str]] = []

        for file_path, chunk in file_chunks:
            reason = self._should_ignore_file(file_path)
            if reason:
                skipped.append((file_path, reason))
                continue
            included.append((file_path, chunk))

        return included, skipped

    def _annotate_chunk(
        self,
        content: str,
        file_path: str,
        chunk_index: int,
        chunk_count: int,
    ) -> str:
        header_lines = [f"File: {file_path}"]
        if chunk_count > 1:
            header_lines.append(f"Chunk: {chunk_index + 1}/{chunk_count}")
        header_lines.append("")
        header = "\n".join(header_lines)
        return f"{header}{content.strip()}"

    def _split_large_chunk(self, text: str) -> List[str]:
        """Recursively split a chunk until each part fits within token limits."""

        queue: List[str] = [text]
        result: List[str] = []
        safety_counter = 0

        while queue:
            segment = queue.pop(0)
            safety_counter += 1
            if safety_counter > 1000:
                logger.warning("Aborting aggressive chunk splitting due to safety limit")
                result.append(segment)
                result.extend(queue)
                break

            if self._estimate_tokens(segment) <= self.max_tokens_per_chunk:
                result.append(segment)
                continue

            split_segments = self.large_file_splitter.split_text(segment)

            if not split_segments or len(split_segments) == 1:
                logger.warning(
                    "Unable to split diff chunk below token limit (tokens=%s); keeping as-is",
                    self._estimate_tokens(segment),
                )
                result.append(segment)
                continue

            queue = split_segments + queue

        return result

    def _build_documents(
        self,
        file_chunks: List[Tuple[str, str]],
        *,
        files_filtered: int,
    ) -> Tuple[List[Document], Dict[str, int]]:
        """Convert diff chunks into LangChain documents with token-aware splitting."""

        documents: List[Document] = []
        stats = {
            "files_total": len(file_chunks) + files_filtered,
            "files_filtered": files_filtered,
            "files_included": 0,
            "max_tokens": 0,
            "median_tokens": 0,
        }
        token_counts: List[int] = []

        for file_path, chunk in file_chunks:
            tokens = self._estimate_tokens(chunk)
            chunk_parts: List[str]

            if tokens <= self.max_tokens_per_chunk:
                chunk_parts = [chunk]
            else:
                chunk_parts = self._split_large_chunk(chunk)

            total_parts = len(chunk_parts)
            stats["files_included"] += 1

            for idx, part in enumerate(chunk_parts):
                annotated = self._annotate_chunk(part, file_path, idx, total_parts)
                estimated_tokens = self._estimate_tokens(annotated)
                token_counts.append(estimated_tokens)
                stats["max_tokens"] = max(stats["max_tokens"], estimated_tokens)

                documents.append(
                    Document(
                        page_content=annotated,
                        metadata={
                            "file_path": file_path,
                            "chunk_index": idx,
                            "chunk_count": total_parts,
                            "estimated_tokens": estimated_tokens,
                            "char_length": len(annotated),
                        },
                    )
                )

        if token_counts:
            sorted_tokens = sorted(token_counts)
            mid = len(sorted_tokens) // 2
            if len(sorted_tokens) % 2 == 0:
                stats["median_tokens"] = (sorted_tokens[mid - 1] + sorted_tokens[mid]) // 2
            else:
                stats["median_tokens"] = sorted_tokens[mid]

        return documents, stats

    @staticmethod
    def _sanitize_map_response(raw_text: str, file_path: str) -> str:
        """Normalize map response formatting and enforce bullet structure."""

        if not raw_text:
            return ""

        sanitized_lines: List[str] = []
        for line in raw_text.splitlines():
            stripped = line.strip()
            if not stripped:
                continue
            # Remove markdown formatting and numbering
            stripped = stripped.lstrip("-•*").lstrip()
            if stripped.startswith(tuple(str(i) + "." for i in range(1, 10))):
                stripped = stripped.split(".", 1)[1].strip()
            if stripped.lower().startswith("observed changes"):
                continue
            # Enforce presence of diff evidence (quoted snippet)
            if '"' not in stripped:
                if stripped:
                    normalized = stripped.lower()
                    if normalized not in _FILLER_MARKERS:
                        logger.debug("⤷ skipped (no quoted diff): %s", stripped)
                continue
            if not stripped.startswith("[FILE:"):
                stripped = f"[FILE: {file_path}] {stripped}"
            sanitized_lines.append(f"- {stripped}")

        return "\n".join(sanitized_lines[:4])

    @staticmethod
    def _normalize_bullet_text(text: str) -> str:
        """Create a normalized fingerprint for deduplication."""

        cleaned = text.strip().lower()
        cleaned = cleaned.replace("—", "-")
        cleaned = " ".join(cleaned.split())
        return cleaned

    @staticmethod
    def _deduplicate_map_results(map_results: List[str]) -> str:
        """Remove duplicate bullets across all map responses while preserving order."""

        seen: set[str] = set()
        deduped_lines: List[str] = []

        for result in map_results:
            for line in result.splitlines():
                normalized = PRDiffAnalyzer._normalize_bullet_text(line)
                if not normalized:
                    continue
                if normalized in seen:
                    continue
                seen.add(normalized)
                deduped_lines.append(line)

        return "\n".join(deduped_lines)

    @staticmethod
    def _post_process_reduce_output(text: str) -> str:
        """Deduplicate and tighten final reduce output bullets."""

        if not text:
            return text

        lines = text.splitlines()
        seen: set[str] = set()
        cleaned: List[str] = []

        for line in lines:
            stripped = line.rstrip()
            if not stripped:
                cleaned.append(stripped)
                continue

            if stripped.startswith("-"):
                normalized = PRDiffAnalyzer._normalize_bullet_text(stripped)
                if normalized in seen:
                    continue
                seen.add(normalized)
                cleaned.append(stripped)
            else:
                cleaned.append(stripped)

        return "\n".join(cleaned)

    def _augment_with_uncovered_files(
        self,
        combined_map_summary: str,
        file_coverage: Dict[str, Dict[str, object]],
    ) -> str:
        """Ensure every file yields at least one bullet, adding warnings when needed."""

        lines = combined_map_summary.splitlines() if combined_map_summary else []
        uncovered: List[str] = []

        for file_path, coverage in file_coverage.items():
            has_findings = bool(coverage.get("has_findings"))
            if has_findings:
                continue
            uncovered.append(file_path)
            synthetic_bullet = (
                f"- [FILE: {file_path}] No explicit changes extracted; verify manually."
            )
            logger.debug(
                "Adding synthetic finding for uncovered file '%s' with %s chunks",
                file_path,
                coverage.get("chunks"),
            )
            lines.append(synthetic_bullet)

        if uncovered:
            logger.warning(
                "Map phase produced no findings for %d files: %s",
                len(uncovered),
                ", ".join(uncovered),
            )

        # Return joined string preserving order: original lines then synthetic ones
        return "\n".join(lines)

    def _limit_reduce_output_by_file(self, text: str, max_per_file: int = 2) -> str:
        """Keep at most max_per_file bullets per file in the reduce output."""

        lines = text.splitlines()
        counts: Dict[str, int] = {}
        limited: List[str] = []

        for line in lines:
            if not line.startswith("- [FILE:"):
                limited.append(line)
                continue
            end_idx = line.find("]")
            if end_idx == -1:
                limited.append(line)
                continue
            file_path = line[3:end_idx]
            counts[file_path] = counts.get(file_path, 0) + 1
            if counts[file_path] <= max_per_file:
                limited.append(line)

        return "\n".join(limited)

# --- USAGE EXAMPLE ---
if __name__ == '__main__':
    import asyncio
    from datetime import datetime
    
    logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')

    # Sample PR data for testing
    sample_pr = PRAnalysisData(
        pr_number=123,
        title="feat: Add user authentication service",
        body="This PR implements a new authentication service using OAuth2.\n\nKey features:\n- JWT token validation\n- Role-based access control\n- Session management",
        author="developer@example.com",
        created_at=datetime.now(),
        merged_at=datetime.now(),
        base_ref="main",
        github_base_sha="abc123",
        head_commit_sha="def456",
        merge_commit_sha="ghi789",
    )

    async def test_analyzer():
        analyzer = PRDiffAnalyzer(
            ollama_model_name="phi3",
            repo_path="/path/to/repo"  # Would need actual repo path
        )
        
        # In real usage, this would analyze actual PR diff
        print("PRDiffAnalyzer initialized successfully!")
        print(f"Ready to analyze PR: {sample_pr.title}")

    # Run the test
    asyncio.run(test_analyzer())
