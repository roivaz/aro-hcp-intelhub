"""Generic repository manager for cloning, updating, and checking out Git repositories."""

from __future__ import annotations

import os
from contextlib import contextmanager
from pathlib import Path
from typing import Iterator, Optional, Tuple

import git


class RepoManager:
    """Utility for ensuring a local clone of a remote Git repository."""

    def __init__(
        self,
        repo_url: str,
        local_path: str | os.PathLike[str],
        default_branch: str = "main",
        allow_fetch: bool = True,
    ) -> None:
        self.repo_url = repo_url
        self.local_path = Path(local_path)
        self.default_branch = default_branch
        self.allow_fetch = allow_fetch
        self.repo: Optional[git.Repo] = None

    def ensure_ready(self) -> Path:
        """Ensure the repository exists locally and is up to date."""

        if self.local_path.exists():
            self.repo = git.Repo(self.local_path)
            if self.allow_fetch:
                self.repo.remotes.origin.fetch()
        else:
            self.local_path.parent.mkdir(parents=True, exist_ok=True)
            self.repo = git.Repo.clone_from(self.repo_url, self.local_path)
        return self.local_path

    def checkout(self, ref: str | None = None, clean: bool = False) -> None:
        """Checkout the provided ref (defaults to default_branch)."""

        repo = self._ensure_repo()
        target = ref or self.default_branch
        if clean:
            repo.git.reset("--hard")
            repo.git.clean("-fd")
        repo.git.checkout(target)

    def pull(self) -> None:
        """Pull latest changes for the default branch."""

        repo = self._ensure_repo()
        repo.remotes.origin.pull(self.default_branch)

    def current_commit(self) -> str:
        repo = self._ensure_repo()
        return repo.head.commit.hexsha

    @contextmanager
    def temporary_checkout(self, ref: str) -> Iterator[Path]:
        """Temporarily checkout a ref and restore the previous state afterwards."""

        repo = self._ensure_repo()
        previous_ref, was_detached = self._get_current_ref(repo)
        self.checkout(ref)
        try:
            yield self.local_path
        finally:
            if was_detached:
                repo.git.checkout(previous_ref)
            else:
                self.checkout(previous_ref)

    def _ensure_repo(self) -> git.Repo:
        if not self.repo:
            self.ensure_ready()
        assert self.repo is not None
        return self.repo

    @staticmethod
    def _get_current_ref(repo: git.Repo) -> Tuple[str, bool]:
        if repo.head.is_detached:
            return repo.head.commit.hexsha, True
        return repo.head.reference.name, False


