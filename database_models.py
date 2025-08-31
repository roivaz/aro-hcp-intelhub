#!/usr/bin/env python3
"""
SQLModel database models for ARO-HCP AI-Assisted Observability
"""
from datetime import datetime, timezone
from typing import List, Optional
from sqlmodel import SQLModel, Field, Column, Integer, DateTime, Boolean, Text
from pgvector.sqlalchemy import Vector


def utc_now() -> datetime:
    """Return timezone-aware UTC datetime"""
    return datetime.now(timezone.utc)


class ProcessingState(SQLModel, table=True):
    """Tracks processing state for idempotent operations"""
    __tablename__ = "processing_state"
    
    id: Optional[int] = Field(default=None, primary_key=True)
    repository_url: str = Field()
    last_processed_commit: Optional[str] = Field(default=None)
    last_processed_date: Optional[datetime] = Field(default=None, sa_column=Column(DateTime(timezone=True), nullable=True))
    last_processed_pr_number: Optional[int] = Field(default=None)
    last_processed_pr_date: Optional[datetime] = Field(default=None, sa_column=Column(DateTime(timezone=True), nullable=True))
    earliest_processed_pr_number: Optional[int] = Field(default=None)
    earliest_processed_pr_date: Optional[datetime] = Field(default=None, sa_column=Column(DateTime(timezone=True), nullable=True))
    created_at: datetime = Field(default_factory=utc_now, sa_column=Column(DateTime(timezone=True), nullable=False))
    updated_at: Optional[datetime] = Field(default=None, sa_column=Column(DateTime(timezone=True), nullable=True))


class PREmbedding(SQLModel, table=True):
    """Stores PR data with semantic embeddings"""
    __tablename__ = "pr_embeddings"
    
    id: Optional[int] = Field(default=None, primary_key=True)
    pr_number: int = Field(unique=True, index=True)
    pr_title: str = Field(sa_column=Column(Text, nullable=False))
    pr_body: str = Field(sa_column=Column(Text, nullable=False))
    author: str = Field()
    created_at: datetime = Field(sa_column=Column(DateTime(timezone=True), nullable=False))
    merged_at: Optional[datetime] = Field(default=None, sa_column=Column(DateTime(timezone=True), nullable=True))
    state: str = Field()  # open, closed, merged
    base_ref: str = Field(default="main")
    github_base_sha: Optional[str] = Field(default=None, sa_column=Column(Text, nullable=True, index=True))
    base_merge_base_sha: Optional[str] = Field(default=None, sa_column=Column(Text, nullable=True, index=True))
    head_commit_sha: Optional[str] = Field(default=None, sa_column=Column(Text, nullable=True, index=True))
    merge_commit_sha: Optional[str] = Field(default=None, sa_column=Column(Text, nullable=True, index=True))
    embedding: List[float] = Field(sa_column=Column(Vector(768), nullable=False))  # 768-dimensional for nomic-embed-text
    # Enhanced PR analysis fields
    rich_description: Optional[str] = Field(default=None, sa_column=Column(Text, nullable=True))
    analysis_successful: bool = Field(default=True)
    failure_reason: Optional[str] = Field(default=None, sa_column=Column(Text, nullable=True))
